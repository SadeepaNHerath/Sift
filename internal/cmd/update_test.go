package cmd

import (
	"strings"
	"testing"
)

func TestUpdateCmd_Flags(t *testing.T) {
	if updateCmd.Flags().Lookup("check") == nil {
		t.Error("expected --check flag on updateCmd")
	}
	if updateCmd.Flags().Lookup("force") == nil {
		t.Error("expected --force flag on updateCmd")
	}
}

func TestUpdateCmd_Run(t *testing.T) {
	out, err := captureOutput(func() error {
		_ = updateCmd.Flags().Set("check", "true")
		return updateCmd.RunE(updateCmd, nil)
	})
	// Network could fail or succeed depending on environment, but should handle gracefully
	if err != nil && !strings.Contains(err.Error(), "network error") && !strings.Contains(err.Error(), "GitHub API") {
		t.Fatalf("unexpected update error: %v", err)
	}
	_ = out
}

func TestIsHomebrewInstallation(t *testing.T) {
	_ = isHomebrewInstallation()
}
