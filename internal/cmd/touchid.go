package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
	"github.com/SadeepaNHerath/Spacefold/internal/engine"
)

var (
	touchidEnable  bool
	touchidDisable bool
	touchidStatus  bool
)

var touchidCmd = &cobra.Command{
	Use:   "touchid",
	Short: "Configure Touch ID authentication for sudo operations",
	Long: `Enable or verify Touch ID PAM module authentication for sudo in Terminal sessions.
On macOS Sonoma (14.0+) and Sequoia (15.0+), this manages /etc/pam.d/sudo_local which persists across macOS updates.`,
	RunE: runTouchID,
}

func init() {
	touchidCmd.Run = func(cmd *cobra.Command, args []string) {
		_ = runTouchID(cmd, args)
	}
	touchidCmd.Flags().BoolVarP(&touchidEnable, "enable", "e", false, "Enable Touch ID authentication for sudo")
	touchidCmd.Flags().BoolVarP(&touchidDisable, "disable", "d", false, "Disable Touch ID authentication for sudo")
	touchidCmd.Flags().BoolVarP(&touchidStatus, "status", "s", false, "Check current Touch ID for sudo status")
}

// runTouchID handles the sf touchid CLI entrypoint.
func runTouchID(cmd *cobra.Command, args []string) error {
	enable, _ := cmd.Flags().GetBool("enable")
	disable, _ := cmd.Flags().GetBool("disable")
	statusOnly, _ := cmd.Flags().GetBool("status")

	if enable && disable {
		return fmt.Errorf("cannot specify both --enable and --disable")
	}

	logger, logErr := core.NewLogger()
	if logErr == nil {
		defer logger.Close()
	}

	// 1. Explicit --status flag
	if statusOnly {
		return displayTouchIDStatus()
	}

	// 2. Explicit --enable flag
	if enable {
		return handleEnableTouchID(logger)
	}

	// 3. Explicit --disable flag
	if disable {
		return handleDisableTouchID(logger)
	}

	// 4. Interactive guided toggle (or status display if non-interactive)
	enabled, isSupported, configPath, err := engine.CheckStatus()
	if err != nil {
		return fmt.Errorf("failed to query Touch ID status: %w", err)
	}

	if err := renderTouchIDCard(enabled, isSupported, configPath); err != nil {
		return err
	}

	// If not in a TTY or piped, finish after showing status
	if !core.IsTTY() {
		return nil
	}

	// Interactive prompt to toggle
	reader := bufio.NewReader(os.Stdin)
	if enabled {
		fmt.Print(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("  Touch ID is currently enabled. Disable it? [y/N]: "))
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp == "y" || resp == "yes" {
			return handleDisableTouchID(logger)
		}
		fmt.Println("  No changes made.")
		return nil
	}

	fmt.Print(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("  Touch ID is currently disabled. Enable it for sudo? [y/N]: "))
	resp, _ := reader.ReadString('\n')
	resp = strings.TrimSpace(strings.ToLower(resp))
	if resp == "y" || resp == "yes" {
		return handleEnableTouchID(logger)
	}
	fmt.Println("  No changes made.")
	return nil
}

// displayTouchIDStatus renders current configuration status without prompting.
func displayTouchIDStatus() error {
	enabled, isSupported, configPath, err := engine.CheckStatus()
	if err != nil {
		return fmt.Errorf("failed to check Touch ID status: %w", err)
	}
	return renderTouchIDCard(enabled, isSupported, configPath)
}

// renderTouchIDCard formats and prints the status information card.
func renderTouchIDCard(enabled, isSupported bool, configPath string) error {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#BD93F9")).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9"))

	var hwStatus string
	if isSupported {
		hwStatus = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("Supported ✓")
	} else {
		hwStatus = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("Not Detected / External Keyboard")
	}

	var sudoStatus string
	if enabled {
		sudoStatus = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("Enabled ✓")
	} else {
		sudoStatus = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244")).Render("Disabled (Password Only)")
	}

	persistenceNote := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("Preserved across macOS system updates")

	content := fmt.Sprintf("%s\n\n  %s %s\n  %s %s\n  %s %s\n  %s %s",
		headerStyle.Render("🔐 Touch ID for Sudo Configuration"),
		labelStyle.Render("Hardware Sensor:"), hwStatus,
		labelStyle.Render("Sudo Elevation: "), sudoStatus,
		labelStyle.Render("Target Config:  "), lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(configPath),
		labelStyle.Render("Persistence:    "), persistenceNote,
	)

	fmt.Println()
	fmt.Println(boxStyle.Render(content))
	fmt.Println()
	return nil
}

// handleEnableTouchID applies pam_tid.so to /etc/pam.d/sudo_local.
func handleEnableTouchID(logger *core.Logger) error {
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("▸ Sift: Enabling Touch ID for sudo..."))
	if err := engine.Enable(); err != nil {
		return fmt.Errorf("failed to enable Touch ID for sudo: %w\n(Note: macOS administrator credentials may be required for /etc/pam.d/sudo_local)", err)
	}

	successStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	fmt.Println()
	fmt.Println(successStyle.Render("  ✓ Touch ID for sudo successfully ENABLED!"))
	fmt.Println(infoStyle.Render("    Fingerprint authentication is now active for sudo commands in Terminal."))
	fmt.Println(infoStyle.Render(fmt.Sprintf("    Configuration written to %s", engine.PamSudoLocalPath)))
	fmt.Println()

	if logger != nil {
		logger.LogOperation("touchid", "enable", "success", "Enabled pam_tid.so in "+engine.PamSudoLocalPath)
	}
	return nil
}

// handleDisableTouchID comments out pam_tid.so in /etc/pam.d/sudo_local.
func handleDisableTouchID(logger *core.Logger) error {
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("▸ Sift: Disabling Touch ID for sudo..."))
	if err := engine.Disable(); err != nil {
		return fmt.Errorf("failed to disable Touch ID for sudo: %w\n(Note: macOS administrator credentials may be required for /etc/pam.d/sudo_local)", err)
	}

	noticeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	fmt.Println()
	fmt.Println(noticeStyle.Render("  ✓ Touch ID for sudo successfully DISABLED."))
	fmt.Println(infoStyle.Render("    Standard password authentication will now be required for sudo."))
	fmt.Println(infoStyle.Render(fmt.Sprintf("    Configuration updated at %s", engine.PamSudoLocalPath)))
	fmt.Println()

	if logger != nil {
		logger.LogOperation("touchid", "disable", "success", "Disabled pam_tid.so in "+engine.PamSudoLocalPath)
	}
	return nil
}
