package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
	"github.com/SadeepaNHerath/Spacefold/internal/engine"
	"github.com/SadeepaNHerath/Spacefold/internal/tui/menu"
)

var (
	flagDryRun bool
	flagAll    bool
	flagYes    bool
	flagData   bool
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [app-name]",
	Short: "Thoroughly uninstall macOS applications and leftovers",
	Long: `Scan for installed applications, locate all associated support files,
preferences, caches, containers, and launch daemons, and safely remove them.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runUninstall(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview resources to remove without deleting anything")
	uninstallCmd.Flags().BoolVar(&flagAll, "all", false, "Remove all attributed resources (including Medium and Low confidence)")
	uninstallCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "Skip confirmation prompt")
	uninstallCmd.Flags().BoolVar(&flagData, "data", false, "Also remove personal data (default protects user documents)")
}

func runUninstall(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	// 1. Resolve target application
	var targetApp *engine.AppInfo
	var err error

	if len(args) > 0 {
		searchTerm := strings.Join(args, " ")
		targetApp, err = resolveAppByNameOrPath(searchTerm)
		if err != nil {
			return err
		}
	} else {
		// Non-interactive check
		if !core.IsTTY() {
			return errors.New("application name is required in non-interactive mode. Usage: sf uninstall [app-name]")
		}

		targetApp, err = selectAppInteractively()
		if err != nil {
			return err
		}
		if targetApp == nil {
			fmt.Fprintln(out, "Uninstallation cancelled.")
			return nil
		}
	}

	// 2. Protect macOS system components
	if targetApp.IsApple || targetApp.Protected || core.IsProtectedBundleID(targetApp.BundleID) {
		return fmt.Errorf("cannot uninstall protected macOS application %q (%s)", targetApp.Name, targetApp.BundleID)
	}

	fmt.Fprintf(out, "\nUninstalling: %s\n", targetApp.Name)
	if targetApp.Version != "" {
		fmt.Fprintf(out, "Version:      %s\n", targetApp.Version)
	}
	if targetApp.BundleID != "" {
		fmt.Fprintf(out, "Bundle ID:    %s\n", targetApp.BundleID)
	}
	fmt.Fprintf(out, "Location:     %s\n\n", targetApp.Path)

	// 3. Inspect user configuration files (.zshrc, .bashrc, .config)
	configRefs := checkConfigReferences(*targetApp)
	if len(configRefs) > 0 {
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFB86C"))
		fmt.Fprintln(out, warnStyle.Render("⚠️  Warning: Detected references in shell configuration files:"))
		for _, ref := range configRefs {
			home := core.GetHomeDir()
			displayPath := ref.File
			if home != "" && strings.HasPrefix(ref.File, home) {
				displayPath = "~" + ref.File[len(home):]
			}
			fmt.Fprintf(out, "  • %s (line %d): %s\n", displayPath, ref.Line, strings.TrimSpace(ref.Text))
		}
		fmt.Fprintln(out, "  (Remember to remove these aliases/paths after uninstallation)")
		fmt.Fprintln(out)
	}

	// 4. Discover associated application resources
	resources, err := engine.FindAppResources(*targetApp)
	if err != nil {
		return fmt.Errorf("failed to discover application resources: %w", err)
	}

	if len(resources) == 0 {
		fmt.Fprintln(out, "No files or resources found to remove.")
		return nil
	}

	// 5. Partition resources by confidence and filter based on flags
	var (
		highItems   []engine.AttributedResource
		mediumItems []engine.AttributedResource
		lowItems    []engine.AttributedResource

		toRemove    []engine.AttributedResource
		skippedData []engine.AttributedResource
		skippedLow  []engine.AttributedResource
	)

	for _, res := range resources {
		switch res.Confidence {
		case engine.ConfidenceHigh:
			highItems = append(highItems, res)
		case engine.ConfidenceMedium:
			mediumItems = append(mediumItems, res)
		case engine.ConfidenceLow:
			lowItems = append(lowItems, res)
		}

		// Check personal data protection
		if !flagData && isPersonalDataResource(res) {
			skippedData = append(skippedData, res)
			continue
		}

		// Check confidence inclusion
		if res.Confidence == engine.ConfidenceHigh {
			toRemove = append(toRemove, res)
		} else if flagAll {
			toRemove = append(toRemove, res)
		} else {
			skippedLow = append(skippedLow, res)
		}
	}

	// 6. Display Breakdown
	renderBreakdown(out, highItems, mediumItems, lowItems, skippedData, skippedLow)

	var totalBytes int64
	for _, item := range toRemove {
		totalBytes += item.Size
	}

	fmt.Fprintf(out, "\nTotal to remove: %d items (%s)\n", len(toRemove), core.FormatBytes(totalBytes))

	// 7. Dry run check
	if flagDryRun {
		infoStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8BE9FD"))
		fmt.Fprintln(out, infoStyle.Render("\n[DRY RUN] No files were removed."))
		return nil
	}

	if len(toRemove) == 0 {
		fmt.Fprintln(out, "No items eligible for removal.")
		return nil
	}

	// 8. Confirmation prompt
	if !flagYes {
		if !core.IsTTY() {
			return errors.New("confirmation required. Use --yes / -y to proceed non-interactively")
		}
		fmt.Fprintf(out, "\nAre you sure you want to uninstall %s and move %d items to Trash? [y/N]: ", targetApp.Name, len(toRemove))
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			fmt.Fprintln(out, "Uninstallation aborted.")
			return nil
		}
	}

	// 9. Execute uninstallation
	logger, _ := core.NewLogger()
	defer func() {
		if logger != nil {
			logger.Close()
		}
	}()

	if logger != nil {
		logger.LogOperation(
			"uninstall",
			"start",
			"in_progress",
			fmt.Sprintf("App: %s, BundleID: %s, Items: %d", targetApp.Name, targetApp.BundleID, len(toRemove)),
		)
	}

	fmt.Fprintln(out, "\nRemoving application resources...")

	var removedCount int
	var freedBytes int64
	var failedItems []string

	for _, item := range toRemove {
		// Bootout launchd job if removing launch agent / daemon
		if item.Category == engine.CategoryLaunchAgent || item.Category == engine.CategoryLaunchDaemon {
			_ = engine.BootoutLaunchd(item.Path)
		}

		err := core.SafeRemove(item.Path)
		if err != nil {
			failedItems = append(failedItems, fmt.Sprintf("%s: %v", item.Path, err))
			continue
		}

		removedCount++
		freedBytes += item.Size

		if logger != nil {
			logger.LogDeletion(item.Path, item.Size, "trash", "uninstall")
		}
	}

	if logger != nil {
		status := "success"
		if len(failedItems) > 0 {
			status = "partial_success"
		}
		logger.LogOperation(
			"uninstall",
			"complete",
			status,
			fmt.Sprintf("Removed: %d, Freed: %d bytes, Failures: %d", removedCount, freedBytes, len(failedItems)),
		)
	}

	// 10. Summary
	successStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B"))
	fmt.Fprintf(out, "\n%s Successfully uninstalled %s! Freed %s.\n",
		successStyle.Render("✓"), targetApp.Name, core.FormatBytes(freedBytes))

	if len(failedItems) > 0 {
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555"))
		fmt.Fprintf(out, "\n%s Failed to remove %d item(s):\n", warnStyle.Render("✗"), len(failedItems))
		for _, f := range failedItems {
			fmt.Fprintf(out, "  • %s\n", f)
		}
	}

	return nil
}

func resolveAppByNameOrPath(term string) (*engine.AppInfo, error) {
	cleanTerm := filepath.Clean(term)

	// If a direct bundle path was passed
	if strings.HasSuffix(cleanTerm, ".app") {
		if fi, err := os.Stat(cleanTerm); err == nil && fi.IsDir() {
			return engine.ParseInfo(cleanTerm)
		}
	}

	apps, err := engine.ScanInstalledApps(false)
	if err != nil {
		return nil, fmt.Errorf("failed to scan installed applications: %w", err)
	}

	lowerTerm := strings.ToLower(strings.TrimSpace(term))

	// 1. Exact path match
	for _, a := range apps {
		if strings.EqualFold(a.Path, term) {
			return &a, nil
		}
	}

	// 2. Exact BundleID match
	for _, a := range apps {
		if strings.EqualFold(a.BundleID, lowerTerm) {
			return &a, nil
		}
	}

	// 3. Exact Name match
	for _, a := range apps {
		if strings.EqualFold(a.Name, lowerTerm) {
			return &a, nil
		}
	}

	// 4. Prefix or substring match
	var matches []engine.AppInfo
	for _, a := range apps {
		lowerName := strings.ToLower(a.Name)
		if strings.HasPrefix(lowerName, lowerTerm) || strings.Contains(lowerName, lowerTerm) {
			matches = append(matches, a)
		}
	}

	if len(matches) == 1 {
		return &matches[0], nil
	}

	if len(matches) > 1 {
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%s (%s)", m.Name, m.BundleID)
		}
		return nil, fmt.Errorf("ambiguous match for %q; please specify one of: %s", term, strings.Join(names, ", "))
	}

	return nil, fmt.Errorf("no installed application found matching %q", term)
}

func selectAppInteractively() (*engine.AppInfo, error) {
	apps, err := engine.ScanInstalledApps(false)
	if err != nil {
		return nil, fmt.Errorf("failed to scan installed applications: %w", err)
	}

	var thirdParty []engine.AppInfo
	for _, a := range apps {
		if !a.IsApple && !a.Protected {
			thirdParty = append(thirdParty, a)
		}
	}

	if len(thirdParty) == 0 {
		return nil, errors.New("no third-party applications found to uninstall")
	}

	items := make([]menu.Item, len(thirdParty))
	for i, a := range thirdParty {
		desc := a.BundleID
		if a.Version != "" {
			desc += " • v" + a.Version
		}
		items[i] = menu.Item{
			Title:       a.Name,
			Description: desc,
			Size:        a.Size,
			Path:        a.Path,
		}
	}

	m := menu.New("Select an application to uninstall", items, menu.WithSizes(), menu.WithHeight(14))
	p := tea.NewProgram(m)
	res, err := p.Run()
	if err != nil {
		return nil, err
	}

	finalModel, ok := res.(menu.Model)
	if !ok || !finalModel.IsConfirmed() || finalModel.IsCancelled() {
		return nil, nil
	}

	cursor := finalModel.Cursor()
	if cursor >= 0 && cursor < len(thirdParty) {
		return &thirdParty[cursor], nil
	}

	return nil, nil
}

type ConfigReference struct {
	File string
	Line int
	Text string
}

func checkConfigReferences(app engine.AppInfo) []ConfigReference {
	home := core.GetHomeDir()
	if home == "" {
		return nil
	}

	configFiles := []string{
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".zshenv"),
	}

	configDir := filepath.Join(home, ".config")
	if entries, err := os.ReadDir(configDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				configFiles = append(configFiles, filepath.Join(configDir, e.Name()))
			}
		}
	}

	var refs []ConfigReference

	for _, cf := range configFiles {
		data, err := os.ReadFile(cf)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for idx, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			lower := strings.ToLower(trimmed)

			matched := false
			if app.Path != "" && strings.Contains(trimmed, app.Path) {
				matched = true
			} else if app.Executable != "" && len(app.Executable) >= 3 && strings.Contains(lower, strings.ToLower(app.Executable)) {
				matched = true
			} else if app.BundleID != "" && strings.Contains(lower, strings.ToLower(app.BundleID)) {
				matched = true
			}

			if matched {
				refs = append(refs, ConfigReference{
					File: cf,
					Line: idx + 1,
					Text: trimmed,
				})
			}
		}
	}

	return refs
}

func isPersonalDataResource(res engine.AttributedResource) bool {
	if res.Category == engine.CategoryGroupContainer {
		return true
	}
	lower := strings.ToLower(res.Path)
	if strings.Contains(lower, "/documents") || strings.Contains(lower, "/data/documents") {
		return true
	}
	return false
}

func renderBreakdown(
	w any,
	high, medium, low []engine.AttributedResource,
	skippedData, skippedLow []engine.AttributedResource,
) {
	writer, ok := w.(interface{ Write([]byte) (int, error) })
	if !ok {
		return
	}

	titleHigh := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B"))
	titleMed := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F1FA8C"))
	titleLow := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF79C6"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))

	printGroup := func(title string, style lipgloss.Style, items []engine.AttributedResource) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(writer, "%s (%d items):\n", style.Render(title), len(items))
		for _, item := range items {
			var sigStrs []string
			for _, s := range item.Signals {
				sigStrs = append(sigStrs, string(s))
			}
			sigFormatted := ""
			if len(sigStrs) > 0 {
				sigFormatted = fmt.Sprintf(" %s", dimStyle.Render("["+strings.Join(sigStrs, ", ")+"]"))
			}

			displayPath := item.Path
			home := core.GetHomeDir()
			if home != "" && strings.HasPrefix(displayPath, home) {
				displayPath = "~" + displayPath[len(home):]
			}

			fmt.Fprintf(writer, "  • %s (%s)%s\n", displayPath, core.FormatBytes(item.Size), sigFormatted)
		}
		fmt.Fprintln(writer)
	}

	printGroup("[High Confidence]", titleHigh, high)
	printGroup("[Medium Confidence]", titleMed, medium)
	printGroup("[Low Confidence]", titleLow, low)

	if len(skippedData) > 0 {
		fmt.Fprintf(writer, "%s: %d item(s) protected (use --data to delete)\n",
			dimStyle.Render("Personal Data"), len(skippedData))
	}
	if len(skippedLow) > 0 {
		fmt.Fprintf(writer, "%s: %d item(s) skipped (use --all to delete Medium/Low confidence)\n",
			dimStyle.Render("Lower Confidence"), len(skippedLow))
	}
}
