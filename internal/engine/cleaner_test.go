package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

// setupMockHome sets up an isolated temporary home directory and config environment.
func setupMockHome(t *testing.T) string {
	t.Helper()
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempHome, ".cache"))
	t.Setenv("SF_LOG_DIR", filepath.Join(tempHome, "Library", "Logs", "sf"))
	return tempHome
}

func TestCategoryMetadata(t *testing.T) {
	if len(AllCategories) != 14 {
		t.Fatalf("expected 14 cleanup categories, got %d", len(AllCategories))
	}

	for _, meta := range AllCategories {
		if meta.ID == "" {
			t.Errorf("category ID should not be empty")
		}
		if meta.Name == "" {
			t.Errorf("category Name should not be empty")
		}
		if meta.Description == "" {
			t.Errorf("category Description should not be empty")
		}

		retrieved, err := GetCategoryMeta(meta.ID)
		if err != nil {
			t.Errorf("GetCategoryMeta(%q) returned error: %v", meta.ID, err)
		}
		if retrieved.ID != meta.ID {
			t.Errorf("expected ID %q, got %q", meta.ID, retrieved.ID)
		}
	}

	// Unknown category ID
	_, err := GetCategoryMeta("non-existent-category")
	if err == nil {
		t.Errorf("expected error for non-existent category, got nil")
	}

	// External volume meta
	extMeta, err := GetCategoryMeta(CatExternalVolume)
	if err != nil {
		t.Errorf("expected GetCategoryMeta(CatExternalVolume) to succeed, got %v", err)
	}
	if extMeta.ID != CatExternalVolume {
		t.Errorf("expected %q, got %q", CatExternalVolume, extMeta.ID)
	}
}

func TestScanCategory_UnknownCategory(t *testing.T) {
	_, err := ScanCategory("invalid-id", nil)
	if err == nil {
		t.Fatalf("expected error for invalid category, got nil")
	}
}

func TestScanCategory_UserEssentials(t *testing.T) {
	home := setupMockHome(t)

	// Create test cache folder and log folder
	testCache := filepath.Join(home, "Library", "Caches", "com.example.testtool")
	if err := os.MkdirAll(testCache, 0755); err != nil {
		t.Fatalf("failed to create test cache: %v", err)
	}
	testFile := filepath.Join(testCache, "cache.dat")
	if err := os.WriteFile(testFile, []byte("test cache contents"), 0644); err != nil {
		t.Fatalf("failed to write test cache file: %v", err)
	}

	logsDir := filepath.Join(home, "Library", "Logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatalf("failed to create test logs dir: %v", err)
	}
	testLog := filepath.Join(logsDir, "com.example.testtool.log")
	if err := os.WriteFile(testLog, []byte("log data 12345"), 0644); err != nil {
		t.Fatalf("failed to write test log file: %v", err)
	}

	cat, err := ScanCategory(CatUserEssentials, nil)
	if err != nil {
		t.Fatalf("ScanCategory failed: %v", err)
	}

	if cat.ID != CatUserEssentials {
		t.Errorf("expected ID %q, got %q", CatUserEssentials, cat.ID)
	}
	if cat.TotalSize <= 0 {
		t.Errorf("expected TotalSize > 0, got %d", cat.TotalSize)
	}

	foundCache := false
	foundLog := false
	for _, itm := range cat.Items {
		if strings.Contains(itm.Path, "com.example.testtool") {
			foundCache = true
			if !itm.Safe {
				t.Errorf("expected item %q to be safe", itm.Path)
			}
		}
		if strings.Contains(itm.Path, "com.example.testtool.log") {
			foundLog = true
		}
	}

	if !foundCache {
		t.Errorf("expected test cache directory to be scanned")
	}
	if !foundLog {
		t.Errorf("expected test log file to be scanned")
	}
}

func TestScanCategory_Browsers_PreservesSensitiveData(t *testing.T) {
	home := setupMockHome(t)

	// Setup Chrome cache directory
	chromeCache := filepath.Join(home, "Library", "Caches", "Google", "Chrome", "Default", "Cache")
	if err := os.MkdirAll(chromeCache, 0755); err != nil {
		t.Fatalf("failed to create chrome cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chromeCache, "data_0"), []byte("cached content"), 0644); err != nil {
		t.Fatalf("failed to create cache file: %v", err)
	}

	// Setup Chrome user data directory that must NEVER be cleaned
	chromeUserData := filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default")
	if err := os.MkdirAll(chromeUserData, 0755); err != nil {
		t.Fatalf("failed to create chrome user data dir: %v", err)
	}
	historyFile := filepath.Join(chromeUserData, "History")
	_ = os.WriteFile(historyFile, []byte("sensitive browsing history"), 0600)
	cookiesFile := filepath.Join(chromeUserData, "Cookies")
	_ = os.WriteFile(cookiesFile, []byte("sensitive user cookies"), 0600)
	bookmarksFile := filepath.Join(chromeUserData, "Bookmarks")
	_ = os.WriteFile(bookmarksFile, []byte("user bookmarks"), 0600)
	loginDataFile := filepath.Join(chromeUserData, "Login Data")
	_ = os.WriteFile(loginDataFile, []byte("saved passwords"), 0600)

	cat, err := ScanCategory(CatBrowsers, nil)
	if err != nil {
		t.Fatalf("ScanCategory failed: %v", err)
	}

	foundCache := false
	for _, itm := range cat.Items {
		if strings.Contains(itm.Path, filepath.Join("Google", "Chrome", "Default", "Cache")) {
			foundCache = true
		}
		// Strict invariant verification
		lower := strings.ToLower(itm.Path)
		if strings.HasSuffix(lower, "/history") || strings.HasSuffix(lower, "/cookies") ||
			strings.HasSuffix(lower, "/bookmarks") || strings.HasSuffix(lower, "/login data") {
			t.Fatalf("CRITICAL SAFETY VIOLATION: Browser cleaner targeted protected profile item: %s", itm.Path)
		}
	}

	if !foundCache {
		t.Errorf("expected Chrome disk cache to be detected")
	}
}

func TestScanCategory_DeveloperTools(t *testing.T) {
	home := setupMockHome(t)

	derivedData := filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData", "App-abc123xyz")
	if err := os.MkdirAll(derivedData, 0755); err != nil {
		t.Fatalf("failed to create derived data: %v", err)
	}
	_ = os.WriteFile(filepath.Join(derivedData, "build.obj"), []byte("binary data"), 0644)

	npmCache := filepath.Join(home, ".npm", "_cacache", "content-v2")
	if err := os.MkdirAll(npmCache, 0755); err != nil {
		t.Fatalf("failed to create npm cache: %v", err)
	}
	_ = os.WriteFile(filepath.Join(npmCache, "pkg.tgz"), []byte("npm package archive"), 0644)

	cat, err := ScanCategory(CatDeveloperTools, nil)
	if err != nil {
		t.Fatalf("ScanCategory failed: %v", err)
	}

	foundDerivedData := false
	foundNPM := false
	for _, itm := range cat.Items {
		if strings.Contains(itm.Path, "DerivedData") {
			foundDerivedData = true
		}
		if strings.Contains(itm.Path, "_cacache") {
			foundNPM = true
		}
	}

	if !foundDerivedData {
		t.Errorf("expected Xcode DerivedData to be found")
	}
	if !foundNPM {
		t.Errorf("expected npm global cache to be found")
	}
}

func TestScanCategory_Homebrew(t *testing.T) {
	home := setupMockHome(t)

	brewCache := filepath.Join(home, "Library", "Caches", "Homebrew")
	if err := os.MkdirAll(brewCache, 0755); err != nil {
		t.Fatalf("failed to create brew cache: %v", err)
	}
	_ = os.WriteFile(filepath.Join(brewCache, "formula.bottle.tar.gz"), []byte("brew bottle archive"), 0644)

	cat, err := ScanCategory(CatHomebrew, nil)
	if err != nil {
		t.Fatalf("ScanCategory failed: %v", err)
	}

	if len(cat.Items) == 0 {
		t.Fatalf("expected Homebrew cache item to be scanned")
	}
	if !strings.Contains(cat.Items[0].Path, "Homebrew") {
		t.Errorf("expected path to contain Homebrew, got %s", cat.Items[0].Path)
	}
}

func TestScanCategory_StaleBrowserProfiles(t *testing.T) {
	home := setupMockHome(t)

	playwrightCache := filepath.Join(home, ".cache", "ms-playwright")
	if err := os.MkdirAll(playwrightCache, 0755); err != nil {
		t.Fatalf("failed to create playwright cache: %v", err)
	}
	_ = os.WriteFile(filepath.Join(playwrightCache, "chromium.zip"), []byte("browser bundle"), 0644)

	cat, err := ScanCategory(CatBrowserProfiles, nil)
	if err != nil {
		t.Fatalf("ScanCategory failed: %v", err)
	}

	foundPlaywright := false
	for _, itm := range cat.Items {
		if strings.Contains(itm.Path, "ms-playwright") {
			foundPlaywright = true
		}
	}
	if !foundPlaywright {
		t.Errorf("expected Playwright cache to be detected")
	}
}

func TestScanCategory_AppLeftovers(t *testing.T) {
	home := setupMockHome(t)

	// Create an uninstalled app saved state
	savedState := filepath.Join(home, "Library", "Saved Application State", "com.obsolete.tool.savedState")
	if err := os.MkdirAll(savedState, 0755); err != nil {
		t.Fatalf("failed to create saved state: %v", err)
	}
	_ = os.WriteFile(filepath.Join(savedState, "windows.plist"), []byte("saved window state"), 0644)

	cat, err := ScanCategory(CatAppLeftovers, nil)
	if err != nil {
		t.Fatalf("ScanCategory failed: %v", err)
	}

	foundLeftover := false
	for _, itm := range cat.Items {
		if strings.Contains(itm.Path, "com.obsolete.tool.savedState") {
			foundLeftover = true
		}
	}
	if !foundLeftover {
		t.Errorf("expected orphaned savedState leftover to be detected")
	}
}

func TestScanCategory_WhitelistHonored(t *testing.T) {
	home := setupMockHome(t)

	testCache := filepath.Join(home, "Library", "Caches", "com.mycompany.whitelistedapp")
	if err := os.MkdirAll(testCache, 0755); err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	_ = os.WriteFile(filepath.Join(testCache, "cache.db"), []byte("protected app cache"), 0644)

	wl := &core.Whitelist{}
	if err := wl.Add(testCache); err != nil {
		t.Fatalf("failed to add path to whitelist: %v", err)
	}

	cat, err := ScanCategory(CatUserEssentials, wl)
	if err != nil {
		t.Fatalf("ScanCategory failed: %v", err)
	}

	for _, itm := range cat.Items {
		if itm.Path == testCache {
			if itm.Safe {
				t.Fatalf("expected whitelisted item to have Safe=false, got true")
			}
			if !strings.Contains(itm.Description, "whitelisted") {
				t.Errorf("expected description to contain 'whitelisted', got %q", itm.Description)
			}
		}
	}
}

func TestScanAll(t *testing.T) {
	setupMockHome(t)

	categories, err := ScanAll(nil)
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}

	if len(categories) != 14 {
		t.Fatalf("expected 14 categories from ScanAll, got %d", len(categories))
	}

	for i, c := range categories {
		if c.ID != AllCategories[i].ID {
			t.Errorf("expected category %d to be %q, got %q", i, AllCategories[i].ID, c.ID)
		}
	}
}

func TestCleanCategoryExecute_DryRun(t *testing.T) {
	home := setupMockHome(t)

	testDir := filepath.Join(home, "Library", "Caches", "com.test.dryrun")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
	testFile := filepath.Join(testDir, "file.bin")
	if err := os.WriteFile(testFile, []byte("dry run content data"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cat := &CleanCategory{
		ID:        CatUserEssentials,
		Name:      "User Essentials",
		TotalSize: int64(len("dry run content data")),
		Items: []CleanItem{
			{
				Path:        testFile,
				Size:        int64(len("dry run content data")),
				Description: "Dry run test file",
				Safe:        true,
			},
		},
	}

	logger, _ := core.NewLogger()
	if logger != nil {
		defer logger.Close()
	}

	rep, err := CleanCategoryExecute(cat, true, logger)
	if err != nil {
		t.Fatalf("CleanCategoryExecute dry run failed: %v", err)
	}

	if len(rep.Removed) != 1 {
		t.Fatalf("expected 1 item in Removed (dry run), got %d", len(rep.Removed))
	}
	if rep.Removed[0].TrashPath != "[dry-run]" {
		t.Errorf("expected TrashPath '[dry-run]', got %q", rep.Removed[0].TrashPath)
	}
	if rep.TotalBytes != int64(len("dry run content data")) {
		t.Errorf("expected %d bytes, got %d", len("dry run content data"), rep.TotalBytes)
	}

	// Invariant: file must NOT have been deleted
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatalf("CRITICAL: dry run deleted the file %s from disk!", testFile)
	}
}

func TestCleanCategoryExecute_ActualClean(t *testing.T) {
	home := setupMockHome(t)

	testDir := filepath.Join(home, "Library", "Caches", "com.test.realclean")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
	testFile := filepath.Join(testDir, "real_clean_file.bin")
	content := []byte("clean this real file")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cat := &CleanCategory{
		ID:        CatUserEssentials,
		Name:      "User Essentials",
		TotalSize: int64(len(content)),
		Items: []CleanItem{
			{
				Path:        testFile,
				Size:        int64(len(content)),
				Description: "Real clean test file",
				Safe:        true,
			},
		},
	}

	logger, err := core.NewLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Close()

	rep, err := CleanCategoryExecute(cat, false, logger)
	if err != nil {
		t.Fatalf("CleanCategoryExecute failed: %v", err)
	}

	if len(rep.Removed) != 1 {
		t.Fatalf("expected 1 removed item, got %d (failed: %v)", len(rep.Removed), rep.Failed)
	}
	if rep.TotalBytes != int64(len(content)) {
		t.Errorf("expected %d bytes reclaimed, got %d", len(content), rep.TotalBytes)
	}

	// File should no longer exist at original path
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be moved to trash, but it still exists", testFile)
	}
}

func TestCleanCategoryExecute_BlocksProtectedPaths(t *testing.T) {
	setupMockHome(t)

	// An item targeting Apple system path or protected directory
	cat := &CleanCategory{
		ID:   CatUserEssentials,
		Name: "User Essentials",
		Items: []CleanItem{
			{
				Path:        "/System/Library",
				Size:        1000,
				Description: "System library",
				Safe:        false,
			},
			{
				Path:        "/usr/bin/tar",
				Size:        2000,
				Description: "Tar binary",
				Safe:        false,
			},
		},
	}

	rep, err := CleanCategoryExecute(cat, false, nil)
	if err != nil {
		t.Fatalf("CleanCategoryExecute returned error: %v", err)
	}

	if len(rep.Removed) > 0 {
		t.Fatalf("CRITICAL SAFETY VIOLATION: Removed protected system paths! %v", rep.Removed)
	}
	if len(rep.Failed) != 2 {
		t.Errorf("expected 2 failed items, got %d", len(rep.Failed))
	}
}

func TestScanExternalVolume(t *testing.T) {
	tempVol := t.TempDir()

	dsStore := filepath.Join(tempVol, ".DS_Store")
	_ = os.WriteFile(dsStore, []byte("desktop services store"), 0644)
	appleDouble := filepath.Join(tempVol, "._sample_file")
	_ = os.WriteFile(appleDouble, []byte("apple double resource fork"), 0644)
	tempItems := filepath.Join(tempVol, ".TemporaryItems")
	_ = os.MkdirAll(tempItems, 0755)

	cat, err := ScanExternalVolume(tempVol, nil)
	if err != nil {
		t.Fatalf("ScanExternalVolume failed: %v", err)
	}

	if cat.ID != CatExternalVolume {
		t.Errorf("expected ID %q, got %q", CatExternalVolume, cat.ID)
	}
	if len(cat.Items) < 2 {
		t.Fatalf("expected at least 2 junk items, got %d", len(cat.Items))
	}

	// Clean external volume items
	rep, err := CleanCategoryExecute(cat, false, nil)
	if err != nil {
		t.Fatalf("CleanCategoryExecute on external volume failed: %v", err)
	}

	if len(rep.Removed) == 0 {
		t.Errorf("expected items to be removed from external volume")
	}
}
