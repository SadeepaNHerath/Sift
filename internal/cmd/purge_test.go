package cmd

import (
	"github.com/SadeepaNHerath/Spacefold/internal/engine"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetPurgeFlags() {
	_ = purgeCmd.Flags().Set("dry-run", "false")
	_ = purgeCmd.Flags().Set("yes", "false")
	_ = purgeCmd.Flags().Set("all", "false")
	_ = purgeCmd.Flags().Set("paths", "")
	_ = purgeCmd.Flags().Set("min-age", "168h")
	_ = purgeCmd.Flags().Set("ai", "false")
	_ = purgeCmd.Flags().Set("models", "false")
	_ = purgeCmd.Flags().Set("non-interactive", "false")
	_ = purgeCmd.Flags().Set("quiet", "false")
	_ = purgeCmd.Flags().Set("ci", "false")
}

func executePurgeWithArgs(args []string) (string, error) {
	resetPurgeFlags()
	rootCmd.SetArgs(append([]string{"purge"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func TestPurgeCmd_NoTargetsFound(t *testing.T) {
	tempHome := setupTestHome(t)
	emptyDir := filepath.Join(tempHome, "EmptyProject")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}

	out, err := executePurgeWithArgs([]string{"--dry-run", "--paths", emptyDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No purgeable developer artifacts found") {
		t.Errorf("expected no targets output, got: %s", out)
	}
}

func TestPurgeCmd_DryRun(t *testing.T) {
	tempHome := setupTestHome(t)
	projDir := filepath.Join(tempHome, "Projects", "my-app")
	nodeModules := filepath.Join(projDir, "node_modules")
	if err := os.MkdirAll(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "index.js"), []byte("console.log(1);"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := executePurgeWithArgs([]string{"--dry-run", "--paths", projDir, "--min-age", "0s"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "DRY RUN PREVIEW") {
		t.Errorf("expected dry run banner, got: %s", out)
	}
	if !strings.Contains(out, "my-app") {
		t.Errorf("expected project name in output, got: %s", out)
	}
	if !strings.Contains(out, "node_modules") {
		t.Errorf("expected node_modules in output, got: %s", out)
	}

	// Verify node_modules still exists
	if _, err := os.Stat(nodeModules); err != nil {
		t.Errorf("node_modules was deleted during dry run!")
	}
}

func TestPurgeCmd_NonInteractiveRequiresConfirmation(t *testing.T) {
	tempHome := setupTestHome(t)
	projDir := filepath.Join(tempHome, "Projects", "cli-app")
	buildDir := filepath.Join(projDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "out.bin"), []byte("bin"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run without --yes and without --dry-run
	_, err := executePurgeWithArgs([]string{"--paths", projDir})
	if err == nil {
		t.Fatal("expected error requiring confirmation in non-interactive environment")
	}

	if !strings.Contains(err.Error(), "requires confirmation") {
		t.Errorf("expected confirmation error message, got: %v", err)
	}
}

func TestPurgeCmd_ExecuteWithYes(t *testing.T) {
	tempHome := setupTestHome(t)
	projDir := filepath.Join(tempHome, "Projects", "frontend")
	distDir := filepath.Join(projDir, "dist")
	if err := os.MkdirAll(distDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "bundle.js"), []byte("packed code"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := executePurgeWithArgs([]string{"--yes", "--paths", projDir, "--min-age", "0s"})
	if err != nil {
		t.Fatalf("purge failed: %v", err)
	}

	if !strings.Contains(out, "Purge completed") {
		t.Errorf("expected completion message, got: %s", out)
	}

	// Verify dist was removed
	if _, err := os.Stat(distDir); !os.IsNotExist(err) {
		t.Errorf("dist directory should have been removed")
	}
}

func TestPurgeCmd_AllFlagPurgesUnsafe(t *testing.T) {
	tempHome := setupTestHome(t)
	engine.ClearGitCache()

	projDir := filepath.Join(tempHome, "Projects", "backend")
	targetDir := filepath.Join(projDir, "target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(targetDir, "backend_bin")
	if err := os.WriteFile(targetFile, []byte("binary data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Artifact was just created, so with min-age=168h it is NOT old enough (SafeToPurge=false)
	// Without --all, purge with --yes should report no safe targets
	out1, err1 := executePurgeWithArgs([]string{"--yes", "--paths", projDir, "--min-age", "168h"})
	if err1 != nil {
		t.Fatalf("unexpected error: %v", err1)
	}
	if !strings.Contains(out1, "No targets safe to purge") {
		t.Errorf("expected warning about no safe targets, got: %s", out1)
	}
	// Directory still exists
	if _, err := os.Stat(targetDir); err != nil {
		t.Fatalf("target directory should not have been purged: %v", err)
	}

	// Now run with --all -> should purge it
	out2, err2 := executePurgeWithArgs([]string{"--yes", "--all", "--paths", projDir, "--min-age", "168h"})
	if err2 != nil {
		t.Fatalf("unexpected error with --all: %v", err2)
	}
	if !strings.Contains(out2, "Purge completed") {
		t.Errorf("expected purge completed with --all, got: %s", out2)
	}
	// Directory was removed
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Errorf("target directory should have been purged with --all")
	}
}

func TestPurgeCmd_AI_DryRun(t *testing.T) {
	tempHome := setupTestHome(t)
	// Create mock Hugging Face hub
	hfDir := filepath.Join(tempHome, ".cache", "huggingface", "hub", "models--test--model")
	if err := os.MkdirAll(hfDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hfDir, "weights.bin"), []byte("model weights"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := executePurgeWithArgs([]string{"--ai", "--dry-run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "AI/LLM DRY RUN PREVIEW") {
		t.Errorf("expected AI dry run preview banner, got: %s", out)
	}
	if !strings.Contains(out, "Hugging Face") {
		t.Errorf("expected Hugging Face provider in output, got: %s", out)
	}
}
