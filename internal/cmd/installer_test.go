package cmd

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetInstallerFlags() {
	_ = installerCmd.Flags().Set("dry-run", "false")
	_ = installerCmd.Flags().Set("yes", "false")
}

func executeInstallerWithArgs(args []string) (string, error) {
	resetInstallerFlags()
	rootCmd.SetArgs(append([]string{"installer"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func createTestZipFile(t *testing.T, path string, innerFiles []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for _, name := range innerFiles {
		ew, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = ew.Write([]byte("installer content"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallerCmd_NoFilesFound(t *testing.T) {
	_ = setupTestHome(t)

	out, err := executeInstallerWithArgs([]string{"--dry-run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No leftover installer packages") {
		t.Errorf("expected no installers message, got: %s", out)
	}
}

func TestInstallerCmd_DryRun(t *testing.T) {
	tempHome := setupTestHome(t)
	downloads := filepath.Join(tempHome, "Downloads")
	if err := os.MkdirAll(downloads, 0755); err != nil {
		t.Fatal(err)
	}

	dmgPath := filepath.Join(downloads, "Chrome.dmg")
	if err := os.WriteFile(dmgPath, []byte("dmg data"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := executeInstallerWithArgs([]string{"--dry-run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "DRY RUN PREVIEW") {
		t.Errorf("expected dry run preview banner, got: %s", out)
	}
	if !strings.Contains(out, "Chrome.dmg") {
		t.Errorf("expected Chrome.dmg in dry run output, got: %s", out)
	}

	// Verify file was NOT removed
	if _, err := os.Stat(dmgPath); err != nil {
		t.Errorf("installer file was deleted during dry run!")
	}
}

func TestInstallerCmd_NonInteractiveRequiresConfirmation(t *testing.T) {
	tempHome := setupTestHome(t)
	downloads := filepath.Join(tempHome, "Downloads")
	if err := os.MkdirAll(downloads, 0755); err != nil {
		t.Fatal(err)
	}
	pkgPath := filepath.Join(downloads, "Node.pkg")
	if err := os.WriteFile(pkgPath, []byte("pkg data"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := executeInstallerWithArgs([]string{})
	if err == nil {
		t.Fatal("expected error requiring confirmation in non-interactive environment")
	}

	if !strings.Contains(err.Error(), "requires confirmation") {
		t.Errorf("expected confirmation error message, got: %v", err)
	}
}

func TestInstallerCmd_ExecuteWithYes(t *testing.T) {
	tempHome := setupTestHome(t)
	downloads := filepath.Join(tempHome, "Downloads")
	if err := os.MkdirAll(downloads, 0755); err != nil {
		t.Fatal(err)
	}

	dmgPath := filepath.Join(downloads, "VSCode.dmg")
	if err := os.WriteFile(dmgPath, []byte("dmg data"), 0644); err != nil {
		t.Fatal(err)
	}

	zipInstaller := filepath.Join(downloads, "Spotify.zip")
	createTestZipFile(t, zipInstaller, []string{"Spotify.app/Contents/MacOS/Spotify"})

	out, err := executeInstallerWithArgs([]string{"--yes"})
	if err != nil {
		t.Fatalf("installer cleanup failed: %v", err)
	}

	if !strings.Contains(out, "Installer cleanup completed") {
		t.Errorf("expected completion message, got: %s", out)
	}

	// Verify files were removed
	if _, err := os.Stat(dmgPath); !os.IsNotExist(err) {
		t.Errorf("dmg installer should have been removed")
	}
	if _, err := os.Stat(zipInstaller); !os.IsNotExist(err) {
		t.Errorf("zip installer should have been removed")
	}
}
