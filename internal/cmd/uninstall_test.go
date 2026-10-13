package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Spacefold/internal/engine"
)

func TestUninstallCmdFlagsRegistered(t *testing.T) {
	flags := []string{"dry-run", "all", "yes", "data"}
	for _, f := range flags {
		flag := uninstallCmd.Flags().Lookup(f)
		if flag == nil {
			t.Errorf("expected flag --%s to be registered on uninstallCmd", f)
		}
	}

	// Verify short flag -y
	shortFlag := uninstallCmd.Flags().ShorthandLookup("y")
	if shortFlag == nil || shortFlag.Name != "yes" {
		t.Errorf("expected shorthand flag -y to map to --yes")
	}
}

func TestUninstallCmd_NonInteractiveRequiresArg(t *testing.T) {
	buf := new(bytes.Buffer)
	uninstallCmd.SetOut(buf)
	uninstallCmd.SetErr(buf)

	err := runUninstall(uninstallCmd, []string{})
	if err == nil {
		t.Fatalf("expected error when running uninstall without arguments non-interactively")
	}
	if !strings.Contains(err.Error(), "application name is required") {
		t.Errorf("expected error message about required app name, got: %v", err)
	}
}

func TestUninstallCmd_AppNotFound(t *testing.T) {
	buf := new(bytes.Buffer)
	uninstallCmd.SetOut(buf)
	uninstallCmd.SetErr(buf)

	err := runUninstall(uninstallCmd, []string{"CompletelyNonExistentApplication12345"})
	if err == nil {
		t.Fatalf("expected error for non-existent application")
	}
	if !strings.Contains(err.Error(), "no installed application found matching") {
		t.Errorf("expected 'no installed application found matching' error, got: %v", err)
	}
}

func TestUninstallCmd_ProtectedAppleApp(t *testing.T) {
	buf := new(bytes.Buffer)
	uninstallCmd.SetOut(buf)
	uninstallCmd.SetErr(buf)

	// Safari is always protected on macOS
	err := runUninstall(uninstallCmd, []string{"Safari"})
	if err == nil {
		t.Fatalf("expected error when attempting to uninstall Safari")
	}
	if !strings.Contains(err.Error(), "cannot uninstall protected macOS application") {
		t.Errorf("expected protected macOS application error, got: %v", err)
	}
}

func TestCheckConfigReferences(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	zshrcPath := filepath.Join(tempHome, ".zshrc")
	zshrcContent := `# User configuration
alias testapp="/Applications/TestApp.app/Contents/MacOS/TestApp"
export PATH="$PATH:/usr/local/bin"
`
	if err := os.WriteFile(zshrcPath, []byte(zshrcContent), 0644); err != nil {
		t.Fatalf("failed to write mock .zshrc: %v", err)
	}

	app := engine.AppInfo{
		Name:       "TestApp",
		Executable: "TestApp",
		BundleID:   "com.example.testapp",
		Path:       "/Applications/TestApp.app",
	}

	refs := checkConfigReferences(app)
	if len(refs) == 0 {
		t.Fatalf("expected to find reference in .zshrc")
	}

	if refs[0].Line != 2 {
		t.Errorf("expected line 2, got line %d", refs[0].Line)
	}
	if !strings.Contains(refs[0].Text, "TestApp") {
		t.Errorf("expected text to mention TestApp, got %q", refs[0].Text)
	}
}

func createTestBundleForUninstall(t *testing.T, dir, appName, bundleID string) string {
	t.Helper()
	appPath := filepath.Join(dir, appName+".app")
	contentsDir := filepath.Join(appPath, "Contents")
	macosDir := filepath.Join(contentsDir, "MacOS")
	if err := os.MkdirAll(macosDir, 0755); err != nil {
		t.Fatalf("failed to create bundle dirs: %v", err)
	}
	_ = os.WriteFile(filepath.Join(macosDir, appName), []byte("#!/bin/sh\n"), 0755)

	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleName</key><string>` + appName + `</string>
<key>CFBundleIdentifier</key><string>` + bundleID + `</string>
<key>CFBundleExecutable</key><string>` + appName + `</string>
</dict></plist>`
	_ = os.WriteFile(filepath.Join(contentsDir, "Info.plist"), []byte(plist), 0644)
	return appPath
}

func TestUninstallCmd_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	appPath := createTestBundleForUninstall(t, tempDir, "DryRunTestApp", "com.dryrun.testapp")

	buf := new(bytes.Buffer)
	cmd := &cobra.Command{Use: "uninstall"}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Set dry run flag
	flagDryRun = true
	flagYes = true
	defer func() {
		flagDryRun = false
		flagYes = false
	}()

	err := runUninstall(cmd, []string{appPath})
	if err != nil {
		t.Fatalf("runUninstall with --dry-run failed: %v", err)
	}

	outStr := buf.String()
	if !strings.Contains(outStr, "[DRY RUN]") {
		t.Errorf("expected output to contain '[DRY RUN]', got:\n%s", outStr)
	}

	// Verify app was NOT deleted
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		t.Errorf("app bundle %s was deleted during dry run!", appPath)
	}
}
