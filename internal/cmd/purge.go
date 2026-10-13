package cmd

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
	"github.com/SadeepaNHerath/Spacefold/internal/engine"
	"github.com/SadeepaNHerath/Spacefold/internal/tui/menu"
)

var (
	purgeDryRun bool
	purgeYes    bool
	purgePaths  []string
	purgeAll    bool
	purgeMinAge time.Duration
)

var purgeCmd = &cobra.Command{
	Use:   "purge [paths...]",
	Short: "Deep purge developer dependencies and build artifacts",
	Long: `Deep purge development caches, dependencies, and build artifacts including
node_modules, target, build, dist, .venv, Pods, DerivedData, and vendor directories
to safely reclaim significant disk space.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runPurge(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runPurge,
}

func init() {
	purgeCmd.Flags().BoolVarP(&purgeDryRun, "dry-run", "n", false, "Preview targets without deleting")
	purgeCmd.Flags().BoolVarP(&purgeYes, "yes", "y", false, "Execute purge without confirmation prompt")
	purgeCmd.Flags().StringSliceVar(&purgePaths, "paths", nil, "Custom project root paths to scan (comma-separated or repeated)")
	purgeCmd.Flags().BoolVarP(&purgeAll, "all", "a", false, "Purge all discovered artifacts regardless of age or git dirty status")
	purgeCmd.Flags().DurationVarP(&purgeMinAge, "min-age", "m", 7*24*time.Hour, "Minimum age threshold for purgeable artifacts (default 7d)")
}

func runPurge(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")
	all, _ := cmd.Flags().GetBool("all")
	minAge, _ := cmd.Flags().GetDuration("min-age")
	flagPaths, _ := cmd.Flags().GetStringSlice("paths")

	var searchRoots []string
	searchRoots = append(searchRoots, flagPaths...)
	searchRoots = append(searchRoots, args...)

	logger, err := core.NewLogger()
	if err != nil {
		logger = nil
	}

	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("▸ Sift: Scanning for project build artifacts and dependencies..."))

	targets, err := engine.ScanPurgeTargets(searchRoots, minAge, all)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if len(targets) == 0 {
		fmt.Fprintln(out, lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ No purgeable developer artifacts found."))
		return nil
	}

	// Handle dry-run mode
	if dryRun {
		return renderPurgeDryRun(cmd, targets, logger)
	}

	// Interactive TUI mode if attached to TTY and not confirmed via --yes
	if !yes && core.IsTTY() {
		selectedTargets, err := runInteractivePurgeSelect(targets, all)
		if err != nil {
			return err
		}
		if len(selectedTargets) == 0 {
			fmt.Fprintln(out, "Purge cancelled: No artifacts selected.")
			return nil
		}
		targets = selectedTargets
	} else if !yes && !core.IsTTY() {
		return fmt.Errorf("purge requires confirmation: pass --yes/-y to proceed non-interactively, or --dry-run to preview")
	} else if !all {
		// Non-interactive without --all: only purge safe targets
		var safeTargets []engine.PurgeTarget
		for _, t := range targets {
			if t.SafeToPurge {
				safeTargets = append(safeTargets, t)
			}
		}
		if len(safeTargets) == 0 {
			fmt.Fprintln(out, "No targets safe to purge. Use --all to purge recently modified or git-dirty artifacts.")
			return nil
		}
		targets = safeTargets
	}

	// Execute purge
	report, err := engine.PurgeTargets(targets, false, logger)
	if err != nil {
		return fmt.Errorf("purge failed: %w", err)
	}

	renderPurgeExecutionSummary(cmd, report)
	return nil
}

// runInteractivePurgeSelect opens the bubbletea multi-select menu.
func runInteractivePurgeSelect(targets []engine.PurgeTarget, selectAll bool) ([]engine.PurgeTarget, error) {
	menuItems := make([]menu.Item, len(targets))
	for i, t := range targets {
		var warnTag string
		if t.Warning != "" {
			warnTag = fmt.Sprintf(" [%s]", t.Warning)
		}

		menuItems[i] = menu.Item{
			Title:       fmt.Sprintf("%s (%s)", t.ProjectName, t.ArtifactType),
			Description: fmt.Sprintf("%s • %s%s", formatPurgeAge(time.Since(t.ModTime)), t.Path, warnTag),
			Size:        t.Size,
			Path:        t.Path,
			Selected:    selectAll || t.SafeToPurge,
			Protected:   !t.SafeToPurge && !selectAll,
		}
	}

	m := menu.New("Select Project Artifacts to Purge", menuItems,
		menu.WithMultiSelect(),
		menu.WithSizes(),
		menu.WithHeight(15),
	)

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("interactive menu failed: %w", err)
	}

	finalM, ok := finalModel.(menu.Model)
	if !ok || finalM.IsCancelled() || !finalM.IsConfirmed() {
		return nil, nil
	}

	selected := finalM.SelectedItems()
	selectedMap := make(map[string]bool)
	for _, itm := range selected {
		selectedMap[itm.Path] = true
	}

	var res []engine.PurgeTarget
	for _, t := range targets {
		if selectedMap[t.Path] {
			res = append(res, t)
		}
	}

	return res, nil
}

// renderPurgeDryRun renders detailed dry run information.
func renderPurgeDryRun(cmd *cobra.Command, targets []engine.PurgeTarget, logger *core.Logger) error {
	out := cmd.OutOrStdout()

	// Perform dry run engine reporting and logging
	report, _ := engine.PurgeTargets(targets, true, logger)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	projStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	typeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	safeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, titleStyle.Render("▸ DRY RUN PREVIEW — No files will be deleted"))
	fmt.Fprintln(out, strings.Repeat("─", 72))

	for _, t := range targets {
		var statusStr string
		if t.SafeToPurge {
			statusStr = safeStyle.Render("✔ Safe")
		} else {
			statusStr = warnStyle.Render(fmt.Sprintf("⚠ %s", t.Warning))
		}

		fmt.Fprintf(out, " %s %s  %s  %s\n",
			projStyle.Render(t.ProjectName),
			typeStyle.Render(fmt.Sprintf("[%s]", t.ArtifactType)),
			sizeStyle.Render(core.FormatBytes(t.Size)),
			statusStr,
		)
		fmt.Fprintf(out, "   %s (age: %s)\n",
			dimStyle.Render(t.Path),
			dimStyle.Render(formatPurgeAge(time.Since(t.ModTime))),
		)
	}

	fmt.Fprintln(out, strings.Repeat("─", 72))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(0, 1)

	summary := fmt.Sprintf("Targets: %d  │  Reclaimable: %s",
		len(report.Removed),
		sizeStyle.Render(core.FormatBytes(report.TotalBytes)),
	)
	fmt.Fprintln(out, boxStyle.Render(summary))
	fmt.Fprintln(out)

	return nil
}

// renderPurgeExecutionSummary prints the result of the purge operation.
func renderPurgeExecutionSummary(cmd *cobra.Command, report *core.RemovalReport) {
	out := cmd.OutOrStdout()

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, headerStyle.Render(fmt.Sprintf("✔ Purge completed! Reclaimed %s", sizeStyle.Render(core.FormatBytes(report.TotalBytes)))))
	fmt.Fprintf(out, "  Removed: %d artifact directories\n", len(report.Removed))

	if len(report.Failed) > 0 {
		fmt.Fprintf(out, "  %s %d items could not be removed:\n", errStyle.Render("Errors:"), len(report.Failed))
		for _, f := range report.Failed {
			fmt.Fprintf(out, "   • %s: %v\n", f.Path, f.Error)
		}
	}
	fmt.Fprintln(out)
}

// formatPurgeAge formats a duration into a human-friendly age string.
func formatPurgeAge(d time.Duration) string {
	if d <= 0 {
		return "just now"
	}
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd old", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh old", hours)
	}
	mins := int(d.Minutes())
	if mins > 0 {
		return fmt.Sprintf("%dm old", mins)
	}
	return fmt.Sprintf("%ds old", int(d.Seconds()))
}
