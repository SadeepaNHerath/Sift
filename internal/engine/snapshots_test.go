package engine

import (
	"errors"
	"testing"
)

func TestParseLocalSnapshots(t *testing.T) {
	raw := []byte(`Snapshots for disk /:
com.apple.TimeMachine.2026-10-14-192341.local
com.apple.TimeMachine.2026-10-15-081200.local
`)

	snaps := ParseLocalSnapshots(raw)
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}

	if snaps[0].Name != "com.apple.TimeMachine.2026-10-14-192341.local" {
		t.Errorf("unexpected snapshot name: %s", snaps[0].Name)
	}
	if snaps[0].DateStr != "2026-10-14 19:23:41" {
		t.Errorf("unexpected snapshot date: %s", snaps[0].DateStr)
	}

	// Empty output
	empty := ParseLocalSnapshots([]byte("Snapshots for disk /:\n"))
	if len(empty) != 0 {
		t.Errorf("expected 0 snapshots for empty output, got %d", len(empty))
	}
}

func TestListLocalSnapshotsMock(t *testing.T) {
	origExecList := execTmutilList
	defer func() { execTmutilList = origExecList }()

	execTmutilList = func(mountPoint string) ([]byte, error) {
		return []byte("Snapshots for disk /:\ncom.apple.TimeMachine.2026-10-15-120000.local\n"), nil
	}

	snaps, err := ListLocalSnapshots()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].DateStr != "2026-10-15 12:00:00" {
		t.Errorf("unexpected date string: %s", snaps[0].DateStr)
	}

	// Error case
	execTmutilList = func(mountPoint string) ([]byte, error) {
		return nil, errors.New("command failed")
	}
	_, err = ListLocalSnapshots()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestThinLocalSnapshotsMock(t *testing.T) {
	origExecThin := execTmutilThin
	defer func() { execTmutilThin = origExecThin }()

	execTmutilThin = func(mountPoint string, bytes int64, urgency int) ([]byte, error) {
		return []byte("Thinned local snapshots:\ncom.apple.TimeMachine.2026-10-15-120000.local\n"), nil
	}

	out, err := ThinLocalSnapshots(10*1000*1000*1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected output from thinning")
	}
}
