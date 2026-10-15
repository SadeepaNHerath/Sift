package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
	"github.com/SadeepaNHerath/Sift/internal/tui/menu"
)

var (
	optimizeDryRun    bool
	optimizeAll       bool
	optimizeYes       bool
	optimizeWhitelist bool
)

var optimizeCmd = &cobra.Command{
	Use:   "optimize [task...]",
	Short: "Run macOS system and storage optimization tasks",
	Long: `Run macOS system tune-ups including DNS cache flushes, QuickLook thumbnail resets,
icon cache rebuilding, SQLite database vacuuming, launch services audits, and .DS_Store prevention.`,
	RunE: runOptimize,
}

func init() {
	optimizeCmd.Run = func(cmd *cobra.Command, args []string) {
		_ = runOptimize(cmd, args)
	}
	optimizeCmd.Flags().BoolVarP(&optimizeDryRun, "dry-run", "n", false, "Preview optimization tasks without applying changes")
	optimizeCmd.Flags().BoolVarP(&optimizeAll, "all", "a", false, "Run all optimization tasks without prompting")
	optimizeCmd.Flags().BoolVarP(&optimizeYes, "yes", "y", false, "Execute optimizations without interactive prompt")
	optimizeCmd.Flags().BoolVarP(&optimizeWhitelist, "whitelist", "w", false, "Show active whitelist configuration")
}

// runOptimize handles the sf optimize CLI entrypoint.
func runOptimize(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	all, _ := cmd.Flags().GetBool("all")
	yes, _ := cmd.Flags().GetBool("yes")
	showWl, _ := cmd.Flags().GetBool("whitelist")

	// 1. Handle --whitelist flag
	if showWl {
		return handleWhitelistDisplay()
	}

	tasks := engine.TaskCatalog()

	// 2. Filter tasks if specific arguments were passed (e.g. sf optimize dns sqlite)
	if len(args) > 0 {
		var filtered []engine.OptimizeTask
		argMap := make(map[string]bool)
		for _, a := range args {
			argMap[strings.ToLower(strings.TrimSpace(a))] = true
		}

		for _, t := range tasks {
			if argMap[strings.ToLower(t.ID)] || argMap[strings.ToLower(t.Name)] {
				filtered = append(filtered, t)
			}
		}

		if len(filtered) == 0 {
			return fmt.Errorf("no matching optimization tasks found for: %s", strings.Join(args, ", "))
		}
		tasks = filtered
	}

	// 3. Interactive TUI multi-select mode if attached to TTY and not confirmed via --yes
	if !yes && core.IsTTY() && len(args) == 0 {
		selectedTasks, err := runInteractiveTaskSelect(tasks, all)
		if err != nil {
			return err
		}
		if len(selectedTasks) == 0 {
			fmt.Println("Optimization cancelled: No tasks selected.")
			return nil
		}
		tasks = selectedTasks
	} else if !yes && !core.IsTTY() && len(args) == 0 && !all {
		// Non-interactive default without --yes: preview tasks safely
		dryRun = true
	}

	// 4. Initialize forensic logger
	logger, logErr := core.NewLogger()
	if logErr == nil {
		defer logger.Close()
	}

	// 5. Execute dry-run preview or live tasks
	if dryRun {
		return executeOptimizeDryRun(tasks, logger)
	}

	return executeOptimizeLive(tasks, logger)
}

// runInteractiveTaskSelect presents a multi-select Bubble Tea menu for optimization tasks.
func runInteractiveTaskSelect(tasks []engine.OptimizeTask, selectAll bool) ([]engine.OptimizeTask, error) {
	menuItems := make([]menu.Item, len(tasks))
	for i, t := range tasks {
		menuItems[i] = menu.Item{
			Title:       fmt.Sprintf("⚡ %s", t.Name),
			Description: t.Description,
			Path:        t.ID,
			Selected:    selectAll || t.Safe,
		}
	}

	m := menu.New("Select Optimization Tasks to Run", menuItems,
		menu.WithMultiSelect(),
		menu.WithHeight(12),
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
	for _, s := range selected {
		selectedMap[s.Path] = true
	}

	var res []engine.OptimizeTask
	for _, t := range tasks {
		if selectedMap[t.ID] {
			res = append(res, t)
		}
	}

	return res, nil
}

// executeOptimizeDryRun displays what each selected task would do.
func executeOptimizeDryRun(tasks []engine.OptimizeTask, logger *core.Logger) error {
	headerBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#BD93F9")).
		Padding(0, 1).
		Bold(true).
		Foreground(lipgloss.Color("#BD93F9")).
		Render("⚡ Sift System Optimizer: Dry Run Preview (No modifications applied)")

	fmt.Println()
	fmt.Println(headerBox)
	fmt.Println()

	headerLine := fmt.Sprintf("  %-25s %-50s", "Task", "Planned Action")
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(headerLine))
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("─", 78)))

	for _, task := range tasks {
		outcome, _ := engine.RunTask(task.ID, true, logger)

		checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("•")
		taskLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render(task.Name)
		detailsLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(outcome.Details)

		line := fmt.Sprintf("  %s %-23s %s", checkMark, taskLabel, detailsLabel)
		fmt.Println(line)
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("─", 78)))
	fmt.Println()
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("  (Run 'sf optimize --yes' to apply all optimizations)"))
	fmt.Println()

	return nil
}

// executeOptimizeLive executes the optimization tasks and renders live progress.
func executeOptimizeLive(tasks []engine.OptimizeTask, logger *core.Logger) error {
	headerBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#50FA7B")).
		Padding(0, 1).
		Bold(true).
		Foreground(lipgloss.Color("#50FA7B")).
		Render("⚡ Sift System Optimizer: Applying Optimizations")

	fmt.Println()
	fmt.Println(headerBox)
	fmt.Println()

	headerLine := fmt.Sprintf("  %-25s %-12s %-40s", "Task", "Status", "Details")
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(headerLine))
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("─", 78)))

	appliedCount := 0
	failedCount := 0

	for _, task := range tasks {
		outcome, err := engine.RunTask(task.ID, false, logger)
		if err != nil || outcome.Err != nil {
			failMark := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("✗")
			statusStr := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("Failed")
			errDesc := ""
			if err != nil {
				errDesc = err.Error()
			} else if outcome.Err != nil {
				errDesc = outcome.Err.Error()
			}
			line := fmt.Sprintf("  %s %-23s %-12s %s", failMark, task.Name, statusStr, errDesc)
			fmt.Println(line)
			failedCount++
			continue
		}

		checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✓")
		statusStr := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("Done")
		line := fmt.Sprintf("  %s %-23s %-12s %s", checkMark, task.Name, statusStr, outcome.Details)
		fmt.Println(line)
		appliedCount++
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("─", 78)))
	summary := fmt.Sprintf("  Optimization Complete: %d tasks applied, %d failed", appliedCount, failedCount)
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(summary))

	fmt.Println()
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
		fmt.Sprintf("  Audit log written to %s", core.LogDir()),
	))
	fmt.Println()

	return nil
}
