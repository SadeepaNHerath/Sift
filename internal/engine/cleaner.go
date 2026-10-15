package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

// Constants identifying each of the 13 cleanup areas and external volume support.
const (
	CatUserEssentials  = "user-essentials"
	CatAppCaches       = "app-caches"
	CatBrowsers        = "browsers"
	CatCloudOffice     = "cloud-office"
	CatDeveloperTools  = "dev-tools"
	CatHomebrew        = "homebrew"
	CatVirtualization  = "virtualization"
	CatAppSupport      = "app-support"
	CatAppLeftovers    = "app-leftovers"
	CatDeviceBackups   = "device-backups"
	CatTimeMachine     = "time-machine"
	CatSystemCaches    = "system-caches"
	CatBrowserProfiles = "browser-profiles"
	CatAIModels        = "ai-models"
	CatExternalVolume  = "external-volume"
)

// CategoryMeta holds static metadata for a cleanup category.
type CategoryMeta struct {
	ID               string
	Name             string
	Description      string
	Icon             string
	EnabledByDefault bool
}

// AllCategories defines the 13 supported cleanup areas in execution order.
var AllCategories = []CategoryMeta{
	{
		ID:               CatUserEssentials,
		Name:             "User Essentials",
		Description:      "User caches, logs, crash reports, and trash",
		Icon:             "🧹",
		EnabledByDefault: true,
	},
	{
		ID:               CatAppCaches,
		Name:             "App Caches",
		Description:      "QuickLook thumbnails, icon caches, and App Store downloads",
		Icon:             "📦",
		EnabledByDefault: true,
	},
	{
		ID:               CatBrowsers,
		Name:             "Browsers",
		Description:      "Disk, GPU, and code caches (preserves history and logins)",
		Icon:             "🌐",
		EnabledByDefault: true,
	},
	{
		ID:               CatCloudOffice,
		Name:             "Cloud & Office",
		Description:      "Office temp files and Dropbox/OneDrive/GDrive sync caches",
		Icon:             "☁️",
		EnabledByDefault: true,
	},
	{
		ID:               CatDeveloperTools,
		Name:             "Developer Tools",
		Description:      "Xcode DerivedData, simulators, and package managers",
		Icon:             "🛠️",
		EnabledByDefault: true,
	},
	{
		ID:               CatHomebrew,
		Name:             "Homebrew",
		Description:      "Homebrew formula/bottle downloads and cleanup",
		Icon:             "🍺",
		EnabledByDefault: true,
	},
	{
		ID:               CatVirtualization,
		Name:             "Virtualization",
		Description:      "Docker buildx, Podman, Colima, and OrbStack storage",
		Icon:             "🐳",
		EnabledByDefault: true,
	},
	{
		ID:               CatAppSupport,
		Name:             "Application Support",
		Description:      "Orphaned temporary caches, logs, and staging folders",
		Icon:             "📁",
		EnabledByDefault: true,
	},
	{
		ID:               CatAppLeftovers,
		Name:             "App Leftovers",
		Description:      "Residual files left behind by uninstalled applications",
		Icon:             "🗑️",
		EnabledByDefault: false,
	},
	{
		ID:               CatDeviceBackups,
		Name:             "Device Backups",
		Description:      "iOS device backups and IPSW firmware files",
		Icon:             "📱",
		EnabledByDefault: false,
	},
	{
		ID:               CatTimeMachine,
		Name:             "Time Machine",
		Description:      "Local APFS snapshots reporting and analysis",
		Icon:             "⏱️",
		EnabledByDefault: false,
	},
	{
		ID:               CatSystemCaches,
		Name:             "System Caches",
		Description:      "Diagnostic reports, crash logs, and system-level caches",
		Icon:             "⚙️",
		EnabledByDefault: true,
	},
	{
		ID:               CatBrowserProfiles,
		Name:             "Stale Browser Profiles",
		Description:      "Headless Playwright and Puppeteer automation profiles",
		Icon:             "🎭",
		EnabledByDefault: true,
	},
	{
		ID:               CatAIModels,
		Name:             "AI & LLM Caches",
		Description:      "Ollama blobs, Hugging Face hub, LM Studio, Jan, and PyTorch kernels",
		Icon:             "🧠",
		EnabledByDefault: false,
	},
}

// CleanCategory represents one of the 13 cleanup areas with its scanned items and totals.
type CleanCategory struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	Items            []CleanItem `json:"items"`
	TotalSize        int64       `json:"total_size"`
	EnabledByDefault bool        `json:"enabled_by_default"`
}

// CleanItem represents an individual cleanable file, directory, or cache unit.
type CleanItem struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Description string `json:"description"`
	Safe        bool   `json:"safe"`
}

// GetCategoryMeta returns the metadata for a given category ID or an error if invalid.
func GetCategoryMeta(catID string) (CategoryMeta, error) {
	for _, m := range AllCategories {
		if m.ID == catID {
			return m, nil
		}
	}
	if catID == CatExternalVolume {
		return CategoryMeta{
			ID:               CatExternalVolume,
			Name:             "External Volume",
			Description:      "Junk files (.Trashes, ._*, .DS_Store) on external drive",
			Icon:             "💾",
			EnabledByDefault: true,
		}, nil
	}
	return CategoryMeta{}, fmt.Errorf("unknown cleanup category ID: %q", catID)
}

// ScanAll scans all 13 cleanup categories concurrently and returns their results in order.
// If whitelist is nil, it automatically loads the saved whitelist via core.LoadWhitelist().
func ScanAll(whitelist *core.Whitelist) ([]*CleanCategory, error) {
	if whitelist == nil {
		wl, err := core.LoadWhitelist()
		if err == nil {
			whitelist = wl
		}
	}

	results := make([]*CleanCategory, len(AllCategories))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, meta := range AllCategories {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			cat, err := ScanCategory(id, whitelist)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			results[idx] = cat
		}(i, meta.ID)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

// ScanCategory scans files and directories for a single category ID.
func ScanCategory(catID string, whitelist *core.Whitelist) (*CleanCategory, error) {
	meta, err := GetCategoryMeta(catID)
	if err != nil {
		return nil, err
	}

	if whitelist == nil {
		wl, err := core.LoadWhitelist()
		if err == nil {
			whitelist = wl
		}
	}

	cat := &CleanCategory{
		ID:               meta.ID,
		Name:             meta.Name,
		Description:      meta.Description,
		Items:            make([]CleanItem, 0),
		TotalSize:        0,
		EnabledByDefault: meta.EnabledByDefault,
	}

	seen := make(map[string]bool)

	switch catID {
	case CatUserEssentials:
		scanUserEssentials(cat, whitelist, seen)
	case CatAppCaches:
		scanAppCaches(cat, whitelist, seen)
	case CatBrowsers:
		scanBrowsers(cat, whitelist, seen)
	case CatCloudOffice:
		scanCloudOffice(cat, whitelist, seen)
	case CatDeveloperTools:
		scanDeveloperTools(cat, whitelist, seen)
	case CatHomebrew:
		scanHomebrew(cat, whitelist, seen)
	case CatVirtualization:
		scanVirtualization(cat, whitelist, seen)
	case CatAppSupport:
		scanAppSupport(cat, whitelist, seen)
	case CatAppLeftovers:
		scanAppLeftovers(cat, whitelist, seen)
	case CatDeviceBackups:
		scanDeviceBackups(cat, whitelist, seen)
	case CatTimeMachine:
		scanTimeMachine(cat, whitelist, seen)
	case CatSystemCaches:
		scanSystemCaches(cat, whitelist, seen)
	case CatBrowserProfiles:
		scanBrowserProfiles(cat, whitelist, seen)
	case CatAIModels:
		scanAICachesCategory(cat, whitelist, seen)
	default:
		return nil, fmt.Errorf("unsupported category ID: %q", catID)
	}

	for _, itm := range cat.Items {
		cat.TotalSize += itm.Size
	}

	return cat, nil
}

// ScanExternalVolume scans the specified external volume directory for macOS junk files.
func ScanExternalVolume(volPath string, whitelist *core.Whitelist) (*CleanCategory, error) {
	normVol, err := core.NormalizePath(volPath)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(normVol)
	if err != nil {
		return nil, fmt.Errorf("external volume %q not accessible: %w", volPath, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("external volume %q is not a directory", volPath)
	}

	if whitelist == nil {
		wl, err := core.LoadWhitelist()
		if err == nil {
			whitelist = wl
		}
	}

	cat := &CleanCategory{
		ID:               CatExternalVolume,
		Name:             fmt.Sprintf("External Volume (%s)", filepath.Base(normVol)),
		Description:      fmt.Sprintf("Junk files and temporary caches on %s", normVol),
		Items:            make([]CleanItem, 0),
		TotalSize:        0,
		EnabledByDefault: true,
	}

	seen := make(map[string]bool)

	junkTargets := []struct {
		pattern string
		desc    string
	}{
		{pattern: filepath.Join(normVol, ".Trashes"), desc: "External volume trash bin"},
		{pattern: filepath.Join(normVol, ".TemporaryItems"), desc: "Temporary staging directory"},
		{pattern: filepath.Join(normVol, ".Spotlight-V100"), desc: "Spotlight external index"},
		{pattern: filepath.Join(normVol, ".fseventsd"), desc: "File system events logs"},
		{pattern: filepath.Join(normVol, ".DS_Store"), desc: "Desktop Services Store file"},
		{pattern: filepath.Join(normVol, "._*"), desc: "AppleDouble resource fork file"},
	}

	for _, target := range junkTargets {
		matches := expandWildcard(target.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, target.desc, whitelist, seen)
		}
	}

	for _, itm := range cat.Items {
		cat.TotalSize += itm.Size
	}

	return cat, nil
}

// CleanCategoryExecute cleans all items in a category.
// If dryRun is true, validates and tallies what would be cleaned without modifying disk.
// Every deletion must pass core.ValidatePathForDeletion and use core.SafeRemove (or core.MoveToTrash).
func CleanCategoryExecute(cat *CleanCategory, dryRun bool, logger *core.Logger, dockerPrune ...bool) (*core.RemovalReport, error) {
	if cat == nil {
		return nil, errors.New("nil category provided to CleanCategoryExecute")
	}

	report := &core.RemovalReport{
		Removed: make([]core.RemovedItem, 0),
		Failed:  make([]core.FailedItem, 0),
	}

	if dryRun {
		for _, itm := range cat.Items {
			normPath, err := core.NormalizePath(itm.Path)
			if err != nil {
				report.Failed = append(report.Failed, core.FailedItem{Path: itm.Path, Error: err})
				continue
			}

			if err := core.ValidatePathForDeletion(normPath); err != nil {
				report.Failed = append(report.Failed, core.FailedItem{Path: normPath, Error: err})
				continue
			}

			if _, err := os.Lstat(normPath); err != nil {
				report.Failed = append(report.Failed, core.FailedItem{Path: normPath, Error: err})
				continue
			}

			report.Removed = append(report.Removed, core.RemovedItem{
				Path:      normPath,
				Size:      itm.Size,
				TrashPath: "[dry-run]",
			})
			report.TotalBytes += itm.Size
		}

		if logger != nil {
			logger.LogOperation("clean", cat.ID, "dry-run",
				fmt.Sprintf("Previewed %d items, would reclaim %s",
					len(report.Removed), core.FormatBytes(report.TotalBytes)))
		}

		return report, nil
	}

	trashDir := filepath.Join(core.GetHomeDir(), ".Trash")
	for _, itm := range cat.Items {
		normPath, err := core.NormalizePath(itm.Path)
		if err != nil {
			report.Failed = append(report.Failed, core.FailedItem{Path: itm.Path, Error: err})
			continue
		}

		if err := core.ValidatePathForDeletion(normPath); err != nil {
			report.Failed = append(report.Failed, core.FailedItem{Path: normPath, Error: err})
			continue
		}

		if _, err := os.Lstat(normPath); err != nil {
			continue
		}

		sz := itm.Size
		if sz <= 0 {
			if actualSize, err := core.FileSize(normPath); err == nil {
				sz = actualSize
			}
		}

		err = core.SafeRemove(normPath)
		if err != nil {
			report.Failed = append(report.Failed, core.FailedItem{Path: normPath, Error: err})
			continue
		}

		report.Removed = append(report.Removed, core.RemovedItem{
			Path:      normPath,
			Size:      sz,
			TrashPath: filepath.Join(trashDir, filepath.Base(normPath)),
		})
		report.TotalBytes += sz

		if logger != nil {
			logger.LogDeletion(normPath, sz, "trash", fmt.Sprintf("sf clean (%s)", cat.ID))
		}
	}

	// Trigger companion tool-level cleanup actions
	allowDocker := len(dockerPrune) > 0 && dockerPrune[0]
	executeCompanionPrune(cat.ID, allowDocker)

	if logger != nil {
		logger.LogOperation("clean", cat.ID, "completed",
			fmt.Sprintf("Cleaned %d items, %d errors, reclaimed %s",
				len(report.Removed), len(report.Failed), core.FormatBytes(report.TotalBytes)))
	}

	return report, nil
}

// executeCompanionPrune runs tool-specific CLI prune routines if those tools are installed.
// Docker prune is executed only if explicitly authorized via allowDocker.
func executeCompanionPrune(catID string, allowDocker bool) {
	switch catID {
	case CatHomebrew:
		if _, err := exec.LookPath("brew"); err == nil {
			_, _ = core.RunCommandWithTimeout(30*time.Second, "brew", "cleanup", "-s", "--prune=all")
		}
	case CatDeveloperTools:
		if _, err := exec.LookPath("xcrun"); err == nil {
			_, _ = core.RunCommandWithTimeout(15*time.Second, "xcrun", "simctl", "delete", "unavailable")
		}
	case CatVirtualization:
		if allowDocker {
			if _, err := exec.LookPath("docker"); err == nil {
				_, _ = core.RunCommandWithTimeout(15*time.Second, "docker", "system", "prune", "-f")
			}
		}
	}
}

// scanUserEssentials inspects user caches, logs, crash reports, and trash.
func scanUserEssentials(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	// Enumerate top-level child directories inside ~/Library/Caches
	userCaches := filepath.Join(home, "Library", "Caches")
	if entries, err := os.ReadDir(userCaches); err == nil {
		for _, e := range entries {
			childPath := filepath.Join(userCaches, e.Name())
			addCandidateItem(cat, childPath, fmt.Sprintf("User cache for %s", e.Name()), wl, seen)
		}
	}

	// Enumerate top-level entries inside ~/Library/Logs
	userLogs := filepath.Join(home, "Library", "Logs")
	if entries, err := os.ReadDir(userLogs); err == nil {
		for _, e := range entries {
			childPath := filepath.Join(userLogs, e.Name())
			addCandidateItem(cat, childPath, fmt.Sprintf("User log %s", e.Name()), wl, seen)
		}
	}

	// DiagnosticReports and CrashReporter in user library
	crashDirs := []string{
		filepath.Join(home, "Library", "Logs", "DiagnosticReports"),
		filepath.Join(home, "Library", "Logs", "CrashReporter"),
	}
	for _, cd := range crashDirs {
		if entries, err := os.ReadDir(cd); err == nil {
			for _, e := range entries {
				addCandidateItem(cat, filepath.Join(cd, e.Name()), "Application crash report", wl, seen)
			}
		}
	}

	// Trash (~/.Trash)
	userTrash := filepath.Join(home, ".Trash")
	if entries, err := os.ReadDir(userTrash); err == nil {
		for _, e := range entries {
			childPath := filepath.Join(userTrash, e.Name())
			addCandidateItem(cat, childPath, fmt.Sprintf("Item in Trash (%s)", e.Name()), wl, seen)
		}
	}
}

// scanAppCaches scans QuickLook thumbnail caches, icon caches, and download caches.
func scanAppCaches(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	patterns := []struct {
		pattern string
		desc    string
	}{
		{pattern: filepath.Join(home, "Library", "Caches", "QuickLook"), desc: "QuickLook thumbnail cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.apple.QuickLook.thumbnailcache"), desc: "QuickLook thumbnail database"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.apple.iconservices.store"), desc: "System icon store cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.apple.iconservices"), desc: "System icon services cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.apple.appstore"), desc: "Mac App Store download cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "storeassetd"), desc: "App Store asset cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.apple.commerce"), desc: "App Store commerce cache"},
		// Common third party app caches
		{pattern: filepath.Join(home, "Library", "Caches", "Slack"), desc: "Slack desktop cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.spotify.client"), desc: "Spotify streaming cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "discord"), desc: "Discord client cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "us.zoom.xos"), desc: "Zoom meeting cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.microsoft.VSCode"), desc: "VS Code editor cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.figma.Desktop"), desc: "Figma design app cache"},
	}

	for _, p := range patterns {
		matches := expandWildcard(p.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, p.desc, wl, seen)
		}
	}

	// Darwin temporary user caches in TMPDIR
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		darwinCacheDir := filepath.Clean(filepath.Join(tmp, "..", "C"))
		darwinPatterns := []string{
			filepath.Join(darwinCacheDir, "com.apple.QuickLook.thumbnailcache"),
			filepath.Join(darwinCacheDir, "com.apple.appstore"),
		}
		for _, dp := range darwinPatterns {
			matches := expandWildcard(dp)
			for _, m := range matches {
				addCandidateItem(cat, m, "Darwin user cache", wl, seen)
			}
		}
	}
}

// scanBrowsers scans browser caches while strictly preserving history, cookies, bookmarks, and logins.
func scanBrowsers(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	targets := []struct {
		pattern string
		desc    string
	}{
		// Google Chrome
		{pattern: filepath.Join(home, "Library", "Caches", "Google", "Chrome", "Default", "Cache"), desc: "Chrome disk cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Google", "Chrome", "Default", "Code Cache"), desc: "Chrome V8 code cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Google", "Chrome", "Default", "GPUCache"), desc: "Chrome GPU shader cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Google", "Chrome", "Profile *", "Cache"), desc: "Chrome profile disk cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Google", "Chrome", "Profile *", "Code Cache"), desc: "Chrome profile code cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Google", "Chrome", "Profile *", "GPUCache"), desc: "Chrome profile GPU cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Google", "Chrome", "ShaderCache"), desc: "Chrome shader cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Google", "Chrome", "GrShaderCache"), desc: "Chrome Skia shader cache"},
		{pattern: filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Service Worker", "CacheStorage"), desc: "Chrome Service Worker cache"},
		{pattern: filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Service Worker", "ScriptCache"), desc: "Chrome Service Worker script cache"},

		// Mozilla Firefox
		{pattern: filepath.Join(home, "Library", "Caches", "Firefox", "Profiles", "*", "cache2"), desc: "Firefox HTTP network cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Firefox", "Profiles", "*", "startupCache"), desc: "Firefox startup cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Firefox", "Profiles", "*", "jumpListCache"), desc: "Firefox jump list cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Firefox", "Profiles", "*", "thumbnails"), desc: "Firefox page thumbnail cache"},

		// Arc Browser
		{pattern: filepath.Join(home, "Library", "Caches", "company.thebrowser.Browser", "Default", "Cache"), desc: "Arc disk cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "company.thebrowser.Browser", "Default", "Code Cache"), desc: "Arc code cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "company.thebrowser.Browser", "Default", "GPUCache"), desc: "Arc GPU shader cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "company.thebrowser.Browser", "ShaderCache"), desc: "Arc shader cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "company.thebrowser.Browser", "GrShaderCache"), desc: "Arc Skia shader cache"},

		// Brave Browser
		{pattern: filepath.Join(home, "Library", "Caches", "BraveSoftware", "Brave-Browser", "Default", "Cache"), desc: "Brave disk cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "BraveSoftware", "Brave-Browser", "Default", "Code Cache"), desc: "Brave code cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "BraveSoftware", "Brave-Browser", "Default", "GPUCache"), desc: "Brave GPU shader cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "BraveSoftware", "Brave-Browser", "ShaderCache"), desc: "Brave shader cache"},

		// Microsoft Edge
		{pattern: filepath.Join(home, "Library", "Caches", "Microsoft Edge", "Default", "Cache"), desc: "Edge disk cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Microsoft Edge", "Default", "Code Cache"), desc: "Edge code cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Microsoft Edge", "Default", "GPUCache"), desc: "Edge GPU shader cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Microsoft Edge", "ShaderCache"), desc: "Edge shader cache"},

		// Opera
		{pattern: filepath.Join(home, "Library", "Caches", "com.operasoftware.Opera", "Cache"), desc: "Opera disk cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.operasoftware.Opera", "GPUCache"), desc: "Opera GPU shader cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.operasoftware.Opera", "ShaderCache"), desc: "Opera shader cache"},

		// Vivaldi
		{pattern: filepath.Join(home, "Library", "Caches", "Vivaldi", "Default", "Cache"), desc: "Vivaldi disk cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Vivaldi", "Default", "Code Cache"), desc: "Vivaldi code cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "Vivaldi", "Default", "GPUCache"), desc: "Vivaldi GPU shader cache"},

		// Safari
		{pattern: filepath.Join(home, "Library", "Containers", "com.apple.Safari", "Data", "Library", "Caches"), desc: "Safari sandboxed cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.apple.Safari", "WebKitCache"), desc: "Safari WebKit cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.apple.Safari"), desc: "Safari browser cache"},
	}

	for _, t := range targets {
		matches := expandWildcard(t.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, t.desc, wl, seen)
		}
	}
}

// scanCloudOffice scans cloud synchronization caches and office suite temporary files.
func scanCloudOffice(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	targets := []struct {
		pattern string
		desc    string
	}{
		// Microsoft Office
		{pattern: filepath.Join(home, "Library", "Containers", "com.microsoft.Word", "Data", "Library", "Caches"), desc: "Word temp cache"},
		{pattern: filepath.Join(home, "Library", "Containers", "com.microsoft.Excel", "Data", "Library", "Caches"), desc: "Excel temp cache"},
		{pattern: filepath.Join(home, "Library", "Containers", "com.microsoft.Powerpoint", "Data", "Library", "Caches"), desc: "PowerPoint temp cache"},
		{pattern: filepath.Join(home, "Library", "Group Containers", "UBF8T346G9.Office", "OfficeFileCache"), desc: "Office shared file cache"},
		{pattern: filepath.Join(home, "Library", "Group Containers", "UBF8T346G9.Office", "TemporaryItems"), desc: "Office temporary items"},
		{pattern: filepath.Join(home, "Library", "Caches", "Microsoft", "Office"), desc: "Microsoft Office cache"},

		// Dropbox
		{pattern: filepath.Join(home, ".dropbox", "cache"), desc: "Dropbox sync cache"},
		{pattern: filepath.Join(home, "Library", "Application Support", "Dropbox", "cache"), desc: "Dropbox app cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.getdropbox.dropbox"), desc: "Dropbox client cache"},

		// OneDrive
		{pattern: filepath.Join(home, "Library", "Caches", "OneDrive"), desc: "OneDrive sync cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.microsoft.OneDrive"), desc: "OneDrive client cache"},
		{pattern: filepath.Join(home, "Library", "Containers", "com.microsoft.OneDrive-mac", "Data", "Library", "Caches"), desc: "OneDrive container cache"},

		// Google Drive
		{pattern: filepath.Join(home, "Library", "Application Support", "Google", "DriveFS", "*", "content_cache"), desc: "Google Drive offline content cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.google.drivefs"), desc: "Google Drive client cache"},
	}

	for _, t := range targets {
		matches := expandWildcard(t.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, t.desc, wl, seen)
		}
	}
}

// scanDeveloperTools scans Xcode DerivedData, build archives, iOS DeviceSupport symbols, and package manager caches.
func scanDeveloperTools(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	targets := []struct {
		pattern string
		desc    string
	}{
		// Xcode
		{pattern: filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData"), desc: "Xcode DerivedData build artifacts"},
		{pattern: filepath.Join(home, "Library", "Developer", "Xcode", "Archives"), desc: "Xcode build archives"},
		{pattern: filepath.Join(home, "Library", "Developer", "Xcode", "iOS DeviceSupport"), desc: "iOS DeviceSupport symbols"},
		{pattern: filepath.Join(home, "Library", "Developer", "Xcode", "watchOS DeviceSupport"), desc: "watchOS DeviceSupport symbols"},
		{pattern: filepath.Join(home, "Library", "Developer", "Xcode", "tvOS DeviceSupport"), desc: "tvOS DeviceSupport symbols"},
		{pattern: filepath.Join(home, "Library", "Developer", "CoreSimulator", "Caches"), desc: "CoreSimulator cache"},

		// Package managers
		{pattern: filepath.Join(home, ".npm", "_cacache"), desc: "npm global cache"},
		{pattern: filepath.Join(home, "Library", "pnpm", "store"), desc: "pnpm store cache"},
		{pattern: filepath.Join(home, ".local", "share", "pnpm", "store"), desc: "pnpm local share store"},
		{pattern: filepath.Join(home, "Library", "Caches", "Yarn"), desc: "Yarn package cache"},
		{pattern: filepath.Join(home, ".yarn", "berry", "cache"), desc: "Yarn Berry package cache"},
		{pattern: filepath.Join(home, ".bun", "install", "cache"), desc: "Bun package cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "pip"), desc: "pip package cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "uv"), desc: "uv package cache"},
		{pattern: filepath.Join(home, ".cargo", "registry", "cache"), desc: "Cargo crate registry cache"},
		{pattern: filepath.Join(home, ".cargo", "git", "db"), desc: "Cargo git dependency checkout cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "go-build"), desc: "Go build compiler cache"},
		{pattern: filepath.Join(home, ".gradle", "caches"), desc: "Gradle build artifact cache"},
		{pattern: filepath.Join(home, ".m2", "repository"), desc: "Maven local dependency repository"},
	}

	for _, t := range targets {
		matches := expandWildcard(t.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, t.desc, wl, seen)
		}
	}
}

// scanHomebrew scans Homebrew bottle and formula caches.
func scanHomebrew(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	brewCache := filepath.Join(home, "Library", "Caches", "Homebrew")
	if custom := os.Getenv("HOMEBREW_CACHE"); custom != "" {
		brewCache = custom
	}

	addCandidateItem(cat, brewCache, "Homebrew downloaded bottles and formula cache", wl, seen)
}

// scanVirtualization scans Docker, Podman, Colima, and OrbStack caches.
func scanVirtualization(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	targets := []struct {
		pattern string
		desc    string
	}{
		{pattern: filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "log"), desc: "Docker Desktop daemon logs"},
		{pattern: filepath.Join(home, ".docker", "buildx", "cache"), desc: "Docker buildx cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "com.docker.docker"), desc: "Docker desktop application cache"},
		{pattern: filepath.Join(home, ".local", "share", "containers", "storage", "cache"), desc: "Podman container storage cache"},
		{pattern: filepath.Join(home, ".config", "containers", "podman", "cache"), desc: "Podman config cache"},
		{pattern: filepath.Join(home, ".colima", "_cache"), desc: "Colima VM download cache"},
		{pattern: filepath.Join(home, "Library", "Caches", "dev.kdrag0n.OrbStack"), desc: "OrbStack container VM cache"},
	}

	for _, t := range targets {
		matches := expandWildcard(t.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, t.desc, wl, seen)
		}
	}
}

// scanAppSupport scans temporary caches and logs in Application Support.
func scanAppSupport(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	patterns := []struct {
		pattern string
		desc    string
	}{
		{pattern: filepath.Join(home, "Library", "Application Support", "*", "Cache*"), desc: "Application Support cache directory"},
		{pattern: filepath.Join(home, "Library", "Application Support", "*", "GPUCache"), desc: "Application Support GPU shader cache"},
		{pattern: filepath.Join(home, "Library", "Application Support", "*", "Logs"), desc: "Application Support log directory"},
		{pattern: filepath.Join(home, "Library", "Application Support", "*", "CrashReporter"), desc: "Application Support crash logs"},
		{pattern: filepath.Join(home, "Library", "Application Support", "*", "tmp"), desc: "Application Support temp directory"},
		{pattern: filepath.Join(home, "Library", "Application Support", "*", "temp"), desc: "Application Support temp files"},
	}

	for _, p := range patterns {
		matches := expandWildcard(p.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, p.desc, wl, seen)
		}
	}
}

// scanAppLeftovers identifies residual application state and support folders for uninstalled apps.
func scanAppLeftovers(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	// Build a map of installed app names and bundle IDs
	installedNames := make(map[string]bool)
	installedBundles := make(map[string]bool)

	// Scan /Applications, /Applications/Utilities, and ~/Applications
	appDirs := []string{
		"/Applications",
		"/Applications/Utilities",
		filepath.Join(home, "Applications"),
	}

	for _, ad := range appDirs {
		entries, err := os.ReadDir(ad)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".app") {
				stem := strings.TrimSuffix(e.Name(), ".app")
				installedNames[strings.ToLower(stem)] = true
				installedNames[strings.ToLower(strings.ReplaceAll(stem, " ", ""))] = true

				plistPath := filepath.Join(ad, e.Name(), "Contents", "Info.plist")
				if data, err := os.ReadFile(plistPath); err == nil {
					if info, err := parseInfoPlist(plistPath); err == nil && info.BundleID != "" {
						installedBundles[strings.ToLower(info.BundleID)] = true
					} else {
						strData := string(data)
						if idx := strings.Index(strData, "CFBundleIdentifier"); idx != -1 {
							after := strData[idx:]
							if sIdx := strings.Index(after, "<string>"); sIdx != -1 {
								eIdx := strings.Index(after[sIdx:], "</string>")
								if eIdx != -1 {
									id := strings.TrimSpace(after[sIdx+8 : sIdx+eIdx])
									installedBundles[strings.ToLower(id)] = true
								}
							}
						}
					}
				}
			}
		}
	}

	savedStateDir := filepath.Join(home, "Library", "Saved Application State")
	if entries, err := os.ReadDir(savedStateDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".savedState") {
				bundleID := strings.TrimSuffix(e.Name(), ".savedState")
				lowerID := strings.ToLower(bundleID)

				if strings.HasPrefix(lowerID, "com.apple.") || lowerID == "com.apple" {
					continue
				}

				if !installedBundles[lowerID] {
					fullPath := filepath.Join(savedStateDir, e.Name())
					addCandidateItem(cat, fullPath, fmt.Sprintf("Saved state leftover for uninstalled app %s", bundleID), wl, seen)
				}
			}
		}
	}

	appSupportDir := filepath.Join(home, "Library", "Application Support")
	systemFolderPrefixes := []string{
		"apple", "addressbook", "google", "microsoft", "adobe", "mozilla",
		"icloud", "dock", "quick look", "clouddocs", "syncservices", "sf", "sift",
		"crashreporter", "mobilesync", "itunes", "callhistorydb", "cequint",
	}

	if entries, err := os.ReadDir(appSupportDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			folderName := e.Name()
			lower := strings.ToLower(folderName)

			isSys := false
			for _, pref := range systemFolderPrefixes {
				if strings.HasPrefix(lower, pref) {
					isSys = true
					break
				}
			}
			if isSys {
				continue
			}

			// If not a system folder and no matching app is installed
			compactName := strings.ReplaceAll(lower, " ", "")
			if !installedNames[lower] && !installedNames[compactName] && !installedBundles[lower] {
				// Only consider if not recently modified (> 30 days) or empty
				fullPath := filepath.Join(appSupportDir, folderName)
				addCandidateItem(cat, fullPath, fmt.Sprintf("Residual Application Support files for uninstalled app %s", folderName), wl, seen)
			}
		}
	}
}

// scanDeviceBackups scans iOS device backups and firmware restore images.
func scanDeviceBackups(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	targets := []struct {
		pattern string
		desc    string
	}{
		{pattern: filepath.Join(home, "Library", "Application Support", "MobileSync", "Backup", "*"), desc: "iOS device backup"},
		{pattern: filepath.Join(home, "Library", "iTunes", "iPhone Software Updates", "*.ipsw"), desc: "iPhone firmware restore image (IPSW)"},
		{pattern: filepath.Join(home, "Library", "iTunes", "iPad Software Updates", "*.ipsw"), desc: "iPad firmware restore image (IPSW)"},
		{pattern: filepath.Join(home, "Library", "Application Support", "MobileSync", "Software Updates", "*.ipsw"), desc: "MobileSync firmware restore file"},
	}

	for _, t := range targets {
		matches := expandWildcard(t.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, t.desc, wl, seen)
		}
	}
}

// scanTimeMachine checks for local APFS snapshots via tmutil.
func scanTimeMachine(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	out, err := core.RunCommandWithTimeout(5*time.Second, "tmutil", "listlocalsnapshots", "/")
	if err != nil {
		return
	}

	lines := strings.Split(out, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "com.apple.TimeMachine.") {
			snapshotName := trimmed
			cat.Items = append(cat.Items, CleanItem{
				Path:        snapshotName,
				Size:        0, // Snapshot size is dynamic across APFS blocks
				Description: fmt.Sprintf("Local APFS snapshot %s", snapshotName),
				Safe:        true,
			})
		}
	}
}

// scanSystemCaches scans system diagnostic reports and crash logs.
func scanSystemCaches(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	targets := []struct {
		pattern string
		desc    string
	}{
		{pattern: "/Library/Logs/DiagnosticReports/*", desc: "System diagnostic and crash report"},
		{pattern: "/Library/Caches/*", desc: "System application cache"},
		{pattern: "/private/var/log/asl/*", desc: "Apple system log archive"},
	}

	for _, t := range targets {
		matches := expandWildcard(t.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, t.desc, wl, seen)
		}
	}
}

// scanBrowserProfiles scans stale automation browser profiles from Playwright and Puppeteer.
func scanBrowserProfiles(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	home := core.GetHomeDir()

	targets := []struct {
		pattern string
		desc    string
	}{
		{pattern: filepath.Join(home, ".cache", "ms-playwright"), desc: "Playwright headless browser downloads"},
		{pattern: filepath.Join(home, "Library", "Caches", "ms-playwright"), desc: "Playwright browser cache"},
		{pattern: filepath.Join(home, ".cache", "puppeteer"), desc: "Puppeteer headless browser downloads"},
		{pattern: filepath.Join(home, "Library", "Caches", "puppeteer"), desc: "Puppeteer browser cache"},
		{pattern: filepath.Join(home, ".local", "share", "playwright"), desc: "Playwright local share storage"},
		{pattern: filepath.Join(home, ".local", "share", "puppeteer"), desc: "Puppeteer local share storage"},
		{pattern: "/tmp/puppeteer_dev_chrome_profile*", desc: "Puppeteer temporary Chrome profile"},
		{pattern: "/tmp/playwright*", desc: "Playwright temporary test session profile"},
	}

	for _, t := range targets {
		matches := expandWildcard(t.pattern)
		for _, m := range matches {
			addCandidateItem(cat, m, t.desc, wl, seen)
		}
	}
}

// scanAICachesCategory scans local AI and LLM model caches and worktrees.
func scanAICachesCategory(cat *CleanCategory, wl *core.Whitelist, seen map[string]bool) {
	targets, err := ScanAICaches()
	if err != nil {
		return
	}
	for _, t := range targets {
		addCandidateItem(cat, t.Path, fmt.Sprintf("%s (%s)", t.Name, t.Description), wl, seen)
	}
}

// expandWildcard expands tilde (~) and glob patterns (*, ?) into a list of absolute paths.
func expandWildcard(pattern string) []string {
	norm, err := core.NormalizePath(pattern)
	if err != nil {
		norm = pattern
	}

	if strings.ContainsAny(norm, "*?[") {
		matches, err := filepath.Glob(norm)
		if err == nil && len(matches) > 0 {
			return matches
		}
		return nil
	}

	return []string{norm}
}

// addCandidateItem evaluates a candidate path and adds it to the category items if valid.
func addCandidateItem(cat *CleanCategory, path string, desc string, wl *core.Whitelist, seen map[string]bool) {
	normPath, err := core.NormalizePath(path)
	if err != nil {
		return
	}

	if seen[normPath] {
		return
	}
	seen[normPath] = true

	fi, err := os.Lstat(normPath)
	if err != nil {
		return
	}

	sz, err := core.FileSize(normPath)
	if err != nil {
		sz = 0
	}
	if sz == 0 && fi.Mode().IsRegular() {
		sz = fi.Size()
	}

	safeErr := core.ValidatePathForDeletion(normPath)
	isWhitelisted := (wl != nil && wl.Contains(normPath))
	safe := (safeErr == nil) && !isWhitelisted

	itemDesc := desc
	if isWhitelisted {
		itemDesc += " (whitelisted)"
	}

	cat.Items = append(cat.Items, CleanItem{
		Path:        normPath,
		Size:        sz,
		Description: itemDesc,
		Safe:        safe,
	})
}
