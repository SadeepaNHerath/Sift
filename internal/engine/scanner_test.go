package engine

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNewDiskScanner(t *testing.T) {
	s1 := NewDiskScanner(10)
	if s1.concurrency != 10 {
		t.Fatalf("expected concurrency 10, got %d", s1.concurrency)
	}

	s2 := NewDiskScanner(0)
	if s2.concurrency < 4 || s2.concurrency > 32 {
		t.Fatalf("expected concurrency between 4 and 32, got %d", s2.concurrency)
	}
}

func TestScanDirProgressive(t *testing.T) {
	tempDir := t.TempDir()

	// Create test file structure:
	// tempDir/
	//   file_small.txt (100 bytes)
	//   file_large.txt (500 bytes)
	//   subdir1/ (contains file_mid.txt: 300 bytes)
	//   subdir2/ (empty)

	smallContent := make([]byte, 100)
	largeContent := make([]byte, 500)
	midContent := make([]byte, 300)

	if err := os.WriteFile(filepath.Join(tempDir, "file_small.txt"), smallContent, 0644); err != nil {
		t.Fatalf("failed to create file_small.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "file_large.txt"), largeContent, 0644); err != nil {
		t.Fatalf("failed to create file_large.txt: %v", err)
	}

	sub1 := filepath.Join(tempDir, "subdir1")
	if err := os.Mkdir(sub1, 0755); err != nil {
		t.Fatalf("failed to create subdir1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub1, "file_mid.txt"), midContent, 0644); err != nil {
		t.Fatalf("failed to create file_mid.txt: %v", err)
	}

	sub2 := filepath.Join(tempDir, "subdir2")
	if err := os.Mkdir(sub2, 0755); err != nil {
		t.Fatalf("failed to create subdir2: %v", err)
	}

	var mu sync.Mutex
	var streamed []FileEntry
	onEntry := func(fe FileEntry) {
		mu.Lock()
		streamed = append(streamed, fe)
		mu.Unlock()
	}

	scanner := NewDiskScanner(4)
	rootEntry, err := scanner.ScanDirProgressive(tempDir, onEntry)
	if err != nil {
		t.Fatalf("ScanDirProgressive failed: %v", err)
	}

	if rootEntry == nil {
		t.Fatal("expected non-nil rootEntry")
	}

	// Total size = 500 + 300 + 100 = 900
	if rootEntry.Size != 900 {
		t.Fatalf("expected total size 900, got %d", rootEntry.Size)
	}

	// 4 immediate entries: file_large, subdir1, file_small, subdir2
	if len(rootEntry.Children) != 4 {
		t.Fatalf("expected 4 children, got %d", len(rootEntry.Children))
	}

	// Children should be sorted descending by size
	if rootEntry.Children[0].Size != 500 {
		t.Errorf("expected first child size 500, got %d", rootEntry.Children[0].Size)
	}
	if rootEntry.Children[1].Size != 300 {
		t.Errorf("expected second child size 300, got %d", rootEntry.Children[1].Size)
	}
	if rootEntry.Children[2].Size != 100 {
		t.Errorf("expected third child size 100, got %d", rootEntry.Children[2].Size)
	}
	if rootEntry.Children[3].Size != 0 {
		t.Errorf("expected fourth child size 0, got %d", rootEntry.Children[3].Size)
	}

	// Verify streaming callback caught all items
	mu.Lock()
	streamedCount := len(streamed)
	mu.Unlock()
	if streamedCount != 4 {
		t.Errorf("expected 4 streamed items, got %d", streamedCount)
	}
}

func TestScanDirProgressive_LeafFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")
	content := []byte("hello world disk scanner")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	var streamed []FileEntry
	root, err := ScanDirProgressive(filePath, func(fe FileEntry) {
		streamed = append(streamed, fe)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), root.Size)
	}
	if root.IsDir {
		t.Errorf("expected IsDir to be false")
	}
	if len(streamed) != 1 {
		t.Errorf("expected 1 streamed entry, got %d", len(streamed))
	}
}

func TestScanDirProgressive_NonExistent(t *testing.T) {
	_, err := ScanDirProgressive("/nonexistent_path_spacefold_test", nil)
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
}

func TestScanTopFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create 5 files with distinct sizes
	sizes := map[string]int{
		"f10.bin": 10,
		"f50.bin": 50,
		"f20.bin": 20,
		"f40.bin": 40,
		"f30.bin": 30,
	}

	sub := filepath.Join(tempDir, "nested")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}

	for name, sz := range sizes {
		p := filepath.Join(tempDir, name)
		if sz == 40 || sz == 50 {
			p = filepath.Join(sub, name)
		}
		if err := os.WriteFile(p, make([]byte, sz), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Ask for top 3 largest files
	top, err := ScanTopFiles(tempDir, 3)
	if err != nil {
		t.Fatalf("ScanTopFiles failed: %v", err)
	}

	if len(top) != 3 {
		t.Fatalf("expected 3 top files, got %d", len(top))
	}

	// Should be 50, 40, 30 in descending order
	if top[0].Size != 50 {
		t.Errorf("expected top[0] size 50, got %d (%s)", top[0].Size, top[0].Name)
	}
	if top[1].Size != 40 {
		t.Errorf("expected top[1] size 40, got %d (%s)", top[1].Size, top[1].Name)
	}
	if top[2].Size != 30 {
		t.Errorf("expected top[2] size 30, got %d (%s)", top[2].Size, top[2].Name)
	}
}

func TestScanTopFiles_LeafFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "solo.txt")
	if err := os.WriteFile(filePath, []byte("single"), 0644); err != nil {
		t.Fatal(err)
	}

	top, err := ScanTopFiles(filePath, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(top) != 1 || top[0].Size != 6 {
		t.Errorf("expected 1 file of size 6, got %+v", top)
	}
}

func TestScanInsights(t *testing.T) {
	scanner := NewDiskScanner(4)
	insights, err := scanner.ScanInsights()
	if err != nil {
		t.Fatalf("ScanInsights returned error: %v", err)
	}

	// Insights may be empty or contain real caches on macOS, but should not fail
	for _, entry := range insights {
		if entry.InsightsCategory == "" {
			t.Errorf("insight entry %s missing InsightsCategory", entry.Name)
		}
	}
}
