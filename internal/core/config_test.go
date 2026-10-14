package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDirs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmpDir, "cache"))

	cfg := ConfigDir()
	if cfg != filepath.Join(tmpDir, "config", "sf") {
		t.Fatalf("unexpected ConfigDir: %s", cfg)
	}

	cache := CacheDir()
	if cache != filepath.Join(tmpDir, "cache", "sf") {
		t.Fatalf("unexpected CacheDir: %s", cache)
	}

	log := LogDir()
	if log == "" {
		t.Fatalf("LogDir should not be empty")
	}

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	for _, d := range []string{cfg, cache, log} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Fatalf("expected directory %s to exist", d)
		}
	}
}

func TestWhitelist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	w, err := LoadWhitelist()
	if err != nil {
		t.Fatalf("LoadWhitelist error: %v", err)
	}
	if len(w.List()) != 0 {
		t.Fatalf("expected empty initial whitelist")
	}

	if err := w.Add("/Users/test/Protected"); err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if err := w.Add("/Users/test/Protected"); err != nil { // duplicate
		t.Fatalf("Duplicate Add should succeed: %v", err)
	}

	if !w.Contains("/Users/test/Protected") {
		t.Fatalf("Contains failed for exact match")
	}
	if !w.Contains("/users/test/protected") {
		t.Fatalf("Contains failed for case-insensitive match on APFS")
	}
	if !w.Contains("/Users/test/Protected/subdir/file.txt") {
		t.Fatalf("Contains failed for child path")
	}
	if !w.Contains("/USERS/TEST/PROTECTED/SUBDIR/FILE.TXT") {
		t.Fatalf("Contains failed for case-insensitive child path on APFS")
	}
	if w.Contains("/Users/test/Other") {
		t.Fatalf("Contains returned true for non-whitelisted path")
	}

	if err := w.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload
	w2, err := LoadWhitelist()
	if err != nil {
		t.Fatalf("LoadWhitelist reloaded failed: %v", err)
	}
	if !w2.Contains("/Users/test/Protected") {
		t.Fatalf("Reloaded whitelist missing path")
	}

	if err := w2.Remove("/Users/test/Protected"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if w2.Contains("/Users/test/Protected") {
		t.Fatalf("Path should be removed")
	}

	if err := w2.Remove("/non/existent"); err == nil {
		t.Fatalf("Remove of non-existent path should return error")
	}
}

func TestPreferences(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	p, err := LoadPreferences()
	if err != nil {
		t.Fatalf("LoadPreferences failed: %v", err)
	}
	if !p.ShowCat {
		t.Fatalf("expected ShowCat to default to true")
	}

	p.ColorScheme = "dracula"
	p.ShowCat = false
	if err := p.Save(); err != nil {
		t.Fatalf("Save preferences failed: %v", err)
	}

	p2, err := LoadPreferences()
	if err != nil {
		t.Fatalf("Reload preferences failed: %v", err)
	}
	if p2.ColorScheme != "dracula" || p2.ShowCat {
		t.Fatalf("Preferences did not persist: %+v", p2)
	}
}

func TestPurgePaths(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	defaults := DefaultPurgePaths()
	if len(defaults) != 5 {
		t.Fatalf("expected 5 default purge paths, got %d", len(defaults))
	}

	loaded, err := LoadPurgePaths()
	if err != nil {
		t.Fatalf("LoadPurgePaths failed: %v", err)
	}
	if len(loaded) != len(defaults) {
		t.Fatalf("expected defaults when file missing")
	}

	custom := []string{"/custom/projects", "/custom/work"}
	if err := SavePurgePaths(custom); err != nil {
		t.Fatalf("SavePurgePaths failed: %v", err)
	}

	loaded2, err := LoadPurgePaths()
	if err != nil {
		t.Fatalf("LoadPurgePaths after save failed: %v", err)
	}
	if len(loaded2) != 2 || loaded2[0] != "/custom/projects" {
		t.Fatalf("Purge paths mismatch: %v", loaded2)
	}
}
