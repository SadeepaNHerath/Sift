package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckStatus_NotExists(t *testing.T) {
	orig := PamSudoLocalPath
	defer func() { PamSudoLocalPath = orig }()

	PamSudoLocalPath = filepath.Join(t.TempDir(), "nonexistent_sudo_local")

	enabled, isSupported, configPath, err := CheckStatus()
	if err != nil {
		t.Fatalf("unexpected error from CheckStatus: %v", err)
	}
	if enabled {
		t.Errorf("expected enabled=false for nonexistent config file")
	}
	if configPath != PamSudoLocalPath {
		t.Errorf("expected configPath %q, got %q", PamSudoLocalPath, configPath)
	}
	_ = isSupported
}

func TestEnable_CreatesFile(t *testing.T) {
	orig := PamSudoLocalPath
	defer func() { PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	PamSudoLocalPath = tempFile

	if err := Enable(); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}

	content, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}

	if !strings.Contains(string(content), PamTouchIDLine) {
		t.Errorf("expected content to contain %q, got: %s", PamTouchIDLine, string(content))
	}

	enabled, _, _, err := CheckStatus()
	if err != nil {
		t.Fatalf("CheckStatus() failed: %v", err)
	}
	if !enabled {
		t.Errorf("expected enabled=true after Enable()")
	}
}

func TestEnable_Idempotent(t *testing.T) {
	orig := PamSudoLocalPath
	defer func() { PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	PamSudoLocalPath = tempFile

	if err := Enable(); err != nil {
		t.Fatalf("first Enable() failed: %v", err)
	}
	if err := Enable(); err != nil {
		t.Fatalf("second Enable() failed: %v", err)
	}

	content, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	count := strings.Count(string(content), "pam_tid.so")
	if count != 1 {
		t.Errorf("expected exactly 1 pam_tid.so line, got %d in: %s", count, string(content))
	}
}

func TestDisable_CommentsLine(t *testing.T) {
	orig := PamSudoLocalPath
	defer func() { PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	PamSudoLocalPath = tempFile

	// Enable first
	if err := Enable(); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}

	// Disable
	if err := Disable(); err != nil {
		t.Fatalf("Disable() failed: %v", err)
	}

	content, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if !strings.Contains(string(content), "#"+PamTouchIDLine) {
		t.Errorf("expected content to contain commented line, got: %s", string(content))
	}

	enabled, _, _, err := CheckStatus()
	if err != nil {
		t.Fatalf("CheckStatus() failed: %v", err)
	}
	if enabled {
		t.Errorf("expected enabled=false after Disable()")
	}
}

func TestDisable_Idempotent(t *testing.T) {
	orig := PamSudoLocalPath
	defer func() { PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	PamSudoLocalPath = tempFile

	if err := Enable(); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}
	if err := Disable(); err != nil {
		t.Fatalf("first Disable() failed: %v", err)
	}
	if err := Disable(); err != nil {
		t.Fatalf("second Disable() failed: %v", err)
	}

	enabled, _, _, err := CheckStatus()
	if err != nil {
		t.Fatalf("CheckStatus() failed: %v", err)
	}
	if enabled {
		t.Errorf("expected enabled=false")
	}
}

func TestEnable_UncommentsExistingCommentedLine(t *testing.T) {
	orig := PamSudoLocalPath
	defer func() { PamSudoLocalPath = orig }()

	tempFile := filepath.Join(t.TempDir(), "sudo_local")
	PamSudoLocalPath = tempFile

	// Create initial template with commented line
	initialContent := "# sudo_local: template\n#auth       sufficient     pam_tid.so\n"
	if err := os.WriteFile(tempFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial template: %v", err)
	}

	// Enable should uncomment it
	if err := Enable(); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}

	content, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if strings.Contains(string(content), "#auth") {
		t.Errorf("expected commented line to be uncommented, but found #auth in: %s", string(content))
	}
	if !strings.Contains(string(content), PamTouchIDLine) {
		t.Errorf("expected uncommented line %q, got: %s", PamTouchIDLine, string(content))
	}

	enabled, _, _, err := CheckStatus()
	if err != nil {
		t.Fatalf("CheckStatus() failed: %v", err)
	}
	if !enabled {
		t.Errorf("expected enabled=true")
	}
}

func TestDisable_FileNotExists(t *testing.T) {
	orig := PamSudoLocalPath
	defer func() { PamSudoLocalPath = orig }()

	PamSudoLocalPath = filepath.Join(t.TempDir(), "nonexistent_sudo_local")

	if err := Disable(); err != nil {
		t.Fatalf("Disable() on nonexistent file returned error: %v", err)
	}
}

func TestIsTouchIDSupported(t *testing.T) {
	// Should run cleanly without panics
	_ = IsTouchIDSupported()
}

func TestEnable_CreatesBackupOfExistingFile(t *testing.T) {
	orig := PamSudoLocalPath
	defer func() { PamSudoLocalPath = orig }()

	dir := t.TempDir()
	tempFile := filepath.Join(dir, "sudo_local")
	PamSudoLocalPath = tempFile

	initialContent := "# initial config\nauth required pam_permit.so\n"
	if err := os.WriteFile(tempFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	if err := Enable(); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}

	bakFile := tempFile + ".bak"
	bakContent, err := os.ReadFile(bakFile)
	if err != nil {
		t.Fatalf("expected backup file %s to exist: %v", bakFile, err)
	}
	if string(bakContent) != initialContent {
		t.Errorf("backup content mismatch: expected %q, got %q", initialContent, string(bakContent))
	}
}

