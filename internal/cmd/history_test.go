package cmd

import (
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

func TestHistoryCmd_Flags(t *testing.T) {
	if historyCmd.Flags().Lookup("limit") == nil {
		t.Error("expected --limit flag on historyCmd")
	}
	if historyCmd.Flags().Lookup("json") == nil {
		t.Error("expected --json flag on historyCmd")
	}
	if historyCmd.Flags().Lookup("deletions") == nil {
		t.Error("expected --deletions flag on historyCmd")
	}
}

func TestHistoryCmd_RunOperations(t *testing.T) {
	// Log an operation first
	logger, err := core.NewLogger()
	if err == nil {
		logger.LogOperation("test_cmd", "test_action", "success", "test_details")
		logger.Close()
	}

	out, err := captureOutput(func() error {
		_ = historyCmd.Flags().Set("json", "false")
		_ = historyCmd.Flags().Set("deletions", "false")
		return historyCmd.RunE(historyCmd, nil)
	})
	if err != nil {
		t.Fatalf("historyCmd failed: %v", err)
	}

	if !strings.Contains(out, "Audit Log") && !strings.Contains(out, "No recorded") {
		t.Errorf("unexpected history output: %s", out)
	}
}

func TestHistoryCmd_RunDeletions(t *testing.T) {
	out, err := captureOutput(func() error {
		_ = historyCmd.Flags().Set("json", "false")
		_ = historyCmd.Flags().Set("deletions", "true")
		return historyCmd.RunE(historyCmd, nil)
	})
	if err != nil {
		t.Fatalf("historyCmd with deletions failed: %v", err)
	}
	_ = out
}

func TestHistoryCmd_JSON(t *testing.T) {
	out, err := captureOutput(func() error {
		_ = historyCmd.Flags().Set("json", "true")
		_ = historyCmd.Flags().Set("deletions", "false")
		return historyCmd.RunE(historyCmd, nil)
	})
	if err != nil {
		t.Fatalf("historyCmd JSON failed: %v", err)
	}
	_ = out
}
