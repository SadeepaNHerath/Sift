package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
	"github.com/SadeepaNHerath/Spacefold/internal/engine"
	"github.com/SadeepaNHerath/Spacefold/internal/tui/menu"
)

var (
	privacyDryRun         bool
	privacyYes            bool
	privacyJSON           bool
	privacyClipboardOnly  bool
	privacyRecentOnly     bool
	privacyDiagnosticOnly bool
	privacyBrowserOnly    bool
)

var privacyCmd = &cobra.Command{
	Use:   "privacy",
	Short: "Forensic history wiper: recent items, clipboard, crash logs, and browser storage",
	Long: `Securely wipe forensic history artifacts from macOS:
1. macOS Recent Items (.sfl3 / LSSharedFileList) and QuickLook thumbnail databases.
2. Terminal and system pasteboard buffers (clears copied secrets / tokens).
3. System diagnostic reports and crash logs (/Library/Logs/DiagnosticReports).
4. Safari, Chrome, and Brave local storage databases (strictly preserving bookmarks, logins, and passwords).`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runPrivacy(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runPrivacy,
}

func init() {
	privacyCmd.Flags().BoolVarP(&privacyDryRun, "dry-run", "n", false, "Preview privacy targets to be wiped without deleting")
	privacyCmd.Flags().BoolVarP(&privacyYes, "yes", "y", false, "Execute wiping without confirmation prompt")
	privacyCmd.Flags().BoolVar(&privacyJSON, "json", false, "Output privacy targets in JSON format")
	privacyCmd.Flags().BoolVar(&privacyClipboardOnly, "clipboard", false, "Wipe only the system clipboard / pasteboard buffer")
	privacyCmd.Flags().BoolVar(&privacyRecentOnly, "recent", false, "Wipe only macOS recent items and QuickLook caches")
	privacyCmd.Flags().BoolVar(&privacyDiagnosticOnly, "diagnostics", false, "Wipe only diagnostic reports and crash reporter logs")
	privacyCmd.Flags().BoolVar(&privacyBrowserOnly, "browser-storage", false, "Trim only browser local storage databases")
}

func runPrivacy(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	targets, err := engine.ScanPrivacyTargets()
	if err != nil {
		return fmt.Errorf("failed to scan privacy targets: %w", err)
	}

	// Filter by flags if specific switches are passed
	if privacyClipboardOnly || privacyRecentOnly || privacyDiagnosticOnly || privacyBrowserOnly {
		var filtered []engine.PrivacyTarget
		for _, t := range targets {
			if privacyClipboardOnly && t.IsClipboard {
				filtered = append(filtered, t)
			}
			if privacyRecentOnly && t.ID == "recent-items" {
				filtered = append(filtered, t)
			}
			if privacyDiagnosticOnly && t.ID == "diagnostics" {
				filtered = append(filtered, t)
			}
			if privacyBrowserOnly && t.ID == "browser-storage" {
				filtered = append(filtered, t)
			}
		}
		targets = filtered
	}

	// 1. JSON mode
	if privacyJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(targets)
	}

	logger, _ := core.NewLogger()
	if logger != nil {
		defer logger.Close()
	}

	// 2. Dry run preview
	if privacyDryRun {
		return renderPrivacyDryRun(cmd, targets, logger)
	}

	// 3. Interactive multi-select mode if attached to TTY
	if !privacyYes && core.IsTTY() && !privacyClipboardOnly && !privacyRecentOnly && !privacyDiagnosticOnly && !privacyBrowserOnly {
		selectedTargets, err := runInteractivePrivacySelect(targets)
		if err != nil {
			return err
		}
		if len(selectedTargets) == 0 {
			fmt.Fprintln(out, "Privacy wipe cancelled: No targets selected.")
			return nil
		}
		targets = selectedTargets
	} else if !privacyYes && !core.IsTTY() {
		return fmt.Errorf("privacy wiping requires confirmation: pass --yes/-y to execute non-interactively, or --dry-run to preview")
	}

	// 4. Execute wiping
	report, err := engine.WipePrivacyTargets(targets, false, logger)
	if err != nil {
		return fmt.Errorf("privacy wipe failed: %w", err)
	}

	renderPrivacyExecutionSummary(cmd, report)
	return nil
}

func runInteractivePrivacySelect(targets []engine.PrivacyTarget) ([]engine.PrivacyTarget, error) {
	menuItems := make([]menu.Item, len(targets))
	for i, t := range targets {
		menuItems[i] = menu.Item{
			Title:       t.Name,
			Description: t.Description,
			Size:        t.Size,
			Path:        t.ID,
			Selected:    true,
		}
	}

	m := menu.New("Select Privacy Artifacts to Wipe", menuItems,
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

	var res []engine.PrivacyTarget
	for _, t := range targets {
		if selectedMap[t.ID] {
			res = append(res, t)
		}
	}

	return res, nil
}

func renderPrivacyDryRun(cmd *cobra.Command, targets []engine.PrivacyTarget, logger *core.Logger) error {
	out := cmd.OutOrStdout()
	report, _ := engine.WipePrivacyTargets(targets, true, logger)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	headStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, titleStyle.Render("▸ PRIVACY DRY RUN PREVIEW — No files or clipboard will be modified"))
	fmt.Fprintln(out, strings.Repeat("─", 72))

	for _, t := range targets {
		itemsStr := fmt.Sprintf("%d items", t.ItemCount)
		if t.IsClipboard {
			itemsStr = "pasteboard buffer"
		}

		fmt.Fprintf(out, " %s  %-20s  %s\n",
			headStyle.Render(t.Name),
			dimStyle.Render("("+itemsStr+")"),
			sizeStyle.Render(core.FormatBytes(t.Size)),
		)
		fmt.Fprintf(out, "   %s\n", dimStyle.Render(t.Description))
	}

	fmt.Fprintln(out, strings.Repeat("─", 72))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(0, 1)

	summary := fmt.Sprintf("Categories: %d  │  Reclaimable: %s",
		len(targets),
		sizeStyle.Render(core.FormatBytes(report.TotalBytes)),
	)
	fmt.Fprintln(out, boxStyle.Render(summary))
	fmt.Fprintln(out)

	return nil
}

func renderPrivacyExecutionSummary(cmd *cobra.Command, report *core.RemovalReport) {
	out := cmd.OutOrStdout()

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, headerStyle.Render(fmt.Sprintf("✔ Privacy wipe completed! Reclaimed %s", sizeStyle.Render(core.FormatBytes(report.TotalBytes)))))
	fmt.Fprintf(out, "  Wiped: %d forensic artifacts\n", len(report.Removed))

	if len(report.Failed) > 0 {
		fmt.Fprintf(out, "  %s %d items could not be removed:\n", errStyle.Render("Errors:"), len(report.Failed))
		for _, f := range report.Failed {
			fmt.Fprintf(out, "   • %s: %v\n", f.Path, f.Error)
		}
	}
	fmt.Fprintln(out)
}
