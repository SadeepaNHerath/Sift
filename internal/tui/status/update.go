package status

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles incoming Bubble Tea events: keyboard navigation, ticks, and window resize.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "Q", "esc", "ctrl+c":
			m.Quitting = true
			return m, tea.Quit

		case "k", "K":
			// Easter egg: toggle ASCII cat mascot and persist preference
			m.ShowCat = !m.ShowCat
			if m.Prefs != nil {
				m.Prefs.ShowCat = m.ShowCat
				_ = m.Prefs.Save()
			}
			return m, nil

		case "c", "C":
			// Cycle CPU core limit: 2 -> 4 -> 8 -> 0 (all) -> 2
			switch m.CoreLimit {
			case 2:
				m.CoreLimit = 4
			case 4:
				m.CoreLimit = 8
			case 8:
				m.CoreLimit = 0 // all
			default:
				m.CoreLimit = 2
			}
			if m.Prefs != nil {
				m.Prefs.CPUCores = m.CoreLimit
				_ = m.Prefs.Save()
			}
			return m, nil

		case "r", "R":
			// Immediate manual refresh
			return m, fetchMetricsCmd()
		}

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case metricsMsg:
		if msg.err == nil && msg.metrics != nil {
			m.Metrics = msg.metrics
			m.Err = nil
			m.CatFrame++
		} else if msg.err != nil {
			m.Err = msg.err
		}
		// Schedule next sampling tick
		return m, tickCmd(m.Interval)

	case tickMsg:
		// Execute sampling in background
		return m, fetchMetricsCmd()
	}

	return m, nil
}
