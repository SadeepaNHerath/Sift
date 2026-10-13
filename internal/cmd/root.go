package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var version = "dev"

// SetVersion sets the application version string.
func SetVersion(v string) {
	version = v
	rootCmd.Version = v
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "sf",
	Short: "Terminal-first macOS maintenance & disk analysis toolkit",
	Long: `Sift (sf) is a high-performance, terminal-first macOS maintenance and disk analysis
toolkit. It provides unified, safety-first tools to clean caches, uninstall applications
with zero leftovers, analyze disk usage, tune system caches, and purge developer artifacts.`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// EUID check: refuse direct execution as root / with sudo.
		// Sift must run as the regular macOS console user. Privileged operations
		// will prompt via Touch ID or sudo on demand.
		if os.Geteuid() == 0 {
			return fmt.Errorf("sf should not be run directly with sudo or as root.\nPlease run sf as your regular user. Sift requests elevation via Touch ID or sudo only when necessary")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// When piped or not attached to a TTY, skip interactive menu and display help.
		if !isTTY() {
			return cmd.Help()
		}
		return runInteractiveMenu(cmd)
	},
}

// Stub subcommand declarations
var (
	cleanCmd = &cobra.Command{
		Use:   "clean",
		Short: "Scan and clean system, user, and application caches",
		Long:  "Scan and clean system caches, user caches, application logs, temporary files, and junk to recover free disk space safely.",
		Run: func(cmd *cobra.Command, args []string) {
			printStubMessage(cmd.Name())
		},
	}

	analyzeCmd = &cobra.Command{
		Use:   "analyze [path]",
		Short: "Analyze disk space usage and large files",
		Long:  "Perform high-speed multi-threaded disk usage analysis across volumes, identify large files, directory hogs, and space consumers.",
		Run: func(cmd *cobra.Command, args []string) {
			printStubMessage(cmd.Name())
		},
	}

	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Display system health, storage, and security status",
		Long:  "Display an overview of system health, disk volume breakdown, memory utilization, and security posture.",
		Run: func(cmd *cobra.Command, args []string) {
			printStubMessage(cmd.Name())
		},
	}
)

func init() {
	// Register all subcommands
	rootCmd.AddCommand(
		cleanCmd,
		uninstallCmd,
		analyzeCmd,
		statusCmd,
		purgeCmd,
		dedupCmd,
		startupCmd,
		privacyCmd,
		installerCmd,
		optimizeCmd,
		touchidCmd,
		historyCmd,
		doctorCmd,
		updateCmd,
		completionCmd,
	)
}

func printStubMessage(cmdName string) {
	fmt.Printf("%s: Coming soon...\n", cmdName)
}

// menuItem defines an item on the interactive main menu.
type menuItem struct {
	num         string
	name        string
	icon        string
	description string
	cmd         *cobra.Command
}

// isTTY reports whether both stdin and stdout are attached to a terminal.
func isTTY() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil || (stdinInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	stdoutInfo, err := os.Stdout.Stat()
	if err != nil || (stdoutInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	return true
}

// sysSummary holds system overview metadata for display.
type sysSummary struct {
	hostname  string
	osVersion string
	diskFree  string
	diskTotal string
	diskPct   float64
	username  string
}

// collectSystemSummary queries local macOS metrics.
func collectSystemSummary() sysSummary {
	summary := sysSummary{
		hostname:  "Mac",
		osVersion: fmt.Sprintf("macOS (%s)", runtime.GOARCH),
		username:  "user",
	}

	if h, err := os.Hostname(); err == nil && h != "" {
		summary.hostname = h
	}

	if u, err := user.Current(); err == nil && u != nil && u.Username != "" {
		summary.username = u.Username
	}

	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		prodVer := strings.TrimSpace(string(out))
		if prodVer != "" {
			summary.osVersion = fmt.Sprintf("macOS %s (%s)", prodVer, runtime.GOARCH)
		}
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		bsize := uint64(stat.Bsize)
		totalBytes := stat.Blocks * bsize
		availBytes := stat.Bavail * bsize
		usedBytes := totalBytes - (stat.Bfree * bsize)

		totalGB := float64(totalBytes) / (1024 * 1024 * 1024)
		freeGB := float64(availBytes) / (1024 * 1024 * 1024)
		pct := float64(usedBytes) / float64(totalBytes) * 100.0

		summary.diskFree = fmt.Sprintf("%.1f GB free", freeGB)
		summary.diskTotal = fmt.Sprintf("%.1f GB", totalGB)
		summary.diskPct = pct
	} else {
		summary.diskFree = "N/A free"
		summary.diskTotal = "N/A"
	}

	return summary
}

// enableRawMode puts the terminal in raw mode and returns a cleanup function
// to restore the original terminal state and cursor.
func enableRawMode() (func(), error) {
	saveCmd := exec.Command("stty", "-g")
	saveCmd.Stdin = os.Stdin
	stateBytes, err := saveCmd.Output()
	if err != nil {
		return func() {}, err
	}
	savedState := strings.TrimSpace(string(stateBytes))

	rawCmd := exec.Command("stty", "raw", "-echo")
	rawCmd.Stdin = os.Stdin
	if err := rawCmd.Run(); err != nil {
		return func() {}, err
	}

	// Hide cursor
	fmt.Print("\033[?25l")

	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		// Show cursor
		fmt.Print("\033[?25h")
		// Restore terminal attributes
		restoreCmd := exec.Command("stty", savedState)
		restoreCmd.Stdin = os.Stdin
		_ = restoreCmd.Run()
	}

	return restore, nil
}

// runInteractiveMenu renders an interactive Lipgloss-styled menu with raw keyboard navigation.
func runInteractiveMenu(parentCmd *cobra.Command) error {
	restoreTerminal, err := enableRawMode()
	if err != nil {
		// Fallback to displaying help if terminal raw mode cannot be acquired
		return parentCmd.Help()
	}
	defer restoreTerminal()

	menuItems := []menuItem{
		{
			num:         "1",
			name:        "Clean",
			icon:        "🧹",
			description: "Clean system caches, app logs, and temporary junk",
			cmd:         cleanCmd,
		},
		{
			num:         "2",
			name:        "Uninstall",
			icon:        "🗑️ ",
			description: "Safely remove applications and all associated leftovers",
			cmd:         uninstallCmd,
		},
		{
			num:         "3",
			name:        "Optimize",
			icon:        "⚡",
			description: "Rebuild caches, flush DNS, and run system optimizations",
			cmd:         optimizeCmd,
		},
		{
			num:         "4",
			name:        "Analyze",
			icon:        "📊",
			description: "Analyze disk space usage and locate large files",
			cmd:         analyzeCmd,
		},
		{
			num:         "5",
			name:        "Status",
			icon:        "🩺",
			description: "Check system health, disk metrics, and security overview",
			cmd:         statusCmd,
		},
		{
			num:         "6",
			name:        "Purge",
			icon:        "🔥",
			description: "Deep purge Xcode, Docker, Homebrew, and dev caches",
			cmd:         purgeCmd,
		},
	}

	sys := collectSystemSummary()
	selectedIndex := 0

	// Lipgloss styles
	bannerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")) // bright cyan

	versionBadgeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")). // purple/indigo
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	valueStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255"))

	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	menuTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255"))

	selectedItemStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("51")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)

	normalItemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	itemDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244"))

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))

	const asciiBanner = `   ____ ___ _____ _____ 
  / ___|_ _|  ___|_   _|
  \___ \ | || |_    | |  
   ___) || ||  _|   | |  
  |____/|___|_|     |_|  `

	render := func() {
		var b strings.Builder

		// Clear screen and home cursor
		b.WriteString("\033[H\033[2J")

		// Banner
		b.WriteString(bannerStyle.Render(asciiBanner))
		b.WriteString("\n")
		b.WriteString(versionBadgeStyle.Render(fmt.Sprintf("  v%s • Terminal-first macOS maintenance toolkit", version)))
		b.WriteString("\n\n")

		// Disk color calculation
		var diskColor lipgloss.Color
		switch {
		case sys.diskPct >= 90.0:
			diskColor = lipgloss.Color("196") // Red
		case sys.diskPct >= 80.0:
			diskColor = lipgloss.Color("214") // Orange/Amber
		default:
			diskColor = lipgloss.Color("42") // Green
		}
		diskUsageStyle := lipgloss.NewStyle().Bold(true).Foreground(diskColor)

		// System summary line
		summaryContent := fmt.Sprintf(
			"%s %s  %s  %s %s  %s  %s %s / %s (%s)  %s  %s %s",
			labelStyle.Render(" Host:"),
			valueStyle.Render(sys.hostname),
			sepStyle.Render("│"),
			labelStyle.Render("OS:"),
			valueStyle.Render(sys.osVersion),
			sepStyle.Render("│"),
			labelStyle.Render("Disk:"),
			valueStyle.Render(sys.diskFree),
			valueStyle.Render(sys.diskTotal),
			diskUsageStyle.Render(fmt.Sprintf("%.1f%% used", sys.diskPct)),
			sepStyle.Render("│"),
			labelStyle.Render("User:"),
			valueStyle.Render(sys.username),
		)
		b.WriteString(boxStyle.Render(summaryContent))
		b.WriteString("\n\n")

		// Menu items
		b.WriteString(menuTitleStyle.Render("  Shortcut Commands:"))
		b.WriteString("\n\n")

		for i, itm := range menuItems {
			if i == selectedIndex {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Render("❯")
				num := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(fmt.Sprintf("[%s]", itm.num))
				name := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(fmt.Sprintf("%s %-10s", itm.icon, itm.name))
				desc := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Render(itm.description)
				line := fmt.Sprintf(" %s %s %s  %s", cursor, num, name, desc)
				b.WriteString(selectedItemStyle.Render(line))
			} else {
				cursor := " "
				num := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241")).Render(fmt.Sprintf("[%s]", itm.num))
				name := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(fmt.Sprintf("%s %-10s", itm.icon, itm.name))
				desc := itemDescStyle.Render(itm.description)
				line := fmt.Sprintf(" %s %s %s  %s", cursor, num, name, desc)
				b.WriteString(normalItemStyle.Render(line))
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(footerStyle.Render("  ↑/k Up  •  ↓/j Down  •  1-6 Quick Jump  •  ↵ Enter Launch  •  q/Esc Quit"))
		b.WriteString("\n")

		fmt.Print(b.String())
	}

	buf := make([]byte, 16)
	for {
		render()

		n, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}

		// Ctrl+C (0x03)
		if n == 1 && buf[0] == 3 {
			restoreTerminal()
			fmt.Println()
			return nil
		}

		// Enter (CR or LF)
		if (n == 1 && (buf[0] == '\r' || buf[0] == '\n')) || (n == 2 && buf[0] == '\r' && buf[1] == '\n') {
			restoreTerminal()
			fmt.Print("\033[H\033[2J") // Clear screen
			selected := menuItems[selectedIndex]
			fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(fmt.Sprintf("▸ Sift: Running %s %s...\n", selected.icon, selected.name)))
			if selected.cmd != nil {
				if selected.cmd.RunE != nil {
					return selected.cmd.RunE(selected.cmd, []string{})
				}
				if selected.cmd.Run != nil {
					selected.cmd.Run(selected.cmd, []string{})
					return nil
				}
			}
			return nil
		}

		// Quit keys: 'q', 'Q', Escape (0x1b when single byte)
		if (n == 1 && (buf[0] == 'q' || buf[0] == 'Q')) || (n == 1 && buf[0] == 27) {
			restoreTerminal()
			fmt.Println()
			return nil
		}

		// Vim navigation: 'k' / 'K' (up), 'j' / 'J' (down)
		if n == 1 && (buf[0] == 'k' || buf[0] == 'K') {
			selectedIndex = (selectedIndex - 1 + len(menuItems)) % len(menuItems)
			continue
		}
		if n == 1 && (buf[0] == 'j' || buf[0] == 'J') {
			selectedIndex = (selectedIndex + 1) % len(menuItems)
			continue
		}

		// Number shortcuts: '1' through '6'
		if n == 1 && buf[0] >= '1' && buf[0] <= '6' {
			target := int(buf[0] - '1')
			if target >= 0 && target < len(menuItems) {
				selectedIndex = target
			}
			continue
		}

		// ANSI escape sequences (Arrow keys)
		if n >= 3 && buf[0] == 27 && (buf[1] == '[' || buf[1] == 'O') {
			switch buf[2] {
			case 'A': // Up arrow
				selectedIndex = (selectedIndex - 1 + len(menuItems)) % len(menuItems)
			case 'B': // Down arrow
				selectedIndex = (selectedIndex + 1) % len(menuItems)
			}
			continue
		}
	}
}
