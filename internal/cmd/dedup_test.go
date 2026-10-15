package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func resetDedupFlags() {
	_ = dedupCmd.Flags().Set("min-size", "1MB")
	_ = dedupCmd.Flags().Set("keep-oldest", "false")
	_ = dedupCmd.Flags().Set("keep-newest", "false")
	_ = dedupCmd.Flags().Set("dry-run", "false")
	_ = dedupCmd.Flags().Set("yes", "false")
	_ = dedupCmd.Flags().Set("json", "false")
	_ = dedupCmd.Flags().Set("clone", "false")
	_ = dedupCmd.Flags().Set("hardlink", "false")
}

func executeDedupWithArgs(args []string) (string, error) {
	resetDedupFlags()
	rootCmd.SetArgs(append([]string{"dedup"}, args...))
	return captureOutput(func() error {
		return rootCmd.Execute()
	})
}

func TestDedupCmd_FlagsRegistered(t *testing.T) {
	flags := []string{"min-size", "keep-oldest", "keep-newest", "dry-run", "yes", "json", "clone", "hardlink"}
	for _, f := range flags {
		flag := dedupCmd.Flags().Lookup(f)
		if flag == nil {
			t.Errorf("expected flag --%s to be registered on dedupCmd", f)
		}
	}
}

func TestDedupCmd_NoDuplicatesFound(t *testing.T) {
	tempHome := setupTestHome(t)
	emptyDir := filepath.Join(tempHome, "EmptyFolder")
	_ = os.MkdirAll(emptyDir, 0755)

	out, err := executeDedupWithArgs([]string{"--dry-run", emptyDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No duplicate files found") {
		t.Errorf("expected no duplicates output, got: %s", out)
	}
}

func TestDedupCmd_DryRun(t *testing.T) {
	tempHome := setupTestHome(t)
	dir := filepath.Join(tempHome, "Downloads")
	_ = os.MkdirAll(dir, 0755)

	data := bytes.Repeat([]byte("DuplicateDataBytes!"), 200) // ~4KB
	f1 := filepath.Join(dir, "file1.bin")
	f2 := filepath.Join(dir, "file2.bin")
	_ = os.WriteFile(f1, data, 0644)
	_ = os.WriteFile(f2, data, 0644)

	out, err := executeDedupWithArgs([]string{"--dry-run", "--min-size", "1KB", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "DEDUP DRY RUN PREVIEW") {
		t.Errorf("expected dry run preview banner, got: %s", out)
	}
	if !strings.Contains(out, "KEEP") || !strings.Contains(out, "TRASH") {
		t.Errorf("expected KEEP and TRASH markers in output, got: %s", out)
	}

	if _, err := os.Stat(f1); err != nil {
		t.Errorf("file1 was deleted during dry run!")
	}
	if _, err := os.Stat(f2); err != nil {
		t.Errorf("file2 was deleted during dry run!")
	}
}

func TestDedupCmd_JSONOutput(t *testing.T) {
	tempHome := setupTestHome(t)
	dir := filepath.Join(tempHome, "Downloads")
	_ = os.MkdirAll(dir, 0755)

	data := bytes.Repeat([]byte("DuplicateJSONContent!"), 200)
	f1 := filepath.Join(dir, "j1.bin")
	f2 := filepath.Join(dir, "j2.bin")
	_ = os.WriteFile(f1, data, 0644)
	_ = os.WriteFile(f2, data, 0644)

	out, err := executeDedupWithArgs([]string{"--json", "--min-size", "1KB", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var groups []engine.DuplicateGroup
	if err := json.Unmarshal([]byte(out), &groups); err != nil {
		t.Fatalf("failed to parse JSON output: %v, raw output: %s", err, out)
	}

	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group in JSON, got %d", len(groups))
	}
	if len(groups[0].Files) != 2 {
		t.Errorf("expected 2 files in duplicate group, got %d", len(groups[0].Files))
	}
}

func TestDedupCmd_NonInteractiveRequiresConfirmation(t *testing.T) {
	tempHome := setupTestHome(t)
	dir := filepath.Join(tempHome, "Downloads")
	_ = os.MkdirAll(dir, 0755)

	data := []byte("identical-content")
	_ = os.WriteFile(filepath.Join(dir, "a.bin"), data, 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.bin"), data, 0644)

	_, err := executeDedupWithArgs([]string{"--min-size", "10B", dir})
	if err == nil {
		t.Fatal("expected error requiring confirmation in non-interactive environment")
	}

	if !strings.Contains(err.Error(), "requires confirmation") {
		t.Errorf("expected confirmation error message, got: %v", err)
	}
}

func TestDedupCmd_ExecuteWithYesKeepOldest(t *testing.T) {
	tempHome := setupTestHome(t)
	dir := filepath.Join(tempHome, "Downloads")
	_ = os.MkdirAll(dir, 0755)

	data := bytes.Repeat([]byte("ExecutionContentTest"), 300)
	fOld := filepath.Join(dir, "old.bin")
	fNew := filepath.Join(dir, "new.bin")
	_ = os.WriteFile(fOld, data, 0644)
	_ = os.WriteFile(fNew, data, 0644)

	out, err := executeDedupWithArgs([]string{"--yes", "--keep-oldest", "--min-size", "1KB", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Dedup completed") {
		t.Errorf("expected completion message, got: %s", out)
	}

	if _, err := os.Stat(fOld); err != nil {
		t.Errorf("old file was erroneously removed: %v", err)
	}

	if _, err := os.Stat(fNew); !os.IsNotExist(err) {
		t.Errorf("new duplicate file was not removed: %v", err)
	}
}

func TestDedupCmd_CloneDryRun(t *testing.T) {
	tempHome := setupTestHome(t)
	dir := filepath.Join(tempHome, "Documents")
	_ = os.MkdirAll(dir, 0755)

	data := bytes.Repeat([]byte("CloneDryRunTestingData123"), 100)
	f1 := filepath.Join(dir, "clone1.txt")
	f2 := filepath.Join(dir, "clone2.txt")
	_ = os.WriteFile(f1, data, 0644)
	_ = os.WriteFile(f2, data, 0644)

	out, err := executeDedupWithArgs([]string{"--clone", "--dry-run", "--min-size", "10B", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "[CLONE]") {
		t.Errorf("expected [CLONE] label in dry run output, got: %s", out)
	}
	if !strings.Contains(out, "APFS Copy-on-Write") {
		t.Errorf("expected APFS explanatory message, got: %s", out)
	}
}

func TestDedupCmd_MutualExclusion(t *testing.T) {
	_, err := executeDedupWithArgs([]string{"--clone", "--hardlink", "."})
	if err == nil {
		t.Fatal("expected error when both --clone and --hardlink are passed, got nil")
	}
	if !strings.Contains(err.Error(), "cannot specify both --clone and --hardlink") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDedupCmd_HardlinkExecute(t *testing.T) {
	tempHome := setupTestHome(t)
	dir := filepath.Join(tempHome, "Links")
	_ = os.MkdirAll(dir, 0755)

	data := bytes.Repeat([]byte("HardlinkExecutionBytes!"), 200)
	f1 := filepath.Join(dir, "link1.bin")
	f2 := filepath.Join(dir, "link2.bin")
	_ = os.WriteFile(f1, data, 0644)
	_ = os.WriteFile(f2, data, 0644)

	out, err := executeDedupWithArgs([]string{"--hardlink", "--yes", "--keep-oldest", "--min-size", "10B", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Hardlink Deduplication completed") {
		t.Errorf("expected hardlink completion message, got: %s", out)
	}

	if _, err := os.Stat(f1); err != nil {
		t.Errorf("f1 was erroneously removed: %v", err)
	}
	if _, err := os.Stat(f2); err != nil {
		t.Errorf("f2 was erroneously removed: %v", err)
	}
}
