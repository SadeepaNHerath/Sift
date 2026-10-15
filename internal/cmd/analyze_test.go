package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func resetAnalyzeFlags() {
	_ = analyzeCmd.Flags().Set("json", "false")
	_ = analyzeCmd.Flags().Set("top", "0")
	_ = analyzeCmd.Flags().Set("system-data", "false")
}

func executeAnalyzeWithArgs(args []string) (string, error) {
	resetAnalyzeFlags()
	rootCmd.SetArgs(append([]string{"analyze"}, args...))
	return captureOutput(func() error {
		rootCmd.SetOut(os.Stdout)
		return rootCmd.Execute()
	})
}

func TestAnalyzeCmd_FlagsRegistered(t *testing.T) {
	flags := []string{"json", "top", "system-data"}
	for _, f := range flags {
		flag := analyzeCmd.Flags().Lookup(f)
		if flag == nil {
			t.Errorf("expected flag --%s to be registered on analyzeCmd", f)
		}
	}
}

func TestAnalyzeCmd_JSON_Directory(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "file1.txt")
	f2 := filepath.Join(tempDir, "file2.txt")
	_ = os.WriteFile(f1, []byte("hello"), 0644)
	_ = os.WriteFile(f2, []byte("world!"), 0644)

	outStr, err := executeAnalyzeWithArgs([]string{"--json", tempDir})
	if err != nil {
		t.Fatalf("unexpected error executing analyzeCmd: %v", err)
	}

	var out AnalyzeDirectoryJSON
	if err := json.Unmarshal([]byte(outStr), &out); err != nil {
		t.Fatalf("failed to parse JSON output: %v, raw output: %s", err, outStr)
	}

	if out.Path != tempDir {
		t.Errorf("expected path %s, got %s", tempDir, out.Path)
	}
	if out.TotalSize != 11 { // 5 + 6 = 11
		t.Errorf("expected total size 11, got %d", out.TotalSize)
	}
	if len(out.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(out.Items))
	}
}

func TestAnalyzeCmd_JSON_Top(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "small.bin")
	f2 := filepath.Join(tempDir, "medium.bin")
	f3 := filepath.Join(tempDir, "large.bin")
	_ = os.WriteFile(f1, make([]byte, 10), 0644)
	_ = os.WriteFile(f2, make([]byte, 20), 0644)
	_ = os.WriteFile(f3, make([]byte, 30), 0644)

	outStr, err := executeAnalyzeWithArgs([]string{"--json", "--top", "2", tempDir})
	if err != nil {
		t.Fatalf("unexpected error executing analyzeCmd with --top: %v", err)
	}

	var out AnalyzeTopJSON
	if err := json.Unmarshal([]byte(outStr), &out); err != nil {
		t.Fatalf("failed to parse JSON output: %v, raw output: %s", err, outStr)
	}

	if out.Count != 2 {
		t.Fatalf("expected 2 top files, got %d", out.Count)
	}
	if len(out.TopFiles) != 2 {
		t.Fatalf("expected 2 top files in slice, got %d", len(out.TopFiles))
	}
	if out.TopFiles[0].Size != 30 {
		t.Errorf("expected largest file size 30, got %d", out.TopFiles[0].Size)
	}
	if out.TopFiles[1].Size != 20 {
		t.Errorf("expected second file size 20, got %d", out.TopFiles[1].Size)
	}
}

func TestAnalyzeCmd_NonExistentPath(t *testing.T) {
	_, err := executeAnalyzeWithArgs([]string{"--json", "/nonexistent_sift_analyze_dir"})
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
}

func TestAnalyzeCmd_SystemData(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Mock xcode derived data
	derivedData := filepath.Join(tempHome, "Library", "Developer", "Xcode", "DerivedData")
	_ = os.MkdirAll(derivedData, 0755)
	_ = os.WriteFile(filepath.Join(derivedData, "test.log"), []byte("system data mock"), 0644)

	outStr, err := executeAnalyzeWithArgs([]string{"--system-data", "--json"})
	if err != nil {
		t.Fatalf("sf analyze --system-data --json failed: %v", err)
	}

	var report engine.SystemDataReport
	if err := json.Unmarshal([]byte(outStr), &report); err != nil {
		t.Fatalf("failed to parse system data JSON: %v, raw output: %s", err, outStr)
	}

	if report.TotalBytes == 0 {
		t.Errorf("expected non-zero TotalBytes in system-data report")
	}

	outStr, err = executeAnalyzeWithArgs([]string{"--system-data"})
	if err != nil {
		t.Fatalf("sf analyze --system-data failed: %v", err)
	}

	if !strings.Contains(outStr, "System Data Breakdown") {
		t.Errorf("expected header in output, got: %s", outStr)
	}
	if !strings.Contains(outStr, "Xcode DerivedData") {
		t.Errorf("expected Xcode DerivedData component, got: %s", outStr)
	}
}
