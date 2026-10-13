package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoggerOperationsAndDeletions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SF_LOG_DIR", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	if logger.sessionID == "" {
		t.Fatalf("expected non-empty session ID")
	}

	logger.LogOperation("clean", "scan", "started", "scanned 50 items")
	logger.LogOperation("clean", "delete", "success", "deleted 10 items")

	logger.LogDeletion("/tmp/file1.txt", 1024, "trash", "clean")
	logger.LogDeletion("/tmp/file2.txt", 2048, "permanent", "clean")

	logger.Close()

	ops, err := ReadOperations(10)
	if err != nil {
		t.Fatalf("ReadOperations failed: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(ops))
	}
	// Verify most recent first
	if ops[0].Action != "delete" || ops[1].Action != "scan" {
		t.Fatalf("expected operations in reverse chronological order, got %+v", ops)
	}

	dels, err := ReadDeletions(1)
	if err != nil {
		t.Fatalf("ReadDeletions failed: %v", err)
	}
	if len(dels) != 1 {
		t.Fatalf("expected 1 deletion due to limit, got %d", len(dels))
	}
	if dels[0].Path != "/tmp/file2.txt" || dels[0].Bytes != 2048 {
		t.Fatalf("unexpected deletion: %+v", dels[0])
	}

	// Verify permissions
	opsFi, err := os.Stat(filepath.Join(tmpDir, "operations.tsv"))
	if err != nil {
		t.Fatalf("stat operations.tsv failed: %v", err)
	}
	if perm := opsFi.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", perm)
	}
}
