package core

import (
	"os"
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

func TestIsRunningAsSudo_NotSpoofable(t *testing.T) {
	// An unprivileged process setting SUDO_USER must not trick IsRunningAsSudo()
	t.Setenv("SUDO_USER", "root")
	t.Setenv("USER", "root")

	// In test environment running as regular non-root user
	if !IsRunningAsSudo() {
		// As expected, fake SUDO_USER did not grant sudo status
		return
	}
	// If the test runner actually is root (e.g. CI container as root), verify Geteuid
	if os.Geteuid() != 0 {
		t.Fatalf("IsRunningAsSudo() returned true for non-root EUID %d when SUDO_USER=root", os.Geteuid())
	}
}
