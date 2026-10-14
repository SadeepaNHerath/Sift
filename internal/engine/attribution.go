package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

// Signal represents an attribution evidence signal linking a resource to an application.
type Signal string

const (
	// SignalExactBundleID indicates resource matches the exact reverse-DNS bundle identifier (0.95).
	SignalExactBundleID Signal = "ExactBundleID"
	// SignalContainerBundleID indicates resource is an App Sandbox or Group Container (0.90).
	SignalContainerBundleID Signal = "ContainerBundleID"
	// SignalLaunchdProgramPath indicates a LaunchAgent/Daemon plist points to the app binary (0.85).
	SignalLaunchdProgramPath Signal = "LaunchdProgramPath"
	// SignalPkgReceipt indicates an installer package receipt or bill-of-materials (.bom) (0.80).
	SignalPkgReceipt Signal = "PkgReceipt"
	// SignalSpotlightBundleID indicates Spotlight metadata reports the bundle identifier (0.75).
	SignalSpotlightBundleID Signal = "SpotlightBundleID"
	// SignalPlistContentRef indicates plist contents explicitly reference the bundle identifier (0.70).
	SignalPlistContentRef Signal = "PlistContentRef"
	// SignalSavedStateName indicates Saved Application State directory matches the app (0.65).
	SignalSavedStateName Signal = "SavedStateName"
	// SignalTeamIDMatch indicates code signature developer Team ID matches the app (0.60).
	SignalTeamIDMatch Signal = "TeamIDMatch"
	// SignalVendorPrefix indicates shared reverse-DNS vendor prefix matches (e.g., com.google.*) (0.40).
	SignalVendorPrefix Signal = "VendorPrefix"
	// SignalAppNameInPath indicates application name appears within the resource path (0.30).
	SignalAppNameInPath Signal = "AppNameInPath"
)

// SignalWeights maps each AttributionSignal to its mathematical probability weight.
var SignalWeights = map[Signal]float64{
	SignalExactBundleID:      0.95,
	SignalContainerBundleID:  0.90,
	SignalLaunchdProgramPath: 0.85,
	SignalPkgReceipt:         0.80,
	SignalSpotlightBundleID:  0.75,
	SignalPlistContentRef:    0.70,
	SignalSavedStateName:     0.65,
	SignalTeamIDMatch:        0.60,
	SignalVendorPrefix:       0.40,
	SignalAppNameInPath:      0.30,
}

// ConfidenceLevel classifies resource attribution certainty into High, Medium, or Low.
type ConfidenceLevel string

const (
	// ConfidenceHigh represents high-certainty attribution (requires ExactBundleID or ContainerBundleID).
	ConfidenceHigh ConfidenceLevel = "High"
	// ConfidenceMedium represents moderate-certainty attribution.
	ConfidenceMedium ConfidenceLevel = "Medium"
	// ConfidenceLow represents weak or tentative attribution.
	ConfidenceLow ConfidenceLevel = "Low"
)

// ResourceCategory categorizes the filesystem location and purpose of an attributed resource.
type ResourceCategory string

const (
	CategoryApplicationSupport ResourceCategory = "ApplicationSupport"
	CategoryCaches             ResourceCategory = "Caches"
	CategoryPreferences        ResourceCategory = "Preferences"
	CategorySavedState         ResourceCategory = "SavedState"
	CategoryContainer          ResourceCategory = "Container"
	CategoryGroupContainer     ResourceCategory = "GroupContainer"
	CategoryWebKit             ResourceCategory = "WebKit"
	CategoryLaunchAgent        ResourceCategory = "LaunchAgent"
	CategoryLaunchDaemon       ResourceCategory = "LaunchDaemon"
	CategoryReceipt            ResourceCategory = "Receipt"
	CategoryOther              ResourceCategory = "Other"
)

// AttributedResource represents a filesystem resource attributed to an application.
type AttributedResource struct {
	Path       string           `json:"path"`
	Size       int64            `json:"size"`
	Category   ResourceCategory `json:"category"`
	Confidence ConfidenceLevel  `json:"confidence"`
	Score      float64          `json:"score"`
	Signals    []Signal         `json:"signals"`
	Inode      core.InodeRef    `json:"inode"`
}

// CompositeConfidence calculates the combined attribution confidence score using:
// 1 - ∏(1 - w_i)
func CompositeConfidence(signals []Signal) float64 {
	if len(signals) == 0 {
		return 0.0
	}
	product := 1.0
	seen := make(map[Signal]bool)
	for _, s := range signals {
		if seen[s] {
			continue
		}
		seen[s] = true
		w, ok := SignalWeights[s]
		if !ok {
			continue
		}
		if w > 0.99 {
			w = 0.99
		}
		product *= (1.0 - w)
	}
	score := 1.0 - product
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}
	return score
}

// EvaluateConfidence assigns a ConfidenceLevel based on the composite score and signal promotion rules.
// Rules:
// - Weak signals alone (AppNameInPath, VendorPrefix) NEVER promote to High.
// - High requires ExactBundleID or ContainerBundleID.
func EvaluateConfidence(score float64, signals []Signal) ConfidenceLevel {
	hasExact := false
	hasContainer := false
	onlyWeak := true

	for _, s := range signals {
		if s == SignalExactBundleID {
			hasExact = true
		}
		if s == SignalContainerBundleID {
			hasContainer = true
		}
		if s != SignalAppNameInPath && s != SignalVendorPrefix {
			onlyWeak = false
		}
	}

	// Rule 1: Weak signals alone NEVER promote to High
	if onlyWeak {
		if score >= 0.50 {
			return ConfidenceMedium
		}
		return ConfidenceLow
	}

	// Rule 2: High requires ExactBundleID or ContainerBundleID and score >= 0.80
	if (hasExact || hasContainer) && score >= 0.80 {
		return ConfidenceHigh
	}

	if score >= 0.50 {
		return ConfidenceMedium
	}
	return ConfidenceLow
}

// DiscoveryRoot defines a directory to search for app resources.
type DiscoveryRoot struct {
	Path     string
	Category ResourceCategory
}

// StandardDiscoveryRoots returns the standard macOS user and system locations to search.
func StandardDiscoveryRoots(homeDir string) []DiscoveryRoot {
	if homeDir == "" {
		homeDir = core.GetHomeDir()
	}
	userLib := filepath.Join(homeDir, "Library")

	return []DiscoveryRoot{
		{Path: filepath.Join(userLib, "Application Support"), Category: CategoryApplicationSupport},
		{Path: filepath.Join(userLib, "Caches"), Category: CategoryCaches},
		{Path: filepath.Join(userLib, "Preferences"), Category: CategoryPreferences},
		{Path: filepath.Join(userLib, "Saved Application State"), Category: CategorySavedState},
		{Path: filepath.Join(userLib, "Containers"), Category: CategoryContainer},
		{Path: filepath.Join(userLib, "Group Containers"), Category: CategoryGroupContainer},
		{Path: filepath.Join(userLib, "WebKit"), Category: CategoryWebKit},
		{Path: filepath.Join(userLib, "HTTPStorages"), Category: CategoryCaches},
		{Path: filepath.Join(userLib, "LaunchAgents"), Category: CategoryLaunchAgent},
		{Path: filepath.Join(userLib, "Logs"), Category: CategoryOther},

		// System library roots
		{Path: "/Library/Application Support", Category: CategoryApplicationSupport},
		{Path: "/Library/Caches", Category: CategoryCaches},
		{Path: "/Library/Preferences", Category: CategoryPreferences},
		{Path: "/Library/LaunchAgents", Category: CategoryLaunchAgent},
		{Path: "/Library/LaunchDaemons", Category: CategoryLaunchDaemon},
		{Path: "/Library/Receipts", Category: CategoryReceipt},
		{Path: "/var/db/receipts", Category: CategoryReceipt},
	}
}

// FindAppResources searches standard macOS user and system library directories for files and folders
// attributed to the given application. Each returned resource is validated for safe deletion.
func FindAppResources(app AppInfo) ([]AttributedResource, error) {
	return FindAppResourcesWithRoots(app, StandardDiscoveryRoots(""))
}

// FindAppResourcesWithRoots searches the specified discovery roots for resources attributed to app.
func FindAppResourcesWithRoots(app AppInfo, roots []DiscoveryRoot) ([]AttributedResource, error) {
	var results []AttributedResource
	seenPaths := make(map[string]bool)

	// 1. Check the app bundle itself if it exists and passes safety validation
	if app.Path != "" {
		if fi, err := os.Lstat(app.Path); err == nil {
			if valErr := core.ValidatePathForDeletion(app.Path); valErr == nil {
				signals := []Signal{SignalExactBundleID}
				if app.Name != "" {
					signals = append(signals, SignalAppNameInPath)
				}
				score := CompositeConfidence(signals)
				conf := EvaluateConfidence(score, signals)
				inode, _ := core.BindInode(app.Path)
				size := app.Size
				if size == 0 {
					size, _ = core.FileSize(app.Path)
				}
				results = append(results, AttributedResource{
					Path:       app.Path,
					Size:       size,
					Category:   CategoryOther,
					Confidence: conf,
					Score:      score,
					Signals:    signals,
					Inode:      inode,
				})
				seenPaths[app.Path] = true
			}
			_ = fi
		}
	}

	// 2. Scan discovery roots
	for _, root := range roots {
		entries, err := os.ReadDir(root.Path)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			entryPath := filepath.Join(root.Path, entry.Name())
			if seenPaths[entryPath] {
				continue
			}

			// Validate candidate against safety engine
			if err := core.ValidatePathForDeletion(entryPath); err != nil {
				continue
			}

			signals := evaluateCandidateSignals(entryPath, entry.Name(), root.Category, app)
			if len(signals) == 0 {
				continue
			}

			score := CompositeConfidence(signals)
			conf := EvaluateConfidence(score, signals)
			size, _ := core.FileSize(entryPath)
			inode, _ := core.BindInode(entryPath)

			// Sort signals by descending weight
			sort.Slice(signals, func(i, j int) bool {
				return SignalWeights[signals[i]] > SignalWeights[signals[j]]
			})

			results = append(results, AttributedResource{
				Path:       entryPath,
				Size:       size,
				Category:   root.Category,
				Confidence: conf,
				Score:      score,
				Signals:    signals,
				Inode:      inode,
			})
			seenPaths[entryPath] = true
		}
	}

	// Sort results: High confidence first, then Medium, then Low, then alphabetically by Path
	sort.Slice(results, func(i, j int) bool {
		order := map[ConfidenceLevel]int{
			ConfidenceHigh:   0,
			ConfidenceMedium: 1,
			ConfidenceLow:    2,
		}
		if order[results[i].Confidence] != order[results[j].Confidence] {
			return order[results[i].Confidence] < order[results[j].Confidence]
		}
		return results[i].Path < results[j].Path
	})

	return results, nil
}

func evaluateCandidateSignals(path, name string, category ResourceCategory, app AppInfo) []Signal {
	var signals []Signal
	lowerName := strings.ToLower(name)
	bundleID := strings.ToLower(strings.TrimSpace(app.BundleID))
	vendorPrefix := strings.ToLower(strings.TrimSpace(app.VendorPrefix()))
	appName := strings.ToLower(strings.TrimSpace(app.Name))
	normApp := app.NormalizedName()

	// 1. ExactBundleID
	if bundleID != "" {
		if lowerName == bundleID ||
			lowerName == bundleID+".plist" ||
			lowerName == bundleID+".savedstate" ||
			lowerName == bundleID+".bom" {
			signals = append(signals, SignalExactBundleID)
		}
	}

	// 2. ContainerBundleID
	if category == CategoryContainer || category == CategoryGroupContainer {
		if bundleID != "" {
			if lowerName == bundleID ||
				strings.HasSuffix(lowerName, "."+bundleID) ||
				strings.HasPrefix(lowerName, "group."+bundleID) {
				signals = append(signals, SignalContainerBundleID)
			}
		}
	}

	// 3. SavedStateName
	if category == CategorySavedState || strings.HasSuffix(lowerName, ".savedstate") {
		if bundleID != "" && strings.HasPrefix(lowerName, bundleID) {
			signals = append(signals, SignalSavedStateName)
			if !containsSignal(signals, SignalExactBundleID) {
				signals = append(signals, SignalExactBundleID)
			}
		} else if normApp != "" && len(normApp) >= 3 && strings.Contains(lowerName, normApp) {
			signals = append(signals, SignalSavedStateName)
		}
	}

	// 4. PkgReceipt
	if category == CategoryReceipt || strings.Contains(path, "/receipts") {
		if bundleID != "" && strings.HasPrefix(lowerName, bundleID) {
			signals = append(signals, SignalPkgReceipt)
		}
	}

	// 5. LaunchdProgramPath & PlistContentRef
	if category == CategoryLaunchAgent || category == CategoryLaunchDaemon || strings.HasSuffix(lowerName, ".plist") {
		if strings.HasSuffix(lowerName, ".plist") {
			if launchdPointsToApp(path, app) {
				signals = append(signals, SignalLaunchdProgramPath)
			}
			if bundleID != "" && plistReferencesBundleID(path, bundleID) {
				signals = append(signals, SignalPlistContentRef)
			}
		}
	}

	// 6. TeamIDMatch
	if app.TeamID != "" && (category == CategoryLaunchDaemon || category == CategoryLaunchAgent || !isDir(path)) {
		if checkBinaryTeamID(path, app.TeamID) {
			signals = append(signals, SignalTeamIDMatch)
		}
	}

	// 7. SpotlightBundleID
	if bundleID != "" {
		if checkSpotlightBundleID(path, bundleID) {
			signals = append(signals, SignalSpotlightBundleID)
		}
	}

	// 8. VendorPrefix
	if vendorPrefix != "" && len(vendorPrefix) >= 4 {
		if lowerName == vendorPrefix ||
			strings.HasPrefix(lowerName, vendorPrefix+".") ||
			strings.HasPrefix(lowerName, vendorPrefix+"-") {
			if !containsSignal(signals, SignalExactBundleID) {
				signals = append(signals, SignalVendorPrefix)
			}
		}
	}

	// 9. AppNameInPath
	if normApp != "" && len(normApp) >= 3 {
		lowerPath := strings.ToLower(path)
		normPath := strings.ReplaceAll(lowerPath, " ", "")
		normPath = strings.ReplaceAll(normPath, "-", "")
		normPath = strings.ReplaceAll(normPath, "_", "")

		if strings.Contains(normPath, normApp) || (appName != "" && strings.Contains(lowerName, appName)) {
			signals = append(signals, SignalAppNameInPath)
		}
	}

	return deduplicateSignals(signals)
}

func containsSignal(signals []Signal, s Signal) bool {
	for _, item := range signals {
		if item == s {
			return true
		}
	}
	return false
}

func deduplicateSignals(signals []Signal) []Signal {
	var result []Signal
	seen := make(map[Signal]bool)
	for _, s := range signals {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func launchdPointsToApp(plistPath string, app AppInfo) bool {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return false
	}
	content := string(data)

	// Check if app.Path or app.Executable is referenced
	if app.Path != "" && strings.Contains(content, app.Path) {
		return true
	}
	if app.Executable != "" && len(app.Executable) >= 3 && strings.Contains(content, app.Executable) {
		return true
	}
	return false
}

func plistReferencesBundleID(plistPath string, bundleID string) bool {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return false
	}
	// Case-insensitive bundle ID match in plist content
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(bundleID))
}

func checkBinaryTeamID(path string, expectedTeamID string) bool {
	if expectedTeamID == "" {
		return false
	}
	signing := ExtractSigningInfo(path)
	return signing.TeamID == expectedTeamID
}

func checkSpotlightBundleID(path string, expectedBundleID string) bool {
	if expectedBundleID == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "mdls", "-name", "kMDItemCFBundleIdentifier", "-raw", path)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	val := strings.TrimSpace(string(out))
	return strings.EqualFold(val, expectedBundleID)
}

// BootoutLaunchd unloads a launchd job associated with a plist path.
// It executes /bin/launchctl bootout for the appropriate domain (gui/<uid> or system).
func BootoutLaunchd(plistPath string) error {
	label := extractLaunchdLabel(plistPath)
	uid := os.Getuid()

	// If in system /Library/LaunchDaemons
	if strings.HasPrefix(plistPath, "/Library/LaunchDaemons") {
		target := fmt.Sprintf("system/%s", label)
		cmd := exec.Command("/bin/launchctl", "bootout", target)
		_ = cmd.Run()
		return nil
	}

	// User GUI domain
	target := fmt.Sprintf("gui/%d/%s", uid, label)
	cmd := exec.Command("/bin/launchctl", "bootout", target)
	if err := cmd.Run(); err != nil {
		// Fallback to bootout with file path
		fallbackCmd := exec.Command("/bin/launchctl", "bootout", fmt.Sprintf("gui/%d", uid), plistPath)
		_ = fallbackCmd.Run()
	}

	return nil
}

func extractLaunchdLabel(plistPath string) string {
	data, err := os.ReadFile(plistPath)
	if err == nil {
		re := regexp.MustCompile(`(?s)<key>Label</key>\s*<string>([^<]+)</string>`)
		if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	base := filepath.Base(plistPath)
	return strings.TrimSuffix(base, ".plist")
}
