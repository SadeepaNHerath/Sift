package cmd

import (
	"bytes"
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
	subcommands := []*struct {
		name string
		cmd  func()
	}{
		{"clean", func() { cleanCmd.Run(cleanCmd, nil) }},
		{"uninstall", func() { uninstallCmd.Run(uninstallCmd, nil) }},
		{"analyze", func() { analyzeCmd.Run(analyzeCmd, nil) }},
		{"status", func() { statusCmd.Run(statusCmd, nil) }},
		{"purge", func() { purgeCmd.Run(purgeCmd, nil) }},
		{"installer", func() { installerCmd.Run(installerCmd, nil) }},
		{"optimize", func() { optimizeCmd.Run(optimizeCmd, nil) }},
		{"touchid", func() { touchidCmd.Run(touchidCmd, nil) }},
		{"history", func() { historyCmd.Run(historyCmd, nil) }},
		{"doctor", func() { doctorCmd.Run(doctorCmd, nil) }},
		{"update", func() { updateCmd.Run(updateCmd, nil) }},
		{"completion", func() { completionCmd.Run(completionCmd, nil) }},
	}

	for _, sc := range subcommands {
		t.Run(sc.name, func(t *testing.T) {
			// Ensure running stub command does not panic
			sc.cmd()
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
