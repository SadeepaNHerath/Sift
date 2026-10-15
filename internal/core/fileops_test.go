package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMoveToTrash_TempFile tests moving a temporary file to macOS Trash.
func TestMoveToTrash_TempFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "trash_test.txt")

	if err := os.WriteFile(testFile, []byte("temporary data for trash test"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	trashPath, err := moveToTrashWithDest(testFile)
	if err != nil {
		t.Fatalf("moveToTrashWithDest failed: %v", err)
	}

	// Verify file is gone from original location
	if _, err := os.Lstat(testFile); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed from original location %q, but it still exists", testFile)
	}

	// Cleanup trash destination if created
	if trashPath != "" {
		_ = os.Remove(trashPath)
	}
}

// TestMoveToTrash_NonExistentFile tests that moving a non-existent file returns an error.
func TestMoveToTrash_NonExistentFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does_not_exist.txt")
	err := MoveToTrash(missing)
	if err == nil {
		t.Fatalf("expected error moving non-existent file to trash, got nil")
	}
}

// TestSafeRemove_BlocksProtectedPaths verifies that SafeRemove refuses to touch protected paths.
func TestSafeRemove_BlocksProtectedPaths(t *testing.T) {
	protectedPaths := []string{
		"/System",
		"/usr/bin/tar",
		"~/.ssh",
		"~/Desktop",
		"~/Library/Caches/com.apple.Safari",
		"",
		"relative/path/test.txt",
	}

	for _, p := range protectedPaths {
		t.Run(p, func(t *testing.T) {
			err := SafeRemove(p)
			if err == nil {
				t.Fatalf("expected SafeRemove to block protected path %q, got nil", p)
			}
		})
	}
}

// TestSafeRemove_ValidTempFile tests that SafeRemove successfully deletes a valid temporary file.
func TestSafeRemove_ValidTempFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "safe_remove_test.txt")

	if err := os.WriteFile(filePath, []byte("safe remove sample content"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if err := SafeRemove(filePath); err != nil {
		t.Fatalf("SafeRemove failed on valid temp file: %v", err)
	}

	if _, err := os.Lstat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, but still exists: %q", filePath)
	}

	// Cleanup potential trash file
	trashTarget := filepath.Join(getTrashDir(), filepath.Base(filePath))
	_ = os.Remove(trashTarget)
}

// TestSafeRemove_DirectoryTree tests SafeRemove on a directory containing files.
func TestSafeRemove_DirectoryTree(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "safe_remove_dir")
	if err := os.MkdirAll(filepath.Join(targetDir, "nested"), 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "nested", "child.txt"), []byte("child"), 0644); err != nil {
		t.Fatalf("failed to create child file: %v", err)
	}

	if err := SafeRemove(targetDir); err != nil {
		t.Fatalf("SafeRemove failed on dir tree: %v", err)
	}

	if _, err := os.Lstat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("expected dir to be removed, but still exists: %q", targetDir)
	}

	trashTarget := filepath.Join(getTrashDir(), filepath.Base(targetDir))
	_ = os.RemoveAll(trashTarget)
}

// TestSafeRemove_MissingFile tests that SafeRemove returns an error if target does not exist.
func TestSafeRemove_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not_here.txt")
	err := SafeRemove(missing)
	if err == nil {
		t.Fatalf("expected error on missing file, got nil")
	}
}

// TestSafeRemoveAll_MixedPaths tests SafeRemoveAll with a mix of valid, protected, and missing paths.
func TestSafeRemoveAll_MixedPaths(t *testing.T) {
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "valid1.txt")
	file2 := filepath.Join(tempDir, "valid2.txt")
	missing := filepath.Join(tempDir, "missing.txt")
	protected := "/System"

	_ = os.WriteFile(file1, []byte("file 1 content"), 0644)
	_ = os.WriteFile(file2, []byte("file 2 content"), 0644)

	paths := []string{file1, protected, file2, missing}

	report := SafeRemoveAll(paths)
	if report == nil {
		t.Fatalf("SafeRemoveAll returned nil report")
	}

	if len(report.Removed) != 2 {
		t.Errorf("expected 2 removed items, got %d", len(report.Removed))
	}

	if len(report.Failed) != 2 {
		t.Errorf("expected 2 failed items, got %d", len(report.Failed))
	}

	expectedBytes := int64(len("file 1 content") + len("file 2 content"))
	if report.TotalBytes != expectedBytes {
		t.Errorf("TotalBytes = %d; want %d", report.TotalBytes, expectedBytes)
	}

	// Verify valid files are deleted
	if _, err := os.Lstat(file1); !os.IsNotExist(err) {
		t.Errorf("file1 should have been removed")
	}
	if _, err := os.Lstat(file2); !os.IsNotExist(err) {
		t.Errorf("file2 should have been removed")
	}

	// Clean up trash
	for _, r := range report.Removed {
		if r.TrashPath != "" {
			_ = os.Remove(r.TrashPath)
		}
	}
}

// TestDryRunRemove_DoesNotDelete verifies that DryRunRemove never removes any file from disk.
func TestDryRunRemove_DoesNotDelete(t *testing.T) {
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "dry_file1.txt")
	file2 := filepath.Join(tempDir, "dry_file2.txt")
	missing := filepath.Join(tempDir, "dry_missing.txt")
	protected := "/usr/bin/tar"

	data1 := []byte("dry run test payload 1")
	data2 := []byte("dry run test payload 2")

	if err := os.WriteFile(file1, data1, 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, data2, 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	paths := []string{file1, protected, file2, missing}

	report := DryRunRemove(paths)
	if report == nil {
		t.Fatalf("DryRunRemove returned nil report")
	}

	if len(report.WouldRemove) != 2 {
		t.Errorf("expected 2 WouldRemove items, got %d", len(report.WouldRemove))
	}

	if len(report.Blocked) != 2 {
		t.Errorf("expected 2 Blocked items, got %d", len(report.Blocked))
	}

	expectedBytes := int64(len(data1) + len(data2))
	if report.TotalBytes != expectedBytes {
		t.Errorf("TotalBytes = %d; want %d", report.TotalBytes, expectedBytes)
	}

	// CRITICAL CHECK: Ensure files STILL EXIST on disk!
	if _, err := os.Lstat(file1); err != nil {
		t.Fatalf("DryRunRemove modified file1! Expected file to exist, err: %v", err)
	}
	if _, err := os.Lstat(file2); err != nil {
		t.Fatalf("DryRunRemove modified file2! Expected file to exist, err: %v", err)
	}
}

// TestFormatBytes tests human-readable byte formatting across sizes.
func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{-10, "0 B"},
		{500, "500 B"},
		{999, "999 B"},
		{1000, "1 KB"},
		{1500, "1.5 KB"},
		{1000 * 1000, "1 MB"},
		{456 * 1000 * 1000, "456 MB"},
		{1200000000, "1.2 GB"},
		{1000 * 1000 * 1000 * 1000, "1 TB"},
		{int64(2.5 * 1000 * 1000 * 1000 * 1000), "2.5 TB"},
		{1000 * 1000 * 1000 * 1000 * 1000, "1 PB"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			result := FormatBytes(tc.bytes)
			if result != tc.expected {
				t.Errorf("FormatBytes(%d) = %q; want %q", tc.bytes, result, tc.expected)
			}
		})
	}
}

// TestFileSize_FileAndDirectory tests size calculation for regular files and directory hierarchies.
func TestFileSize_FileAndDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Regular file
	filePayload := []byte(strings.Repeat("a", 500))
	singleFile := filepath.Join(tempDir, "single.dat")
	if err := os.WriteFile(singleFile, filePayload, 0644); err != nil {
		t.Fatalf("failed to write single file: %v", err)
	}

	size, err := FileSize(singleFile)
	if err != nil {
		t.Fatalf("FileSize failed on file: %v", err)
	}
	if size != 500 {
		t.Errorf("FileSize(singleFile) = %d; want 500", size)
	}

	// 2. Directory hierarchy
	treeDir := filepath.Join(tempDir, "tree")
	if err := os.MkdirAll(filepath.Join(treeDir, "sub", "deep"), 0755); err != nil {
		t.Fatalf("failed to create tree dirs: %v", err)
	}

	_ = os.WriteFile(filepath.Join(treeDir, "f1.txt"), []byte(strings.Repeat("x", 200)), 0644)
	_ = os.WriteFile(filepath.Join(treeDir, "sub", "f2.txt"), []byte(strings.Repeat("y", 300)), 0644)
	_ = os.WriteFile(filepath.Join(treeDir, "sub", "deep", "f3.txt"), []byte(strings.Repeat("z", 500)), 0644)

	dirSize, err := FileSize(treeDir)
	if err != nil {
		t.Fatalf("FileSize failed on directory: %v", err)
	}
	if dirSize != 1000 {
		t.Errorf("FileSize(treeDir) = %d; want 1000", dirSize)
	}

	// 3. Empty directory
	emptyDir := filepath.Join(tempDir, "empty")
	_ = os.Mkdir(emptyDir, 0755)
	emptySize, err := FileSize(emptyDir)
	if err != nil {
		t.Fatalf("FileSize failed on empty dir: %v", err)
	}
	if emptySize != 0 {
		t.Errorf("FileSize(emptyDir) = %d; want 0", emptySize)
	}

	// 4. Missing path
	if _, err := FileSize(filepath.Join(tempDir, "missing.dat")); err == nil {
		t.Fatalf("expected error on missing path, got nil")
	}
}

// TestTrashCollisionHandling tests collision name generation when items with the same name are trashed.
func TestTrashCollisionHandling(t *testing.T) {
	tempTrash := t.TempDir()

	base := "document.txt"
	initial := filepath.Join(tempTrash, base)

	// Step 1: No file exists yet -> resolves to initial
	cand1 := ResolveTrashCollision(tempTrash, base)
	if cand1 != initial {
		t.Fatalf("expected initial path %q, got %q", initial, cand1)
	}

	// Create initial file
	if err := os.WriteFile(initial, []byte("version 1"), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	// Step 2: First collision -> resolves to "document 1.txt"
	cand2 := ResolveTrashCollision(tempTrash, base)
	expectedCand2 := filepath.Join(tempTrash, "document 1.txt")
	if cand2 != expectedCand2 {
		t.Fatalf("expected collision path %q, got %q", expectedCand2, cand2)
	}

	// Create first collision file
	if err := os.WriteFile(cand2, []byte("version 2"), 0644); err != nil {
		t.Fatalf("failed to create collision file: %v", err)
	}

	// Step 3: Second collision -> resolves to "document 2.txt"
	cand3 := ResolveTrashCollision(tempTrash, base)
	expectedCand3 := filepath.Join(tempTrash, "document 2.txt")
	if cand3 != expectedCand3 {
		t.Fatalf("expected second collision path %q, got %q", expectedCand3, cand3)
	}
}

// TestResolveTrashCollision_NoExtension tests collision resolution for files without file extensions.
func TestResolveTrashCollision_NoExtension(t *testing.T) {
	tempTrash := t.TempDir()
	base := "README"

	_ = os.WriteFile(filepath.Join(tempTrash, base), []byte("v1"), 0644)
	cand := ResolveTrashCollision(tempTrash, base)
	expected := filepath.Join(tempTrash, "README 1")
	if cand != expected {
		t.Errorf("ResolveTrashCollision(%q) = %q; want %q", base, cand, expected)
	}
}

// TestRemovalReportAccuracy tests the precision of Removed, Failed, and TotalBytes in RemovalReport.
func TestRemovalReportAccuracy(t *testing.T) {
	tempDir := t.TempDir()

	f1 := filepath.Join(tempDir, "item1.bin")
	f2 := filepath.Join(tempDir, "item2.bin")
	f3 := filepath.Join(tempDir, "item3.bin")

	_ = os.WriteFile(f1, []byte(strings.Repeat("1", 100)), 0644)
	_ = os.WriteFile(f2, []byte(strings.Repeat("2", 250)), 0644)
	_ = os.WriteFile(f3, []byte(strings.Repeat("3", 400)), 0644)

	report := SafeRemoveAll([]string{f1, f2, f3})
	if report == nil {
		t.Fatalf("expected non-nil report")
	}

	if len(report.Removed) != 3 {
		t.Errorf("expected 3 removed items, got %d", len(report.Removed))
	}
	if len(report.Failed) != 0 {
		t.Errorf("expected 0 failed items, got %d", len(report.Failed))
	}
	if report.TotalBytes != 750 {
		t.Errorf("TotalBytes = %d; want 750", report.TotalBytes)
	}

	// Clean up trash
	for _, r := range report.Removed {
		if r.TrashPath != "" {
			_ = os.Remove(r.TrashPath)
		}
	}
}

// TestSafeRemoveAll_EmptyList tests SafeRemoveAll on empty input.
func TestSafeRemoveAll_EmptyList(t *testing.T) {
	report := SafeRemoveAll([]string{})
	if report == nil {
		t.Fatalf("expected non-nil report")
	}
	if len(report.Removed) != 0 || len(report.Failed) != 0 || report.TotalBytes != 0 {
		t.Errorf("expected empty report, got %+v", report)
	}
}

// TestDryRunRemove_EmptyList tests DryRunRemove on empty input.
func TestDryRunRemove_EmptyList(t *testing.T) {
	report := DryRunRemove([]string{})
	if report == nil {
		t.Fatalf("expected non-nil report")
	}
	if len(report.WouldRemove) != 0 || len(report.Blocked) != 0 || report.TotalBytes != 0 {
		t.Errorf("expected empty report, got %+v", report)
	}
}

// TestMoveCrossDevice tests copying and moving files and directories across distinct locations.
func TestMoveCrossDevice(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	// File test
	srcFile := filepath.Join(dirA, "src.txt")
	dstFile := filepath.Join(dirB, "dst.txt")
	_ = os.WriteFile(srcFile, []byte("cross device content"), 0644)

	if err := moveCrossDevice(srcFile, dstFile); err != nil {
		t.Fatalf("moveCrossDevice failed on file: %v", err)
	}

	if _, err := os.Lstat(srcFile); !os.IsNotExist(err) {
		t.Errorf("expected srcFile to be removed after move")
	}
	data, err := os.ReadFile(dstFile)
	if err != nil || string(data) != "cross device content" {
		t.Errorf("dstFile content mismatch: %v", err)
	}

	// Directory test
	srcDir := filepath.Join(dirA, "subfolder")
	dstDir := filepath.Join(dirB, "subfolder")
	_ = os.Mkdir(srcDir, 0755)
	_ = os.WriteFile(filepath.Join(srcDir, "sub.txt"), []byte("nested file"), 0644)

	if err := moveCrossDevice(srcDir, dstDir); err != nil {
		t.Fatalf("moveCrossDevice failed on dir: %v", err)
	}

	if _, err := os.Lstat(srcDir); !os.IsNotExist(err) {
		t.Errorf("expected srcDir to be removed after move")
	}
	subData, err := os.ReadFile(filepath.Join(dstDir, "sub.txt"))
	if err != nil || string(subData) != "nested file" {
		t.Errorf("dstDir subfile content mismatch: %v", err)
	}
}
