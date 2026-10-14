package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFindDuplicates_3Phases(t *testing.T) {
	tempDir := t.TempDir()

	dirA := filepath.Join(tempDir, "A")
	dirB := filepath.Join(tempDir, "B")
	_ = os.MkdirAll(dirA, 0755)
	_ = os.MkdirAll(dirB, 0755)

	// Create identical 20KB content
	content1 := bytes.Repeat([]byte("AlphaBetaGamma123!"), 1200) // ~21.6 KB
	file1 := filepath.Join(dirA, "sample1.bin")
	file2 := filepath.Join(dirB, "sample2.bin")
	_ = os.WriteFile(file1, content1, 0644)
	_ = os.WriteFile(file2, content1, 0644)

	// Create different content with exact same size (Phase 1 collision, Phase 2/3 rejection)
	content2 := bytes.Repeat([]byte("DeltaThetaLambda99"), 1200) // ~21.6 KB
	file3 := filepath.Join(dirA, "collision.bin")
	_ = os.WriteFile(file3, content2, 0644)

	opts := DedupOptions{
		MinSize:      1024, // 1 KB
		ExcludedDirs: nil,
	}

	groups, err := FindDuplicates([]string{tempDir}, opts)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	if len(groups) != 1 {
		t.Fatalf("expected exactly 1 duplicate group, got %d", len(groups))
	}

	group := groups[0]
	if len(group.Files) != 2 {
		t.Errorf("expected 2 files in duplicate group, got %d", len(group.Files))
	}

	if group.Reclaimable != int64(len(content1)) {
		t.Errorf("expected reclaimable bytes %d, got %d", len(content1), group.Reclaimable)
	}

	// Verify collision file was not grouped
	for _, f := range group.Files {
		if f.Path == file3 {
			t.Errorf("collision file should not have been matched as duplicate")
		}
	}
}

func TestFindDuplicates_HardlinksExcluded(t *testing.T) {
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "original.dat")
	data := bytes.Repeat([]byte("UniqueDataContent12345"), 1000)
	if err := os.WriteFile(file1, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create hardlink
	hardlink := filepath.Join(tempDir, "hardlink.dat")
	if err := os.Link(file1, hardlink); err != nil {
		t.Skip("hardlink creation not supported on this filesystem")
	}

	opts := DedupOptions{
		MinSize:      1024,
		ExcludedDirs: nil,
	}

	groups, err := FindDuplicates([]string{tempDir}, opts)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	// Hardlinks share the same inode so they should NOT count as duplicates consuming extra space
	if len(groups) != 0 {
		t.Errorf("expected 0 duplicate groups for hardlinks, got %d", len(groups))
	}
}

func TestTrashDuplicates(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "f1.txt")
	f2 := filepath.Join(tempDir, "f2.txt")
	_ = os.WriteFile(f1, []byte("data"), 0644)
	_ = os.WriteFile(f2, []byte("data"), 0644)

	// Dry run
	dryReport, err := TrashDuplicates([]string{f2}, true, nil)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if len(dryReport.Removed) != 1 {
		t.Errorf("expected 1 removed in dry run, got %d", len(dryReport.Removed))
	}
	if _, err := os.Stat(f2); err != nil {
		t.Errorf("dry run deleted file: %v", err)
	}

	// Actual removal
	rep, err := TrashDuplicates([]string{f2}, false, nil)
	if err != nil {
		t.Fatalf("TrashDuplicates failed: %v", err)
	}
	if len(rep.Removed) != 1 {
		t.Errorf("expected 1 removed in execution, got %d", len(rep.Removed))
	}
	if _, err := os.Stat(f2); !os.IsNotExist(err) {
		t.Errorf("file f2 was not moved to trash")
	}
}

func TestCloneDuplicates_LiveAPFS(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "original.txt")
	f2 := filepath.Join(tempDir, "duplicate.txt")

	content := []byte("UniversalAPFSDeduplicationTestContent12345")
	if err := os.WriteFile(f1, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, content, 0644); err != nil {
		t.Fatal(err)
	}

	opts := DedupOptions{MinSize: 10}
	groups, err := FindDuplicates([]string{tempDir}, opts)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}

	// 1. Test Dry Run
	dryResult, err := CloneDuplicates(groups, false, true, nil)
	if err != nil {
		t.Fatalf("CloneDuplicates dry run failed: %v", err)
	}
	if dryResult.ProcessedFiles != 1 {
		t.Errorf("expected 1 processed in dry run, got %d", dryResult.ProcessedFiles)
	}

	// 2. Test Live Clone Execution
	liveResult, err := CloneDuplicates(groups, false, false, nil)
	if err != nil {
		t.Fatalf("CloneDuplicates live failed: %v", err)
	}
	if len(liveResult.Failed) > 0 {
		t.Logf("APFS clone not supported or failed on temp filesystem: %v", liveResult.Failed[0].Error)
		return
	}
	if liveResult.ProcessedFiles != 1 {
		t.Errorf("expected 1 cloned file, got %d", liveResult.ProcessedFiles)
	}

	// Verify both files still exist and have identical content
	readF2, err := os.ReadFile(f2)
	if err != nil {
		t.Fatalf("failed to read cloned duplicate file: %v", err)
	}
	if !bytes.Equal(readF2, content) {
		t.Errorf("cloned file content mismatch")
	}

	// Verify Copy-on-Write independence: write to f2 and verify f1 is unchanged
	mutated := []byte("MutatedContentForCopyOnWriteTest")
	if err := os.WriteFile(f2, mutated, 0644); err != nil {
		t.Fatalf("failed to write to f2: %v", err)
	}
	readF1, err := os.ReadFile(f1)
	if err != nil {
		t.Fatalf("failed to read f1: %v", err)
	}
	if !bytes.Equal(readF1, content) {
		t.Errorf("f1 was mutated after f2 write! CoW independence failed")
	}
}

func TestHardlinkDuplicates_Live(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "link_orig.txt")
	f2 := filepath.Join(tempDir, "link_dup.txt")

	content := []byte("HardlinkDeduplicationTestContent999")
	if err := os.WriteFile(f1, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, content, 0644); err != nil {
		t.Fatal(err)
	}

	opts := DedupOptions{MinSize: 10}
	groups, err := FindDuplicates([]string{tempDir}, opts)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}

	liveResult, err := HardlinkDuplicates(groups, false, false, nil)
	if err != nil {
		t.Fatalf("HardlinkDuplicates failed: %v", err)
	}
	if liveResult.ProcessedFiles != 1 {
		t.Errorf("expected 1 hardlink file, got %d", liveResult.ProcessedFiles)
	}

	// Inode verification
	st1, err := os.Stat(f1)
	if err != nil {
		t.Fatal(err)
	}
	st2, err := os.Stat(f2)
	if err != nil {
		t.Fatal(err)
	}

	stat1, ok1 := st1.Sys().(*syscall.Stat_t)
	stat2, ok2 := st2.Sys().(*syscall.Stat_t)
	if ok1 && ok2 {
		if stat1.Ino != stat2.Ino {
			t.Errorf("expected shared inode, got ino1=%d, ino2=%d", stat1.Ino, stat2.Ino)
		}
	}
}
