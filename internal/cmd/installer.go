package cmd

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
	"github.com/SadeepaNHerath/Sift/internal/tui/menu"
)

var (
	installerDryRun bool
	installerYes    bool
)

var installerCmd = &cobra.Command{
	Use:   "installer",
	Short: "Clean up downloaded installer packages and DMGs",
	Long: `Search for and remove orphaned installer packages (.pkg), disk images (.dmg),
and archive files taking up space in Downloads and system caches.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runInstaller(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runInstaller,
}

func init() {
	installerCmd.Flags().BoolVarP(&installerDryRun, "dry-run", "n", false, "Preview installer files without deleting")
	installerCmd.Flags().BoolVarP(&installerYes, "yes", "y", false, "Execute cleanup without confirmation prompt")
}

func runInstaller(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")

	logger, err := core.NewLogger()
	if err != nil {
		logger = nil
	}

	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("▸ Sift: Scanning for installer packages and disk images..."))

	files, err := engine.ScanInstallers()
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if len(files) == 0 {
		fmt.Fprintln(out, lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ No leftover installer packages or disk images found."))
		return nil
	}

	// Handle dry-run mode
	if dryRun {
		return renderInstallerDryRun(cmd, files, logger)
	}

	// Interactive TUI mode if attached to TTY and not confirmed via --yes
	if !yes && core.IsTTY() {
		selectedFiles, err := runInteractiveInstallerSelect(files)
		if err != nil {
			return err
		}
		if len(selectedFiles) == 0 {
			fmt.Fprintln(out, "Installer cleanup cancelled: No items selected.")
			return nil
		}
		files = selectedFiles
	} else if !yes && !core.IsTTY() {
		return fmt.Errorf("installer cleanup requires confirmation: pass --yes/-y to proceed non-interactively, or --dry-run to preview")
	}

	// Execute removal
	report, err := engine.RemoveInstallers(files, false, logger)
	if err != nil {
		return fmt.Errorf("installer removal failed: %w", err)
	}

	renderInstallerExecutionSummary(cmd, report)
	return nil
}

// runInteractiveInstallerSelect presents the Bubble Tea multi-select menu.
func runInteractiveInstallerSelect(files []engine.InstallerFile) ([]engine.InstallerFile, error) {
	menuItems := make([]menu.Item, len(files))
	for i, f := range files {
		desc := fmt.Sprintf("%s • %s", formatInstallerAge(time.Since(f.ModTime)), f.Path)
		if f.InArchive {
			desc = fmt.Sprintf("[ZIP Installer] %s", desc)
		}

		menuItems[i] = menu.Item{
			Title:       fmt.Sprintf("%s %s", getInstallerIcon(f.Extension), f.Name),
			Description: desc,
			Size:        f.Size,
			Path:        f.Path,
			Selected:    true,
		}
	}

	m := menu.New("Select Installer Packages to Remove", menuItems,
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

	var res []engine.InstallerFile
	for _, f := range files {
		if selectedMap[f.Path] {
			res = append(res, f)
		}
	}

	return res, nil
}

// renderInstallerDryRun prints preview breakdown and summary.
func renderInstallerDryRun(cmd *cobra.Command, files []engine.InstallerFile, logger *core.Logger) error {
	out := cmd.OutOrStdout()

	// Dry-run removal with engine reporting and logging
	report, _ := engine.RemoveInstallers(files, true, logger)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	fileStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, titleStyle.Render("▸ DRY RUN PREVIEW — No files will be deleted"))
	fmt.Fprintln(out, strings.Repeat("─", 72))

	for _, f := range files {
		icon := getInstallerIcon(f.Extension)
		archiveTag := ""
		if f.InArchive {
			archiveTag = tagStyle.Render(" [ZIP Installer]")
		}

		fmt.Fprintf(out, " %s %s%s  %s\n",
			icon,
			fileStyle.Render(f.Name),
			archiveTag,
			sizeStyle.Render(core.FormatBytes(f.Size)),
		)
		fmt.Fprintf(out, "   %s (age: %s)\n",
			dimStyle.Render(f.Path),
			dimStyle.Render(formatInstallerAge(time.Since(f.ModTime))),
		)
	}

	fmt.Fprintln(out, strings.Repeat("─", 72))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(0, 1)

	summary := fmt.Sprintf("Installers: %d  │  Reclaimable: %s",
		len(report.Removed),
		sizeStyle.Render(core.FormatBytes(report.TotalBytes)),
	)
	fmt.Fprintln(out, boxStyle.Render(summary))
	fmt.Fprintln(out)

	return nil
}

// renderInstallerExecutionSummary prints removal results.
func renderInstallerExecutionSummary(cmd *cobra.Command, report *core.RemovalReport) {
	out := cmd.OutOrStdout()

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, headerStyle.Render(fmt.Sprintf("✔ Installer cleanup completed! Reclaimed %s", sizeStyle.Render(core.FormatBytes(report.TotalBytes)))))
	fmt.Fprintf(out, "  Removed: %d installer packages\n", len(report.Removed))

	if len(report.Failed) > 0 {
		fmt.Fprintf(out, "  %s %d items could not be removed:\n", errStyle.Render("Errors:"), len(report.Failed))
		for _, f := range report.Failed {
			fmt.Fprintf(out, "   • %s: %v\n", f.Path, f.Error)
		}
	}
	fmt.Fprintln(out)
}

func getInstallerIcon(ext string) string {
	switch strings.ToLower(ext) {
	case ".dmg":
		return "💿"
	case ".pkg", ".mpkg":
		return "📦"
	case ".iso":
		return "📀"
	case ".zip":
		return "🗜️ "
	case ".xip":
		return "📦"
	default:
		return "📄"
	}
}

func formatInstallerAge(d time.Duration) string {
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
