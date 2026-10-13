package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanAICaches_MockDirs(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tempHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// Create mock Ollama blob
	ollamaBlobDir := filepath.Join(tempHome, ".ollama", "models", "blobs")
	_ = os.MkdirAll(ollamaBlobDir, 0755)
	_ = os.WriteFile(filepath.Join(ollamaBlobDir, "sha256-mockblob123"), make([]byte, 1024*1024), 0644) // 1MB

	// Create mock Hugging Face cache
	hfHubDir := filepath.Join(tempHome, ".cache", "huggingface", "hub", "models--meta-llama--Llama-3")
	_ = os.MkdirAll(hfHubDir, 0755)
	_ = os.WriteFile(filepath.Join(hfHubDir, "model.safetensors"), make([]byte, 2*1024*1024), 0644) // 2MB

	// Create mock PyTorch inductor cache
	torchDir := filepath.Join(tempHome, ".cache", "torch", "inductor")
	_ = os.MkdirAll(torchDir, 0755)
	_ = os.WriteFile(filepath.Join(torchDir, "kernel.so"), make([]byte, 512*1024), 0644) // 512KB

	// Create mock AI worktree
	worktreeDir := filepath.Join(tempHome, ".claude", "worktrees", "run-42")
	_ = os.MkdirAll(worktreeDir, 0755)
	_ = os.WriteFile(filepath.Join(worktreeDir, "scratch.py"), []byte("print('ai')"), 0644)

	targets, err := ScanAICaches()
	if err != nil {
		t.Fatalf("ScanAICaches failed: %v", err)
	}

	if len(targets) < 4 {
		t.Fatalf("expected at least 4 AI targets, got %d", len(targets))
	}

	foundOllama := false
	foundHF := false
	foundTorch := false
	foundWorktree := false

	for _, tgt := range targets {
		switch tgt.Provider {
		case ProviderOllama:
			foundOllama = true
			if !tgt.IsWeights {
				t.Errorf("expected Ollama blobs to be marked IsWeights=true")
			}
		case ProviderHuggingFace:
			foundHF = true
			if !tgt.IsWeights {
				t.Errorf("expected HF hub to be marked IsWeights=true")
			}
		case ProviderPyTorch:
			foundTorch = true
			if tgt.IsWeights {
				t.Errorf("expected PyTorch inductor to be marked IsWeights=false (rebuildable cache)")
			}
		case ProviderAIWorktree:
			foundWorktree = true
			if tgt.IsWeights {
				t.Errorf("expected AI worktrees to be marked IsWeights=false")
			}
		}
	}

	if !foundOllama {
		t.Errorf("missing Ollama target")
	}
	if !foundHF {
		t.Errorf("missing Hugging Face target")
	}
	if !foundTorch {
		t.Errorf("missing PyTorch target")
	}
	if !foundWorktree {
		t.Errorf("missing AI Worktree target")
	}

	// Test DryRun
	dryReport, err := PurgeAICaches(targets, true, nil)
	if err != nil {
		t.Fatalf("PurgeAICaches dry-run failed: %v", err)
	}
	if len(dryReport.Removed) != len(targets) {
		t.Errorf("expected %d items in dry-run report, got %d", len(targets), len(dryReport.Removed))
	}

	// Verify files still exist on disk after dry run
	if _, err := os.Stat(filepath.Join(ollamaBlobDir, "sha256-mockblob123")); err != nil {
		t.Errorf("dry-run deleted file: %v", err)
	}
}
