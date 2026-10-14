package startup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SadeepaNHerath/Spacefold/internal/engine"
)

// Model represents the interactive startup & launchd service manager dashboard.
type Model struct {
	allServices  []engine.LaunchService
	filtered     []engine.LaunchService
	cursor       int
	scrollOffset int
	filterMode   string // "all", "zombies", "user", "global", "system"
	statusMsg    string
	width        int
	height       int
	quitting     bool
}

type toggleServiceMsg struct {
	label  string
	loaded bool
	err    error
}

type removeZombieMsg struct {
	label     string
	plistPath string
	err       error
}

func toggleServiceCmd(svc engine.LaunchService, newLoaded bool) tea.Cmd {
	return func() tea.Msg {
		err := engine.ToggleLaunchService(svc, newLoaded)
		return toggleServiceMsg{
			label:  svc.Label,
			loaded: newLoaded,
			err:    err,
		}
	}
}

func removeZombieCmd(svc engine.LaunchService) tea.Cmd {
	return func() tea.Msg {
		err := engine.RemoveZombiePlist(svc, nil)
		return removeZombieMsg{
			label:     svc.Label,
			plistPath: svc.PlistPath,
			err:       err,
		}
	}
}

// New creates a new startup dashboard model.
func New(services []engine.LaunchService) Model {
	m := Model{
		allServices: services,
		cursor:      0,
		filterMode:  "all",
		width:       80,
		height:      24,
	}
	m.applyFilter()
	return m
}

func (m Model) visibleRows() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	v := h - 9
	if v < 5 {
		v = 5
	}
	return v
}

func (m *Model) adjustScroll() {
	visible := m.visibleRows()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+visible {
		m.scrollOffset = m.cursor - visible + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) applyFilter() {
	var res []engine.LaunchService
	for _, s := range m.allServices {
		switch m.filterMode {
		case "zombies":
			if s.IsZombie {
				res = append(res, s)
			}
		case "user":
			if s.Level == engine.LevelUserAgent {
				res = append(res, s)
			}
		case "global":
			if s.Level == engine.LevelGlobalAgent {
				res = append(res, s)
			}
		case "system":
			if s.Level == engine.LevelSystemDaemon {
				res = append(res, s)
			}
		default:
			res = append(res, s)
		}
	}
	m.filtered = res
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.adjustScroll()
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustScroll()
		return m, nil

	case toggleServiceMsg:
		if msg.err == nil {
			m.statusMsg = fmt.Sprintf("Service %s toggled (loaded: %v)", msg.label, msg.loaded)
			for i := range m.allServices {
				if m.allServices[i].Label == msg.label {
					m.allServices[i].Loaded = msg.loaded
				}
			}
			for i := range m.filtered {
				if m.filtered[i].Label == msg.label {
					m.filtered[i].Loaded = msg.loaded
				}
			}
		} else {
			m.statusMsg = fmt.Sprintf("Toggle error: %v", msg.err)
		}
		return m, nil

	case removeZombieMsg:
		if msg.err == nil {
			m.statusMsg = fmt.Sprintf("Trashed zombie plist: %s", msg.label)
			m.removePathFromAll(msg.plistPath)
			m.applyFilter()
		} else {
			m.statusMsg = fmt.Sprintf("Failed to remove plist: %v", msg.err)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}

		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.adjustScroll()
			}

		case "z", "Z":
			if m.filterMode == "zombies" {
				m.filterMode = "all"
			} else {
				m.filterMode = "zombies"
			}
			m.applyFilter()

		case "u", "U":
			m.filterMode = "user"
			m.applyFilter()

		case "g", "G":
			m.filterMode = "global"
			m.applyFilter()

		case "s", "S":
			m.filterMode = "system"
			m.applyFilter()

		case "a", "A":
			m.filterMode = "all"
			m.applyFilter()

		case " ":
			if len(m.filtered) > 0 {
				curr := m.filtered[m.cursor]
				newLoaded := !curr.Loaded
				m.statusMsg = fmt.Sprintf("Toggling service %s...", curr.Label)
				return m, toggleServiceCmd(curr, newLoaded)
			}

		case "x", "X":
			if len(m.filtered) > 0 {
				curr := m.filtered[m.cursor]
				if curr.IsZombie {
					m.statusMsg = fmt.Sprintf("Trashing zombie plist %s...", curr.Label)
					return m, removeZombieCmd(curr)
				}
				m.statusMsg = "Cannot remove plist: only Zombie services can be removed here"
			}
		}
	}

	return m, nil
}

func (m *Model) removePathFromAll(targetPath string) {
	var updated []engine.LaunchService
	for _, s := range m.allServices {
		if s.PlistPath != targetPath {
			updated = append(updated, s)
		}
	}
	m.allServices = updated
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	runningStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	stoppedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	zombieStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	levelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("141"))

	var s strings.Builder
	s.WriteString(titleStyle.Render("🚀 Sift Startup & Background Service Manager") + "\n")
	s.WriteString(dimStyle.Render("  [Space] Toggle • [x] Trash Zombie • [z] Filter Zombies • [u/g/s/a] Filter Levels • [q] Exit") + "\n\n")

	if len(m.filtered) == 0 {
		s.WriteString("  No matching services found for filter: " + m.filterMode + "\n")
	}

	// Render scrollable viewport based on window height
	visible := m.visibleRows()
	startIdx := m.scrollOffset
	endIdx := startIdx + visible
	if endIdx > len(m.filtered) {
		endIdx = len(m.filtered)
	}

	for i := startIdx; i < endIdx; i++ {
		svc := m.filtered[i]
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("▶ ")
		}

		var statusBadge string
		if svc.IsZombie {
			statusBadge = zombieStyle.Render("⚠ ZOMBIE (Binary Missing)")
		} else if svc.Loaded {
			if svc.PID > 0 {
				statusBadge = runningStyle.Render(fmt.Sprintf("● RUNNING (PID %d)", svc.PID))
			} else {
				statusBadge = runningStyle.Render("● LOADED")
			}
		} else {
			statusBadge = stoppedStyle.Render("○ STOPPED")
		}

		line := fmt.Sprintf("%s%-35s  %-14s  %s\n",
			cursor,
			svc.Label,
			levelStyle.Render(string(svc.Level)),
			statusBadge,
		)
		s.WriteString(line)

		if i == m.cursor {
			var binInfo string
			if svc.BinaryPath != "" {
				binInfo = fmt.Sprintf("     Path: %s", svc.BinaryPath)
			} else {
				binInfo = fmt.Sprintf("     Plist: %s", svc.PlistPath)
			}
			s.WriteString(dimStyle.Render(binInfo) + "\n")
		}
	}

	s.WriteString("\n" + strings.Repeat("─", 72) + "\n")
	if m.statusMsg != "" {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("  "+m.statusMsg) + "\n")
	} else {
		zombieCount := 0
		for _, s := range m.allServices {
			if s.IsZombie {
				zombieCount++
			}
		}
		scrollInfo := ""
		if len(m.filtered) > visible {
			scrollInfo = fmt.Sprintf("  │  [%d-%d of %d]", startIdx+1, endIdx, len(m.filtered))
		}
		info := fmt.Sprintf("Total Services: %d  │  Zombies: %d  │  Filter: %s%s",
			len(m.allServices),
			zombieCount,
			strings.ToUpper(m.filterMode),
			scrollInfo,
		)
		s.WriteString(dimStyle.Render("  "+info) + "\n")
	}

	return s.String()
}
