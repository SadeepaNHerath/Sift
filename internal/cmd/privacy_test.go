package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func resetPrivacyFlags() {
	_ = privacyCmd.Flags().Set("dry-run", "false")
	_ = privacyCmd.Flags().Set("yes", "false")
	_ = privacyCmd.Flags().Set("json", "false")
	_ = privacyCmd.Flags().Set("clipboard", "false")
	_ = privacyCmd.Flags().Set("recent", "false")
	_ = privacyCmd.Flags().Set("diagnostics", "false")
	_ = privacyCmd.Flags().Set("browser-storage", "false")
}

func executePrivacyWithArgs(args []string) (string, error) {
	resetPrivacyFlags()
	rootCmd.SetArgs(append([]string{"privacy"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func TestPrivacyCmd_FlagsRegistered(t *testing.T) {
	flags := []string{"dry-run", "yes", "json", "clipboard", "recent", "diagnostics", "browser-storage"}
	for _, f := range flags {
		flag := privacyCmd.Flags().Lookup(f)
		if flag == nil {
			t.Errorf("expected flag --%s to be registered on privacyCmd", f)
		}
	}
}

func TestPrivacyCmd_DryRun(t *testing.T) {
	tempHome := setupTestHome(t)
	// Create mock recent items
	sharedFileListDir := filepath.Join(tempHome, "Library", "Application Support", "com.apple.sharedfilelist")
	_ = os.MkdirAll(sharedFileListDir, 0755)
	_ = os.WriteFile(filepath.Join(sharedFileListDir, "test.sfl3"), []byte("recent-test"), 0644)

	out, err := executePrivacyWithArgs([]string{"--dry-run"})
	if err != nil {
		t.Fatalf("unexpected error running sf privacy --dry-run: %v", err)
	}

	if !strings.Contains(out, "PRIVACY DRY RUN PREVIEW") {
		t.Errorf("expected dry run preview banner, got: %s", out)
	}
}

func TestPrivacyCmd_JSONOutput(t *testing.T) {
	setupTestHome(t)

	out, err := executePrivacyWithArgs([]string{"--json"})
	if err != nil {
		t.Fatalf("unexpected error running sf privacy --json: %v", err)
	}

	var targets []engine.PrivacyTarget
	if err := json.Unmarshal([]byte(out), &targets); err != nil {
		t.Fatalf("failed to parse JSON output: %v, raw: %s", err, out)
	}

	if len(targets) != 4 {
		t.Errorf("expected 4 targets in JSON, got %d", len(targets))
	}
}

func TestPrivacyCmd_NonInteractiveRequiresConfirmation(t *testing.T) {
	setupTestHome(t)

	_, err := executePrivacyWithArgs([]string{})
	if err == nil {
		t.Fatal("expected error requiring confirmation in non-interactive environment")
	}

	if !strings.Contains(err.Error(), "requires confirmation") {
		t.Errorf("expected confirmation error message, got: %v", err)
	}
}

func TestPrivacyCmd_ExecuteWithYes(t *testing.T) {
	tempHome := setupTestHome(t)
	crashDir := filepath.Join(tempHome, "Library", "Logs", "DiagnosticReports")
	_ = os.MkdirAll(crashDir, 0755)
	crashFile := filepath.Join(crashDir, "AppCrash.ips")
	_ = os.WriteFile(crashFile, []byte("crash-data"), 0644)

	out, err := executePrivacyWithArgs([]string{"--yes", "--diagnostics"})
	if err != nil {
		t.Fatalf("unexpected error running sf privacy --yes: %v", err)
	}

	if !strings.Contains(out, "Privacy wipe completed") {
		t.Errorf("expected completion message, got: %s", out)
	}

	// Verify crash file was removed
	if _, err := os.Stat(crashFile); !os.IsNotExist(err) {
		t.Errorf("crash report was not removed: %v", err)
	}
}
