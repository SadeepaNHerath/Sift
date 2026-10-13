package analyze

import (
	"fmt"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// Update handles Bubble Tea messages and state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.adjustOffset()
		return m, nil

	case scanCompleteMsg:
		m.Scanning = false
		if msg.Err != nil {
			m.StatusMsg = fmt.Sprintf("Scan error: %v", msg.Err)
		} else if msg.Root != nil {
			m.RootEntry = msg.Root
			m.Items = msg.Root.Children
			m.CurrentPath = msg.Path
		}
		m.updateFiltered()
		m.clampCursor()
		m.adjustOffset()
		return m, nil

	case topFilesCompleteMsg:
		m.Scanning = false
		if msg.Err != nil {
			m.StatusMsg = fmt.Sprintf("Top files error: %v", msg.Err)
		} else {
			m.TopFiles = msg.Files
			m.Items = msg.Files
		}
		m.updateFiltered()
		m.clampCursor()
		m.adjustOffset()
		return m, nil

	case deleteFinishedMsg:
		m.ConfirmingDelete = false
		m.DeleteTargets = nil
		m.Selected = make(map[string]bool)
		if msg.Err != nil {
			m.StatusMsg = fmt.Sprintf("Moved %d item(s) to Trash (%s freed). Error: %v",
				msg.Count, formatBytes(msg.Freed), msg.Err)
		} else {
			m.StatusMsg = fmt.Sprintf("Successfully moved %d item(s) to Trash (%s freed)",
				msg.Count, formatBytes(msg.Freed))
		}

		m.Scanning = true
		if m.ShowTopFiles {
			return m, m.startTopFilesCmd(m.CurrentPath, m.TopCount)
		}
		return m, m.startScanCmd(m.CurrentPath)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey processes keyboard input according to the active mode.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Quit application
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// 1. Confirmation Modal Mode
	if m.ConfirmingDelete {
		switch key {
		case "y", "Y":
			return m, m.executeDelete()
		case "n", "N", "esc":
			m.ConfirmingDelete = false
			m.DeleteTargets = nil
			return m, nil
		default:
			return m, nil
		}
	}

	// 2. Search / Live Filter Mode
	if m.Searching {
		switch key {
		case "esc":
			m.Searching = false
			m.SearchQuery = ""
			m.updateFiltered()
			m.clampCursor()
			m.adjustOffset()
			return m, nil
		case "enter":
			m.Searching = false
			return m, nil
		case "backspace":
			if len(m.SearchQuery) > 0 {
				m.SearchQuery = m.SearchQuery[:len(m.SearchQuery)-1]
				m.updateFiltered()
				m.clampCursor()
				m.adjustOffset()
			}
			return m, nil
		case "up":
			m.Cursor--
			m.clampCursor()
			m.adjustOffset()
			return m, nil
		case "down":
			m.Cursor++
			m.clampCursor()
			m.adjustOffset()
			return m, nil
		default:
			// Append character if single rune
			if len(msg.Runes) > 0 {
				m.SearchQuery += string(msg.Runes)
				m.updateFiltered()
				m.clampCursor()
				m.adjustOffset()
			}
			return m, nil
		}
	}

	// 3. Normal Directory Exploration Mode
	switch key {
	case "q":
		return m, tea.Quit

	case "up", "k":
		m.Cursor--
		m.clampCursor()
		m.adjustOffset()
		return m, nil

	case "down", "j":
		m.Cursor++
		m.clampCursor()
		m.adjustOffset()
		return m, nil

	case "enter", "right", "l":
		return m.handleDrillDown()

	case "esc", "left", "h", "b":
		return m.handleGoUp()

	case "t":
		m.ShowTopFiles = !m.ShowTopFiles
		m.Cursor = 0
		m.Offset = 0
		m.SearchQuery = ""
		m.Scanning = true
		m.StatusMsg = ""
		if m.ShowTopFiles {
			return m, m.startTopFilesCmd(m.CurrentPath, m.TopCount)
		}
		return m, m.startScanCmd(m.CurrentPath)

	case "/":
		m.Searching = true
		return m, nil

	case " ":
		// Toggle multi-selection on current item
		if len(m.Filtered) > 0 {
			item := m.Items[m.Filtered[m.Cursor]]
			if m.Selected[item.Path] {
				delete(m.Selected, item.Path)
			} else {
				m.Selected[item.Path] = true
			}
		}
		return m, nil

	case "o":
		if len(m.Filtered) > 0 {
			item := m.Items[m.Filtered[m.Cursor]]
			cmd := exec.Command("open", item.Path)
			_ = cmd.Start()
			m.StatusMsg = fmt.Sprintf("Opened %s in default application", item.Name)
		}
		return m, nil

	case "f":
		if len(m.Filtered) > 0 {
			item := m.Items[m.Filtered[m.Cursor]]
			cmd := exec.Command("open", "-R", item.Path)
			_ = cmd.Start()
			m.StatusMsg = fmt.Sprintf("Revealed %s in Finder", item.Name)
		}
		return m, nil

	case "p":
		if len(m.Filtered) > 0 {
			item := m.Items[m.Filtered[m.Cursor]]
			cmd := exec.Command("qlmanage", "-p", item.Path)
			_ = cmd.Start()
			m.StatusMsg = fmt.Sprintf("Quick Look preview for %s", item.Name)
		}
		return m, nil

	case "delete", "backspace", "d":
		m.prepareDelete()
		return m, nil

	case "r":
		m.Scanning = true
		m.StatusMsg = "Rescanning directory..."
		if m.ShowTopFiles {
			return m, m.startTopFilesCmd(m.CurrentPath, m.TopCount)
		}
		return m, m.startScanCmd(m.CurrentPath)
	}

	return m, nil
}

// handleDrillDown opens the directory under the cursor or executes default application for files.
func (m Model) handleDrillDown() (tea.Model, tea.Cmd) {
	if len(m.Filtered) == 0 {
		return m, nil
	}

	item := m.Items[m.Filtered[m.Cursor]]
	if item.IsDir {
		m.History = append(m.History, HistoryItem{
			Path:   m.CurrentPath,
			Cursor: m.Cursor,
			Offset: m.Offset,
		})
		m.CurrentPath = item.Path
		m.Cursor = 0
		m.Offset = 0
		m.SearchQuery = ""
		m.Searching = false
		m.Selected = make(map[string]bool)
		m.Scanning = true
		m.StatusMsg = ""
		if m.ShowTopFiles {
			return m, m.startTopFilesCmd(m.CurrentPath, m.TopCount)
		}
		return m, m.startScanCmd(m.CurrentPath)
	}

	// Leaf file: open with default app
	cmd := exec.Command("open", item.Path)
	_ = cmd.Start()
	m.StatusMsg = fmt.Sprintf("Opened %s", item.Name)
	return m, nil
}

// handleGoUp navigates to the parent directory or previous history item.
func (m Model) handleGoUp() (tea.Model, tea.Cmd) {
	if m.SearchQuery != "" {
		m.SearchQuery = ""
		m.updateFiltered()
		m.clampCursor()
		m.adjustOffset()
		return m, nil
	}

	if m.ShowTopFiles {
		m.ShowTopFiles = false
		m.Cursor = 0
		m.Offset = 0
		m.Scanning = true
		return m, m.startScanCmd(m.CurrentPath)
	}

	if len(m.History) > 0 {
		prev := m.History[len(m.History)-1]
		m.History = m.History[:len(m.History)-1]
		m.CurrentPath = prev.Path
		m.Cursor = prev.Cursor
		m.Offset = prev.Offset
		m.SearchQuery = ""
		m.Searching = false
		m.Selected = make(map[string]bool)
		m.Scanning = true
		m.StatusMsg = ""
		return m, m.startScanCmd(m.CurrentPath)
	}

	parent := filepath.Dir(m.CurrentPath)
	if parent != m.CurrentPath {
		m.CurrentPath = parent
		m.Cursor = 0
		m.Offset = 0
		m.SearchQuery = ""
		m.Searching = false
		m.Selected = make(map[string]bool)
		m.Scanning = true
		m.StatusMsg = ""
		return m, m.startScanCmd(m.CurrentPath)
	}

	return m, nil
}
