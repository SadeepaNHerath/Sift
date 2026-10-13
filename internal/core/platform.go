package core

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// GetHostname returns the computer name.
func GetHostname() string {
	if name, err := os.Hostname(); err == nil && name != "" {
		return name
	}
	if out, err := RunCommand("scutil", "--get", "ComputerName"); err == nil {
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			return trimmed
		}
	}
	return "localhost"
}

// GetMacOSVersion returns the macOS product version string (e.g., "14.4.1").
func GetMacOSVersion() string {
	if out, err := RunCommand("sw_vers", "-productVersion"); err == nil {
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			return trimmed
		}
	}
	return "unknown"
}

// GetDiskUsage returns total, used, and free disk space in bytes for the root volume ("/").
func GetDiskUsage() (total, used, free int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0, 0, err
	}

	bsize := int64(stat.Bsize)
	total = int64(stat.Blocks) * bsize
	free = int64(stat.Bavail) * bsize
	used = total - free
	if used < 0 {
		used = 0
	}
	return total, used, free, nil
}

// GetUsername returns the current username.
func GetUsername() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if curr, err := user.Current(); err == nil && curr.Username != "" {
		return curr.Username
	}
	if logname := os.Getenv("LOGNAME"); logname != "" {
		return logname
	}
	return "unknown"
}

// GetHomeDir returns the user's home directory.
func GetHomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	return ""
}

// IsFullDiskAccessGranted checks if the process has Full Disk Access permissions.
// On macOS, access to protected directories like ~/Library/Safari or ~/Library/Mail
// requires TCC Full Disk Access.
func IsFullDiskAccessGranted() bool {
	home := GetHomeDir()
	if home == "" {
		return false
	}

	testPaths := []string{
		filepath.Join(home, "Library", "Safari"),
		filepath.Join(home, "Library", "Mail"),
		"/Library/Application Support/com.apple.TCC/TCC.db",
	}

	for _, p := range testPaths {
		f, err := os.Open(p)
		if err == nil {
			// Successfully opened the protected resource
			f.Close()
			return true
		}

		// If the error is specifically permission-related (Operation not permitted / EPERM),
		// we know FDA is denied for this existing protected path.
		if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM) {
			return false
		}

		// Try directory reading if it's a directory
		if fi, statErr := os.Stat(p); statErr == nil && fi.IsDir() {
			dirFile, openErr := os.Open(p)
			if openErr == nil {
				_, readErr := dirFile.Readdirnames(1)
				dirFile.Close()
				if readErr == nil || errors.Is(readErr, io.EOF) {
					return true
				}
				if errors.Is(readErr, os.ErrPermission) || errors.Is(readErr, syscall.EPERM) {
					return false
				}
			}
		}
	}

	return false
}

// RunCommand executes a shell command and returns stdout as a string.
func RunCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// RunCommandWithTimeout executes a command with a maximum timeout and returns stdout.
func RunCommandWithTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// IsTTY checks if stdout is connected to a terminal.
func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// IsRunningAsSudo checks if the current process is running under sudo privileges.
func IsRunningAsSudo() bool {
	return os.Getenv("SUDO_USER") != "" || os.Geteuid() == 0
}
