package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
	"github.com/SadeepaNHerath/Spacefold/internal/engine"
	tuiDedup "github.com/SadeepaNHerath/Spacefold/internal/tui/dedup"
)

var (
	dedupMinSize    string
	dedupKeepOldest bool
	dedupKeepNewest bool
	dedupDryRun     bool
	dedupYes        bool
	dedupJSON       bool
)

var dedupCmd = &cobra.Command{
	Use:   "dedup [paths...]",
	Short: "Find and safely trash duplicate files using 3-phase sparse hashing",
	Long: `High-performance duplicate file finder using 3-phase verification:
Phase 1: Quick exact byte size bucketing.
Phase 2: 12KB sparse xxhash probing (first 4KB + mid 4KB + last 4KB).
Phase 3: Full xxhash collision verification with Darwin inode hardlink detection.
Provides an interactive Bubble Tea resolver to preview files with macOS QuickLook
and safely move copies to the Trash.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDedup(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runDedup,
}

func init() {
	dedupCmd.Flags().StringVarP(&dedupMinSize, "min-size", "m", "1MB", "Minimum file size threshold (e.g. 500KB, 1MB, 10MB)")
	dedupCmd.Flags().BoolVar(&dedupKeepOldest, "keep-oldest", false, "Automatically keep oldest file in each duplicate group and trash copies")
	dedupCmd.Flags().BoolVar(&dedupKeepNewest, "keep-newest", false, "Automatically keep newest file in each duplicate group and trash copies")
	dedupCmd.Flags().BoolVarP(&dedupDryRun, "dry-run", "n", false, "Preview duplicate groups without moving to Trash")
	dedupCmd.Flags().BoolVarP(&dedupYes, "yes", "y", false, "Execute duplicate trashing without interactive prompt")
	dedupCmd.Flags().BoolVar(&dedupJSON, "json", false, "Output duplicate groups in JSON format")
}

func runDedup(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	minBytes, err := parseByteSize(dedupMinSize)
	if err != nil {
		return fmt.Errorf("invalid --min-size value %q: %w", dedupMinSize, err)
	}

	opts := engine.DefaultDedupOptions()
	opts.MinSize = minBytes

	if !dedupJSON {
		fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("▸ Sift: Scanning directories for duplicate files..."))
	}

	groups, err := engine.FindDuplicates(args, opts)
	if err != nil {
		return fmt.Errorf("duplicate search failed: %w", err)
	}

	// 1. JSON output mode
	if dedupJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(groups)
	}

	if len(groups) == 0 {
		fmt.Fprintln(out, lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ No duplicate files found."))
		return nil
	}

	logger, _ := core.NewLogger()
	if logger != nil {
		defer logger.Close()
	}

	// 2. Dry run preview
	if dedupDryRun {
		return renderDedupDryRun(cmd, groups, dedupKeepNewest, logger)
	}

	// 3. Interactive TUI selection
	if !dedupYes && core.IsTTY() && !dedupKeepOldest && !dedupKeepNewest {
		m := tuiDedup.New(groups)
		p := tea.NewProgram(m)
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("interactive resolver failed: %w", err)
		}

		finalM, ok := finalModel.(tuiDedup.Model)
		if !ok || finalM.IsCancelled() || !finalM.IsConfirmed() {
			fmt.Fprintln(out, "Duplicate resolution cancelled.")
			return nil
		}

		selectedPaths := finalM.SelectedPaths()
		if len(selectedPaths) == 0 {
			fmt.Fprintln(out, "No files selected for deletion.")
			return nil
		}

		report, err := engine.TrashDuplicates(selectedPaths, false, logger)
		if err != nil {
			return fmt.Errorf("failed to trash duplicates: %w", err)
		}

		renderDedupExecutionSummary(cmd, report)
		return nil
	}

	// 4. Non-interactive guard
	if !dedupYes && !core.IsTTY() {
		return fmt.Errorf("dedup requires confirmation: pass --yes/-y along with --keep-oldest or --keep-newest to run non-interactively")
	}

	// 5. Automated resolution (keep-oldest or keep-newest)
	var pathsToTrash []string
	for _, g := range groups {
		if dedupKeepNewest {
			// Keep newest (last item), trash older copies
			for i := 0; i < len(g.Files)-1; i++ {
				pathsToTrash = append(pathsToTrash, g.Files[i].Path)
			}
		} else {
			// Keep oldest (first item), trash newer copies
			for i := 1; i < len(g.Files); i++ {
				pathsToTrash = append(pathsToTrash, g.Files[i].Path)
			}
		}
	}

	report, err := engine.TrashDuplicates(pathsToTrash, false, logger)
	if err != nil {
		return fmt.Errorf("failed to trash duplicates: %w", err)
	}

	renderDedupExecutionSummary(cmd, report)
	return nil
}

func renderDedupDryRun(cmd *cobra.Command, groups []engine.DuplicateGroup, keepNewest bool, logger *core.Logger) error {
	out := cmd.OutOrStdout()

	var totalReclaimable int64
	var duplicateFilesCount int

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	headStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	keptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	trashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, titleStyle.Render("▸ DEDUP DRY RUN PREVIEW — No files will be modified"))
	fmt.Fprintln(out, strings.Repeat("─", 72))

	for i, g := range groups {
		totalReclaimable += g.Reclaimable
		duplicateFilesCount += len(g.Files) - 1

		fmt.Fprintln(out, headStyle.Render(fmt.Sprintf("Set #%d (%s each • %d files • %s reclaimable)",
			i+1, core.FormatBytes(g.Size), len(g.Files), core.FormatBytes(g.Reclaimable))))

		for fIdx, f := range g.Files {
			isKept := (fIdx == 0 && !keepNewest) || (fIdx == len(g.Files)-1 && keepNewest)
			var action string
			if isKept {
				action = keptStyle.Render("[KEEP]")
			} else {
				action = trashStyle.Render("[TRASH]")
			}

			fmt.Fprintf(out, "  %s %s %s\n",
				action,
				f.Path,
				dimStyle.Render("("+f.ModTime.Format("2006-01-02 15:04")+")"),
			)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintln(out, strings.Repeat("─", 72))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(0, 1)

	summary := fmt.Sprintf("Duplicate Groups: %d  │  Files to Trash: %d  │  Reclaimable: %s",
		len(groups),
		duplicateFilesCount,
		sizeStyle.Render(core.FormatBytes(totalReclaimable)),
	)
	fmt.Fprintln(out, boxStyle.Render(summary))
	fmt.Fprintln(out)

	if logger != nil {
		logger.LogOperation("dedup", "dry-run", "success", summary)
	}

	return nil
}

func renderDedupExecutionSummary(cmd *cobra.Command, report *core.RemovalReport) {
	out := cmd.OutOrStdout()

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, headerStyle.Render(fmt.Sprintf("✔ Dedup completed! Reclaimed %s", sizeStyle.Render(core.FormatBytes(report.TotalBytes)))))
	fmt.Fprintf(out, "  Moved %d duplicate files to Trash\n", len(report.Removed))

	if len(report.Failed) > 0 {
		fmt.Fprintf(out, "  %s %d items could not be moved to trash:\n", errStyle.Render("Errors:"), len(report.Failed))
		for _, f := range report.Failed {
			fmt.Fprintf(out, "   • %s: %v\n", f.Path, f.Error)
		}
	}
	fmt.Fprintln(out)
}

func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 1024 * 1024, nil
	}

	multiplier := int64(1)
	cleanStr := s

	if strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		cleanStr = strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
	} else if strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		cleanStr = strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
	} else if strings.HasSuffix(s, "KB") || strings.HasSuffix(s, "K") {
		multiplier = 1024
		cleanStr = strings.TrimSuffix(strings.TrimSuffix(s, "KB"), "K")
	} else if strings.HasSuffix(s, "B") {
		cleanStr = strings.TrimSuffix(s, "B")
	}

	val, err := strconv.ParseFloat(cleanStr, 64)
	if err != nil {
		return 0, err
	}

	return int64(val * float64(multiplier)), nil
}
