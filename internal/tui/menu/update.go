package menu

import (
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// Update handles Bubble Tea messages and updates menu state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		// Adjust visible rows to fit the terminal window dynamically
		if msg.Height > 6 {
			m.height = msg.Height - 6
		}
		m.clampCursor()
		m.adjustOffset()
		return m, nil

	case tea.KeyMsg:
		if m.searching {
			return m.handleSearchKeys(msg)
		}
		return m.handleNormalKeys(msg)
	}

	return m, nil
}

// handleNormalKeys handles keyboard events when not actively typing a search query.
func (m Model) handleNormalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "up", "k":
		m.gPending = false
		if m.cursor > 0 {
			m.cursor--
			m.adjustOffset()
		}
		return m, nil

	case "down", "j":
		m.gPending = false
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.adjustOffset()
		}
		return m, nil

	case " ":
		m.gPending = false
		if len(m.filtered) == 0 {
			return m, nil
		}
		itemIdx := m.filtered[m.cursor]
		if m.items[itemIdx].Protected {
			return m, nil
		}

		if m.multiSelect {
			m.items[itemIdx].Selected = !m.items[itemIdx].Selected
		} else {
			wasSelected := m.items[itemIdx].Selected
			for i := range m.items {
				m.items[i].Selected = false
			}
			m.items[itemIdx].Selected = !wasSelected
		}
		return m, nil

	case "a":
		m.gPending = false
		if !m.multiSelect {
			return m, nil
		}
		// Check if all non-protected items are already selected
		allSelected := true
		hasSelectable := false
		for _, it := range m.items {
			if !it.Protected {
				hasSelectable = true
				if !it.Selected {
					allSelected = false
					break
				}
			}
		}

		if !hasSelectable {
			return m, nil
		}

		// Toggle select-all / deselect-all
		target := !allSelected
		for i := range m.items {
			if !m.items[i].Protected {
				m.items[i].Selected = target
			}
		}
		return m, nil

	case "i":
		m.gPending = false
		if !m.multiSelect {
			return m, nil
		}
		// Invert selection for non-protected items
		for i := range m.items {
			if !m.items[i].Protected {
				m.items[i].Selected = !m.items[i].Selected
			}
		}
		return m, nil

	case "/":
		m.gPending = false
		m.searching = true
		return m, nil

	case "g":
		if m.gPending {
			// Second 'g' in "gg" sequence: jump to top
			m.cursor = 0
			m.adjustOffset()
			m.gPending = false
		} else {
			m.gPending = true
		}
		return m, nil

	case "G":
		m.gPending = false
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
			m.adjustOffset()
		}
		return m, nil

	case "home":
		m.gPending = false
		m.cursor = 0
		m.adjustOffset()
		return m, nil

	case "end":
		m.gPending = false
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
			m.adjustOffset()
		}
		return m, nil

	case "pgup", "pageup", "ctrl+u":
		m.gPending = false
		m.cursor -= m.height
		m.clampCursor()
		m.adjustOffset()
		return m, nil

	case "pgdown", "pagedown", "ctrl+d":
		m.gPending = false
		m.cursor += m.height
		m.clampCursor()
		m.adjustOffset()
		return m, nil

	case "enter":
		m.gPending = false
		m.confirmed = true
		m.cancelled = false
		return m, tea.Quit

	case "q", "esc", "ctrl+c":
		m.gPending = false
		m.cancelled = true
		m.confirmed = false
		return m, tea.Quit

	default:
		m.gPending = false
		return m, nil
	}
}

// handleSearchKeys handles keyboard input when actively in search mode.
func (m Model) handleSearchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		// Exit search and restore full item list
		m.searching = false
		m.searchQuery = ""
		m.initFiltered()
		m.clampCursor()
		m.adjustOffset()
		return m, nil

	case "enter":
		// Finish typing; keep filtered items in view
		m.searching = false
		return m, nil

	case "backspace":
		runes := []rune(m.searchQuery)
		if len(runes) > 0 {
			m.searchQuery = string(runes[:len(runes)-1])
			m.updateFiltered()
			m.clampCursor()
			m.adjustOffset()
		}
		return m, nil

	case "up", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
			m.adjustOffset()
		}
		return m, nil

	case "down", "ctrl+n":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.adjustOffset()
		}
		return m, nil

	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit

	default:
		// Append regular printable runes to search query
		runes := []rune(key)
		if len(runes) == 1 && (unicode.IsPrint(runes[0]) || unicode.IsGraphic(runes[0])) {
			m.searchQuery += key
			m.updateFiltered()
			m.clampCursor()
			m.adjustOffset()
		}
		return m, nil
	}
}
