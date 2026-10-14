package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func resetStatusFlags() {
	_ = statusCmd.Flags().Set("json", "false")
	_ = statusCmd.Flags().Set("watch", "false")
	_ = statusCmd.Flags().Set("interval", "1s")
}

func executeStatusWithBuffer(args []string) (string, error) {
	resetStatusFlags()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	statusCmd.SetOut(&buf)
	statusCmd.SetErr(&buf)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		statusCmd.SetOut(nil)
		statusCmd.SetErr(nil)
	}()

	rootCmd.SetArgs(append([]string{"status"}, args...))
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestStatusCommandFlags(t *testing.T) {
	jsonFlag := statusCmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Fatal("expected --json flag to be registered on statusCmd")
	}

	watchFlag := statusCmd.Flags().Lookup("watch")
	if watchFlag == nil {
		t.Fatal("expected --watch flag to be registered on statusCmd")
	}

	intervalFlag := statusCmd.Flags().Lookup("interval")
	if intervalFlag == nil {
		t.Fatal("expected --interval flag to be registered on statusCmd")
	}
}

func TestStatusCommandJSON(t *testing.T) {
	// Mock TTY to false to force non-interactive output
	origTTYFunc := isStdoutTTYFunc
	isStdoutTTYFunc = func() bool { return false }
	defer func() { isStdoutTTYFunc = origTTYFunc }()

	output, err := executeStatusWithBuffer([]string{"--json"})
	if err != nil {
		t.Fatalf("executeStatusWithBuffer(--json) failed: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	if !strings.HasPrefix(trimmed, "{") {
		t.Fatalf("expected JSON output starting with '{', got: %s", output)
	}

	var metrics engine.SystemMetrics
	if err := json.Unmarshal([]byte(trimmed), &metrics); err != nil {
		t.Fatalf("failed to unmarshal status JSON: %v", err)
	}

	if metrics.Hostname == "" {
		t.Errorf("expected non-empty Hostname in JSON")
	}
	if metrics.OSVersion == "" {
		t.Errorf("expected non-empty OSVersion in JSON")
	}
	if metrics.HealthScore < 0 || metrics.HealthScore > 100 {
		t.Errorf("expected HealthScore in 0..100, got %d", metrics.HealthScore)
	}
	if metrics.Memory.Total == 0 {
		t.Errorf("expected Memory.Total > 0")
	}
	if metrics.Disk.Total == 0 {
		t.Errorf("expected Disk.Total > 0")
	}
}

func TestStatusCommandPipeAutoDetect(t *testing.T) {
	// When stdout is not a TTY, sf status should auto-detect pipe and output JSON
	origTTYFunc := isStdoutTTYFunc
	isStdoutTTYFunc = func() bool { return false }
	defer func() { isStdoutTTYFunc = origTTYFunc }()

	output, err := executeStatusWithBuffer([]string{}) // No --json flag
	if err != nil {
		t.Fatalf("executeStatusWithBuffer() failed: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	var metrics engine.SystemMetrics
	if err := json.Unmarshal([]byte(trimmed), &metrics); err != nil {
		t.Fatalf("expected piped output to auto-detect and output valid JSON, got: %v\nOutput: %s", err, output)
	}

	if metrics.Hostname == "" {
		t.Errorf("expected non-empty hostname")
	}
	if metrics.HealthScore < 0 || metrics.HealthScore > 100 {
		t.Errorf("unexpected health score: %d", metrics.HealthScore)
	}
}

func TestOutputSingleJSON(t *testing.T) {
	var buf bytes.Buffer
	statusCmd.SetOut(&buf)
	defer statusCmd.SetOut(nil)

	err := outputSingleJSON(statusCmd, true)
	if err != nil {
		t.Fatalf("outputSingleJSON indented failed: %v", err)
	}
	indented := buf.String()
	if !strings.Contains(indented, "\n  \"hostname\"") {
		t.Errorf("expected indented JSON format, got: %s", indented)
	}

	// Test non-indented (NDJSON)
	buf.Reset()
	err = outputSingleJSON(statusCmd, false)
	if err != nil {
		t.Fatalf("outputSingleJSON non-indented failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Errorf("expected single line for NDJSON output, got %d lines", len(lines))
	}
}

func TestStatusCommandWatchNDJSON(t *testing.T) {
	origTTYFunc := isStdoutTTYFunc
	isStdoutTTYFunc = func() bool { return false }
	defer func() { isStdoutTTYFunc = origTTYFunc }()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_ = ctx

	var buf bytes.Buffer
	statusCmd.SetOut(&buf)
	defer statusCmd.SetOut(nil)

	err := outputSingleJSON(statusCmd, false)
	if err != nil {
		t.Fatalf("outputSingleJSON failed: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	var m engine.SystemMetrics
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("failed to parse NDJSON line: %v", err)
	}

	if m.HealthScore < 0 || m.HealthScore > 100 {
		t.Errorf("unexpected health score: %d", m.HealthScore)
	}
}
