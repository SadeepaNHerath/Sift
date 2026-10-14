package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
	"github.com/SadeepaNHerath/Sift/internal/tui/analyze"
)

var (
	analyzeJSON bool
	analyzeTop  int
)

func init() {
	analyzeCmd.Run = func(cmd *cobra.Command, args []string) {
		if err := runAnalyze(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	}
	analyzeCmd.RunE = runAnalyze

	analyzeCmd.Flags().BoolVar(&analyzeJSON, "json", false, "Output disk analysis as structured JSON")
	analyzeCmd.Flags().IntVarP(&analyzeTop, "top", "t", 0, "Show top N largest files")
}

// AnalyzeDirectoryJSON represents the JSON output for directory tree analysis.
type AnalyzeDirectoryJSON struct {
	Path      string             `json:"path"`
	TotalSize int64              `json:"total_size"`
	ItemCount int                `json:"item_count"`
	Items     []engine.FileEntry `json:"items"`
	Insights  []engine.FileEntry `json:"insights,omitempty"`
}

// AnalyzeTopJSON represents the JSON output for top largest files analysis.
type AnalyzeTopJSON struct {
	Path     string             `json:"path"`
	Count    int                `json:"count"`
	TopFiles []engine.FileEntry `json:"top_files"`
	Insights []engine.FileEntry `json:"insights,omitempty"`
}

// runAnalyze handles the sf analyze CLI entrypoint.
func runAnalyze(cmd *cobra.Command, args []string) error {
	targetPath := "."
	if len(args) > 0 && args[0] != "" {
		targetPath = args[0]
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		absPath = core.GetHomeDir()
		if absPath == "" {
			absPath = "/"
		}
	}

	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("target path does not exist: %w", err)
	}

	jsonFlag, _ := cmd.Flags().GetBool("json")
	topFlag, _ := cmd.Flags().GetInt("top")

	// If not connected to a TTY or --json is explicitly set: emit JSON output
	if !core.IsTTY() || jsonFlag {
		return runAnalyzeJSON(cmd, absPath, topFlag)
	}

	// Interactive Bubble Tea TUI
	var opts []analyze.Option
	if topFlag > 0 {
		opts = append(opts, analyze.WithTopCount(topFlag), analyze.WithTopViewInitial(true))
	}

	m := analyze.New(absPath, opts...)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// runAnalyzeJSON produces formatted JSON output for non-interactive / script usage.
func runAnalyzeJSON(cmd *cobra.Command, absPath string, topN int) error {
	insights, _ := engine.ScanInsights()

	if topN > 0 {
		topFiles, err := engine.ScanTopFiles(absPath, topN)
		if err != nil {
			return fmt.Errorf("failed to scan top files: %w", err)
		}

		out := AnalyzeTopJSON{
			Path:     absPath,
			Count:    len(topFiles),
			TopFiles: topFiles,
			Insights: insights,
		}

		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	root, err := engine.ScanDirProgressive(absPath, nil)
	if err != nil {
		return fmt.Errorf("failed to scan directory: %w", err)
	}

	out := AnalyzeDirectoryJSON{
		Path:      absPath,
		TotalSize: root.Size,
		ItemCount: root.ChildrenCount,
		Items:     root.Children,
		Insights:  insights,
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
