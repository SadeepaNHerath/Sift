package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// AIProvider identifies the AI/LLM tool or framework.
type AIProvider string

const (
	ProviderOllama      AIProvider = "Ollama"
	ProviderHuggingFace AIProvider = "Hugging Face"
	ProviderLMStudio    AIProvider = "LM Studio"
	ProviderJan         AIProvider = "Jan"
	ProviderLocalAI     AIProvider = "LocalAI"
	ProviderPyTorch     AIProvider = "PyTorch"
	ProviderVLLM        AIProvider = "vLLM"
	ProviderTriton      AIProvider = "Triton"
	ProviderAIWorktree  AIProvider = "AI Worktree"
)

// AICacheTarget represents an AI model weight repository, compilation cache, or worktree.
type AICacheTarget struct {
	Name       string     `json:"name"`
	Provider   AIProvider `json:"provider"`
	Path       string     `json:"path"`
	Size       int64      `json:"size"`
	ItemCount  int        `json:"item_count"`
	ModTime    time.Time  `json:"mod_time"`
	IsWeights  bool       `json:"is_weights"` // true = model weights/blobs; false = rebuildable compiler/kernel cache
	Description string    `json:"description"`
}

// ScanAICaches audits the filesystem for local AI models, weights, compiler caches, and AI agent worktrees.
func ScanAICaches() ([]AICacheTarget, error) {
	home := core.GetHomeDir()
	var targets []AICacheTarget

	type candidate struct {
		provider    AIProvider
		path        string
		name        string
		isWeights   bool
		description string
		subItemScan bool
	}

	candidates := []candidate{
		// 1. Ollama
		{
			provider:    ProviderOllama,
			path:        filepath.Join(home, ".ollama", "models", "blobs"),
			name:        "Ollama Model Blobs",
			isWeights:   true,
			description: "Local GGUF/Safetensors model weight blobs downloaded via Ollama",
			subItemScan: false,
		},
		{
			provider:    ProviderOllama,
			path:        filepath.Join(home, ".ollama", "models", "manifests"),
			name:        "Ollama Manifests",
			isWeights:   true,
			description: "Ollama model version manifests and tags",
			subItemScan: false,
		},
		// 2. Hugging Face Hub
		{
			provider:    ProviderHuggingFace,
			path:        filepath.Join(home, ".cache", "huggingface", "hub"),
			name:        "Hugging Face Model Cache",
			isWeights:   true,
			description: "Cached model weights and tokenizer files from Hugging Face Hub",
			subItemScan: true, // We can inspect individual repo folders
		},
		{
			provider:    ProviderHuggingFace,
			path:        filepath.Join(home, ".cache", "huggingface", "transformers"),
			name:        "Hugging Face Transformers Cache",
			isWeights:   false,
			description: "PyTorch/Transformers compilation and model index cache",
			subItemScan: false,
		},
		// 3. LM Studio
		{
			provider:    ProviderLMStudio,
			path:        filepath.Join(home, ".cache", "lm-studio", "models"),
			name:        "LM Studio Models",
			isWeights:   true,
			description: "Downloaded GGUF models managed by LM Studio",
			subItemScan: true,
		},
		{
			provider:    ProviderLMStudio,
			path:        filepath.Join(home, ".lmstudio", "models"),
			name:        "LM Studio Models (Legacy)",
			isWeights:   true,
			description: "Legacy LM Studio models storage directory",
			subItemScan: true,
		},
		// 4. Jan
		{
			provider:    ProviderJan,
			path:        filepath.Join(home, "jan", "models"),
			name:        "Jan Local Models",
			isWeights:   true,
			description: "Local models downloaded via Jan AI assistant",
			subItemScan: true,
		},
		// 5. LocalAI
		{
			provider:    ProviderLocalAI,
			path:        filepath.Join(home, ".localai", "models"),
			name:        "LocalAI Model Storage",
			isWeights:   true,
			description: "Downloaded backend models for LocalAI",
			subItemScan: false,
		},
		// 6. PyTorch & Inductor kernel caches
		{
			provider:    ProviderPyTorch,
			path:        filepath.Join(home, ".cache", "torch", "kernels"),
			name:        "PyTorch Compiled Kernels",
			isWeights:   false,
			description: "Cached PyTorch JIT and inductor kernel binaries",
			subItemScan: false,
		},
		{
			provider:    ProviderPyTorch,
			path:        filepath.Join(home, ".cache", "torch", "inductor"),
			name:        "PyTorch Inductor Cache",
			isWeights:   false,
			description: "Intermediate AOTAutograd and Triton compiler artifacts",
			subItemScan: false,
		},
		// 7. vLLM
		{
			provider:    ProviderVLLM,
			path:        filepath.Join(home, ".cache", "vllm"),
			name:        "vLLM Compilation Cache",
			isWeights:   false,
			description: "Cached compiled attention kernels and engine caches",
			subItemScan: false,
		},
		// 8. Triton
		{
			provider:    ProviderTriton,
			path:        filepath.Join(home, ".triton", "cache"),
			name:        "Triton GPU Kernel Cache",
			isWeights:   false,
			description: "Compiled Triton autotuned GPU kernel binaries",
			subItemScan: false,
		},
		{
			provider:    ProviderTriton,
			path:        filepath.Join(home, ".cache", "triton"),
			name:        "Triton Cache (~/.cache)",
			isWeights:   false,
			description: "Cached Triton compilation artifacts",
			subItemScan: false,
		},
		// 9. AI Worktrees
		{
			provider:    ProviderAIWorktree,
			path:        filepath.Join(home, ".codex", "worktrees"),
			name:        "Codex Agent Worktrees",
			isWeights:   false,
			description: "Ephemeral git worktrees created during automated coding runs",
			subItemScan: true,
		},
		{
			provider:    ProviderAIWorktree,
			path:        filepath.Join(home, ".claude", "worktrees"),
			name:        "Claude Agent Worktrees",
			isWeights:   false,
			description: "Worktrees and scratch directories generated by Claude agent runs",
			subItemScan: true,
		},
		{
			provider:    ProviderAIWorktree,
			path:        filepath.Join(home, ".cursor", "worktrees"),
			name:        "Cursor Agent Worktrees",
			isWeights:   false,
			description: "Detached agent workspace clones from Cursor Composer",
			subItemScan: true,
		},
	}

	for _, cand := range candidates {
		info, err := os.Stat(cand.path)
		if err != nil || !info.IsDir() {
			continue
		}

		if cand.subItemScan {
			// Inspect individual subfolders (e.g. models--meta-llama--Llama-3, specific worktrees)
			entries, err := os.ReadDir(cand.path)
			if err == nil && len(entries) > 0 {
				subItemFound := false
				for _, e := range entries {
					if strings.HasPrefix(e.Name(), ".") {
						continue
					}
					subPath := filepath.Join(cand.path, e.Name())
					subInfo, subErr := os.Stat(subPath)
					if subErr != nil {
						continue
					}

					subSize, _ := core.FileSize(subPath)
					if subSize > 0 {
						cleanName := strings.TrimPrefix(e.Name(), "models--")
						cleanName = strings.ReplaceAll(cleanName, "--", "/")
						targets = append(targets, AICacheTarget{
							Name:        fmt.Sprintf("%s (%s)", cleanName, cand.provider),
							Provider:    cand.provider,
							Path:        subPath,
							Size:        subSize,
							ItemCount:   countFiles(subPath),
							ModTime:     subInfo.ModTime(),
							IsWeights:   cand.isWeights,
							Description: cand.description,
						})
						subItemFound = true
					}
				}
				if subItemFound {
					continue
				}
			}
		}

		// Fallback or whole-directory aggregation
		sz, _ := core.FileSize(cand.path)
		if sz > 0 {
			targets = append(targets, AICacheTarget{
				Name:        cand.name,
				Provider:    cand.provider,
				Path:        cand.path,
				Size:        sz,
				ItemCount:   countFiles(cand.path),
				ModTime:     info.ModTime(),
				IsWeights:   cand.isWeights,
				Description: cand.description,
			})
		}
	}

	return targets, nil
}

// PurgeAICaches moves selected AI model targets or compilation caches to Trash safely.
func PurgeAICaches(targets []AICacheTarget, dryRun bool, logger *core.Logger) (*core.RemovalReport, error) {
	var paths []string
	for _, t := range targets {
		paths = append(paths, t.Path)
	}

	if dryRun {
		dry := core.DryRunRemove(paths)
		report := &core.RemovalReport{TotalBytes: dry.TotalBytes}
		for _, item := range dry.WouldRemove {
			report.Removed = append(report.Removed, core.RemovedItem{
				Path: item.Path,
				Size: item.Size,
			})
		}
		if logger != nil {
			logger.LogOperation("purge_ai", "dry-run", "success", fmt.Sprintf("Would reclaim %s across %d targets", core.FormatBytes(dry.TotalBytes), len(dry.WouldRemove)))
		}
		return report, nil
	}

	report := core.SafeRemoveAll(paths)
	if logger != nil {
		logger.LogOperation("purge_ai", "execute", "success", fmt.Sprintf("Reclaimed %s across %d targets", core.FormatBytes(report.TotalBytes), len(report.Removed)))
		for _, item := range report.Removed {
			logger.LogDeletion(item.Path, item.Size, "trash", "purge_ai")
		}
	}
	return report, nil
}

func countFiles(path string) int {
	count := 0
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}
