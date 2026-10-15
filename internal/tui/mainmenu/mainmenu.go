package mainmenu

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// MenuItem represents an interactive entry point in the main menu.
type MenuItem struct {
	Num         string
	Name        string
	Icon        string
	Description string
	Cmd         *cobra.Command
}

// SysSummary holds local machine metrics for the dashboard banner.
type SysSummary struct {
	Hostname  string
	OSVersion string
	DiskFree  string
	DiskTotal string
	DiskPct   float64
	Username  string
	TrashSize string
}

// Model is the Bubble Tea model for the Sift interactive home dashboard.
type Model struct {
	version     string
	sys         SysSummary
	items       []MenuItem
	cursor      int
	selectedCmd *cobra.Command
	quitting    bool
	width       int
	height      int
}

// New creates a new main menu model with the given metadata and menu items.
func New(version string, sys SysSummary, items []MenuItem) Model {
	cleanVer := strings.TrimPrefix(version, "v")
	return Model{
		version: cleanVer,
		sys:     sys,
		items:   items,
		cursor:  0,
		width:   80,
		height:  24,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if len(m.items) > 0 {
				m.cursor = (m.cursor - 1 + len(m.items)) % len(m.items)
			}

		case "down", "j":
			if len(m.items) > 0 {
				m.cursor = (m.cursor + 1) % len(m.items)
			}

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(m.items) {
				m.cursor = idx
			}

		case "0":
			// 10th item
			if len(m.items) >= 10 {
				m.cursor = 9
			}

		case "enter":
			if len(m.items) > 0 && m.cursor < len(m.items) {
				m.selectedCmd = m.items[m.cursor].Cmd
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting && m.selectedCmd == nil {
		return ""
	}

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

	var b strings.Builder

	b.WriteString(bannerStyle.Render(asciiBanner))
	b.WriteString("\n")
	b.WriteString(versionBadgeStyle.Render(fmt.Sprintf("  v%s • Terminal macOS maintenance toolkit", m.version)))
	b.WriteString("\n\n")

	var diskColor lipgloss.Color
	switch {
	case m.sys.DiskPct >= 90.0:
		diskColor = lipgloss.Color("196") // Red
	case m.sys.DiskPct >= 80.0:
		diskColor = lipgloss.Color("214") // Orange/Amber
	default:
		diskColor = lipgloss.Color("42") // Green
	}
	diskUsageStyle := lipgloss.NewStyle().Bold(true).Foreground(diskColor)

	trashCompact := ""
	trashFull := ""
	if m.sys.TrashSize != "" {
		trashCompact = fmt.Sprintf("   %s   %s %s", sepStyle.Render("│"), labelStyle.Render("Trash:"), valueStyle.Render(m.sys.TrashSize))
		trashFull = fmt.Sprintf("  %s  %s %s", sepStyle.Render("│"), labelStyle.Render("Trash:"), valueStyle.Render(m.sys.TrashSize))
	}

	var summaryContent string
	if m.width > 0 && m.width < 96 {
		// 2-row compact layout for narrower windows
		row1 := fmt.Sprintf("%s %s   %s   %s %s%s",
			labelStyle.Render(" Host:"), valueStyle.Render(m.sys.Hostname),
			sepStyle.Render("│"),
			labelStyle.Render("User:"), valueStyle.Render(m.sys.Username),
			trashCompact,
		)
		row2 := fmt.Sprintf("%s %s   %s   %s %s / %s (%s)",
			labelStyle.Render("OS:"), valueStyle.Render(m.sys.OSVersion),
			sepStyle.Render("│"),
			labelStyle.Render("Disk:"), valueStyle.Render(m.sys.DiskFree), valueStyle.Render(m.sys.DiskTotal), diskUsageStyle.Render(fmt.Sprintf("%.1f%% used", m.sys.DiskPct)),
		)
		summaryContent = row1 + "\n" + row2
	} else {
		// Single row layout for full terminals
		summaryContent = fmt.Sprintf(
			"%s %s  %s  %s %s  %s  %s %s / %s (%s)%s  %s  %s %s",
			labelStyle.Render(" Host:"),
			valueStyle.Render(m.sys.Hostname),
			sepStyle.Render("│"),
			labelStyle.Render("OS:"),
			valueStyle.Render(m.sys.OSVersion),
			sepStyle.Render("│"),
			labelStyle.Render("Disk:"),
			valueStyle.Render(m.sys.DiskFree),
			valueStyle.Render(m.sys.DiskTotal),
			diskUsageStyle.Render(fmt.Sprintf("%.1f%% used", m.sys.DiskPct)),
			trashFull,
			sepStyle.Render("│"),
			labelStyle.Render("User:"),
			valueStyle.Render(m.sys.Username),
		)
	}

	b.WriteString(boxStyle.Render(summaryContent))
	b.WriteString("\n\n")

	b.WriteString(menuTitleStyle.Render("  Shortcut Commands:"))
	b.WriteString("\n\n")

	for i, itm := range m.items {
		// Use fixed width for command name column to align emojis cleanly
		nameCol := fmt.Sprintf("%-12s", fmt.Sprintf("%s %s", itm.Icon, itm.Name))
		numCol := fmt.Sprintf("[%s]", itm.Num)

		if i == m.cursor {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Render("❯")
			num := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(numCol)
			name := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(nameCol)
			desc := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Render(itm.Description)
			line := fmt.Sprintf(" %s %-4s %s  %s", cursor, num, name, desc)
			b.WriteString(selectedItemStyle.Render(line))
		} else {
			cursor := " "
			num := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241")).Render(numCol)
			name := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(nameCol)
			desc := itemDescStyle.Render(itm.Description)
			line := fmt.Sprintf(" %s %-4s %s  %s", cursor, num, name, desc)
			b.WriteString(normalItemStyle.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("  ↑/k Up  •  ↓/j Down  •  1-0 Quick Select  •  ↵ Enter Launch  •  q/Esc Quit"))
	b.WriteString("\n")

	return b.String()
}

// SelectedCmd returns the command selected by the user, or nil if cancelled.
func (m Model) SelectedCmd() *cobra.Command {
	return m.selectedCmd
}

// IsQuitting returns true if the user decided to exit.
func (m Model) IsQuitting() bool {
	return m.quitting
}
