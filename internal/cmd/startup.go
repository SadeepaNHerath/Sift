package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
	tuiStartup "github.com/SadeepaNHerath/Sift/internal/tui/startup"
)

var (
	startupJSON          bool
	startupZombies       bool
	startupToggle        string
	startupRemoveZombies bool
	startupYes           bool
)

var startupCmd = &cobra.Command{
	Use:     "startup",
	Aliases: []string{"launchd"},
	Short:   "Audit and manage persistent background services & launch daemons",
	Long: `Audit and manage persistent macOS background services across all 3 levels:
~/Library/LaunchAgents (User Agents), /Library/LaunchAgents (Global Agents),
and /Library/LaunchDaemons (System Daemons).
Enables interactive toggling without deleting plists, and highlights orphaned "Zombie"
services left behind by removed applications.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runStartup(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runStartup,
}

func init() {
	startupCmd.Flags().BoolVar(&startupJSON, "json", false, "Output launch services list in JSON format")
	startupCmd.Flags().BoolVar(&startupZombies, "zombies", false, "List only orphaned Zombie services")
	startupCmd.Flags().StringVar(&startupToggle, "toggle", "", "Toggle service loaded state by Label")
	startupCmd.Flags().BoolVar(&startupRemoveZombies, "remove-zombies", false, "Remove all zombie launch plists to Trash")
	startupCmd.Flags().BoolVarP(&startupYes, "yes", "y", false, "Confirm removal without prompt")
}

func runStartup(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	services, err := engine.ScanLaunchServices()
	if err != nil {
		return fmt.Errorf("failed to scan launch services: %w", err)
	}

	if startupToggle != "" {
		for _, s := range services {
			if strings.EqualFold(s.Label, startupToggle) {
				newLoaded := !s.Loaded
				err := engine.ToggleLaunchService(s, newLoaded)
				if err != nil {
					return fmt.Errorf("failed to toggle service %s: %w", s.Label, err)
				}
				action := "stopped"
				if newLoaded {
					action = "started"
				}
				fmt.Fprintf(out, "✔ Service %s successfully %s\n", s.Label, action)
				return nil
			}
		}
		return fmt.Errorf("no launch service found with label %q", startupToggle)
	}

	if startupRemoveZombies {
		var zombies []engine.LaunchService
		for _, s := range services {
			if s.IsZombie {
				zombies = append(zombies, s)
			}
		}
		if len(zombies) == 0 {
			fmt.Fprintln(out, lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ No zombie launch services found."))
			return nil
		}

		if !startupYes && !core.IsTTY() {
			return fmt.Errorf("--remove-zombies requires --yes confirmation in non-interactive mode")
		}

		logger, _ := core.NewLogger()
		if logger != nil {
			defer logger.Close()
		}

		removedCount := 0
		for _, z := range zombies {
			if err := engine.RemoveZombiePlist(z, logger); err == nil {
				removedCount++
			}
		}
		fmt.Fprintf(out, "✔ Successfully trashed %d zombie launch service plists\n", removedCount)
		return nil
	}

	if startupJSON {
		if startupZombies {
			var zombies []engine.LaunchService
			for _, s := range services {
				if s.IsZombie {
					zombies = append(zombies, s)
				}
			}
			return json.NewEncoder(out).Encode(zombies)
		}
		return json.NewEncoder(out).Encode(services)
	}

	if startupZombies || !core.IsTTY() {
		return renderStartupListing(cmd, services, startupZombies)
	}

	m := tuiStartup.New(services)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func renderStartupListing(cmd *cobra.Command, services []engine.LaunchService, zombiesOnly bool) error {
	out := cmd.OutOrStdout()

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	runningStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	stoppedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	zombieStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	fmt.Fprintln(out)
	if zombiesOnly {
		fmt.Fprintln(out, titleStyle.Render("▸ Orphaned Zombie Launch Services"))
	} else {
		fmt.Fprintln(out, titleStyle.Render("▸ macOS Launch Services Audit"))
	}
	fmt.Fprintln(out, strings.Repeat("─", 72))

	displayed := 0
	for _, s := range services {
		if zombiesOnly && !s.IsZombie {
			continue
		}

		var statusStr string
		if s.IsZombie {
			statusStr = zombieStyle.Render("⚠ ZOMBIE")
		} else if s.Loaded {
			statusStr = runningStyle.Render("LOADED")
		} else {
			statusStr = stoppedStyle.Render("STOPPED")
		}

		fmt.Fprintf(out, " %-40s  %-15s  %s\n",
			s.Label,
			dimStyle.Render(string(s.Level)),
			statusStr,
		)
		if s.BinaryPath != "" {
			fmt.Fprintf(out, "   %s\n", dimStyle.Render(s.BinaryPath))
		}
		displayed++
	}

	fmt.Fprintln(out, strings.Repeat("─", 72))
	fmt.Fprintf(out, "Total displayed: %d services\n\n", displayed)
	return nil
}
