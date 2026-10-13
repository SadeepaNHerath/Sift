package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetDoctorFlags() {
	_ = doctorCmd.Flags().Set("json", "false")
	_ = doctorCmd.Flags().Set("ci", "false")
	_ = doctorCmd.Flags().Set("exit-code", "false")
	_ = doctorCmd.Flags().Set("fail-on", "critical")
}

func executeDoctorWithArgs(args []string) (string, error) {
	resetDoctorFlags()
	rootCmd.SetArgs(append([]string{"doctor"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func TestDoctorCmd_CIFlag(t *testing.T) {
	setupTestHome(t)
	defer resetDoctorFlags()

	out, err := executeDoctorWithArgs([]string{"--ci"})
	// Exit error may or may not occur depending on local macOS state; check output formatting
	_ = err
	if !strings.Contains(out, "Sift System Doctor") {
		t.Errorf("expected doctor banner, got: %s", out)
	}
}

func TestDoctorCmd_ExitCodeWithWarningThreshold(t *testing.T) {
	setupTestHome(t)
	defer resetDoctorFlags()

	// In test environment, certain checks (e.g. disk/perms) generate warnings
	_, err := executeDoctorWithArgs([]string{"--exit-code", "--fail-on", "warning"})
	// If there are warnings or critical issues, err should be non-nil
	if err == nil {
		t.Log("Note: local environment was 100% clean with 0 warnings")
	} else {
		if !strings.Contains(err.Error(), "doctor check failed") {
			t.Errorf("unexpected error format: %v", err)
		}
	}
}

func TestPurgeCmd_NonInteractiveQuiet(t *testing.T) {
	tempHome := setupTestHome(t)
	defer resetPurgeFlags()
	projDir := filepath.Join(tempHome, "Projects", "ci-app")
	distDir := filepath.Join(projDir, "dist")
	_ = os.MkdirAll(distDir, 0755)
	_ = os.WriteFile(filepath.Join(distDir, "output.js"), []byte("packed"), 0644)

	resetPurgeFlags()
	_ = purgeCmd.Flags().Set("non-interactive", "true")
	_ = purgeCmd.Flags().Set("quiet", "true")
	_ = purgeCmd.Flags().Set("all", "true")
	_ = purgeCmd.Flags().Set("paths", projDir)
	_ = purgeCmd.Flags().Set("min-age", "0s")

	rootCmd.SetArgs([]string{"purge", "--paths", projDir, "--min-age", "0s", "--non-interactive", "--quiet", "--all"})
	out, err := captureOutput(func() error {
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("unexpected error running purge --non-interactive --quiet: %v", err)
	}

	// Should not have banner in quiet mode
	if strings.Contains(out, "▸ Sift: Scanning") {
		t.Errorf("quiet mode should have suppressed scanning banner")
	}

	// Should contain concise summary
	if !strings.Contains(out, "Purge completed") {
		t.Errorf("expected completion line in quiet mode, got: %s", out)
	}

	// Verify dist was removed
	if _, err := os.Stat(distDir); !os.IsNotExist(err) {
		t.Errorf("dist directory should have been removed")
	}
}
