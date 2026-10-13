package engine

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

func createTestZip(t *testing.T, zipPath string, entryNames []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for _, name := range entryNames {
		ew, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = ew.Write([]byte("mock content"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultInstallerScanDirs(t *testing.T) {
	dirs := DefaultInstallerScanDirs()
	// On standard macOS, ~/Downloads or ~/Desktop usually exists
	if len(dirs) == 0 {
		t.Log("Note: no default installer directories exist in this test environment")
	}
}

func TestScanInstallersDirs(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Direct installer files
	dmgFile := filepath.Join(tempDir, "Software.dmg")
	pkgFile := filepath.Join(tempDir, "Update.pkg")
	mpkgFile := filepath.Join(tempDir, "Bundle.mpkg")
	isoFile := filepath.Join(tempDir, "Disk.iso")
	xipFile := filepath.Join(tempDir, "Xcode.xip")

	for _, p := range []string{dmgFile, pkgFile, mpkgFile, isoFile, xipFile} {
		if err := os.WriteFile(p, []byte("installer binary"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 2. Non-installer files (should be ignored)
	pdfFile := filepath.Join(tempDir, "manual.pdf")
	fakeFile := filepath.Join(tempDir, "notes.dmg.txt")
	if err := os.WriteFile(pdfFile, []byte("pdf"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fakeFile, []byte("txt"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Zip file containing .app (should be detected as installer)
	appZip := filepath.Join(tempDir, "Slack.zip")
	createTestZip(t, appZip, []string{"Slack.app/Contents/MacOS/Slack", "Slack.app/Contents/Info.plist"})

	// 4. Zip file containing .pkg (should be detected as installer)
	pkgZip := filepath.Join(tempDir, "Driver.zip")
	createTestZip(t, pkgZip, []string{"driver_install.pkg", "README.txt"})

	// 5. Zip file containing plain files (should NOT be detected)
	docZip := filepath.Join(tempDir, "Documents.zip")
	createTestZip(t, docZip, []string{"report.docx", "data.csv"})

	// 6. Nested installer in subdirectory
	nestedDir := filepath.Join(tempDir, "subfolder", "downloads")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	nestedDmg := filepath.Join(nestedDir, "Nested.dmg")
	if err := os.WriteFile(nestedDmg, []byte("nested dmg"), 0644); err != nil {
		t.Fatal(err)
	}

	// Perform scan
	files, err := ScanInstallersDirs([]string{tempDir})
	if err != nil {
		t.Fatalf("ScanInstallersDirs failed: %v", err)
	}

	// Expected detected files: Software.dmg, Update.pkg, Bundle.mpkg, Disk.iso, Xcode.xip, Slack.zip, Driver.zip, Nested.dmg = 8
	if len(files) != 8 {
		t.Fatalf("expected 8 detected installer files, got %d", len(files))
	}

	matchedPaths := make(map[string]InstallerFile)
	for _, f := range files {
		matchedPaths[f.Path] = f
	}

	// Verify direct file attributes
	if dmg, ok := matchedPaths[dmgFile]; !ok {
		t.Errorf("missing %s", dmgFile)
	} else {
		if dmg.InArchive {
			t.Errorf("expected InArchive=false for direct dmg")
		}
		if dmg.Extension != ".dmg" {
			t.Errorf("expected extension .dmg, got %s", dmg.Extension)
		}
	}

	// Verify zip installer attributes
	if slack, ok := matchedPaths[appZip]; !ok {
		t.Errorf("missing %s", appZip)
	} else {
		if !slack.InArchive {
			t.Errorf("expected InArchive=true for Slack.zip")
		}
		if slack.ArchivePath != appZip {
			t.Errorf("expected ArchivePath=%s, got %s", appZip, slack.ArchivePath)
		}
	}

	// Verify non-installers were not included
	if _, ok := matchedPaths[pdfFile]; ok {
		t.Errorf("pdf file should not be detected")
	}
	if _, ok := matchedPaths[fakeFile]; ok {
		t.Errorf("fake .dmg.txt file should not be detected")
	}
	if _, ok := matchedPaths[docZip]; ok {
		t.Errorf("plain document zip should not be detected")
	}
}

func TestRemoveInstallers_DryRunAndActual(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	downloadsDir := filepath.Join(tempHome, "Downloads")
	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	installerPath := filepath.Join(downloadsDir, "AppSetup.dmg")
	content := []byte("installer image contents")
	if err := os.WriteFile(installerPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	files := []InstallerFile{
		{
			Path:      installerPath,
			Name:      "AppSetup.dmg",
			Extension: ".dmg",
			Size:      int64(len(content)),
			ModTime:   time.Now(),
		},
	}

	// 1. Dry run test
	dryReport, err := RemoveInstallers(files, true, nil)
	if err != nil {
		t.Fatalf("dry run RemoveInstallers failed: %v", err)
	}
	if len(dryReport.Removed) != 1 {
		t.Fatalf("expected 1 dry run removed item, got %d", len(dryReport.Removed))
	}
	if dryReport.TotalBytes != int64(len(content)) {
		t.Errorf("expected total bytes %d, got %d", len(content), dryReport.TotalBytes)
	}
	// Verify file still exists on disk
	if _, err := os.Stat(installerPath); err != nil {
		t.Fatalf("file should still exist after dry run: %v", err)
	}

	// 2. Actual removal test
	logger, _ := core.NewLogger()
	actualReport, err := RemoveInstallers(files, false, logger)
	if err != nil {
		t.Fatalf("actual RemoveInstallers failed: %v", err)
	}
	if len(actualReport.Removed) != 1 {
		t.Fatalf("expected 1 removed item, got %d", len(actualReport.Removed))
	}
	// Verify file no longer exists at original path
	if _, err := os.Stat(installerPath); !os.IsNotExist(err) {
		t.Errorf("expected installer file to be moved to trash, still exists")
	}
}

func TestRemoveInstallers_SafetyBlock(t *testing.T) {
	// Attempt removal of system protected file
	blockedFile := []InstallerFile{
		{
			Path:      "/System/Installers/update.pkg",
			Name:      "update.pkg",
			Extension: ".pkg",
			Size:      2048,
		},
	}

	report, err := RemoveInstallers(blockedFile, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Failed) != 1 {
		t.Fatalf("expected 1 failed item due to safety protection, got %d", len(report.Failed))
	}
	if len(report.Removed) != 0 {
		t.Fatalf("expected 0 removed items, got %d", len(report.Removed))
	}
}
