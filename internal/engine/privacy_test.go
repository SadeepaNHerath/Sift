package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanPrivacyTargets_MockEnvironment(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tempHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	sharedFileListDir := filepath.Join(tempHome, "Library", "Application Support", "com.apple.sharedfilelist")
	_ = os.MkdirAll(sharedFileListDir, 0755)
	_ = os.WriteFile(filepath.Join(sharedFileListDir, "com.apple.LSSharedFileList.ApplicationRecentDocuments.sfl3"), []byte("mock-recent"), 0644)

	crashDir := filepath.Join(tempHome, "Library", "Logs", "DiagnosticReports")
	_ = os.MkdirAll(crashDir, 0755)
	_ = os.WriteFile(filepath.Join(crashDir, "Safari-crash.ips"), []byte("crash-stack-trace"), 0644)

	chromeDir := filepath.Join(tempHome, "Library", "Application Support", "Google", "Chrome", "Default", "Local Storage", "leveldb")
	_ = os.MkdirAll(chromeDir, 0755)
	_ = os.WriteFile(filepath.Join(chromeDir, "000003.log"), []byte("leveldb-local-storage"), 0644)

	targets, err := ScanPrivacyTargets()
	if err != nil {
		t.Fatalf("ScanPrivacyTargets failed: %v", err)
	}

	if len(targets) != 4 {
		t.Fatalf("expected 4 privacy targets, got %d", len(targets))
	}

	foundRecent := false
	foundCrash := false
	foundBrowser := false
	foundClipboard := false

	for _, tgt := range targets {
		switch tgt.ID {
		case "recent-items":
			foundRecent = true
			if tgt.ItemCount < 1 {
				t.Errorf("expected at least 1 recent item, got %d", tgt.ItemCount)
			}
		case "diagnostics":
			foundCrash = true
			if tgt.ItemCount < 1 {
				t.Errorf("expected at least 1 diagnostic item, got %d", tgt.ItemCount)
			}
		case "browser-storage":
			foundBrowser = true
			if tgt.ItemCount < 1 {
				t.Errorf("expected at least 1 browser storage item, got %d", tgt.ItemCount)
			}
		case "clipboard":
			foundClipboard = true
			if !tgt.IsClipboard {
				t.Errorf("expected IsClipboard=true")
			}
		}
	}

	if !foundRecent || !foundCrash || !foundBrowser || !foundClipboard {
		t.Errorf("missing privacy targets: recent=%v, crash=%v, browser=%v, clip=%v",
			foundRecent, foundCrash, foundBrowser, foundClipboard)
	}

	// Test DryRun
	dryReport, err := WipePrivacyTargets(targets, true, nil)
	if err != nil {
		t.Fatalf("WipePrivacyTargets dry run failed: %v", err)
	}
	if len(dryReport.Removed) == 0 {
		t.Errorf("expected dry run items to be populated")
	}

	// Verify file still exists on disk after dry run
	recentFile := filepath.Join(sharedFileListDir, "com.apple.LSSharedFileList.ApplicationRecentDocuments.sfl3")
	if _, err := os.Stat(recentFile); err != nil {
		t.Errorf("dry-run deleted file: %v", err)
	}

	// Test Actual Execution
	execReport, err := WipePrivacyTargets(targets, false, nil)
	if err != nil {
		t.Fatalf("WipePrivacyTargets execute failed: %v", err)
	}
	if len(execReport.Removed) == 0 {
		t.Errorf("expected removed items after wipe")
	}

	// Verify file was moved to Trash / deleted
	if _, err := os.Stat(recentFile); !os.IsNotExist(err) {
		t.Errorf("file should have been removed: %s", recentFile)
	}
}
