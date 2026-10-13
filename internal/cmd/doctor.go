package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Spacefold/internal/engine"
)

var doctorJSON bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose system health, disk pressure, and configuration issues",
	Long: `Run comprehensive diagnostic checks across Full Disk Access permissions,
storage capacity, memory pressure, cache accumulation, background daemons,
and orphaned application leftovers with actionable repair suggestions.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDoctor(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Output doctor diagnostics as JSON")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	isJSON, _ := cmd.Flags().GetBool("json")

	report, err := engine.RunDoctorChecks()
	if err != nil {
		return fmt.Errorf("doctor diagnostics failed: %w", err)
	}

	if isJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	// Rich TUI rendering
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	okBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")).Render("  ✔ OK      ")
	warnBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("  ▲ WARNING ")
	critBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("  ✖ CRITICAL")

	fmt.Fprintln(out, headerStyle.Render("\n🩺 Sift System Doctor"))
	fmt.Fprintln(out, "────────────────────────────────────────────────────────────────────────")

	var fixSuggestions []engine.CheckResult

	for _, check := range report.Results {
		var badge string
		switch check.Severity {
		case engine.SeverityOK:
			badge = okBadge
		case engine.SeverityWarning:
			badge = warnBadge
			if check.FixCommand != "" {
				fixSuggestions = append(fixSuggestions, check)
			}
		case engine.SeverityCritical:
			badge = critBadge
			if check.FixCommand != "" {
				fixSuggestions = append(fixSuggestions, check)
			}
		}

		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(fmt.Sprintf("%-26s", check.Name))
		summaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(check.Summary)

		fmt.Fprintf(out, "%s %s %s\n", badge, nameStyle, summaryStyle)
		if check.Details != "" && check.Severity != engine.SeverityOK {
			fmt.Fprintf(out, "             ↳ %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render(check.Details))
		}
	}

	fmt.Fprintln(out, "────────────────────────────────────────────────────────────────────────")
	summaryLine := fmt.Sprintf("Diagnostics summary: %d healthy, %d warning, %d critical",
		report.OKCount, report.WarnCount, report.CritCount)
	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(summaryLine))

	if len(fixSuggestions) > 0 {
		fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("\nRecommended Actions:"))
		for i, fix := range fixSuggestions {
			fmt.Fprintf(out, "  %d. %s:\n     $ %s\n",
				i+1,
				lipgloss.NewStyle().Bold(true).Render(fix.Name),
				lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Render(fix.FixCommand),
			)
		}
	}
	fmt.Fprintln(out)

	return nil
}
