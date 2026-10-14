package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func resetStartupFlags() {
	_ = startupCmd.Flags().Set("json", "false")
	_ = startupCmd.Flags().Set("zombies", "false")
	_ = startupCmd.Flags().Set("toggle", "")
	_ = startupCmd.Flags().Set("remove-zombies", "false")
	_ = startupCmd.Flags().Set("yes", "false")
}

func executeStartupWithArgs(args []string) (string, error) {
	resetStartupFlags()
	rootCmd.SetArgs(append([]string{"startup"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func TestStartupCmd_FlagsRegistered(t *testing.T) {
	flags := []string{"json", "zombies", "toggle", "remove-zombies", "yes"}
	for _, f := range flags {
		flag := startupCmd.Flags().Lookup(f)
		if flag == nil {
			t.Errorf("expected flag --%s to be registered on startupCmd", f)
		}
	}

	// Verify alias "launchd"
	foundAlias := false
	for _, a := range startupCmd.Aliases {
		if a == "launchd" {
			foundAlias = true
			break
		}
	}
	if !foundAlias {
		t.Errorf("expected launchd alias on startupCmd")
	}
}

func TestStartupCmd_Listing(t *testing.T) {
	setupTestHome(t)

	out, err := executeStartupWithArgs([]string{})
	if err != nil {
		t.Fatalf("unexpected error running sf startup: %v", err)
	}

	if !strings.Contains(out, "Launch Services Audit") {
		t.Errorf("expected audit header in output, got: %s", out)
	}
}

func TestStartupCmd_JSONOutput(t *testing.T) {
	setupTestHome(t)

	out, err := executeStartupWithArgs([]string{"--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var services []engine.LaunchService
	if err := json.Unmarshal([]byte(out), &services); err != nil {
		t.Fatalf("failed to parse JSON output: %v, raw: %s", err, out)
	}
}

func TestStartupCmd_ZombiesOnly(t *testing.T) {
	setupTestHome(t)

	out, err := executeStartupWithArgs([]string{"--zombies"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Zombie Launch Services") {
		t.Errorf("expected zombie header, got: %s", out)
	}
}

func TestStartupCmd_RemoveZombiesRequiresYes(t *testing.T) {
	setupTestHome(t)

	// Without --yes in test environment, should fail or report no zombies
	out, err := executeStartupWithArgs([]string{"--remove-zombies"})
	if err != nil {
		if !strings.Contains(err.Error(), "--yes") {
			t.Errorf("expected --yes requirement error, got: %v", err)
		}
	} else {
		if !strings.Contains(out, "No zombie") && !strings.Contains(out, "trashed") {
			t.Errorf("unexpected output: %s", out)
		}
	}
}
