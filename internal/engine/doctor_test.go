package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunDoctorChecks(t *testing.T) {
	report, err := RunDoctorChecks()
	if err != nil {
		t.Fatalf("RunDoctorChecks failed: %v", err)
	}

	if len(report.Results) == 0 {
		t.Fatal("expected at least one diagnostic result")
	}

	total := report.OKCount + report.WarnCount + report.CritCount
	if total != len(report.Results) {
		t.Errorf("expected sum of counts (%d) to equal result count (%d)", total, len(report.Results))
	}

	for _, r := range report.Results {
		if r.ID == "" {
			t.Errorf("result missing ID: %+v", r)
		}
		if r.Name == "" {
			t.Errorf("result missing Name: %+v", r)
		}
		if r.Category == "" {
			t.Errorf("result missing Category: %+v", r)
		}
		if r.Severity != SeverityOK && r.Severity != SeverityWarning && r.Severity != SeverityCritical {
			t.Errorf("invalid severity %q in result: %+v", r.Severity, r)
		}
		if r.Summary == "" {
			t.Errorf("result missing Summary: %+v", r)
		}
	}
}

func TestDetectBinaryShadowing(t *testing.T) {
	// Normal run
	_, err := DetectBinaryShadowing()
	if err != nil {
		t.Fatalf("DetectBinaryShadowing returned unexpected error: %v", err)
	}

	// Verify checkBinaryShadowing generates a result
	report := &DoctorReport{}
	checkBinaryShadowing(report)
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result from checkBinaryShadowing, got %d", len(report.Results))
	}
	if report.Results[0].ID != "binary_path" {
		t.Errorf("expected ID 'binary_path', got %q", report.Results[0].ID)
	}
}

func TestDetectBinaryShadowingConflict(t *testing.T) {
	tmpDir := t.TempDir()
	binDir1 := filepath.Join(tmpDir, "bin1")
	binDir2 := filepath.Join(tmpDir, "bin2")
	_ = os.MkdirAll(binDir1, 0755)
	_ = os.MkdirAll(binDir2, 0755)

	sf1 := filepath.Join(binDir1, "sf")
	sf2 := filepath.Join(binDir2, "sf")
	_ = os.WriteFile(sf1, []byte("#!/bin/sh\necho sf1\n"), 0755)
	_ = os.WriteFile(sf2, []byte("#!/bin/sh\necho sf2\n"), 0755)

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", binDir1+":"+binDir2)

	conflict, err := DetectBinaryShadowing()
	if err != nil {
		t.Fatalf("DetectBinaryShadowing failed: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected conflict between bin1/sf and bin2/sf, got nil")
	}
	if conflict.ActivePath != sf1 {
		t.Errorf("expected active path %s, got %s", sf1, conflict.ActivePath)
	}
	if len(conflict.ShadowedPaths) == 0 || conflict.ShadowedPaths[0] != sf2 {
		t.Errorf("expected shadowed path %s, got %v", sf2, conflict.ShadowedPaths)
	}

	report := &DoctorReport{}
	checkBinaryShadowing(report)
	if len(report.Results) == 0 {
		t.Fatal("expected report result for conflict")
	}
	res := report.Results[0]
	if res.Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning for conflict, got %s", res.Severity)
	}
	if res.FixCommand == "" {
		t.Error("expected non-empty FixCommand for conflict")
	}
}
