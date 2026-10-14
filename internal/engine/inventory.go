package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

// DefaultCacheTTL is the expiration duration for the app inventory cache (7 days).
const DefaultCacheTTL = 7 * 24 * time.Hour

// AppInfo represents metadata and security attributes of an installed macOS application bundle.
type AppInfo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	BundleID   string `json:"bundle_id"`
	Version    string `json:"version"`
	Executable string `json:"executable"`
	TeamID     string `json:"team_id"`
	Authority  string `json:"authority"`
	Size       int64  `json:"size"`
	IsApple    bool   `json:"is_apple"`
	Protected  bool   `json:"protected"`
	ModTime    int64  `json:"mod_time"`
}

// NormalizedName returns a lowercased alphanumeric string representation of the app name.
func (a AppInfo) NormalizedName() string {
	s := strings.ToLower(a.Name)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, ".", "")
	return s
}

// VendorPrefix returns the first two reverse-DNS labels of the bundle identifier (e.g., "com.google").
func (a AppInfo) VendorPrefix() string {
	id := strings.ToLower(strings.TrimSpace(a.BundleID))
	if strings.HasPrefix(id, "group.") {
		id = strings.TrimPrefix(id, "group.")
	}
	parts := strings.Split(id, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return id
}

// AppCache represents the on-disk cache format stored in ~/.cache/sf/app_metadata.json.
type AppCache struct {
	Timestamp time.Time `json:"timestamp"`
	Apps      []AppInfo `json:"apps"`
}

// SigningInfo captures codesign extraction details.
type SigningInfo struct {
	TeamID    string
	Authority string
	IsApple   bool
}

var (
	reTeamID    = regexp.MustCompile(`(?m)^TeamIdentifier=(.+)$`)
	reAuthority = regexp.MustCompile(`(?m)^Authority=(.+)$`)
)

// ExtractSigningInfo inspects code signing attributes of a bundle using `codesign -dv --verbose=4`.
func ExtractSigningInfo(appBundlePath string) SigningInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "codesign", "-dv", "--verbose=4", appBundlePath)
	// codesign prints diagnostic metadata to stderr
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	_ = cmd.Run()
	output := outBuf.String()

	info := SigningInfo{}

	if m := reTeamID.FindStringSubmatch(output); len(m) > 1 {
		team := strings.TrimSpace(m[1])
		if team != "not set" {
			info.TeamID = team
		}
	}

	authorities := reAuthority.FindAllStringSubmatch(output, -1)
	if len(authorities) > 0 && len(authorities[0]) > 1 {
		info.Authority = strings.TrimSpace(authorities[0][1])
	}

	for _, a := range authorities {
		if len(a) > 1 {
			val := a[1]
			if strings.Contains(val, "Apple") || strings.Contains(val, "Software Signing") {
				info.IsApple = true
				break
			}
		}
	}

	return info
}

// ParseInfo inspects an application bundle path and parses its Info.plist and signing identity.
func ParseInfo(appBundlePath string) (*AppInfo, error) {
	cleanPath := filepath.Clean(appBundlePath)
	fi, err := os.Lstat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat application bundle: %w", err)
	}
	if !fi.IsDir() && fi.Mode()&os.ModeSymlink == 0 {
		return nil, fmt.Errorf("%q is not a directory or bundle", cleanPath)
	}

	plistPath := filepath.Join(cleanPath, "Contents", "Info.plist")
	plistData, err := parseInfoPlist(plistPath)
	if err != nil {
		// Fallback to minimal bundle info if Info.plist cannot be read
		baseName := strings.TrimSuffix(filepath.Base(cleanPath), ".app")
		signing := ExtractSigningInfo(cleanPath)
		size, _ := core.FileSize(cleanPath)
		isApple := isAppleBundle(cleanPath, "", signing)

		return &AppInfo{
			Name:       baseName,
			Path:       cleanPath,
			BundleID:   "",
			Version:    "",
			Executable: "",
			TeamID:     signing.TeamID,
			Authority:  signing.Authority,
			Size:       size,
			IsApple:    isApple,
			Protected:  isApple,
			ModTime:    fi.ModTime().Unix(),
		}, nil
	}

	bundleID := plistData.BundleID
	name := plistData.DisplayName
	if name == "" {
		name = plistData.Name
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(cleanPath), ".app")
	}

	version := plistData.ShortVersion
	if version == "" {
		version = plistData.Version
	}

	signing := ExtractSigningInfo(cleanPath)
	isApple := isAppleBundle(cleanPath, bundleID, signing)
	size, _ := core.FileSize(cleanPath)

	return &AppInfo{
		Name:       name,
		Path:       cleanPath,
		BundleID:   bundleID,
		Version:    version,
		Executable: plistData.Executable,
		TeamID:     signing.TeamID,
		Authority:  signing.Authority,
		Size:       size,
		IsApple:    isApple,
		Protected:  isApple,
		ModTime:    fi.ModTime().Unix(),
	}, nil
}

type infoPlistFields struct {
	DisplayName  string
	Name         string
	BundleID     string
	ShortVersion string
	Version      string
	Executable   string
}

// parseInfoPlist extracts key metadata from Info.plist via plutil or XML fallback.
func parseInfoPlist(plistPath string) (*infoPlistFields, error) {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return nil, err
	}

	// Try plutil -convert json first for native binary/XML plist support
	cmd := exec.Command("plutil", "-convert", "json", "-o", "-", plistPath)
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		var raw map[string]any
		if jsonErr := json.Unmarshal(out, &raw); jsonErr == nil {
			fields := &infoPlistFields{}
			if v, ok := raw["CFBundleDisplayName"].(string); ok {
				fields.DisplayName = v
			}
			if v, ok := raw["CFBundleName"].(string); ok {
				fields.Name = v
			}
			if v, ok := raw["CFBundleIdentifier"].(string); ok {
				fields.BundleID = v
			}
			if v, ok := raw["CFBundleShortVersionString"].(string); ok {
				fields.ShortVersion = v
			}
			if v, ok := raw["CFBundleVersion"].(string); ok {
				fields.Version = v
			}
			if v, ok := raw["CFBundleExecutable"].(string); ok {
				fields.Executable = v
			}
			return fields, nil
		}
	}

	// Fallback: parse XML format
	return parsePlistXML(data)
}

// parsePlistXML extracts dictionary keys from standard Apple XML property lists.
func parsePlistXML(data []byte) (*infoPlistFields, error) {
	type xmlDict struct {
		XMLName xml.Name
		Content []byte `xml:",innerxml"`
	}

	content := string(data)
	fields := &infoPlistFields{}

	extractKey := func(key string) string {
		re := regexp.MustCompile(fmt.Sprintf(`(?s)<key>%s</key>\s*<string>([^<]*)</string>`, regexp.QuoteMeta(key)))
		m := re.FindStringSubmatch(content)
		if len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
		return ""
	}

	fields.DisplayName = extractKey("CFBundleDisplayName")
	fields.Name = extractKey("CFBundleName")
	fields.BundleID = extractKey("CFBundleIdentifier")
	fields.ShortVersion = extractKey("CFBundleShortVersionString")
	fields.Version = extractKey("CFBundleVersion")
	fields.Executable = extractKey("CFBundleExecutable")

	if fields.BundleID == "" && fields.Name == "" && fields.DisplayName == "" {
		return nil, errors.New("unable to extract plist fields")
	}

	return fields, nil
}

func isAppleBundle(bundlePath, bundleID string, signing SigningInfo) bool {
	if core.IsProtectedBundleID(bundleID) {
		return true
	}
	clean := filepath.Clean(bundlePath)
	if strings.HasPrefix(clean, "/System/") || strings.HasPrefix(clean, "/System/Applications") {
		return true
	}
	if signing.IsApple {
		return true
	}
	return false
}

// AppCacheFile returns the absolute path to the app metadata cache file.
func AppCacheFile() string {
	return filepath.Join(core.CacheDir(), "app_metadata.json")
}

// ScanInstalledApps discovers installed applications across standard search locations and Spotlight.
// Results are cached in ~/.cache/sf/app_metadata.json unless skipCache is true or the cache is invalid.
func ScanInstalledApps(skipCache bool) ([]AppInfo, error) {
	cacheFile := AppCacheFile()

	if !skipCache {
		if cached, ok := loadValidCache(cacheFile, DefaultCacheTTL); ok {
			return cached, nil
		}
	}

	// Scan search locations
	searchRoots := []string{
		"/Applications",
		"/Applications/Utilities",
		filepath.Join(core.GetHomeDir(), "Applications"),
	}

	discoveredPaths := make(map[string]struct{})

	for _, root := range searchRoots {
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".app") {
				appPath := filepath.Join(root, e.Name())
				discoveredPaths[appPath] = struct{}{}
			}
		}
	}

	// Query Spotlight via mdfind if available
	spotlightPaths := querySpotlightApps()
	for _, p := range spotlightPaths {
		discoveredPaths[p] = struct{}{}
	}

	var (
		mu   sync.Mutex
		apps []AppInfo
		wg   sync.WaitGroup
	)

	// Bound concurrent workers
	sem := make(chan struct{}, 8)

	for p := range discoveredPaths {
		path := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			info, err := ParseInfo(path)
			if err == nil && info != nil {
				mu.Lock()
				apps = append(apps, *info)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})

	// Save to cache
	_ = saveCache(cacheFile, apps)

	return apps, nil
}

// querySpotlightApps uses mdfind to find application bundles.
func querySpotlightApps() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "mdfind", "kMDItemContentType == 'com.apple.application-bundle'")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var results []string
	scanner := strings.Split(string(out), "\n")
	for _, line := range scanner {
		p := strings.TrimSpace(line)
		if p == "" || !strings.HasSuffix(p, ".app") {
			continue
		}
		// Filter out trash, build directories, and temporary paths
		lower := strings.ToLower(p)
		if strings.Contains(lower, "/.trash") ||
			strings.Contains(lower, "/trash/") ||
			strings.Contains(lower, "/deriveddata/") ||
			strings.Contains(lower, "/tmp/") ||
			strings.Contains(lower, "/node_modules/") {
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			results = append(results, p)
		}
	}

	return results
}

func loadValidCache(cacheFile string, ttl time.Duration) ([]AppInfo, bool) {
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}

	var cache AppCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}

	if time.Since(cache.Timestamp) > ttl {
		return nil, false
	}

	if len(cache.Apps) == 0 {
		return nil, false
	}

	// Verify that cached app entries are still consistent with filesystem
	for _, app := range cache.Apps {
		fi, err := os.Lstat(app.Path)
		if err != nil {
			// An app was deleted or moved
			return nil, false
		}
		if fi.ModTime().Unix() != app.ModTime {
			// App modified since cache creation
			return nil, false
		}
	}

	return cache.Apps, true
}

func saveCache(cacheFile string, apps []AppInfo) error {
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err != nil {
		return err
	}

	cache := AppCache{
		Timestamp: time.Now().UTC(),
		Apps:      apps,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cacheFile, data, 0644)
}
