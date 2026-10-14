package dedup

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
)

// Item represents a single file row in the duplicate explorer.
type Item struct {
	GroupIndex int
	FileIndex  int
	File       engine.DuplicateFile
	Selected   bool
	IsOldest   bool
	IsNewest   bool
}

// Model is the Bubble Tea model for interactive duplicate file resolution.
type Model struct {
	groups       []engine.DuplicateGroup
	items        []Item
	cursor       int
	scrollOffset int
	confirmed    bool
	cancelled    bool
	width        int
	height       int
	statusMsg    string
}

// New creates a new interactive duplicate resolution TUI model.
func New(groups []engine.DuplicateGroup) Model {
	var items []Item

	for gIdx, g := range groups {
		for fIdx, f := range g.Files {
			isOld := (fIdx == 0)
			isNew := (fIdx == len(g.Files)-1)
			// By default, select all except the oldest
			items = append(items, Item{
				GroupIndex: gIdx,
				FileIndex:  fIdx,
				File:       f,
				Selected:   !isOld,
				IsOldest:   isOld,
				IsNewest:   isNew,
			})
		}
	}

	return Model{
		groups: groups,
		items:  items,
		cursor: 0,
		height: 24,
		width:  80,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustScroll()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.adjustScroll()
			}

		case " ":
			if len(m.items) > 0 {
				curr := &m.items[m.cursor]
				if !curr.Selected {
					// Check if selecting this item would select ALL files in its group
					groupAllSelected := true
					for i, itm := range m.items {
						if itm.GroupIndex == curr.GroupIndex && i != m.cursor {
							if !itm.Selected {
								groupAllSelected = false
								break
							}
						}
					}
					if groupAllSelected {
						m.statusMsg = "⚠️  Cannot select all copies: at least 1 copy in group must be kept!"
						return m, nil
					}
				}
				curr.Selected = !curr.Selected
				m.statusMsg = ""
			}

		case "o", "O":
			// Select all except the oldest in each group
			for i := range m.items {
				m.items[i].Selected = !m.items[i].IsOldest
			}
			m.statusMsg = ""

		case "n", "N":
			// Select all except the newest in each group
			for i := range m.items {
				m.items[i].Selected = !m.items[i].IsNewest
			}
			m.statusMsg = ""

		case "a", "A":
			// Select all except the oldest in each group, or deselect all if already selected
			hasUnselected := false
			for _, itm := range m.items {
				if !itm.IsOldest && !itm.Selected {
					hasUnselected = true
					break
				}
			}
			for i := range m.items {
				if m.items[i].IsOldest {
					m.items[i].Selected = false
				} else {
					m.items[i].Selected = hasUnselected
				}
			}
			m.statusMsg = ""

		case "p", "P":
			// Trigger macOS QuickLook in background
			if len(m.items) > 0 {
				currPath := m.items[m.cursor].File.Path
				_ = exec.Command("qlmanage", "-p", currPath).Start()
			}

		case "enter":
			m.confirmed = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.cancelled {
		return "Duplicate resolution cancelled.\n"
	}
	if m.confirmed {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	keptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	groupHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	warnStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

	var s strings.Builder
	s.WriteString(titleStyle.Render("🔍 Sift Duplicate File Finder — Resolve Duplicates") + "\n")
	s.WriteString(dimStyle.Render("  [Space] Toggle • [o] Keep Oldest • [n] Keep Newest • [a] Toggle All • [p] QuickLook • [Enter] Trash • [q] Cancel") + "\n\n")

	// Calculate selected size
	var selectedBytes int64
	var selectedCount int
	for _, itm := range m.items {
		if itm.Selected {
			selectedBytes += itm.File.Size
			selectedCount++
		}
	}

	visible := m.visibleRows()
	startIdx := m.scrollOffset
	endIdx := startIdx + visible
	if endIdx > len(m.items) {
		endIdx = len(m.items)
	}

	currentGroup := -1
	for idx := startIdx; idx < endIdx; idx++ {
		itm := m.items[idx]
		if itm.GroupIndex != currentGroup {
			currentGroup = itm.GroupIndex
			grp := m.groups[currentGroup]
			s.WriteString(groupHeaderStyle.Render(fmt.Sprintf("▸ Duplicate Set #%d (%s each • %d files • %s reclaimable)",
				currentGroup+1,
				core.FormatBytes(grp.Size),
				len(grp.Files),
				core.FormatBytes(grp.Reclaimable),
			)) + "\n")
		}

		cursor := "  "
		if idx == m.cursor {
			cursor = cursorStyle.Render("▶ ")
		}

		var check string
		if itm.Selected {
			check = selectedStyle.Render("[✗]") // Red X = will be deleted
		} else {
			check = keptStyle.Render("[✓]") // Green check = kept
		}

		var tag string
		if itm.IsOldest {
			tag = keptStyle.Render("(Oldest)")
		} else if itm.IsNewest {
			tag = dimStyle.Render("(Newest)")
		}

		dateStr := itm.File.ModTime.Format("2006-01-02 15:04")
		line := fmt.Sprintf("%s%s %s %s  %s\n",
			cursor,
			check,
			itm.File.Path,
			dimStyle.Render(dateStr),
			tag,
		)
		s.WriteString(line)
	}

	if m.statusMsg != "" {
		s.WriteString("\n" + warnStyle.Render(m.statusMsg))
	}

	s.WriteString("\n" + strings.Repeat("─", 72) + "\n")
	scrollIndicator := ""
	if len(m.items) > visible {
		scrollIndicator = dimStyle.Render(fmt.Sprintf("  │  [%d-%d of %d]", startIdx+1, endIdx, len(m.items)))
	}

	statusLine := fmt.Sprintf("Selected: %d files to Trash  │  Reclaimable: %s%s",
		selectedCount,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")).Render(core.FormatBytes(selectedBytes)),
		scrollIndicator,
	)
	s.WriteString(statusLine + "\n")

	return s.String()
}

// SelectedPaths returns the list of file paths chosen for deletion,
// strictly enforcing that at least one copy in every duplicate group is preserved.
func (m Model) SelectedPaths() []string {
	keptPerGroup := make(map[int]int)
	for _, itm := range m.items {
		if !itm.Selected {
			keptPerGroup[itm.GroupIndex]++
		}
	}

	var paths []string
	for _, itm := range m.items {
		if itm.Selected {
			// Safety invariant: preserve at least one file per group
			if keptPerGroup[itm.GroupIndex] == 0 && itm.IsOldest {
				continue
			}
			paths = append(paths, itm.File.Path)
		}
	}
	return paths
}

// IsCancelled returns true if the user cancelled the dialog.
func (m Model) IsCancelled() bool {
	return m.cancelled
}

// IsConfirmed returns true if the user pressed Enter to confirm.
func (m Model) IsConfirmed() bool {
	return m.confirmed
}
