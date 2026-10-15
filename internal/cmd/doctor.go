package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

var (
	doctorJSON     bool
	doctorCI       bool
	doctorExitCode bool
	doctorFailOn   string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose system health, disk pressure, and configuration issues",
	Long: `Run diagnostic health checks across Full Disk Access permissions,
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
	doctorCmd.Flags().BoolVar(&doctorCI, "ci", false, "Enable CI runner mode (outputs GitHub Actions annotations and enables exit codes)")
	doctorCmd.Flags().BoolVar(&doctorExitCode, "exit-code", false, "Exit with code 1 if critical health issues are detected")
	doctorCmd.Flags().StringVar(&doctorFailOn, "fail-on", "critical", "Threshold to trigger exit code 1 ('critical' or 'warning')")
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

	// CI Mode Output
	if doctorCI {
		for _, check := range report.Results {
			title := escapeWorkflowCommand(check.Name)
			msg := escapeWorkflowCommand(fmt.Sprintf("%s - %s", check.Summary, check.Details))
			switch check.Severity {
			case engine.SeverityCritical:
				fmt.Fprintf(out, "::error title=%s::%s\n", title, msg)
			case engine.SeverityWarning:
				fmt.Fprintf(out, "::warning title=%s::%s\n", title, msg)
			}
		}
	}

	// Exit code evaluation
	if doctorExitCode || doctorCI {
		if strings.EqualFold(doctorFailOn, "warning") && (report.CritCount > 0 || report.WarnCount > 0) {
			return fmt.Errorf("doctor check failed with %d critical, %d warnings (threshold: warning)", report.CritCount, report.WarnCount)
		}
		if report.CritCount > 0 {
			return fmt.Errorf("doctor check failed: %d critical issues detected (threshold: critical)", report.CritCount)
		}
	}

	return nil
}

func escapeWorkflowCommand(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}
