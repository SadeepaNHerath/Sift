package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createMockAppBundle(t *testing.T, dir, appName, bundleID, version, execName string, isApple bool) string {
	t.Helper()

	appPath := filepath.Join(dir, appName+".app")
	contentsDir := filepath.Join(appPath, "Contents")
	macosDir := filepath.Join(contentsDir, "MacOS")

	if err := os.MkdirAll(macosDir, 0755); err != nil {
		t.Fatalf("failed to create mock bundle directory: %v", err)
	}

	execPath := filepath.Join(macosDir, execName)
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test\n"), 0755); err != nil {
		t.Fatalf("failed to write mock executable: %v", err)
	}

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDisplayName</key>
    <string>` + appName + `</string>
    <key>CFBundleName</key>
    <string>` + appName + `</string>
    <key>CFBundleIdentifier</key>
    <string>` + bundleID + `</string>
    <key>CFBundleShortVersionString</key>
    <string>` + version + `</string>
    <key>CFBundleVersion</key>
    <string>` + version + `</string>
    <key>CFBundleExecutable</key>
    <string>` + execName + `</string>
</dict>
</plist>`

	plistPath := filepath.Join(contentsDir, "Info.plist")
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		t.Fatalf("failed to write Info.plist: %v", err)
	}

	return appPath
}

func TestParseInfo_ThirdPartyApp(t *testing.T) {
	tempDir := t.TempDir()
	appPath := createMockAppBundle(t, tempDir, "AcmeApp", "com.acme.widget", "2.1.0", "AcmeApp", false)

	info, err := ParseInfo(appPath)
	if err != nil {
		t.Fatalf("expected ParseInfo to succeed, got %v", err)
	}

	if info.Name != "AcmeApp" {
		t.Errorf("expected Name 'AcmeApp', got %q", info.Name)
	}
	if info.BundleID != "com.acme.widget" {
		t.Errorf("expected BundleID 'com.acme.widget', got %q", info.BundleID)
	}
	if info.Version != "2.1.0" {
		t.Errorf("expected Version '2.1.0', got %q", info.Version)
	}
	if info.Executable != "AcmeApp" {
		t.Errorf("expected Executable 'AcmeApp', got %q", info.Executable)
	}
	if info.IsApple {
		t.Errorf("expected IsApple false for third party app")
	}
	if info.Protected {
		t.Errorf("expected Protected false for third party app")
	}
	if info.Size <= 0 {
		t.Errorf("expected Size > 0, got %d", info.Size)
	}
	if info.NormalizedName() != "acmeapp" {
		t.Errorf("expected NormalizedName 'acmeapp', got %q", info.NormalizedName())
	}
	if info.VendorPrefix() != "com.acme" {
		t.Errorf("expected VendorPrefix 'com.acme', got %q", info.VendorPrefix())
	}
}

func TestParseInfo_AppleProtectedApp(t *testing.T) {
	tempDir := t.TempDir()
	appPath := createMockAppBundle(t, tempDir, "AppleMusic", "com.apple.Music", "1.4.0", "Music", true)

	info, err := ParseInfo(appPath)
	if err != nil {
		t.Fatalf("expected ParseInfo to succeed, got %v", err)
	}

	if !info.IsApple {
		t.Errorf("expected IsApple true for com.apple.* bundle ID")
	}
	if !info.Protected {
		t.Errorf("expected Protected true for com.apple.* bundle ID")
	}
}

func TestParseInfo_InvalidPaths(t *testing.T) {
	tempDir := t.TempDir()

	// Non-existent path
	_, err := ParseInfo(filepath.Join(tempDir, "DoesNotExist.app"))
	if err == nil {
		t.Errorf("expected error for non-existent path")
	}

	// Regular file instead of app directory
	filePath := filepath.Join(tempDir, "somefile.txt")
	_ = os.WriteFile(filePath, []byte("plain text"), 0644)
	_, err = ParseInfo(filePath)
	if err == nil {
		t.Errorf("expected error for regular non-bundle file")
	}
}

func TestAppCacheLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	cacheFile := filepath.Join(tempDir, "cache.json")

	appPath := createMockAppBundle(t, tempDir, "SampleApp", "com.sample.tool", "1.0.0", "SampleApp", false)
	info, err := ParseInfo(appPath)
	if err != nil {
		t.Fatalf("ParseInfo failed: %v", err)
	}

	// 1. Save cache
	apps := []AppInfo{*info}
	if err := saveCache(cacheFile, apps); err != nil {
		t.Fatalf("saveCache failed: %v", err)
	}

	// 2. Load valid cache
	loaded, ok := loadValidCache(cacheFile, DefaultCacheTTL)
	if !ok {
		t.Fatalf("loadValidCache failed to load fresh cache")
	}
	if len(loaded) != 1 || loaded[0].BundleID != "com.sample.tool" {
		t.Fatalf("unexpected loaded cache contents: %+v", loaded)
	}

	// 3. Expired cache check
	_, expiredOk := loadValidCache(cacheFile, -1*time.Minute)
	if expiredOk {
		t.Errorf("expected expired cache to return false")
	}

	// 4. Invalidate when bundle directory mod time changes
	futureTime := time.Now().Add(5 * time.Second)
	_ = os.Chtimes(appPath, futureTime, futureTime)
	_, modOk := loadValidCache(cacheFile, DefaultCacheTTL)
	if modOk {
		t.Errorf("expected cache to be invalidated when app mod time changes")
	}
}

func TestScanInstalledApps(t *testing.T) {
	// Real scan across system
	apps, err := ScanInstalledApps(false)
	if err != nil {
		t.Fatalf("ScanInstalledApps failed: %v", err)
	}

	if len(apps) == 0 {
		t.Logf("Notice: ScanInstalledApps returned 0 apps (test sandbox or minimal system)")
	} else {
		foundApple := false
		for _, a := range apps {
			if a.IsApple {
				foundApple = true
				if !a.Protected {
					t.Errorf("app %s has IsApple=true but Protected=false", a.Name)
				}
			}
		}
		t.Logf("Discovered %d installed apps (has Apple apps: %v)", len(apps), foundApple)
	}
}
