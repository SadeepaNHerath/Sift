package cmd

import (
	"strings"
	"testing"
)

func TestCompletionCmd_ValidShells(t *testing.T) {
	shells := []string{"bash", "zsh", "fish", "powershell"}
	for _, sh := range shells {
		t.Run(sh, func(t *testing.T) {
			out, err := captureOutput(func() error {
				return completionCmd.RunE(completionCmd, []string{sh})
			})
			if err != nil {
				t.Fatalf("completion for %s failed: %v", sh, err)
			}
			if len(out) == 0 {
				t.Errorf("expected completion script output for %s", sh)
			}
		})
	}
}

func TestCompletionCmd_AutoDetect(t *testing.T) {
	out, err := captureOutput(func() error {
		return completionCmd.RunE(completionCmd, []string{})
	})
	if err != nil {
		t.Fatalf("completion auto-detect failed: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected completion script for auto-detect")
	}
}

func TestCompletionCmd_InvalidShell(t *testing.T) {
	_, err := captureOutput(func() error {
		return completionCmd.RunE(completionCmd, []string{"unsupported_sh"})
	})
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
	if !strings.Contains(err.Error(), "unsupported shell type") {
		t.Errorf("unexpected error message: %v", err)
	}
}
