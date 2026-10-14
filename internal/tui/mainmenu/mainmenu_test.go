package mainmenu

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func TestMainMenu_VersionPrefixCleaning(t *testing.T) {
	dummyCmd := &cobra.Command{Use: "dummy"}
	items := []MenuItem{
		{Num: "1", Name: "Clean", Icon: "🧹", Description: "Clean caches", Cmd: dummyCmd},
	}
	sys := SysSummary{
		Hostname:  "test-mac",
		OSVersion: "macOS 15.0",
		DiskFree:  "50 GB free",
		DiskTotal: "200 GB",
		DiskPct:   75.0,
		Username:  "tester",
	}

	m1 := New("v2.1.0", sys, items)
	view1 := m1.View()
	if strings.Contains(view1, "vv2.1.0") {
		t.Errorf("version string contains double 'v': %s", view1)
	}
	if !strings.Contains(view1, "v2.1.0") {
		t.Errorf("version string missing 'v2.1.0': %s", view1)
	}

	m2 := New("2.1.0", sys, items)
	view2 := m2.View()
	if !strings.Contains(view2, "v2.1.0") {
		t.Errorf("expected clean version to be prefixed with single v: %s", view2)
	}
}

func TestMainMenu_KeyNavigation(t *testing.T) {
	cmd1 := &cobra.Command{Use: "cmd1"}
	cmd2 := &cobra.Command{Use: "cmd2"}
	cmd3 := &cobra.Command{Use: "cmd3"}

	items := []MenuItem{
		{Num: "1", Name: "First", Icon: "1️⃣", Cmd: cmd1},
		{Num: "2", Name: "Second", Icon: "2️⃣", Cmd: cmd2},
		{Num: "3", Name: "Third", Icon: "3️⃣", Cmd: cmd3},
	}
	sys := SysSummary{Hostname: "mac", OSVersion: "macOS", DiskFree: "10GB", DiskTotal: "100GB", DiskPct: 90.0, Username: "u"}

	m := New("2.1.0", sys, items)
	if m.cursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", m.cursor)
	}

	// Move down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Errorf("expected cursor 1 after down key, got %d", m.cursor)
	}

	// Move up
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.cursor != 0 {
		t.Errorf("expected cursor 0 after up key, got %d", m.cursor)
	}

	// Jump to 3
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)
	if m.cursor != 2 {
		t.Errorf("expected cursor 2 after pressing '3', got %d", m.cursor)
	}

	// Press Enter to select
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Errorf("expected quit cmd after Enter")
	}
	if m.SelectedCmd() != cmd3 {
		t.Errorf("expected selected command to be cmd3")
	}

	// Press 'q'
	m2 := New("2.1.0", sys, items)
	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m2 = updated2.(Model)
	if !m2.IsQuitting() {
		t.Errorf("expected IsQuitting=true after pressing q")
	}
	if m2.SelectedCmd() != nil {
		t.Errorf("expected SelectedCmd=nil on quit")
	}
}

func TestMainMenu_ResponsiveNarrowLayout(t *testing.T) {
	dummyCmd := &cobra.Command{Use: "dummy"}
	items := []MenuItem{
		{Num: "1", Name: "Clean", Icon: "🧹", Description: "Clean caches", Cmd: dummyCmd},
	}
	sys := SysSummary{
		Hostname:  "test-mac-longer-name",
		OSVersion: "macOS 15.0 (arm64)",
		DiskFree:  "50 GB free",
		DiskTotal: "200 GB",
		DiskPct:   75.0,
		Username:  "sadeepaherath",
	}

	m := New("2.1.0", sys, items)
	// Send narrow window size (70 cols)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "test-mac-longer-name") {
		t.Errorf("expected hostname in view")
	}
	if !strings.Contains(view, "sadeepaherath") {
		t.Errorf("expected username in view")
	}
}
