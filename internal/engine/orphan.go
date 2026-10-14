package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

// KnownProductFamilies defines groups of reverse-DNS prefixes that share corporate/product lineage.
// If ANY member of a family is installed, leftovers belonging to any family member are suppressed.
var KnownProductFamilies = [][]string{
	{"net.whatsapp", "com.facebook", "com.instagram", "com.whatsapp", "org.meta"},
	{"com.docker", "com.electron.dockerdesktop"},
	{"com.google", "com.googlecode"},
	{"com.microsoft", "com.ms"},
	{"com.jetbrains", "org.jetbrains"},
	{"org.cups"},
}

// SystemOrphanPrefixes defines bundle ID prefixes that are OS/system-managed and must never be flagged as orphans.
var SystemOrphanPrefixes = []string{
	"com.apple.",
	"com.apple",
	"org.cups.",
	"org.cups",
	"is.workflow.",
	"is.workflow",
}

// OrphanSearchDir represents a directory and its resource category for orphan scanning.
type OrphanSearchDir struct {
	Path     string
	Category ResourceCategory
}

// DefaultOrphanSearchDirs returns the standard user directories where app leftovers accumulate.
func DefaultOrphanSearchDirs(homeDir string) []OrphanSearchDir {
	if homeDir == "" {
		homeDir = core.GetHomeDir()
	}
	userLib := filepath.Join(homeDir, "Library")

	return []OrphanSearchDir{
		{Path: filepath.Join(userLib, "Application Support"), Category: CategoryApplicationSupport},
		{Path: filepath.Join(userLib, "Caches"), Category: CategoryCaches},
		{Path: filepath.Join(userLib, "Preferences"), Category: CategoryPreferences},
		{Path: filepath.Join(userLib, "Saved Application State"), Category: CategorySavedState},
		{Path: filepath.Join(userLib, "Containers"), Category: CategoryContainer},
	}
}

// FindOrphanedResources scans standard macOS user directories for leftover application resources
// that do not match any currently installed application.
func FindOrphanedResources(installedApps []AppInfo) ([]AttributedResource, error) {
	return FindOrphanedResourcesInDirs(installedApps, DefaultOrphanSearchDirs(""))
}

// FindOrphanedResourcesInDirs scans custom directories for orphaned application resources.
func FindOrphanedResourcesInDirs(installedApps []AppInfo, dirs []OrphanSearchDir) ([]AttributedResource, error) {
	// 1. Build index of installed applications
	installedIDs := make(map[string]bool)
	installedVendors := make(map[string]bool)
	installedNames := make(map[string]bool)

	for _, app := range installedApps {
		id := NormalizeBundleID(app.BundleID)
		if id != "" {
			installedIDs[id] = true
			if vendor := ExtractVendorPrefix(id); vendor != "" {
				installedVendors[vendor] = true
			}
		}
		normName := app.NormalizedName()
		if len(normName) >= 3 {
			installedNames[normName] = true
		}
		if app.Name != "" {
			installedNames[strings.ToLower(strings.TrimSpace(app.Name))] = true
		}
	}

	// Active families where at least one member is installed
	activeFamilies := make(map[int]bool)
	for famIdx, fam := range KnownProductFamilies {
		for _, member := range fam {
			if installedVendors[member] || installedIDs[member] {
				activeFamilies[famIdx] = true
				break
			}
		}
	}

	var orphans []AttributedResource
	seenPaths := make(map[string]bool)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir.Path)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			entryPath := filepath.Join(dir.Path, entry.Name())
			if seenPaths[entryPath] {
				continue
			}

			// Validate safety
			if err := core.ValidatePathForDeletion(entryPath); err != nil {
				continue
			}

			name := entry.Name()
			inferredID := InferBundleID(name)
			inferredName := InferAppName(name)

			// Determine if candidate should be suppressed
			if shouldSuppressCandidate(inferredID, inferredName, installedIDs, installedVendors, installedNames, activeFamilies) {
				continue
			}

			// Determine attribution signals for the orphan
			signals := determineOrphanSignals(inferredID, inferredName, dir.Category, name)
			if len(signals) == 0 {
				continue
			}

			score := CompositeConfidence(signals)
			conf := EvaluateConfidence(score, signals)
			size, _ := core.FileSize(entryPath)
			inode, _ := core.BindInode(entryPath)

			orphans = append(orphans, AttributedResource{
				Path:       entryPath,
				Size:       size,
				Category:   dir.Category,
				Confidence: conf,
				Score:      score,
				Signals:    signals,
				Inode:      inode,
			})
			seenPaths[entryPath] = true
		}
	}

	// Sort orphans by confidence (High first, then Medium, then Low), then path
	sort.Slice(orphans, func(i, j int) bool {
		order := map[ConfidenceLevel]int{
			ConfidenceHigh:   0,
			ConfidenceMedium: 1,
			ConfidenceLow:    2,
		}
		if order[orphans[i].Confidence] != order[orphans[j].Confidence] {
			return order[orphans[i].Confidence] < order[orphans[j].Confidence]
		}
		return orphans[i].Path < orphans[j].Path
	})

	return orphans, nil
}

// NormalizeBundleID lowercases the ID and strips "group." prefix.
func NormalizeBundleID(raw string) string {
	id := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(id, "group.") {
		id = strings.TrimPrefix(id, "group.")
	}
	return id
}

// ExtractVendorPrefix extracts the first two reverse-DNS labels (e.g. "com.docker", "net.whatsapp").
func ExtractVendorPrefix(bundleID string) string {
	id := NormalizeBundleID(bundleID)
	parts := strings.Split(id, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return ""
}

// InferBundleID attempts to extract a reverse-DNS bundle identifier from a file or folder name.
func InferBundleID(name string) string {
	candidate := name
	suffixes := []string{".plist", ".savedstate", ".savedState", ".log", ".crash", ".ips", ".bom"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(strings.ToLower(candidate), strings.ToLower(suffix)) {
			candidate = candidate[:len(candidate)-len(suffix)]
			break
		}
	}

	// Strip Team ID prefix (e.g., "ABCD1234EF.com.foo.bar")
	reTeam := regexp.MustCompile(`^[A-Z0-9]{8,12}\.`)
	if reTeam.MatchString(candidate) {
		parts := strings.Split(candidate, ".")
		if len(parts) >= 3 {
			candidate = strings.Join(parts[1:], ".")
		}
	}

	// Strip "group." prefix
	if strings.HasPrefix(strings.ToLower(candidate), "group.") {
		candidate = candidate[len("group."):]
	}

	parts := strings.Split(candidate, ".")
	// Must have at least 2 components and start with a letter
	if len(parts) >= 3 && len(parts[0]) > 0 && unicode.IsLetter(rune(parts[0][0])) {
		return strings.ToLower(candidate)
	}

	return ""
}

// InferAppName extracts a human-readable application name from a directory or file.
func InferAppName(name string) string {
	candidate := name
	suffixes := []string{".plist", ".savedstate", ".savedState", ".app"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(strings.ToLower(candidate), strings.ToLower(suffix)) {
			candidate = candidate[:len(candidate)-len(suffix)]
			break
		}
	}
	return strings.TrimSpace(candidate)
}

func shouldSuppressCandidate(
	inferredID, inferredName string,
	installedIDs, installedVendors, installedNames map[string]bool,
	activeFamilies map[int]bool,
) bool {
	// 1. System prefixes check
	if inferredID != "" {
		for _, sys := range SystemOrphanPrefixes {
			if strings.HasPrefix(inferredID, sys) || inferredID == strings.TrimSuffix(sys, ".") {
				return true
			}
		}
	}

	// 2. Family suppression check
	if inferredID != "" {
		vendor := ExtractVendorPrefix(inferredID)
		for famIdx := range activeFamilies {
			family := KnownProductFamilies[famIdx]
			for _, member := range family {
				if vendor == member || strings.HasPrefix(inferredID, member) {
					return true
				}
			}
		}
	}

	// 3. Exact installed bundle ID check
	if inferredID != "" && installedIDs[inferredID] {
		return true
	}

	// 4. Sub-bundle or parent bundle matching installed app
	if inferredID != "" {
		for installed := range installedIDs {
			if inferredID == installed ||
				strings.HasPrefix(inferredID, installed+".") ||
				strings.HasPrefix(installed, inferredID+".") {
				return true
			}
		}
	}

	// 5. Vendor match check
	if inferredID != "" {
		if vendor := ExtractVendorPrefix(inferredID); vendor != "" && installedVendors[vendor] {
			return true
		}
	}

	// 6. Name match check
	if inferredName != "" {
		normInferred := strings.ToLower(inferredName)
		normInferred = strings.ReplaceAll(normInferred, " ", "")
		normInferred = strings.ReplaceAll(normInferred, "-", "")
		normInferred = strings.ReplaceAll(normInferred, "_", "")

		if len(normInferred) >= 3 && installedNames[normInferred] {
			return true
		}
		if installedNames[strings.ToLower(inferredName)] {
			return true
		}
	}

	// If no inferred bundle ID and no plausible inferred name, suppress
	if inferredID == "" && (inferredName == "" || len(inferredName) < 3) {
		return true
	}

	return false
}

func determineOrphanSignals(inferredID, inferredName string, category ResourceCategory, name string) []Signal {
	var signals []Signal
	lowerName := strings.ToLower(name)

	if inferredID != "" {
		signals = append(signals, SignalExactBundleID)
	}

	if category == CategoryContainer {
		signals = append(signals, SignalContainerBundleID)
	}

	if category == CategoryPreferences || strings.HasSuffix(lowerName, ".plist") {
		signals = append(signals, SignalPlistContentRef)
	}

	if category == CategorySavedState || strings.HasSuffix(lowerName, ".savedstate") {
		signals = append(signals, SignalSavedStateName)
	}

	if inferredName != "" && len(inferredName) >= 3 {
		signals = append(signals, SignalAppNameInPath)
	}

	return deduplicateSignals(signals)
}
