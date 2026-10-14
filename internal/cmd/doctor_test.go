package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func TestDoctorCmd_FlagsRegistered(t *testing.T) {
	flag := doctorCmd.Flags().Lookup("json")
	if flag == nil {
		t.Fatal("expected --json flag on doctorCmd")
	}
}

func TestDoctorCmd_JSON(t *testing.T) {
	resetDoctorFlags()
	out, err := captureOutput(func() error {
		_ = doctorCmd.Flags().Set("json", "true")
		defer resetDoctorFlags()
		return doctorCmd.RunE(doctorCmd, nil)
	})
	if err != nil {
		t.Fatalf("doctorCmd execution failed: %v", err)
	}

	var report engine.DoctorReport
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &report); err != nil {
		t.Fatalf("failed to parse doctor JSON output: %v\nOutput: %s", err, out)
	}

	if len(report.Results) == 0 {
		t.Fatal("expected results in doctor report")
	}
}

func TestDoctorCmd_Text(t *testing.T) {
	resetDoctorFlags()
	out, err := captureOutput(func() error {
		_ = doctorCmd.Flags().Set("json", "false")
		defer resetDoctorFlags()
		return doctorCmd.RunE(doctorCmd, nil)
	})
	if err != nil {
		t.Fatalf("doctorCmd execution failed: %v", err)
	}

	if !strings.Contains(out, "Sift System Doctor") {
		t.Errorf("expected header in doctor output, got: %s", out)
	}
}
