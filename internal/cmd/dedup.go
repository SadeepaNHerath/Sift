package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
	tuiDedup "github.com/SadeepaNHerath/Sift/internal/tui/dedup"
)

var (
	dedupMinSize    string
	dedupKeepOldest bool
	dedupKeepNewest bool
	dedupDryRun     bool
	dedupYes        bool
	dedupJSON       bool
	dedupClone      bool
	dedupHardlink   bool
)

var dedupCmd = &cobra.Command{
	Use:   "dedup [paths...]",
	Short: "Find and safely trash or APFS-clone duplicate files using 3-phase sparse hashing",
	Long: `High-performance duplicate file finder using 3-phase verification:
Phase 1: Quick exact byte size bucketing.
Phase 2: 12KB sparse xxhash probing (first 4KB + mid 4KB + last 4KB).
Phase 3: Full xxhash collision verification with Darwin inode hardlink detection.
Provides an interactive Bubble Tea resolver to preview files with macOS QuickLook
and safely move copies to the Trash, or replace them with APFS Copy-on-Write (CoW) clones.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDedup(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runDedup,
}

func init() {
	dedupCmd.Flags().StringVarP(&dedupMinSize, "min-size", "m", "1MB", "Minimum file size threshold (e.g. 500KB, 1MB, 10MB)")
	dedupCmd.Flags().BoolVar(&dedupKeepOldest, "keep-oldest", false, "Automatically keep oldest file in each duplicate group")
	dedupCmd.Flags().BoolVar(&dedupKeepNewest, "keep-newest", false, "Automatically keep newest file in each duplicate group")
	dedupCmd.Flags().BoolVarP(&dedupDryRun, "dry-run", "n", false, "Preview duplicate groups without modifying files")
	dedupCmd.Flags().BoolVarP(&dedupYes, "yes", "y", false, "Execute duplicate resolution without interactive prompt")
	dedupCmd.Flags().BoolVar(&dedupJSON, "json", false, "Output duplicate groups in JSON format")
	dedupCmd.Flags().BoolVarP(&dedupClone, "clone", "c", false, "Replace duplicates with APFS Copy-on-Write (CoW) clones (frees space without deleting files)")
	dedupCmd.Flags().BoolVarP(&dedupHardlink, "hardlink", "l", false, "Replace duplicates with filesystem hardlinks")
}

func runDedup(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	if dedupClone && dedupHardlink {
		return fmt.Errorf("cannot specify both --clone and --hardlink")
	}

	action := engine.ActionTrash
	if dedupClone {
		action = engine.ActionClone
	} else if dedupHardlink {
		action = engine.ActionHardlink
	}

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
		return renderDedupDryRun(cmd, groups, dedupKeepNewest, action, logger)
	}

	// 3. Interactive TUI selection (Trash mode)
	if !dedupYes && core.IsTTY() && !dedupKeepOldest && !dedupKeepNewest && action == engine.ActionTrash {
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

	// 5. APFS Clone or Hardlink Resolution
	if action == engine.ActionClone {
		result, err := engine.CloneDuplicates(groups, dedupKeepNewest, false, logger)
		if err != nil {
			return fmt.Errorf("failed to clone duplicates: %w", err)
		}
		renderDedupActionResult(cmd, result)
		return nil
	}

	if action == engine.ActionHardlink {
		result, err := engine.HardlinkDuplicates(groups, dedupKeepNewest, false, logger)
		if err != nil {
			return fmt.Errorf("failed to hardlink duplicates: %w", err)
		}
		renderDedupActionResult(cmd, result)
		return nil
	}

	// 6. Automated Trash resolution (keep-oldest or keep-newest)
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

func renderDedupDryRun(cmd *cobra.Command, groups []engine.DuplicateGroup, keepNewest bool, action engine.DedupAction, logger *core.Logger) error {
	out := cmd.OutOrStdout()

	var totalReclaimable int64
	var duplicateFilesCount int

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	headStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	keptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	var targetStyle lipgloss.Style
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	var targetTag string
	switch action {
	case engine.ActionClone:
		targetTag = "[CLONE]"
		targetStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	case engine.ActionHardlink:
		targetTag = "[LINK]"
		targetStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	default:
		targetTag = "[TRASH]"
		targetStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	}

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
			var actionLabel string
			if isKept {
				actionLabel = keptStyle.Render("[KEEP]")
			} else {
				actionLabel = targetStyle.Render(targetTag)
			}

			fmt.Fprintf(out, "  %s %s %s\n",
				actionLabel,
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

	var actionName string
	switch action {
	case engine.ActionClone:
		actionName = "Files to Clone (CoW)"
	case engine.ActionHardlink:
		actionName = "Files to Hardlink"
	default:
		actionName = "Files to Trash"
	}

	summary := fmt.Sprintf("Duplicate Groups: %d  │  %s: %d  │  Reclaimable: %s",
		len(groups),
		actionName,
		duplicateFilesCount,
		sizeStyle.Render(core.FormatBytes(totalReclaimable)),
	)
	fmt.Fprintln(out, boxStyle.Render(summary))

	switch action {
	case engine.ActionClone:
		fmt.Fprintln(out, infoStyle.Render("ℹ  APFS Copy-on-Write clones will share disk blocks instantly with zero risk of breaking dependent applications."))
	case engine.ActionHardlink:
		fmt.Fprintln(out, infoStyle.Render("ℹ  Filesystem hardlinks will point duplicates to the exact same inode, freeing disk space."))
	}
	fmt.Fprintln(out)

	if logger != nil {
		logger.LogOperation("dedup", fmt.Sprintf("dry-run-%s", action), "success", summary)
	}

	return nil
}

func renderDedupActionResult(cmd *cobra.Command, result *engine.DedupActionResult) {
	out := cmd.OutOrStdout()

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	var actionWord string
	switch result.Action {
	case engine.ActionClone:
		actionWord = "APFS Copy-on-Write Deduplication"
	case engine.ActionHardlink:
		actionWord = "Hardlink Deduplication"
	default:
		actionWord = "Deduplication"
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, headerStyle.Render(fmt.Sprintf("✔ %s completed! Reclaimed %s", actionWord, sizeStyle.Render(core.FormatBytes(result.ReclaimedBytes)))))
	fmt.Fprintf(out, "  Deduplicated %d duplicate files (zero file loss)\n", result.ProcessedFiles)

	if len(result.Failed) > 0 {
		fmt.Fprintf(out, "  %s %d items could not be processed:\n", errStyle.Render("Errors:"), len(result.Failed))
		for _, f := range result.Failed {
			fmt.Fprintf(out, "   • %s: %v\n", f.Path, f.Error)
		}
	} else if result.Action == engine.ActionClone {
		fmt.Fprintln(out, dimStyle.Render("  Storage blocks are now shared via APFS Copy-on-Write. Both files remain readable and independently mutable."))
	}
	fmt.Fprintln(out)
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
		return 1000 * 1000, nil
	}

	multiplier := int64(1)
	cleanStr := s

	if strings.HasSuffix(s, "GIB") {
		multiplier = 1024 * 1024 * 1024
		cleanStr = strings.TrimSuffix(s, "GIB")
	} else if strings.HasSuffix(s, "MIB") {
		multiplier = 1024 * 1024
		cleanStr = strings.TrimSuffix(s, "MIB")
	} else if strings.HasSuffix(s, "KIB") {
		multiplier = 1024
		cleanStr = strings.TrimSuffix(s, "KIB")
	} else if strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G") {
		multiplier = 1000 * 1000 * 1000
		cleanStr = strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
	} else if strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M") {
		multiplier = 1000 * 1000
		cleanStr = strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
	} else if strings.HasSuffix(s, "KB") || strings.HasSuffix(s, "K") {
		multiplier = 1000
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
