package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
	"github.com/SadeepaNHerath/Sift/internal/tui/menu"
)

var (
	cleanDryRun      bool
	cleanAll         bool
	cleanYes         bool
	cleanWhitelist   bool
	cleanExternal    string
	cleanModels      bool
	cleanAI          bool
	cleanDockerPrune bool
	cleanTrash       bool
	cleanSnapshots   bool
)

func init() {
	cleanCmd.Run = func(cmd *cobra.Command, args []string) {
		_ = runClean(cmd, args)
	}
	cleanCmd.RunE = runClean
	cleanCmd.Flags().BoolVarP(&cleanDryRun, "dry-run", "n", false, "Preview items to be cleaned without deleting")
	cleanCmd.Flags().BoolVarP(&cleanAll, "all", "a", false, "Clean all categories without prompting")
	cleanCmd.Flags().BoolVarP(&cleanYes, "yes", "y", false, "Execute cleanup without confirmation prompt")
	cleanCmd.Flags().BoolVarP(&cleanWhitelist, "whitelist", "w", false, "Show active whitelist configuration")
	cleanCmd.Flags().StringVarP(&cleanExternal, "external", "e", "", "Clean external volume junk files")
	cleanCmd.Flags().BoolVar(&cleanModels, "models", false, "Clean local AI model weights and compiler caches")
	cleanCmd.Flags().BoolVar(&cleanAI, "ai", false, "Alias for --models")
	cleanCmd.Flags().BoolVar(&cleanDockerPrune, "docker-prune", false, "Prune stopped Docker containers and dangling images during virtualization cleanup")
	cleanCmd.Flags().BoolVarP(&cleanTrash, "trash", "t", false, "Empty ~/.Trash folder permanently")
	cleanCmd.Flags().BoolVarP(&cleanSnapshots, "snapshots", "s", false, "Thin APFS Time Machine local snapshots to release purged disk blocks")
}

// CategoryCleanResult tracks cleanup results per category for the summary screen.
type CategoryCleanResult struct {
	ID        string
	Name      string
	Icon      string
	ItemCount int
	Bytes     int64
	Failed    int
	Status    string
}

// runClean handles the sf clean CLI entrypoint.
func runClean(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	all, _ := cmd.Flags().GetBool("all")
	yes, _ := cmd.Flags().GetBool("yes")
	showWl, _ := cmd.Flags().GetBool("whitelist")
	extVol, _ := cmd.Flags().GetString("external")

	// 1. Handle --whitelist flag
	if showWl {
		return handleWhitelistDisplay()
	}

	// 2. Load user whitelist
	wl, err := core.LoadWhitelist()
	if err != nil {
		wl = nil
	}

	// 3. Handle --external volume cleanup
	if extVol != "" {
		return handleExternalVolumeClean(extVol, dryRun, yes, wl)
	}

	// 4. Handle standalone --trash or --snapshots flags
	trashFlag, _ := cmd.Flags().GetBool("trash")
	snapshotsFlag, _ := cmd.Flags().GetBool("snapshots")

	if trashFlag && len(args) == 0 && !all {
		return handleTrashClean(dryRun, yes)
	}

	if snapshotsFlag && len(args) == 0 && !all {
		return handleSnapshotsClean(dryRun, yes)
	}

	// 5. Standard macOS Deep Cleaner
	modelsFlag, _ := cmd.Flags().GetBool("models")
	aiFlag, _ := cmd.Flags().GetBool("ai")
	if modelsFlag || aiFlag {
		args = append(args, engine.CatAIModels)
	}

	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("▸ Sift: Scanning system and application caches..."))

	categories, err := engine.ScanAll(wl)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Filter categories if specific args were passed (e.g. sf clean dev-tools browsers or --models)
	if len(args) > 0 {
		categories = filterCategoriesByArgs(categories, args)
		if len(categories) == 0 {
			return fmt.Errorf("no matching categories found for: %s", strings.Join(args, ", "))
		}
	}

	// 5. Interactive TUI multi-select mode if attached to TTY and not confirmed via --yes
	if !yes && core.IsTTY() && len(args) == 0 {
		selectedCategories, err := runInteractiveCategorySelect(categories, all)
		if err != nil {
			return err
		}
		if len(selectedCategories) == 0 {
			fmt.Println("Cleanup cancelled: No categories selected.")
			return nil
		}
		categories = selectedCategories
	} else if !all && len(args) == 0 {
		// Non-interactive default: filter by EnabledByDefault
		var defaults []*engine.CleanCategory
		for _, c := range categories {
			if c.EnabledByDefault {
				defaults = append(defaults, c)
			}
		}
		categories = defaults
	}

	// 6. Execute cleanup or dry run
	logger, logErr := core.NewLogger()
	if logErr == nil {
		defer logger.Close()
	}

	if dryRun {
		return executeDryRunSummary(categories, logger)
	}

	return executeCleanSummary(categories, logger, trashFlag, cleanDockerPrune)
}

// handleWhitelistDisplay prints current whitelist configuration.
func handleWhitelistDisplay() error {
	wl, err := core.LoadWhitelist()
	if err != nil {
		return fmt.Errorf("failed to load whitelist: %w", err)
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).MarginBottom(1)
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))

	fmt.Println(titleStyle.Render("🛡️  Sift Whitelist Configuration"))
	paths := wl.List()
	if len(paths) == 0 {
		fmt.Println("  (No custom whitelisted paths configured)")
		fmt.Println(dimStyle.Render(fmt.Sprintf("  Config path: %s/whitelist.json", core.ConfigDir())))
	} else {
		fmt.Println("  Protected paths (exempt from sf clean):")
		for _, p := range paths {
			fmt.Printf("  %s %s\n", itemStyle.Render("✓"), p)
		}
	}
	return nil
}

// handleExternalVolumeClean scans and cleans macOS junk on external volumes.
func handleExternalVolumeClean(volPath string, dryRun, yes bool, wl *core.Whitelist) error {
	normVol, err := core.NormalizePath(volPath)
	if err != nil {
		return fmt.Errorf("invalid external volume path: %w", err)
	}

	if normVol == "/" || core.IsSystemPath(normVol) {
		return fmt.Errorf("refusing to target system root or protected path as external volume: %s", normVol)
	}

	// External volumes must reside under /Volumes/ (allow temporary test directories)
	isTestDir := strings.HasPrefix(normVol, os.TempDir()) || strings.HasPrefix(normVol, "/private/var/folders")
	if !strings.HasPrefix(normVol, "/Volumes/") && !isTestDir {
		return fmt.Errorf("invalid external volume path %q: external volumes must reside under /Volumes/", volPath)
	}

	// Check if target resolves to system root or protected path (e.g., /Volumes/Macintosh HD -> /)
	if target, err := filepath.EvalSymlinks(normVol); err == nil {
		if target == "/" || core.IsSystemPath(target) {
			return fmt.Errorf("invalid external volume path %q: targets system root or protected path (%s)", volPath, target)
		}
	}

	cat, err := engine.ScanExternalVolume(normVol, wl)
	if err != nil {
		return err
	}

	fmt.Printf("Found %d junk items on external volume (%s)\n", len(cat.Items), core.FormatBytes(cat.TotalSize))

	if dryRun {
		return executeDryRunSummary([]*engine.CleanCategory{cat}, nil)
	}

	if !yes && core.IsTTY() {
		fmt.Printf("Clean %d junk files (%s) from %s? [y/N]: ", len(cat.Items), core.FormatBytes(cat.TotalSize), normVol)
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp != "y" && resp != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	logger, _ := core.NewLogger()
	if logger != nil {
		defer logger.Close()
	}

	return executeCleanSummary([]*engine.CleanCategory{cat}, logger, false)
}

// handleTrashClean handles standalone emptying of ~/.Trash.
func handleTrashClean(dryRun, yes bool) error {
	count, totalBytes, err := core.GetTrashSize()
	if err != nil {
		return fmt.Errorf("failed to inspect Trash: %w", err)
	}
	if count == 0 {
		fmt.Println("Trash is already empty.")
		return nil
	}

	if dryRun {
		fmt.Printf("[dry-run] User Trash contains %d item(s) (%s)\n", count, core.FormatBytes(totalBytes))
		return nil
	}

	if !yes && core.IsTTY() {
		fmt.Printf("Permanently delete %d item(s) (%s) from Trash? [y/N]: ", count, core.FormatBytes(totalBytes))
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp != "y" && resp != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	removed, freed, err := core.EmptyTrash()
	if err != nil {
		return fmt.Errorf("error emptying trash: %w", err)
	}
	fmt.Printf("✔ Emptied Trash: %d item(s) removed, %s reclaimed\n", removed, core.FormatBytes(freed))
	return nil
}

// handleSnapshotsClean thins APFS local snapshots on root filesystem.
func handleSnapshotsClean(dryRun, yes bool) error {
	snaps, err := engine.ListLocalSnapshots()
	if err != nil {
		return fmt.Errorf("failed to inspect local snapshots: %w", err)
	}
	if len(snaps) == 0 {
		fmt.Println("No local APFS Time Machine snapshots found.")
		return nil
	}

	if dryRun {
		fmt.Printf("[dry-run] Found %d APFS local snapshot(s):\n", len(snaps))
		for _, s := range snaps {
			fmt.Printf("  • %s (%s)\n", s.Name, s.DateStr)
		}
		return nil
	}

	if !yes && core.IsTTY() {
		fmt.Printf("Thin %d APFS local snapshot(s) to release retained space? [y/N]: ", len(snaps))
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp != "y" && resp != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	out, err := engine.ThinLocalSnapshots(0, 4)
	if err != nil {
		return err
	}
	if out != "" {
		fmt.Println(out)
	}
	fmt.Printf("✔ APFS local snapshot thinning complete.\n")
	return nil
}

// filterCategoriesByArgs matches category IDs or names against user arguments.
func filterCategoriesByArgs(categories []*engine.CleanCategory, args []string) []*engine.CleanCategory {
	argMap := make(map[string]bool)
	for _, a := range args {
		argMap[strings.ToLower(strings.TrimSpace(a))] = true
	}

	var filtered []*engine.CleanCategory
	for _, c := range categories {
		if argMap[strings.ToLower(c.ID)] || argMap[strings.ToLower(c.Name)] {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// runInteractiveCategorySelect presents the multi-select TUI menu.
func runInteractiveCategorySelect(categories []*engine.CleanCategory, selectAll bool) ([]*engine.CleanCategory, error) {
	menuItems := make([]menu.Item, len(categories))
	for i, cat := range categories {
		meta, _ := engine.GetCategoryMeta(cat.ID)
		menuItems[i] = menu.Item{
			Title:       fmt.Sprintf("%s %s", meta.Icon, cat.Name),
			Description: cat.Description,
			Size:        cat.TotalSize,
			Path:        cat.ID,
			Selected:    selectAll || cat.EnabledByDefault,
		}
	}

	m := menu.New("Select Categories to Clean", menuItems,
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
	for _, s := range selected {
		selectedMap[s.Path] = true
	}

	var res []*engine.CleanCategory
	for _, c := range categories {
		if selectedMap[c.ID] {
			res = append(res, c)
		}
	}

	return res, nil
}

// executeDryRunSummary renders the preview breakdown.
func executeDryRunSummary(categories []*engine.CleanCategory, logger *core.Logger) error {
	headerBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#BD93F9")).
		Padding(0, 1).
		Bold(true).
		Foreground(lipgloss.Color("#BD93F9")).
		Render("🧹 Sift Cache Cleaner: Dry Run Preview (No files will be modified)")

	fmt.Println()
	fmt.Println(headerBox)
	fmt.Println()

	var totalReclaimable int64
	var totalItems int

	headerLine := fmt.Sprintf("  %-30s %-12s %15s", "Category", "Items", "Reclaimable")
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(headerLine))
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("─", 60)))

	for _, cat := range categories {
		rep, err := engine.CleanCategoryExecute(cat, true, logger)
		if err != nil {
			continue
		}

		meta, _ := engine.GetCategoryMeta(cat.ID)
		itemsStr := fmt.Sprintf("%d items", len(rep.Removed))
		sizeStr := core.FormatBytes(rep.TotalBytes)

		catLabel := fmt.Sprintf("%s %s", meta.Icon, cat.Name)
		checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✓")

		line := fmt.Sprintf("  %s %-28s %-12s %15s", checkMark, catLabel, itemsStr, sizeStr)
		fmt.Println(line)

		totalReclaimable += rep.TotalBytes
		totalItems += len(rep.Removed)
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("─", 60)))

	totalLine := fmt.Sprintf("  %-30s %-12s %15s",
		lipgloss.NewStyle().Bold(true).Render("Total Reclaimable Space:"),
		fmt.Sprintf("%d items", totalItems),
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render(core.FormatBytes(totalReclaimable)),
	)
	fmt.Println(totalLine)
	fmt.Println()
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("  (Run 'sf clean --yes' to perform actual cleanup)"))
	fmt.Println()

	return nil
}

// executeCleanSummary cleans the categories and renders a summary screen.
func executeCleanSummary(categories []*engine.CleanCategory, logger *core.Logger, cleanTrash bool, dockerPrune ...bool) error {
	headerBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#50FA7B")).
		Padding(0, 1).
		Bold(true).
		Foreground(lipgloss.Color("#50FA7B")).
		Render("🧹 Sift Cache Cleaner: Reclaimed Space Summary")

	fmt.Println()
	fmt.Println(headerBox)
	fmt.Println()

	headerLine := fmt.Sprintf("  %-30s %-12s %15s", "Category", "Status", "Reclaimed")
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(headerLine))
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("─", 60)))

	var totalReclaimed int64
	var totalRemovedCount int
	var totalFailedCount int

	for _, cat := range categories {
		meta, _ := engine.GetCategoryMeta(cat.ID)

		// Print visual spinner / active category marker
		rep, err := engine.CleanCategoryExecute(cat, false, logger, dockerPrune...)
		if err != nil {
			failMark := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("✗")
			line := fmt.Sprintf("  %s %-28s %-12s %15s", failMark, fmt.Sprintf("%s %s", meta.Icon, cat.Name), "Error", "0 B")
			fmt.Println(line)
			continue
		}

		checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✓")
		statusStr := fmt.Sprintf("Done (%d)", len(rep.Removed))
		sizeStr := core.FormatBytes(rep.TotalBytes)
		catLabel := fmt.Sprintf("%s %s", meta.Icon, cat.Name)

		line := fmt.Sprintf("  %s %-28s %-12s %15s", checkMark, catLabel, statusStr, sizeStr)
		fmt.Println(line)

		totalReclaimed += rep.TotalBytes
		totalRemovedCount += len(rep.Removed)
		totalFailedCount += len(rep.Failed)
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("─", 60)))

	totalLine := fmt.Sprintf("  %-30s %-12s %15s",
		lipgloss.NewStyle().Bold(true).Render("Total Space Reclaimed:"),
		fmt.Sprintf("%d files", totalRemovedCount),
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render(core.FormatBytes(totalReclaimed)),
	)
	fmt.Println(totalLine)

	if cleanTrash {
		removed, freed, tErr := core.EmptyTrash()
		if tErr == nil && removed > 0 {
			fmt.Println()
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
				fmt.Sprintf("  ✔ Emptied Trash: %d item(s) permanently removed (%s reclaimed)", removed, core.FormatBytes(freed)),
			))
		}
	} else if _, trashBytes, err := core.GetTrashSize(); err == nil && trashBytes > 0 {
		fmt.Println()
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
			fmt.Sprintf("  Tip: Items moved to Trash (%s). Run 'sf clean --trash' to reclaim disk blocks immediately.", core.FormatBytes(trashBytes)),
		))
	}

	if snaps, err := engine.ListLocalSnapshots(); err == nil && len(snaps) > 0 {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
			fmt.Sprintf("  Tip: %d APFS local snapshot(s) detected. Run 'sf clean --snapshots' to thin retained storage.", len(snaps)),
		))
	}

	if totalFailedCount > 0 {
		fmt.Println()
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
			fmt.Sprintf("  Note: %d items protected by safety policy or whitelist were skipped.", totalFailedCount),
		))
	}

	fmt.Println()
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
		fmt.Sprintf("  Audit log written to %s", core.LogDir()),
	))
	fmt.Println()

	return nil
}
