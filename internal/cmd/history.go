package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

var (
	historyLimit     int
	historyJSON      bool
	historyDeletions bool
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View audit logs and history of cleanup operations",
	Long: `Review the historical record of all maintenance, cleanup, and deletion operations
recorded in the Sift forensic audit ledger (~/Library/Logs/sf/).`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runHistory(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runHistory,
}

func init() {
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "l", 25, "Maximum number of entries to display")
	historyCmd.Flags().BoolVar(&historyJSON, "json", false, "Output history as JSON")
	historyCmd.Flags().BoolVarP(&historyDeletions, "deletions", "d", false, "Display individual file deletions log")
}

func runHistory(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	isJSON, _ := cmd.Flags().GetBool("json")
	showDeletions, _ := cmd.Flags().GetBool("deletions")
	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		limit = 25
	}

	if showDeletions {
		deletions, err := core.ReadDeletions(limit)
		if err != nil {
			return fmt.Errorf("failed to read deletions log: %w", err)
		}

		if isJSON {
			data, err := json.MarshalIndent(deletions, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(out, string(data))
			return nil
		}

		if len(deletions) == 0 {
			fmt.Fprintln(out, lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("No recorded deletions found."))
			return nil
		}

		fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("\n🗑️  Sift Deletions Audit Log"))
		fmt.Fprintln(out, "────────────────────────────────────────────────────────────────────────")
		for _, d := range deletions {
			t := d.Timestamp
			if len(t) > 19 {
				t = t[:19]
			}
			timeStr := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(t)
			sizeStr := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")).Render(fmt.Sprintf("%10s", core.FormatBytes(d.Bytes)))
			modeStr := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf("[%s]", d.Mode))
			pathStr := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(d.Path)
			fmt.Fprintf(out, "%s  %s  %-12s  %s\n", timeStr, sizeStr, modeStr, pathStr)
		}
		fmt.Fprintln(out)
		return nil
	}

	// Operations log
	ops, err := core.ReadOperations(limit)
	if err != nil {
		return fmt.Errorf("failed to read operations log: %w", err)
	}

	if isJSON {
		data, err := json.MarshalIndent(ops, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	if len(ops) == 0 {
		fmt.Fprintln(out, lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("No recorded operations found."))
		return nil
	}

	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("\n📋 Sift Operations Audit Log"))
	fmt.Fprintln(out, "────────────────────────────────────────────────────────────────────────")
	for _, op := range ops {
		t := op.Timestamp
		if len(t) > 19 {
			t = t[:19]
		}
		timeStr := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(t)
		cmdStr := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Render(fmt.Sprintf("%-10s", op.Command))
		statusColor := "42"
		if strings.Contains(strings.ToLower(op.Status), "fail") || strings.Contains(strings.ToLower(op.Status), "err") {
			statusColor = "196"
		}
		statusStr := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("%-8s", op.Status))
		detailsStr := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(op.Details)
		fmt.Fprintf(out, "%s  %s  %s  %s\n", timeStr, cmdStr, statusStr, detailsStr)
	}
	fmt.Fprintln(out)

	return nil
}
