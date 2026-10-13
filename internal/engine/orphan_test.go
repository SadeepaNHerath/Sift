package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferBundleID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"com.google.Chrome.plist", "com.google.chrome"},
		{"com.spotify.client.savedstate", "com.spotify.client"},
		{"group.com.tinyspeck.slackmacgap", "com.tinyspeck.slackmacgap"},
		{"ABCD1234EF.com.apple.developer.team", "com.apple.developer.team"},
		{"not-a-bundle-id", ""},
		{"JustAnAppName", ""},
		{"single.word", ""},
	}

	for _, tc := range tests {
		actual := InferBundleID(tc.input)
		if actual != tc.expected {
			t.Errorf("InferBundleID(%q): expected %q, got %q", tc.input, tc.expected, actual)
		}
	}
}

func TestInferAppName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Postman.plist", "Postman"},
		{"Slack.savedstate", "Slack"},
		{"Visual Studio Code", "Visual Studio Code"},
	}

	for _, tc := range tests {
		actual := InferAppName(tc.input)
		if actual != tc.expected {
			t.Errorf("InferAppName(%q): expected %q, got %q", tc.input, tc.expected, actual)
		}
	}
}

func TestNormalizeBundleID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"group.com.example.app", "com.example.app"},
		{"COM.EXAMPLE.APP", "com.example.app"},
		{"  group.net.whatsapp.WhatsApp  ", "net.whatsapp.whatsapp"},
	}

	for _, tc := range tests {
		actual := NormalizeBundleID(tc.input)
		if actual != tc.expected {
			t.Errorf("NormalizeBundleID(%q): expected %q, got %q", tc.input, tc.expected, actual)
		}
	}
}

func TestExtractVendorPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"com.google.Chrome", "com.google"},
		{"net.whatsapp.WhatsApp", "net.whatsapp"},
		{"org.videolan.vlc", "org.videolan"},
		{"singleword", ""},
	}

	for _, tc := range tests {
		actual := ExtractVendorPrefix(tc.input)
		if actual != tc.expected {
			t.Errorf("ExtractVendorPrefix(%q): expected %q, got %q", tc.input, tc.expected, actual)
		}
	}
}

func TestFamilySuppression(t *testing.T) {
	// If WhatsApp is installed, leftovers from Facebook / Instagram should be suppressed
	installed := []AppInfo{
		{
			Name:     "WhatsApp",
			BundleID: "net.whatsapp.WhatsApp",
		},
	}

	tempDir := t.TempDir()
	appSupportDir := filepath.Join(tempDir, "Library", "Application Support")
	_ = os.MkdirAll(appSupportDir, 0755)

	// Candidate leftover from Facebook family member
	fbPath := filepath.Join(appSupportDir, "com.facebook.Messenger")
	_ = os.Mkdir(fbPath, 0755)

	// Candidate leftover from completely unrelated uninstalled app
	orphanPath := filepath.Join(appSupportDir, "com.oldsoftware.tool")
	_ = os.Mkdir(orphanPath, 0755)

	dirs := []OrphanSearchDir{
		{Path: appSupportDir, Category: CategoryApplicationSupport},
	}

	orphans, err := FindOrphanedResourcesInDirs(installed, dirs)
	if err != nil {
		t.Fatalf("FindOrphanedResourcesInDirs failed: %v", err)
	}

	foundOrphan := false
	for _, o := range orphans {
		if o.Path == fbPath {
			t.Errorf("Facebook family leftover %s should have been suppressed because WhatsApp is installed", fbPath)
		}
		if o.Path == orphanPath {
			foundOrphan = true
		}
	}

	if !foundOrphan {
		t.Errorf("expected unrelated leftover %s to be detected as an orphan", orphanPath)
	}
}

func TestFindOrphanedResourcesInDirs(t *testing.T) {
	tempDir := t.TempDir()

	appSupportDir := filepath.Join(tempDir, "Library", "Application Support")
	cachesDir := filepath.Join(tempDir, "Library", "Caches")
	prefsDir := filepath.Join(tempDir, "Library", "Preferences")

	for _, d := range []string{appSupportDir, cachesDir, prefsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	installed := []AppInfo{
		{
			Name:       "CurrentEditor",
			BundleID:   "com.active.editor",
			Executable: "CurrentEditor",
		},
	}

	// 1. Installed app leftover: should NOT be an orphan
	activePref := filepath.Join(prefsDir, "com.active.editor.plist")
	_ = os.WriteFile(activePref, []byte("plist"), 0644)

	// 2. System com.apple leftover: should NOT be an orphan
	applePref := filepath.Join(prefsDir, "com.apple.finder.plist")
	_ = os.WriteFile(applePref, []byte("apple"), 0644)

	// 3. True orphan leftover from uninstalled app
	orphanPref := filepath.Join(prefsDir, "com.uninstalled.legacyapp.plist")
	_ = os.WriteFile(orphanPref, []byte("legacy"), 0644)

	orphanSupport := filepath.Join(appSupportDir, "com.uninstalled.legacyapp")
	_ = os.Mkdir(orphanSupport, 0755)

	dirs := []OrphanSearchDir{
		{Path: appSupportDir, Category: CategoryApplicationSupport},
		{Path: cachesDir, Category: CategoryCaches},
		{Path: prefsDir, Category: CategoryPreferences},
	}

	orphans, err := FindOrphanedResourcesInDirs(installed, dirs)
	if err != nil {
		t.Fatalf("FindOrphanedResourcesInDirs failed: %v", err)
	}

	foundPaths := make(map[string]bool)
	for _, o := range orphans {
		foundPaths[o.Path] = true
	}

	if foundPaths[activePref] {
		t.Errorf("installed app preference %s was incorrectly marked as orphan", activePref)
	}
	if foundPaths[applePref] {
		t.Errorf("system preference %s was incorrectly marked as orphan", applePref)
	}
	if !foundPaths[orphanPref] {
		t.Errorf("expected orphan preference %s to be found", orphanPref)
	}
	if !foundPaths[orphanSupport] {
		t.Errorf("expected orphan support directory %s to be found", orphanSupport)
	}
}
