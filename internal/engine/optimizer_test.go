package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

func TestTaskCatalog(t *testing.T) {
	tasks := TaskCatalog()
	if len(tasks) != 7 {
		t.Fatalf("expected 7 tasks in catalog, got %d", len(tasks))
	}

	expectedIDs := map[string]bool{
		"dns":         true,
		"quicklook":   true,
		"icons":       true,
		"sqlite":      true,
		"launchd":     true,
		"dsstore":     true,
		"login_items": true,
	}

	foundIDs := make(map[string]bool)
	for _, task := range tasks {
		if task.ID == "" {
			t.Errorf("task has empty ID: %+v", task)
		}
		if task.Name == "" {
			t.Errorf("task has empty Name: %+v", task)
		}
		if task.Description == "" {
			t.Errorf("task has empty Description: %+v", task)
		}
		if !task.Safe {
			t.Errorf("expected task %s to be marked safe", task.ID)
		}
		foundIDs[task.ID] = true
	}

	for id := range expectedIDs {
		if !foundIDs[id] {
			t.Errorf("missing expected task ID: %s", id)
		}
	}
}

func TestRunTask_UnknownTask(t *testing.T) {
	outcome, err := RunTask("nonexistent_task_xyz", true, nil)
	if err == nil {
		t.Fatal("expected error for unknown task, got nil")
	}
	if outcome.Applied {
		t.Fatal("expected Applied=false for unknown task")
	}
	if !strings.Contains(outcome.Details, "Unknown") {
		t.Errorf("expected Details to mention Unknown, got %q", outcome.Details)
	}
}

func TestRunTask_DryRun_AllTasks(t *testing.T) {
	tasks := TaskCatalog()
	for _, task := range tasks {
		t.Run(task.ID, func(t *testing.T) {
			outcome, err := RunTask(task.ID, true, nil)
			if err != nil {
				t.Fatalf("RunTask(%s, dryRun=true) returned error: %v", task.ID, err)
			}
			if outcome.Applied {
				t.Errorf("expected Applied=false for dry-run of %s", task.ID)
			}
			if outcome.Details == "" {
				t.Errorf("expected non-empty Details for dry-run of %s", task.ID)
			}
		})
	}
}

func TestRunTask_DSStore_Live(t *testing.T) {
	outcome, err := RunTask("dsstore", false, nil)
	if err != nil {
		t.Fatalf("RunTask(dsstore, live) failed: %v", err)
	}
	if !outcome.Applied {
		t.Fatal("expected Applied=true for dsstore task")
	}
	if !strings.Contains(outcome.Details, "DS_Store") {
		t.Errorf("expected Details to mention DS_Store, got %q", outcome.Details)
	}
}

func TestRunTask_Launchd_AuditAndRemove(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a broken LaunchAgent plist pointing to non-existent executable
	brokenPlistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.test.broken</string>
    <key>ProgramArguments</key>
    <array>
        <string>/tmp/definitely_nonexistent_binary_xyz12345</string>
    </array>
</dict>
</plist>`
	brokenPlist := filepath.Join(tempDir, "com.test.broken.plist")
	if err := os.WriteFile(brokenPlist, []byte(brokenPlistContent), 0644); err != nil {
		t.Fatalf("failed to write broken plist: %v", err)
	}

	// 2. Create a valid LaunchAgent plist pointing to /bin/sh
	validPlistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.test.valid</string>
    <key>Program</key>
    <string>/bin/sh</string>
</dict>
</plist>`
	validPlist := filepath.Join(tempDir, "com.test.valid.plist")
	if err := os.WriteFile(validPlist, []byte(validPlistContent), 0644); err != nil {
		t.Fatalf("failed to write valid plist: %v", err)
	}

	// Override launchd directories for test isolation
	originalDirs := launchdCustomDirs
	launchdCustomDirs = []string{tempDir}
	defer func() { launchdCustomDirs = originalDirs }()

	// Dry-run should detect 1 broken item
	dryOutcome, err := RunTask("launchd", true, nil)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if !strings.Contains(dryOutcome.Details, "found 1 broken") {
		t.Errorf("expected 'found 1 broken' in dry-run details, got: %s", dryOutcome.Details)
	}

	// Live run should remove the broken plist and keep the valid one
	liveOutcome, err := RunTask("launchd", false, nil)
	if err != nil {
		t.Fatalf("live run failed: %v", err)
	}
	if !liveOutcome.Applied {
		t.Fatal("expected Applied=true for launchd live task")
	}

	if _, err := os.Stat(brokenPlist); !os.IsNotExist(err) {
		t.Errorf("expected broken plist %s to be removed, but it still exists", brokenPlist)
	}
	if _, err := os.Stat(validPlist); err != nil {
		t.Errorf("expected valid plist %s to remain untouched, but error: %v", validPlist, err)
	}
}

func TestRunTask_SQLite_AuditAndVacuum(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "sample.db")

	// Create a real SQLite database file using sqlite3 CLI
	cmd := exec.Command("sqlite3", dbPath, "CREATE TABLE test (id INTEGER PRIMARY KEY, val TEXT); INSERT INTO test VALUES (1, 'hello');")
	if err := cmd.Run(); err != nil {
		t.Skip("sqlite3 CLI not available in test environment, skipping live vacuum test")
	}

	originalDirs := sqliteCustomDirs
	sqliteCustomDirs = []string{tempDir}
	defer func() { sqliteCustomDirs = originalDirs }()

	// Dry run
	dryOutcome, err := RunTask("sqlite", true, nil)
	if err != nil {
		t.Fatalf("sqlite dry run failed: %v", err)
	}
	if !strings.Contains(dryOutcome.Details, "Would vacuum 1 SQLite") {
		t.Errorf("expected dry run to report 1 database, got: %s", dryOutcome.Details)
	}

	// Live run
	liveOutcome, err := RunTask("sqlite", false, nil)
	if err != nil {
		t.Fatalf("sqlite live run failed: %v", err)
	}
	if !liveOutcome.Applied {
		t.Fatal("expected Applied=true for sqlite live task")
	}
	if !strings.Contains(liveOutcome.Details, "Successfully vacuumed 1 SQLite") {
		t.Errorf("expected live run details to report success, got: %s", liveOutcome.Details)
	}
}

func TestExtractPlistExecutable(t *testing.T) {
	tempDir := t.TempDir()

	p1 := filepath.Join(tempDir, "p1.plist")
	content1 := `<plist><dict><key>Program</key><string>/usr/bin/python3</string></dict></plist>`
	_ = os.WriteFile(p1, []byte(content1), 0644)

	exec1, err := extractPlistExecutable(p1)
	if err != nil || exec1 != "/usr/bin/python3" {
		t.Errorf("extractPlistExecutable p1: expected /usr/bin/python3, got %q (err: %v)", exec1, err)
	}

	p2 := filepath.Join(tempDir, "p2.plist")
	content2 := `<plist><dict><key>ProgramArguments</key><array><string>/bin/echo</string><string>hi</string></array></dict></plist>`
	_ = os.WriteFile(p2, []byte(content2), 0644)

	exec2, err := extractPlistExecutable(p2)
	if err != nil || exec2 != "/bin/echo" {
		t.Errorf("extractPlistExecutable p2: expected /bin/echo, got %q (err: %v)", exec2, err)
	}

	p3 := filepath.Join(tempDir, "p3.plist")
	content3 := `<plist><dict><key>OtherKey</key><string>val</string></dict></plist>`
	_ = os.WriteFile(p3, []byte(content3), 0644)

	_, err3 := extractPlistExecutable(p3)
	if err3 == nil {
		t.Errorf("expected error for plist without Program or ProgramArguments, got nil")
	}
}

func TestIsExecutableMissing(t *testing.T) {
	if isExecutableMissing("/bin/sh") {
		t.Errorf("/bin/sh should exist and not be missing")
	}
	if !isExecutableMissing("/definitely/missing/binary/path/123456") {
		t.Errorf("non-existent binary path should report as missing")
	}
	if isExecutableMissing("sh") {
		t.Errorf("'sh' should exist on PATH and not be missing")
	}
	if !isExecutableMissing("nonexistent_binary_name_xyz987654") {
		t.Errorf("non-existent command name should report as missing")
	}
}

func TestRunTask_WithLogger(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))
	t.Setenv("SF_LOG_DIR", filepath.Join(tempHome, "Library", "Logs", "sf"))

	logger, err := core.NewLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Close()

	outcome, err := RunTask("dsstore", false, logger)
	if err != nil {
		t.Fatalf("RunTask dsstore with logger failed: %v", err)
	}
	if !outcome.Applied {
		t.Fatalf("expected Applied=true")
	}

	ops, err := core.ReadOperations(10)
	if err != nil {
		t.Fatalf("failed to read operations log: %v", err)
	}
	found := false
	for _, op := range ops {
		if op.Command == "optimize" && op.Action == "dsstore" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find optimize dsstore operation in log, found: %+v", ops)
	}
}
