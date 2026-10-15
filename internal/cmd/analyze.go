package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
	"github.com/SadeepaNHerath/Sift/internal/tui/analyze"
)

var (
	analyzeJSON       bool
	analyzeTop        int
	analyzeSystemData bool
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
	analyzeCmd.Flags().BoolVarP(&analyzeSystemData, "system-data", "s", false, "Break down macOS System Data contributors (caches, runtimes, VM disks, Trash)")
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
	sysDataFlag, _ := cmd.Flags().GetBool("system-data")
	jsonFlag, _ := cmd.Flags().GetBool("json")

	if sysDataFlag {
		return runAnalyzeSystemData(cmd, jsonFlag)
	}

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

// runAnalyzeSystemData scans and renders the macOS System Data breakdown.
func runAnalyzeSystemData(cmd *cobra.Command, jsonMode bool) error {
	report, err := engine.ScanSystemData()
	if err != nil {
		return fmt.Errorf("system data scan failed: %w", err)
	}

	if jsonMode {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	out := cmd.OutOrStdout()

	headerBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#BD93F9")).
		Padding(0, 1).
		Bold(true).
		Foreground(lipgloss.Color("#BD93F9")).
		Render(fmt.Sprintf("📊 macOS System Data Breakdown: %s Identified", core.FormatBytes(report.TotalBytes)))

	fmt.Fprintln(out)
	fmt.Fprintln(out, headerBox)
	fmt.Fprintln(out)

	// Category summary
	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render("  Category Subtotals:"))
	catNames := make([]string, 0, len(report.Categories))
	for k := range report.Categories {
		catNames = append(catNames, k)
	}
	sort.Strings(catNames)
	for _, c := range catNames {
		sz := report.Categories[c]
		fmt.Fprintf(out, "  • %-26s %12s\n", c, core.FormatBytes(sz))
	}
	fmt.Fprintln(out)

	// Item details table
	headerLine := fmt.Sprintf("  %-28s %-20s %12s   %-22s", "Component", "Category", "Size", "Clean Remedy")
	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(headerLine))
	fmt.Fprintln(out, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  "+strings.Repeat("─", 78)))

	for _, itm := range report.Items {
		remedy := itm.CleanHint
		if remedy == "" {
			remedy = "-"
		}
		fmt.Fprintf(out, "  %-28s %-20s %12s   %s\n",
			itm.Name,
			itm.Category,
			lipgloss.NewStyle().Bold(true).Render(core.FormatBytes(itm.Size)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(remedy),
		)
	}
	fmt.Fprintln(out, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  "+strings.Repeat("─", 78)))

	// Check local snapshots
	if snaps, err := engine.ListLocalSnapshots(); err == nil && len(snaps) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintf(out, lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
			"  Tip: %d APFS local snapshot(s) detected. Run 'sf clean --snapshots' to thin retained storage.\n",
		), len(snaps))
	}

	fmt.Fprintln(out)
	return nil
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
