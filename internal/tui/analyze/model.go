package analyze

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

// HistoryItem stores a previously visited directory state for back-navigation.
type HistoryItem struct {
	Path   string
	Cursor int
	Offset int
}

// Styles holds Lipgloss styling rules for the analyzer TUI.
type Styles struct {
	Title         lipgloss.Style
	Breadcrumb    lipgloss.Style
	DiskUsage     lipgloss.Style
	Normal        lipgloss.Style
	Selected      lipgloss.Style
	Cursor        lipgloss.Style
	FolderIcon    lipgloss.Style
	FileIcon      lipgloss.Style
	SizeBarFilled lipgloss.Style
	SizeBarEmpty  lipgloss.Style
	SizeText      lipgloss.Style
	CountText     lipgloss.Style
	SearchPrompt  lipgloss.Style
	SearchQuery   lipgloss.Style
	Help          lipgloss.Style
	StatusMsg     lipgloss.Style
	ModalBox      lipgloss.Style
	ModalTitle    lipgloss.Style
	ModalWarn     lipgloss.Style
	ModalKeys     lipgloss.Style
}

// DefaultStyles returns modern styling for the analyzer.
func DefaultStyles() Styles {
	return Styles{
		Title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")),
		Breadcrumb:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8BE9FD")),
		DiskUsage:     lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")),
		Normal:        lipgloss.NewStyle().Foreground(lipgloss.Color("#F8F8F2")),
		Selected:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")),
		Cursor:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF79C6")),
		FolderIcon:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C")),
		FileIcon:      lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD")),
		SizeBarFilled: lipgloss.NewStyle().Foreground(lipgloss.Color("#BD93F9")),
		SizeBarEmpty:  lipgloss.NewStyle().Foreground(lipgloss.Color("#44475A")),
		SizeText:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")),
		CountText:     lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")),
		SearchPrompt:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F1FA8C")),
		SearchQuery:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")),
		Help:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")),
		StatusMsg:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")),
		ModalBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF5555")).
			Padding(1, 2),
		ModalTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")),
		ModalWarn:  lipgloss.NewStyle().Foreground(lipgloss.Color("#F8F8F2")),
		ModalKeys:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")),
	}
}

// Model represents the state of the Interactive Disk Analyzer TUI.
type Model struct {
	CurrentPath string
	RootEntry   *engine.FileEntry
	Items       []engine.FileEntry
	Filtered    []int // indices into Items matching search filter
	Cursor      int
	Offset      int
	Width       int
	Height      int

	// History stack for back navigation
	History []HistoryItem

	// Search / live filter mode
	Searching   bool
	SearchQuery string

	// Top files view mode
	ShowTopFiles bool
	TopFiles     []engine.FileEntry
	TopCount     int

	// Multi-selection set (maps item path to true)
	Selected map[string]bool

	// Confirmation modal for deletions
	ConfirmingDelete bool
	DeleteTargets    []engine.FileEntry

	// Status line message
	StatusMsg string

	// Scanning indicator
	Scanning bool

	// Styles
	Styles Styles

	// Scanner instance
	Scanner *engine.DiskScanner
}

// Option configures a Model instance.
type Option func(*Model)

// WithTopCount sets the number of top files to scan/display.
func WithTopCount(n int) Option {
	return func(m *Model) {
		if n > 0 {
			m.TopCount = n
		}
	}
}

// WithScanner sets a custom scanner instance.
func WithScanner(s *engine.DiskScanner) Option {
	return func(m *Model) {
		if s != nil {
			m.Scanner = s
		}
	}
}

// WithTopViewInitial toggles initial view to Top Files mode.
func WithTopViewInitial(top bool) Option {
	return func(m *Model) {
		m.ShowTopFiles = top
	}
}

// New creates and initializes an interactive disk analyzer model.
func New(initialPath string, opts ...Option) Model {
	absPath, err := filepath.Abs(initialPath)
	if err != nil {
		absPath = initialPath
	}

	m := Model{
		CurrentPath:      absPath,
		Items:            make([]engine.FileEntry, 0),
		Filtered:         make([]int, 0),
		Cursor:           0,
		Offset:           0,
		Width:            80,
		Height:           24,
		History:          make([]HistoryItem, 0),
		Searching:        false,
		SearchQuery:      "",
		ShowTopFiles:     false,
		TopCount:         20,
		Selected:         make(map[string]bool),
		ConfirmingDelete: false,
		StatusMsg:        "",
		Scanning:         true,
		Styles:           DefaultStyles(),
		Scanner:          engine.DefaultScanner(),
	}

	for _, opt := range opts {
		opt(&m)
	}

	m.updateFiltered()
	return m
}

// Init implements tea.Model and launches initial background scanning.
func (m Model) Init() tea.Cmd {
	if m.ShowTopFiles {
		return m.startTopFilesCmd(m.CurrentPath, m.TopCount)
	}
	return m.startScanCmd(m.CurrentPath)
}

// updateFiltered filters items based on the active search query.
func (m *Model) updateFiltered() {
	query := strings.TrimSpace(strings.ToLower(m.SearchQuery))
	if query == "" {
		m.Filtered = make([]int, len(m.Items))
		for i := range m.Items {
			m.Filtered[i] = i
		}
		return
	}

	var matches []int
	for i, it := range m.Items {
		if strings.Contains(strings.ToLower(it.Name), query) ||
			strings.Contains(strings.ToLower(it.Path), query) {
			matches = append(matches, i)
		}
	}
	m.Filtered = matches
}

// clampCursor ensures the cursor stays within valid bounds of the filtered slice.
func (m *Model) clampCursor() {
	if len(m.Filtered) == 0 {
		m.Cursor = 0
		m.Offset = 0
		return
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Filtered) {
		m.Cursor = len(m.Filtered) - 1
	}
}

// adjustOffset ensures the cursor is visible within the viewport.
func (m *Model) adjustOffset() {
	if len(m.Filtered) == 0 {
		m.Offset = 0
		return
	}

	visibleRows := m.visibleRowCount()
	if visibleRows <= 0 {
		visibleRows = 10
	}

	if m.Cursor < m.Offset {
		m.Offset = m.Cursor
	} else if m.Cursor >= m.Offset+visibleRows {
		m.Offset = m.Cursor - visibleRows + 1
	}

	if m.Offset < 0 {
		m.Offset = 0
	}
	maxOffset := len(m.Filtered) - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.Offset > maxOffset {
		m.Offset = maxOffset
	}
}

// visibleRowCount computes how many rows can be shown in the table.
func (m Model) visibleRowCount() int {
	// Total terminal height minus header (4 lines), search/status (2 lines), footer (2 lines)
	rows := m.Height - 8
	if rows < 5 {
		return 5
	}
	return rows
}

// Messages used by the Bubble Tea update cycle
type scanCompleteMsg struct {
	Path string
	Root *engine.FileEntry
	Err  error
}

type topFilesCompleteMsg struct {
	Path  string
	Files []engine.FileEntry
	Err   error
}

type deleteFinishedMsg struct {
	Count int
	Freed int64
	Err   error
}

func (m Model) startScanCmd(path string) tea.Cmd {
	scanner := m.Scanner
	return func() tea.Msg {
		root, err := scanner.ScanDirProgressive(path, nil)
		return scanCompleteMsg{
			Path: path,
			Root: root,
			Err:  err,
		}
	}
}

func (m Model) startTopFilesCmd(path string, count int) tea.Cmd {
	scanner := m.Scanner
	return func() tea.Msg {
		files, err := scanner.ScanTopFiles(path, count)
		return topFilesCompleteMsg{
			Path:  path,
			Files: files,
			Err:   err,
		}
	}
}
