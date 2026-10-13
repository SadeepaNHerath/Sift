package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestHome(t *testing.T) string {
	t.Helper()
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempHome, ".cache"))
	t.Setenv("SF_LOG_DIR", filepath.Join(tempHome, "Library", "Logs", "sf"))
	return tempHome
}

func captureOutput(f func() error) (string, error) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	rootCmd.SetOut(w)
	rootCmd.SetErr(w)

	err := f()

	_ = w.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	rootCmd.SetOut(oldStdout)
	rootCmd.SetErr(oldStderr)

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String(), err
}

func resetCleanFlags() {
	_ = cleanCmd.Flags().Set("whitelist", "false")
	_ = cleanCmd.Flags().Set("dry-run", "false")
	_ = cleanCmd.Flags().Set("all", "false")
	_ = cleanCmd.Flags().Set("yes", "false")
	_ = cleanCmd.Flags().Set("external", "")
	_ = cleanCmd.Flags().Set("models", "false")
	_ = cleanCmd.Flags().Set("ai", "false")
}

func executeCleanWithArgs(args []string) (string, error) {
	resetCleanFlags()
	rootCmd.SetArgs(append([]string{"clean"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func TestCleanCmd_FlagsRegistered(t *testing.T) {
	flags := []string{"dry-run", "all", "whitelist", "yes", "external", "models", "ai"}
	for _, f := range flags {
		flag := cleanCmd.Flags().Lookup(f)
		if flag == nil {
			t.Errorf("expected flag --%s to be registered on cleanCmd", f)
		}
	}
}

func TestCleanCmd_Whitelist(t *testing.T) {
	setupTestHome(t)

	out, err := executeCleanWithArgs([]string{"--whitelist"})
	if err != nil {
		t.Fatalf("sf clean --whitelist failed: %v", err)
	}

	if !strings.Contains(out, "Whitelist") {
		t.Errorf("expected output to contain 'Whitelist', got: %s", out)
	}
}

func TestCleanCmd_DryRun(t *testing.T) {
	home := setupTestHome(t)

	testCacheDir := filepath.Join(home, "Library", "Caches", "com.test.cmddryrun")
	if err := os.MkdirAll(testCacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	testFile := filepath.Join(testCacheDir, "data.cache")
	if err := os.WriteFile(testFile, []byte("dry run cache content"), 0644); err != nil {
		t.Fatalf("failed to write cache file: %v", err)
	}

	out, err := executeCleanWithArgs([]string{"--dry-run", "--yes"})
	if err != nil {
		t.Fatalf("sf clean --dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Dry Run") && !strings.Contains(out, "Preview") {
		t.Errorf("expected dry run header in output, got: %s", out)
	}

	// File must NOT be deleted
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatalf("CRITICAL: sf clean --dry-run deleted test file %s", testFile)
	}
}

func TestCleanCmd_ExecuteWithYes(t *testing.T) {
	home := setupTestHome(t)

	testCacheDir := filepath.Join(home, "Library", "Caches", "com.test.cmdrealclean")
	if err := os.MkdirAll(testCacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	testFile := filepath.Join(testCacheDir, "real_clean.cache")
	if err := os.WriteFile(testFile, []byte("real clean content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	out, err := executeCleanWithArgs([]string{"--yes"})
	if err != nil {
		t.Fatalf("sf clean --yes failed: %v", err)
	}

	if !strings.Contains(out, "Reclaimed") && !strings.Contains(out, "Summary") {
		t.Errorf("expected summary in output, got: %s", out)
	}

	// File should be moved to trash
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be removed, but it still exists", testFile)
	}
}

func TestCleanCmd_ExternalVolume(t *testing.T) {
	setupTestHome(t)
	tempVol := t.TempDir()

	dsStore := filepath.Join(tempVol, ".DS_Store")
	_ = os.WriteFile(dsStore, []byte("finder store"), 0644)
	appleDouble := filepath.Join(tempVol, "._sample")
	_ = os.WriteFile(appleDouble, []byte("resource fork"), 0644)

	out, err := executeCleanWithArgs([]string{"--external", tempVol, "--yes"})
	if err != nil {
		t.Fatalf("sf clean --external failed: %v", err)
	}

	if !strings.Contains(out, "External Volume") && !strings.Contains(out, "Reclaimed") {
		t.Errorf("expected external volume output, got: %s", out)
	}

	if _, err := os.Stat(dsStore); !os.IsNotExist(err) {
		t.Errorf("expected .DS_Store to be removed from external volume, but still exists")
	}
}

func TestCleanCmd_FilterByArg(t *testing.T) {
	home := setupTestHome(t)

	brewDir := filepath.Join(home, "Library", "Caches", "Homebrew")
	if err := os.MkdirAll(brewDir, 0755); err != nil {
		t.Fatalf("failed to create brew cache: %v", err)
	}
	_ = os.WriteFile(filepath.Join(brewDir, "bottle.tar.gz"), []byte("bottle archive"), 0644)

	out, err := executeCleanWithArgs([]string{"homebrew", "--dry-run", "--yes"})
	if err != nil {
		t.Fatalf("sf clean homebrew --dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Homebrew") {
		t.Errorf("expected output to contain Homebrew, got: %s", out)
	}
}
