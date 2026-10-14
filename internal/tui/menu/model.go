package menu

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// Item represents a single selectable row in the menu.
type Item struct {
	Title       string
	Description string
	Size        int64  // optional, for showing file sizes
	Path        string // optional, for file items
	Selected    bool
	Protected   bool   // cannot be selected
	Category    string // optional grouping
}

// Styles holds all lipgloss styles used for rendering the menu.
type Styles struct {
	Title     lipgloss.Style
	Normal    lipgloss.Style
	Selected  lipgloss.Style
	Cursor    lipgloss.Style
	Protected lipgloss.Style
	Size      lipgloss.Style
	Search    lipgloss.Style
	Help      lipgloss.Style
	Category  lipgloss.Style
}

// DefaultStyles returns the default modern color scheme for Sift.
func DefaultStyles() Styles {
	return Styles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#BD93F9")).
			MarginBottom(1),
		Normal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8F8F2")),
		Selected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#50FA7B")),
		Cursor: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF79C6")),
		Protected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB86C")),
		Size: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8BE9FD")),
		Search: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F1FA8C")),
		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4")),
		Category: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#BD93F9")),
	}
}

// Model represents the Bubble Tea state for the paginated menu component.
type Model struct {
	items       []Item
	cursor      int
	offset      int // scroll offset
	height      int // visible rows
	width       int
	searching   bool
	searchQuery string
	filtered    []int // indices into items matching search
	title       string
	multiSelect bool
	showSizes   bool
	confirmed   bool
	cancelled   bool
	styles      Styles
	gPending    bool // tracks vim "gg" key sequence
}

// Option configures a Model instance.
type Option func(*Model)

// WithMultiSelect enables multi-item selection.
func WithMultiSelect() Option {
	return func(m *Model) {
		m.multiSelect = true
	}
}

// WithSizes enables displaying right-aligned file sizes.
func WithSizes() Option {
	return func(m *Model) {
		m.showSizes = true
	}
}

// WithHeight sets the maximum visible item rows.
func WithHeight(h int) Option {
	return func(m *Model) {
		if h > 0 {
			m.height = h
		}
	}
}

// WithWidth sets the initial layout width.
func WithWidth(w int) Option {
	return func(m *Model) {
		if w > 0 {
			m.width = w
		}
	}
}

// WithStyles overrides default menu styling.
func WithStyles(s Styles) Option {
	return func(m *Model) {
		m.styles = s
	}
}

// New creates and initializes a new paginated menu Model.
func New(title string, items []Item, opts ...Option) Model {
	m := Model{
		title:       title,
		items:       items,
		cursor:      0,
		offset:      0,
		height:      10,
		width:       80,
		searching:   false,
		searchQuery: "",
		multiSelect: false,
		showSizes:   false,
		confirmed:   false,
		cancelled:   false,
		styles:      DefaultStyles(),
	}

	m.initFiltered()

	for _, opt := range opts {
		opt(&m)
	}

	m.clampCursor()
	m.adjustOffset()

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// SelectedItems returns all items that have Selected set to true.
func (m Model) SelectedItems() []Item {
	var selected []Item
	for _, it := range m.items {
		if it.Selected {
			selected = append(selected, it)
		}
	}
	return selected
}

// IsConfirmed returns true if the user confirmed their choice by pressing Enter.
func (m Model) IsConfirmed() bool {
	return m.confirmed
}

// IsCancelled returns true if the user cancelled or exited the menu without confirming.
func (m Model) IsCancelled() bool {
	return m.cancelled
}

// Items returns a copy of all items in the model.
func (m Model) Items() []Item {
	res := make([]Item, len(m.items))
	copy(res, m.items)
	return res
}

// Cursor returns the index of the currently highlighted item within the filtered list.
func (m Model) Cursor() int {
	return m.cursor
}

// initFiltered initializes the filtered slice with all item indices.
func (m *Model) initFiltered() {
	m.filtered = make([]int, len(m.items))
	for i := range m.items {
		m.filtered[i] = i
	}
}

// updateFiltered filters items based on the active search query.
func (m *Model) updateFiltered() {
	query := strings.TrimSpace(strings.ToLower(m.searchQuery))
	if query == "" {
		m.initFiltered()
		return
	}

	var matches []int
	for i, it := range m.items {
		if strings.Contains(strings.ToLower(it.Title), query) ||
			strings.Contains(strings.ToLower(it.Description), query) ||
			strings.Contains(strings.ToLower(it.Category), query) ||
			strings.Contains(strings.ToLower(it.Path), query) {
			matches = append(matches, i)
		}
	}
	m.filtered = matches
}

// clampCursor ensures the cursor stays within valid bounds of the filtered slice.
func (m *Model) clampCursor() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

// adjustOffset ensures the cursor is visible within the viewport [offset, offset + height).
func (m *Model) adjustOffset() {
	if len(m.filtered) == 0 {
		m.offset = 0
		return
	}
	h := m.height
	if h <= 0 {
		h = 10
	}

	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}

	if m.offset < 0 {
		m.offset = 0
	}
	maxOffset := len(m.filtered) - h
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}

// FormatBytes formats byte counts into human-readable strings via core.FormatBytes.
func FormatBytes(b int64) string {
	return core.FormatBytes(b)
}
