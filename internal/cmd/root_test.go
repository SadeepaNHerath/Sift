package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSetVersion(t *testing.T) {
	testVer := "1.2.3-test"
	SetVersion(testVer)

	if version != testVer {
		t.Fatalf("expected version %q, got %q", testVer, version)
	}

	if rootCmd.Version != testVer {
		t.Fatalf("expected rootCmd.Version %q, got %q", testVer, rootCmd.Version)
	}
}

func TestSubcommandsRegistered(t *testing.T) {
	expectedSubcommands := []string{
		"clean",
		"uninstall",
		"analyze",
		"status",
		"purge",
		"dedup",
		"startup",
		"privacy",
		"installer",
		"optimize",
		"touchid",
		"history",
		"doctor",
		"update",
		"completion",
	}

	commands := rootCmd.Commands()
	registered := make(map[string]bool)
	for _, c := range commands {
		registered[c.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		if !registered[expected] {
			t.Errorf("expected subcommand %q to be registered on rootCmd", expected)
		}
	}
}

func TestCollectSystemSummary(t *testing.T) {
	sys := collectSystemSummary()

	if sys.hostname == "" {
		t.Errorf("expected non-empty hostname")
	}
	if !strings.HasPrefix(sys.osVersion, "macOS") {
		t.Errorf("expected osVersion to start with 'macOS', got %q", sys.osVersion)
	}
	if sys.diskTotal == "" {
		t.Errorf("expected non-empty diskTotal")
	}
	if sys.diskFree == "" {
		t.Errorf("expected non-empty diskFree")
	}
}

func TestStubCommandsRun(t *testing.T) {
	setupTestHome(t)
	rIn, wIn, _ := os.Pipe()
	_ = wIn.Close()
	oldStdin := os.Stdin
	os.Stdin = rIn
	defer func() {
		os.Stdin = oldStdin
		_ = rIn.Close()
	}()

	subcommands := []*struct {
		name string
		cmd  func()
	}{
		{"clean", func() {
			_ = cleanCmd.Flags().Set("dry-run", "true")
			_ = cleanCmd.Flags().Set("yes", "true")
			cleanCmd.Run(cleanCmd, []string{"nonexistent"})
		}},
		{"uninstall", func() { uninstallCmd.Run(uninstallCmd, nil) }},
		{"analyze", func() { analyzeCmd.Run(analyzeCmd, []string{"/tmp"}) }},
		{"status", func() { statusCmd.Run(statusCmd, nil) }},
		{"purge", func() {
			_ = purgeCmd.Flags().Set("dry-run", "true")
			_ = purgeCmd.Flags().Set("yes", "true")
			purgeCmd.Run(purgeCmd, []string{"nonexistent"})
		}},
		{"installer", func() {
			_ = installerCmd.Flags().Set("dry-run", "true")
			installerCmd.Run(installerCmd, []string{"/tmp/nonexistent.dmg"})
		}},
		{"optimize", func() {
			_ = optimizeCmd.Flags().Set("dry-run", "true")
			_ = optimizeCmd.Flags().Set("yes", "true")
			optimizeCmd.Run(optimizeCmd, []string{"dsstore"})
		}},
		{"touchid", func() {
			_ = touchidCmd.Flags().Set("status", "true")
			touchidCmd.Run(touchidCmd, nil)
		}},
		{"history", func() { historyCmd.Run(historyCmd, nil) }},
		{"doctor", func() { doctorCmd.Run(doctorCmd, nil) }},
		{"update", func() { updateCmd.Run(updateCmd, nil) }},
		{"completion", func() { completionCmd.Run(completionCmd, nil) }},
	}

	for _, sc := range subcommands {
		t.Run(sc.name, func(t *testing.T) {
			_, _ = captureOutput(func() error {
				sc.cmd()
				return nil
			})
		})
	}
}

func TestRootHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	err := rootCmd.Help()
	if err != nil {
		t.Fatalf("unexpected error displaying help: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected help output to contain 'Usage:', got: %s", output)
	}
}
