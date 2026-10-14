package status

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SadeepaNHerath/Sift/internal/core"
	"github.com/SadeepaNHerath/Sift/internal/engine"
)

// tickMsg is emitted periodically to trigger metrics collection.
type tickMsg time.Time

// metricsMsg carries freshly gathered metrics or an error.
type metricsMsg struct {
	metrics *engine.SystemMetrics
	err     error
}

// Styles defines the Lipgloss styling for the status dashboard.
type Styles struct {
	BorderColor lipgloss.Color
	TitleColor  lipgloss.Color
	DimColor    lipgloss.Color
	AccentColor lipgloss.Color
	GreenColor  lipgloss.Color
	YellowColor lipgloss.Color
	RedColor    lipgloss.Color
	CyanColor   lipgloss.Color

	HeaderBox lipgloss.Style
	PanelBox  lipgloss.Style
	Title     lipgloss.Style
	Label     lipgloss.Style
	Value     lipgloss.Style
	Badge     lipgloss.Style
	HelpBar   lipgloss.Style
}

// DefaultStyles returns the primary modern terminal color theme.
func DefaultStyles() Styles {
	return Styles{
		BorderColor: lipgloss.Color("#6272A4"),
		TitleColor:  lipgloss.Color("#BD93F9"),
		DimColor:    lipgloss.Color("#6272A4"),
		AccentColor: lipgloss.Color("#FF79C6"),
		GreenColor:  lipgloss.Color("#50FA7B"),
		YellowColor: lipgloss.Color("#F1FA8C"),
		RedColor:    lipgloss.Color("#FF5555"),
		CyanColor:   lipgloss.Color("#8BE9FD"),

		HeaderBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#BD93F9")).
			Padding(0, 1),
		PanelBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#6272A4")).
			Padding(0, 1),
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#BD93F9")),
		Label: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8F8F2")),
		Value: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#8BE9FD")),
		Badge: lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1),
		HelpBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4")),
	}
}

// Model represents the Bubble Tea state for the real-time status dashboard.
type Model struct {
	Metrics   *engine.SystemMetrics
	Err       error
	Width     int
	Height    int
	ShowCat   bool
	CatFrame  int
	CoreLimit int // 2, 4, 8, or 0 (all)
	Prefs     *core.Preferences
	Interval  time.Duration
	Quitting  bool
	Styles    Styles
}

// NewModel instantiates the status dashboard model with stored user preferences.
func NewModel() Model {
	prefs, err := core.LoadPreferences()
	if err != nil || prefs == nil {
		prefs = &core.Preferences{
			ShowCat:  true,
			CPUCores: 0,
		}
	}

	return Model{
		ShowCat:   prefs.ShowCat,
		CoreLimit: prefs.CPUCores,
		Prefs:     prefs,
		Interval:  1 * time.Second,
		Styles:    DefaultStyles(),
	}
}

// Init triggers initial metric collection.
func (m Model) Init() tea.Cmd {
	return fetchMetricsCmd()
}

// fetchMetricsCmd executes CollectMetrics in a background goroutine.
func fetchMetricsCmd() tea.Cmd {
	return func() tea.Msg {
		metrics, err := engine.CollectMetrics()
		return metricsMsg{
			metrics: metrics,
			err:     err,
		}
	}
}

// tickCmd schedules the next refresh after interval.
func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
