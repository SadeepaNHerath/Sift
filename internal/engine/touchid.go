package engine

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// PamSudoLocalPath points to the modern macOS PAM sudo_local configuration file.
// On macOS Sonoma (14.0+), Sequoia (15.0+), and newer, /etc/pam.d/sudo_local
// survives macOS system updates and is automatically included by /etc/pam.d/sudo.
var PamSudoLocalPath = "/etc/pam.d/sudo_local"

// PamTouchIDLine is the required PAM directive to enable Touch ID for sudo.
const PamTouchIDLine = "auth       sufficient     pam_tid.so"

// pamFileWriter writes PAM configuration content, allowing tests to mock writes without sudo.
var pamFileWriter = defaultPamFileWriter

func defaultPamFileWriter(path string, content string) error {
	tmpPath := path + ".sftmp"
	bakPath := path + ".bak"

	// Attempt direct file write first (works in test environments or when running as root)
	err := os.WriteFile(tmpPath, []byte(content), 0644)
	if err == nil {
		// If original file exists, create backup
		if _, statErr := os.Stat(path); statErr == nil {
			_ = copyPamFile(path, bakPath)
		}
		// Atomic rename
		if renErr := os.Rename(tmpPath, path); renErr == nil {
			return nil
		}
		_ = os.Remove(tmpPath)
	}

	// Fall back to elevated write using sudo tee to temporary file first
	cmd := exec.Command("sudo", "tee", tmpPath)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = io.Discard
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if runErr := cmd.Run(); runErr != nil {
		_ = exec.Command("sudo", "rm", "-f", tmpPath).Run()
		return fmt.Errorf("failed to write temporary PAM file %s with sudo: %w (stderr: %s)", tmpPath, runErr, strings.TrimSpace(errBuf.String()))
	}

	// Ensure standard 0644 permissions on the temporary PAM file
	_ = exec.Command("sudo", "chmod", "644", tmpPath).Run()

	// Backup existing file if present
	if _, statErr := os.Stat(path); statErr == nil {
		_ = exec.Command("sudo", "cp", "-p", path, bakPath).Run()
	}

	// Atomically move the validated temporary file into place
	mvCmd := exec.Command("sudo", "mv", tmpPath, path)
	if mvErr := mvCmd.Run(); mvErr != nil {
		_ = exec.Command("sudo", "rm", "-f", tmpPath).Run()
		return fmt.Errorf("failed to atomically install %s with sudo: %w", path, mvErr)
	}

	return nil
}

func copyPamFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// IsTouchIDSupported checks if the host hardware supports Touch ID biometric authentication.
func IsTouchIDSupported() bool {
	if runtime.GOOS != "darwin" {
		return false
	}

	// 1. Check bioutil user Touch ID configuration
	out, err := exec.Command("bioutil", "-r").CombinedOutput()
	if err == nil && (strings.Contains(string(out), "Biometrics") || strings.Contains(string(out), "Touch ID")) {
		return true
	}

	// 2. Check IORegistry for AppleBiometricSensor
	outIO, errIO := exec.Command("ioreg", "-r", "-c", "AppleBiometricSensor").CombinedOutput()
	if errIO == nil && strings.Contains(string(outIO), "AppleBiometricSensor") {
		return true
	}

	// 3. Check presence of /etc/pam.d/sudo_local.template (Sonoma/Sequoia support indicator)
	if _, err := os.Stat("/etc/pam.d/sudo_local.template"); err == nil {
		return true
	}

	return false
}

// CheckStatus verifies whether Touch ID for sudo is supported, enabled, and returns the target config path.
func CheckStatus() (enabled bool, isSupported bool, configPath string, err error) {
	configPath = PamSudoLocalPath
	isSupported = IsTouchIDSupported()

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return false, isSupported, configPath, nil
		}
		return false, isSupported, configPath, readErr
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "pam_tid.so") && strings.Contains(line, "auth") {
			return true, isSupported, configPath, nil
		}
	}

	return false, isSupported, configPath, scanner.Err()
}

// Enable enables Touch ID authentication for sudo by updating or creating /etc/pam.d/sudo_local.
func Enable() error {
	configPath := PamSudoLocalPath

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new sudo_local file with modern header and pam_tid.so
			newContent := fmt.Sprintf("# sudo_local: local config file which survives system updates\n%s\n", PamTouchIDLine)
			return pamFileWriter(configPath, newContent)
		}
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	lines := strings.Split(string(data), "\n")
	alreadyEnabled := false
	uncommented := false
	var updatedLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, "pam_tid.so") && strings.Contains(trimmed, "auth") {
			alreadyEnabled = true
			updatedLines = append(updatedLines, line)
		} else if strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, "pam_tid.so") && strings.Contains(trimmed, "auth") {
			// Uncomment the commented pam_tid.so directive
			updatedLines = append(updatedLines, PamTouchIDLine)
			uncommented = true
		} else {
			updatedLines = append(updatedLines, line)
		}
	}

	if alreadyEnabled {
		return nil
	}

	if !uncommented {
		// Insert pam_tid.so after leading comments
		var result []string
		inserted := false
		for _, l := range updatedLines {
			if !inserted && !strings.HasPrefix(strings.TrimSpace(l), "#") {
				result = append(result, PamTouchIDLine)
				inserted = true
			}
			result = append(result, l)
		}
		if !inserted {
			result = append(result, PamTouchIDLine)
		}
		updatedLines = result
	}

	finalContent := strings.Join(updatedLines, "\n")
	if !strings.HasSuffix(finalContent, "\n") {
		finalContent += "\n"
	}

	return pamFileWriter(configPath, finalContent)
}

// Disable disables Touch ID authentication for sudo by commenting out pam_tid.so in /etc/pam.d/sudo_local.
func Disable() error {
	configPath := PamSudoLocalPath

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to disable if config does not exist
			return nil
		}
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	lines := strings.Split(string(data), "\n")
	modified := false
	var updatedLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, "pam_tid.so") && strings.Contains(trimmed, "auth") {
			// Comment out active pam_tid.so line
			updatedLines = append(updatedLines, "#"+PamTouchIDLine)
			modified = true
		} else {
			updatedLines = append(updatedLines, line)
		}
	}

	if !modified {
		return nil
	}

	finalContent := strings.Join(updatedLines, "\n")
	if !strings.HasSuffix(finalContent, "\n") {
		finalContent += "\n"
	}

	return pamFileWriter(configPath, finalContent)
}
