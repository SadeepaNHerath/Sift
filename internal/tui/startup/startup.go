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
	allServices []engine.LaunchService
	filtered    []engine.LaunchService
	cursor      int
	filterMode  string // "all", "zombies", "user", "global", "system"
	statusMsg   string
	width       int
	height      int
	quitting    bool
}

// New creates a new startup dashboard model.
func New(services []engine.LaunchService) Model {
	m := Model{
		allServices: services,
		cursor:      0,
		filterMode:  "all",
		width:       80,
		height:      20,
	}
	m.applyFilter()
	return m
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
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
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
				curr := &m.filtered[m.cursor]
				newLoaded := !curr.Loaded
				err := engine.ToggleLaunchService(*curr, newLoaded)
				if err == nil {
					curr.Loaded = newLoaded
					m.statusMsg = fmt.Sprintf("Service %s toggled (loaded: %v)", curr.Label, newLoaded)
				} else {
					m.statusMsg = fmt.Sprintf("Toggle error: %v", err)
				}
			}

		case "x", "X":
			if len(m.filtered) > 0 {
				curr := m.filtered[m.cursor]
				if curr.IsZombie {
					err := engine.RemoveZombiePlist(curr, nil)
					if err == nil {
						m.statusMsg = fmt.Sprintf("Trashed zombie plist: %s", curr.Label)
						// Refresh
						m.removeCurrentFromAll()
						m.applyFilter()
					} else {
						m.statusMsg = fmt.Sprintf("Failed to remove plist: %v", err)
					}
				} else {
					m.statusMsg = "Cannot remove plist: only Zombie services can be removed here"
				}
			}
		}
	}

	return m, nil
}

func (m *Model) removeCurrentFromAll() {
	if m.cursor >= len(m.filtered) {
		return
	}
	targetPath := m.filtered[m.cursor].PlistPath
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

	// Render scrollable viewport
	startIdx := max(0, m.cursor-7)
	endIdx := min(len(m.filtered), startIdx+15)

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
		info := fmt.Sprintf("Total Services: %d  │  Zombies: %d  │  Filter: %s",
			len(m.allServices),
			zombieCount,
			strings.ToUpper(m.filterMode),
		)
		s.WriteString(dimStyle.Render("  "+info) + "\n")
	}

	return s.String()
}
