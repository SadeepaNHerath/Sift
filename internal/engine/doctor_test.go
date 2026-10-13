package engine

import (
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
