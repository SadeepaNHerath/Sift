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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SadeepaNHerath/Sift/internal/tui/mainmenu"
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

// runInteractiveMenu renders an interactive Bubble Tea dashboard menu with full keyboard navigation.
func runInteractiveMenu(parentCmd *cobra.Command) error {
	sys := collectSystemSummary()
	mSys := mainmenu.SysSummary{
		Hostname:  sys.hostname,
		OSVersion: sys.osVersion,
		DiskFree:  sys.diskFree,
		DiskTotal: sys.diskTotal,
		DiskPct:   sys.diskPct,
		Username:  sys.username,
	}

	menuItems := []mainmenu.MenuItem{
		{
			Num:         "1",
			Name:        "Clean",
			Icon:        "🧹",
			Description: "Clean system, app, and local AI model caches",
			Cmd:         cleanCmd,
		},
		{
			Num:         "2",
			Name:        "Uninstall",
			Icon:        "🗑️ ",
			Description: "Safely remove applications and all associated leftovers",
			Cmd:         uninstallCmd,
		},
		{
			Num:         "3",
			Name:        "Optimize",
			Icon:        "⚡",
			Description: "Rebuild caches, flush DNS, and run system optimizations",
			Cmd:         optimizeCmd,
		},
		{
			Num:         "4",
			Name:        "Analyze",
			Icon:        "📊",
			Description: "Analyze disk space usage and locate large files",
			Cmd:         analyzeCmd,
		},
		{
			Num:         "5",
			Name:        "Status",
			Icon:        "🩺",
			Description: "Check system health, disk metrics, and security overview",
			Cmd:         statusCmd,
		},
		{
			Num:         "6",
			Name:        "Purge",
			Icon:        "🔥",
			Description: "Deep purge Xcode, Docker, Homebrew, and dev caches",
			Cmd:         purgeCmd,
		},
		{
			Num:         "7",
			Name:        "Dedup",
			Icon:        "🔍",
			Description: "Find and safely trash duplicate files with 3-phase xxhash",
			Cmd:         dedupCmd,
		},
		{
			Num:         "8",
			Name:        "Startup",
			Icon:        "🚀",
			Description: "Manage persistent background services and zombie daemons",
			Cmd:         startupCmd,
		},
		{
			Num:         "9",
			Name:        "Privacy",
			Icon:        "🛡️ ",
			Description: "Wipe recent items, clipboard history, and crash reports",
			Cmd:         privacyCmd,
		},
		{
			Num:         "0",
			Name:        "Doctor",
			Icon:        "🩺",
			Description: "Run full health diagnostics with actionable repairs",
			Cmd:         doctorCmd,
		},
	}

	m := mainmenu.New(version, mSys, menuItems)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return parentCmd.Help()
	}

	finalM, ok := finalModel.(mainmenu.Model)
	if !ok || finalM.SelectedCmd() == nil {
		return nil
	}

	chosen := finalM.SelectedCmd()
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(fmt.Sprintf("▸ Sift: Running sf %s...\n", chosen.Name())))
	if chosen.RunE != nil {
		return chosen.RunE(chosen, []string{})
	}
	if chosen.Run != nil {
		chosen.Run(chosen, []string{})
	}
	return nil
}
