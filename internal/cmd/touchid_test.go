package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Spacefold/internal/engine"
)

func resetTouchIDFlags() {
	_ = touchidCmd.Flags().Set("enable", "false")
	_ = touchidCmd.Flags().Set("disable", "false")
	_ = touchidCmd.Flags().Set("status", "false")
}

func executeTouchIDWithArgs(args []string) (string, error) {
	resetTouchIDFlags()
	rootCmd.SetArgs(append([]string{"touchid"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func TestTouchIDCmd_FlagsRegistered(t *testing.T) {
	flags := []string{"enable", "disable", "status"}
	for _, f := range flags {
		flag := touchidCmd.Flags().Lookup(f)
		if flag == nil {
			t.Errorf("expected flag --%s to be registered on touchidCmd", f)
		}
	}

	shortFlags := map[string]string{
		"e": "enable",
		"d": "disable",
		"s": "status",
	}
	for sh, full := range shortFlags {
		flag := touchidCmd.Flags().ShorthandLookup(sh)
		if flag == nil || flag.Name != full {
			t.Errorf("expected shorthand -%s to map to --%s", sh, full)
		}
	}
}

func TestTouchIDCmd_MutualExclusion(t *testing.T) {
	setupTestHome(t)

	_, err := executeTouchIDWithArgs([]string{"--enable", "--disable"})
	if err == nil {
		t.Fatal("expected error when both --enable and --disable are passed, got nil")
	}
	if !strings.Contains(err.Error(), "cannot specify both --enable and --disable") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestTouchIDCmd_Status(t *testing.T) {
	setupTestHome(t)

	orig := engine.PamSudoLocalPath
	defer func() { engine.PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	engine.PamSudoLocalPath = tempFile

	out, err := executeTouchIDWithArgs([]string{"--status"})
	if err != nil {
		t.Fatalf("sf touchid --status failed: %v", err)
	}

	if !strings.Contains(out, "Touch ID for Sudo Configuration") {
		t.Errorf("expected status header in output, got: %s", out)
	}
	if !strings.Contains(out, tempFile) {
		t.Errorf("expected output to contain config path %s, got: %s", tempFile, out)
	}
}

func TestTouchIDCmd_Enable(t *testing.T) {
	setupTestHome(t)

	orig := engine.PamSudoLocalPath
	defer func() { engine.PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	engine.PamSudoLocalPath = tempFile

	out, err := executeTouchIDWithArgs([]string{"--enable"})
	if err != nil {
		t.Fatalf("sf touchid --enable failed: %v", err)
	}

	if !strings.Contains(out, "ENABLED") {
		t.Errorf("expected output to mention ENABLED, got: %s", out)
	}

	enabled, _, _, err := engine.CheckStatus()
	if err != nil || !enabled {
		t.Fatalf("expected Touch ID to be enabled, enabled=%t, err=%v", enabled, err)
	}
}

func TestTouchIDCmd_Disable(t *testing.T) {
	setupTestHome(t)

	orig := engine.PamSudoLocalPath
	defer func() { engine.PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	engine.PamSudoLocalPath = tempFile

	// Enable first
	if err := engine.Enable(); err != nil {
		t.Fatalf("setup Enable() failed: %v", err)
	}

	out, err := executeTouchIDWithArgs([]string{"--disable"})
	if err != nil {
		t.Fatalf("sf touchid --disable failed: %v", err)
	}

	if !strings.Contains(out, "DISABLED") {
		t.Errorf("expected output to mention DISABLED, got: %s", out)
	}

	enabled, _, _, err := engine.CheckStatus()
	if err != nil || enabled {
		t.Fatalf("expected Touch ID to be disabled, enabled=%t, err=%v", enabled, err)
	}
}

func TestTouchIDCmd_NonInteractiveDefault(t *testing.T) {
	setupTestHome(t)

	orig := engine.PamSudoLocalPath
	defer func() { engine.PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	engine.PamSudoLocalPath = tempFile

	out, err := executeTouchIDWithArgs([]string{})
	if err != nil {
		t.Fatalf("sf touchid (default non-interactive) failed: %v", err)
	}

	if !strings.Contains(out, "Touch ID for Sudo Configuration") {
		t.Errorf("expected default non-interactive call to display status card, got: %s", out)
	}
}
