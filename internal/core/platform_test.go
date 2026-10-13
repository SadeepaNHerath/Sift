package core

import (
	"testing"
	"time"
)

func TestPlatformUtilities(t *testing.T) {
	hostname := GetHostname()
	if hostname == "" {
		t.Fatalf("GetHostname returned empty string")
	}

	version := GetMacOSVersion()
	if version == "" {
		t.Fatalf("GetMacOSVersion returned empty string")
	}

	total, used, free, err := GetDiskUsage()
	if err != nil {
		t.Fatalf("GetDiskUsage error: %v", err)
	}
	if total <= 0 || free <= 0 {
		t.Fatalf("GetDiskUsage returned invalid values: total=%d, free=%d, used=%d", total, free, used)
	}

	user := GetUsername()
	if user == "" {
		t.Fatalf("GetUsername returned empty string")
	}

	home := GetHomeDir()
	if home == "" {
		t.Fatalf("GetHomeDir returned empty string")
	}

	out, err := RunCommand("echo", "hello")
	if err != nil || out != "hello\n" {
		t.Fatalf("RunCommand failed: out=%q, err=%v", out, err)
	}

	outTimeout, err := RunCommandWithTimeout(2*time.Second, "echo", "timeout-test")
	if err != nil || outTimeout != "timeout-test\n" {
		t.Fatalf("RunCommandWithTimeout failed: out=%q, err=%v", outTimeout, err)
	}

	_ = IsFullDiskAccessGranted()
	_ = IsTTY()
	_ = IsRunningAsSudo()
}
