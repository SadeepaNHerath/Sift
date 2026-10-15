package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSystemData(t *testing.T) {
	fakeHome := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", fakeHome)

	// Create mock developer structure
	derivedData := filepath.Join(fakeHome, "Library", "Developer", "Xcode", "DerivedData")
	if err := os.MkdirAll(derivedData, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(derivedData, "build.log"), []byte("xcode mock data"), 0644)

	// Create mock trash
	trashDir := filepath.Join(fakeHome, ".Trash")
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(trashDir, "old.txt"), []byte("trash mock data"), 0644)

	report, err := ScanSystemData()
	if err != nil {
		t.Fatalf("ScanSystemData failed: %v", err)
	}

	if report.TotalBytes == 0 {
		t.Errorf("expected non-zero TotalBytes, got %d", report.TotalBytes)
	}

	foundXcode := false
	foundTrash := false
	for _, itm := range report.Items {
		if itm.Name == "Xcode DerivedData" {
			foundXcode = true
		}
		if itm.Name == "User Trash" {
			foundTrash = true
		}
	}

	if !foundXcode {
		t.Error("expected to find Xcode DerivedData item")
	}
	if !foundTrash {
		t.Error("expected to find User Trash item")
	}
}
