package cmd

import (
	"strings"
	"testing"
)

func resetOptimizeFlags() {
	_ = optimizeCmd.Flags().Set("whitelist", "false")
	_ = optimizeCmd.Flags().Set("dry-run", "false")
	_ = optimizeCmd.Flags().Set("all", "false")
	_ = optimizeCmd.Flags().Set("yes", "false")
}

func executeOptimizeWithArgs(args []string) (string, error) {
	resetOptimizeFlags()
	rootCmd.SetArgs(append([]string{"optimize"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func TestOptimizeCmd_FlagsRegistered(t *testing.T) {
	flags := []string{"dry-run", "all", "whitelist", "yes"}
	for _, f := range flags {
		flag := optimizeCmd.Flags().Lookup(f)
		if flag == nil {
			t.Errorf("expected flag --%s to be registered on optimizeCmd", f)
		}
	}

	shortFlags := map[string]string{
		"n": "dry-run",
		"a": "all",
		"w": "whitelist",
		"y": "yes",
	}
	for sh, full := range shortFlags {
		flag := optimizeCmd.Flags().ShorthandLookup(sh)
		if flag == nil || flag.Name != full {
			t.Errorf("expected shorthand -%s to map to --%s", sh, full)
		}
	}
}

func TestOptimizeCmd_Whitelist(t *testing.T) {
	setupTestHome(t)

	out, err := executeOptimizeWithArgs([]string{"--whitelist"})
	if err != nil {
		t.Fatalf("sf optimize --whitelist failed: %v", err)
	}

	if !strings.Contains(out, "Whitelist") {
		t.Errorf("expected output to contain 'Whitelist', got: %s", out)
	}
}

func TestOptimizeCmd_DryRun(t *testing.T) {
	setupTestHome(t)

	out, err := executeOptimizeWithArgs([]string{"--dry-run", "--yes"})
	if err != nil {
		t.Fatalf("sf optimize --dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Dry Run") && !strings.Contains(out, "Preview") {
		t.Errorf("expected dry run header in output, got: %s", out)
	}
	if !strings.Contains(out, "DNS") {
		t.Errorf("expected output to list DNS task, got: %s", out)
	}
}

func TestOptimizeCmd_SpecificTask(t *testing.T) {
	setupTestHome(t)

	out, err := executeOptimizeWithArgs([]string{"dsstore", "--dry-run", "--yes"})
	if err != nil {
		t.Fatalf("sf optimize dsstore --dry-run failed: %v", err)
	}

	if !strings.Contains(out, "DS_Store") {
		t.Errorf("expected output to mention DS_Store, got: %s", out)
	}
}

func TestOptimizeCmd_InvalidTask(t *testing.T) {
	setupTestHome(t)

	_, err := executeOptimizeWithArgs([]string{"invalid_task_xyz_123"})
	if err == nil {
		t.Fatal("expected error for invalid task argument, got nil")
	}
	if !strings.Contains(err.Error(), "no matching optimization tasks found") {
		t.Errorf("expected 'no matching optimization tasks found' error, got: %v", err)
	}
}

func TestOptimizeCmd_ExecuteLive(t *testing.T) {
	setupTestHome(t)

	out, err := executeOptimizeWithArgs([]string{"dsstore", "--yes"})
	if err != nil {
		t.Fatalf("sf optimize dsstore --yes failed: %v", err)
	}

	if !strings.Contains(out, "Optimization Complete") && !strings.Contains(out, "Done") {
		t.Errorf("expected success summary in output, got: %s", out)
	}
}
