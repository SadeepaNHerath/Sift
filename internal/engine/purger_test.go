package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestIsKnownArtifactDir(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"node_modules", "node_modules"},
		{"NODE_MODULES", "node_modules"},
		{"target", "target"},
		{"build", "build"},
		{"dist", "dist"},
		{".venv", ".venv"},
		{"venv", "venv"},
		{"Pods", "Pods"},
		{"pods", "Pods"},
		{"DerivedData", "DerivedData"},
		{"deriveddata", "DerivedData"},
		{".next", ".next"},
		{".nuxt", ".nuxt"},
		{"vendor", "vendor"},
		{".turbo", ".turbo"},
		{".pytest_cache", ".pytest_cache"},
		{"src", ""},
		{"cmd", ""},
		{"main.go", ""},
		{"random_dir", ""},
	}

	for _, tt := range tests {
		got := IsKnownArtifactDir(tt.input)
		if got != tt.expected {
			t.Errorf("IsKnownArtifactDir(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDefaultPurgeRoots(t *testing.T) {
	roots := DefaultPurgeRoots()
	if len(roots) == 0 {
		t.Fatal("expected at least one default purge root (e.g. cwd)")
	}
}

func TestScanPurgeTargets_Basic(t *testing.T) {
	tempDir := t.TempDir()

	// Project 1: webapp with node_modules and dist
	proj1 := filepath.Join(tempDir, "webapp")
	nodeModules := filepath.Join(proj1, "node_modules")
	dist := filepath.Join(proj1, "dist")
	if err := os.MkdirAll(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dist, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "package.json"), []byte(`{"name": "dep"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "bundle.js"), []byte(`console.log("hello");`), 0644); err != nil {
		t.Fatal(err)
	}

	// Project 2: rustapp with target
	proj2 := filepath.Join(tempDir, "rustapp")
	targetDir := filepath.Join(proj2, "target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "app_bin"), []byte("binary content"), 0755); err != nil {
		t.Fatal(err)
	}

	// Project 3: pyapp with empty .venv
	proj3 := filepath.Join(tempDir, "pyapp")
	venvDir := filepath.Join(proj3, ".venv")
	if err := os.MkdirAll(venvDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Scan without empty
	targets, err := ScanPurgeTargets([]string{tempDir}, 0, false)
	if err != nil {
		t.Fatalf("ScanPurgeTargets failed: %v", err)
	}

	// Expect 3 targets (node_modules, dist, target) because .venv is empty
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	foundTypes := make(map[string]bool)
	for _, trg := range targets {
		foundTypes[trg.ArtifactType] = true
		if trg.Size <= 0 {
			t.Errorf("expected target %s to have positive size, got %d", trg.Path, trg.Size)
		}
	}
	if !foundTypes["node_modules"] || !foundTypes["dist"] || !foundTypes["target"] {
		t.Errorf("missing expected artifact types in %v", foundTypes)
	}

	// Scan with includeEmpty = true
	targetsWithEmpty, err := ScanPurgeTargets([]string{tempDir}, 0, true)
	if err != nil {
		t.Fatalf("ScanPurgeTargets failed: %v", err)
	}
	if len(targetsWithEmpty) != 4 {
		t.Fatalf("expected 4 targets including empty .venv, got %d", len(targetsWithEmpty))
	}
}

func TestScanPurgeTargets_MinAge(t *testing.T) {
	tempDir := t.TempDir()

	proj := filepath.Join(tempDir, "sample-project")
	buildDir := filepath.Join(proj, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(buildDir, "output.txt")
	if err := os.WriteFile(file, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set file time to 10 days ago
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(file, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(buildDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Test with minAge = 7 days -> IsOldEnough should be true
	targets, err := ScanPurgeTargets([]string{tempDir}, 7*24*time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if !targets[0].IsOldEnough {
		t.Errorf("expected target to be old enough")
	}

	// Test with minAge = 14 days -> IsOldEnough should be false
	targets2, err := ScanPurgeTargets([]string{tempDir}, 14*24*time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets2) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets2))
	}
	if targets2[0].IsOldEnough {
		t.Errorf("expected target to NOT be old enough")
	}
	if targets2[0].SafeToPurge {
		t.Errorf("target not old enough should not be safe to purge")
	}
}

func TestScanPurgeTargets_GitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found, skipping git status test")
	}

	tempDir := t.TempDir()
	ClearGitCache()

	// Initialize git repository
	cmd := exec.Command("git", "init", tempDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (%s)", err, string(out))
	}

	// Configure local git user to allow commits
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@example.com").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Test").Run()

	// Create initial file and commit
	readme := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Git Test"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = exec.Command("git", "-C", tempDir, "add", "README.md").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "initial").Run()

	// Create artifact
	nodeModules := filepath.Join(tempDir, "node_modules")
	if err := os.MkdirAll(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "pkg.js"), []byte("exports = 1;"), 0644); err != nil {
		t.Fatal(err)
	}

	// Add .gitignore for node_modules so repo stays clean
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("node_modules/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = exec.Command("git", "-C", tempDir, "add", ".gitignore").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "ignore node_modules").Run()

	ClearGitCache()
	targets, err := ScanPurgeTargets([]string{tempDir}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if !targets[0].GitRepo {
		t.Errorf("expected target to be in git repo")
	}
	if targets[0].GitDirty {
		t.Errorf("expected git repo to be clean, got dirty")
	}
	if !targets[0].SafeToPurge {
		t.Errorf("expected clean git repo target to be safe to purge")
	}

	dirtyFile := filepath.Join(tempDir, "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("uncommitted change"), 0644); err != nil {
		t.Fatal(err)
	}

	ClearGitCache()
	targetsDirty, err := ScanPurgeTargets([]string{tempDir}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targetsDirty) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targetsDirty))
	}
	if !targetsDirty[0].GitDirty {
		t.Errorf("expected target to detect dirty git status")
	}
	if targetsDirty[0].SafeToPurge {
		t.Errorf("target in dirty git repo should not be safe to purge")
	}
	if targetsDirty[0].Warning == "" {
		t.Errorf("expected warning message for dirty git target")
	}
}

func TestPurgeTargets_DryRunAndActual(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Create test target
	projDir := filepath.Join(tempHome, "Projects", "testapp")
	distDir := filepath.Join(projDir, "dist")
	if err := os.MkdirAll(distDir, 0755); err != nil {
		t.Fatal(err)
	}
	distFile := filepath.Join(distDir, "main.js")
	if err := os.WriteFile(distFile, []byte("console.log(1);"), 0644); err != nil {
		t.Fatal(err)
	}

	targets, err := ScanPurgeTargets([]string{projDir}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	dryReport, err := PurgeTargets(targets, true, nil)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if len(dryReport.Removed) != 1 {
		t.Fatalf("expected 1 dry run removed item, got %d", len(dryReport.Removed))
	}
	if dryReport.TotalBytes <= 0 {
		t.Errorf("expected positive total bytes in dry run, got %d", dryReport.TotalBytes)
	}
	if _, err := os.Stat(distDir); err != nil {
		t.Fatalf("dist directory should still exist after dry run: %v", err)
	}

	actualReport, err := PurgeTargets(targets, false, nil)
	if err != nil {
		t.Fatalf("actual purge failed: %v", err)
	}
	if len(actualReport.Removed) != 1 {
		t.Fatalf("expected 1 removed item, got %d (failed: %v)", len(actualReport.Removed), actualReport.Failed)
	}
	// Verify directory was removed
	if _, err := os.Stat(distDir); !os.IsNotExist(err) {
		t.Errorf("expected dist directory to be deleted/moved to trash, still exists")
	}
}

func TestPurgeTargets_SafetyBlock(t *testing.T) {
	// Attempt to purge a system-protected path
	blockedTarget := PurgeTarget{
		Path:         "/System/Library",
		ProjectName:  "system",
		ArtifactType: "build",
		Size:         1024,
		SafeToPurge:  false,
	}

	report, err := PurgeTargets([]PurgeTarget{blockedTarget}, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Failed) != 1 {
		t.Fatalf("expected 1 failed item due to safety block, got %d", len(report.Failed))
	}
	if len(report.Removed) != 0 {
		t.Fatalf("expected 0 removed items, got %d", len(report.Removed))
	}
}
