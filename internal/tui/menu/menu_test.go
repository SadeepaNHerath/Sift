package menu

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleItems() []Item {
	return []Item{
		{
			Title:       "System Logs",
			Description: "Log files older than 30 days",
			Size:        1024 * 1024 * 50, // 50 MB
			Category:    "Caches",
		},
		{
			Title:       "Xcode Caches",
			Description: "DerivedData and Archives",
			Size:        1024 * 1024 * 1024 * 5, // 5 GB
			Category:    "Developer",
		},
		{
			Title:       "Homebrew Cache",
			Description: "Downloaded package tarballs",
			Size:        1024 * 1024 * 200, // 200 MB
			Category:    "Developer",
		},
		{
			Title:       "Protected Config",
			Description: "System configuration file",
			Size:        1024,
			Protected:   true,
			Category:    "System",
		},
	}
}

func sendKey(m Model, key string) Model {
	updated, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(key),
	})
	return updated.(Model)
}

func sendSpecialKey(m Model, keyType tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{
		Type: keyType,
	})
	return updated.(Model)
}

func TestMenuNavigation(t *testing.T) {
	items := sampleItems()
	m := New("Clean Menu", items, WithHeight(2))

	if m.Cursor() != 0 {
		t.Fatalf("expected cursor at 0, got %d", m.Cursor())
	}

	// Move down with 'j'
	m = sendKey(m, "j")
	if m.Cursor() != 1 {
		t.Fatalf("expected cursor at 1 after 'j', got %d", m.Cursor())
	}

	// Move down with down arrow
	m = sendSpecialKey(m, tea.KeyDown)
	if m.Cursor() != 2 {
		t.Fatalf("expected cursor at 2 after Down, got %d", m.Cursor())
	}

	// Move up with 'k'
	m = sendKey(m, "k")
	if m.Cursor() != 1 {
		t.Fatalf("expected cursor at 1 after 'k', got %d", m.Cursor())
	}

	// Jump to bottom with 'G'
	m = sendKey(m, "G")
	if m.Cursor() != len(items)-1 {
		t.Fatalf("expected cursor at bottom (%d), got %d", len(items)-1, m.Cursor())
	}

	// Jump to top with "gg"
	m = sendKey(m, "g")
	m = sendKey(m, "g")
	if m.Cursor() != 0 {
		t.Fatalf("expected cursor at top (0) after 'gg', got %d", m.Cursor())
	}
}

func TestMenuSingleSelect(t *testing.T) {
	items := sampleItems()
	m := New("Clean Menu", items)

	// Select first item
	m = sendKey(m, " ")
	sel := m.SelectedItems()
	if len(sel) != 1 || sel[0].Title != "System Logs" {
		t.Fatalf("expected 1 selected item, got %d", len(sel))
	}

	// Move to second item and select it
	m = sendKey(m, "j")
	m = sendKey(m, " ")
	sel = m.SelectedItems()
	if len(sel) != 1 || sel[0].Title != "Xcode Caches" {
		t.Fatalf("expected single selection to replace previous, got %+v", sel)
	}

	// Move to protected item (index 3)
	m = sendKey(m, "j")
	m = sendKey(m, "j")
	m = sendKey(m, " ")
	sel = m.SelectedItems()
	if len(sel) != 1 || sel[0].Title != "Xcode Caches" {
		t.Fatalf("expected selection to remain on Xcode Caches, got %+v", sel)
	}
}

func TestMenuMultiSelect(t *testing.T) {
	items := sampleItems()
	m := New("Multi Menu", items, WithMultiSelect(), WithSizes())

	// Select item 0
	m = sendKey(m, " ")
	// Move down and select item 1
	m = sendKey(m, "j")
	m = sendKey(m, " ")

	sel := m.SelectedItems()
	if len(sel) != 2 {
		t.Fatalf("expected 2 selected items, got %d", len(sel))
	}

	// Select all with 'a'
	m = sendKey(m, "a")
	sel = m.SelectedItems()
	// Total items = 4, but 1 is protected, so 3 selectable
	if len(sel) != 3 {
		t.Fatalf("expected 3 non-protected items selected after 'a', got %d", len(sel))
	}

	// Invert with 'i'
	m = sendKey(m, "i")
	sel = m.SelectedItems()
	if len(sel) != 0 {
		t.Fatalf("expected 0 selected after inverting all-selected, got %d", len(sel))
	}
}

func TestMenuSearch(t *testing.T) {
	items := sampleItems()
	m := New("Search Menu", items)

	// Enter search mode
	m = sendKey(m, "/")
	if !m.searching {
		t.Fatalf("expected searching to be true")
	}

	// Type "xcode"
	for _, ch := range "xcode" {
		m = sendKey(m, string(ch))
	}

	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 matching item for 'xcode', got %d", len(m.filtered))
	}
	if items[m.filtered[0]].Title != "Xcode Caches" {
		t.Fatalf("expected matching item to be Xcode Caches, got %s", items[m.filtered[0]].Title)
	}

	// Backspace
	m = sendSpecialKey(m, tea.KeyBackspace)
	if m.searchQuery != "xcod" {
		t.Fatalf("expected searchQuery 'xcod', got %q", m.searchQuery)
	}

	// Exit search with esc
	m = sendSpecialKey(m, tea.KeyEsc)
	if m.searching {
		t.Fatalf("expected searching to be false after Esc")
	}
	if len(m.filtered) != len(items) {
		t.Fatalf("expected all items restored after search cancel, got %d", len(m.filtered))
	}
}

func TestMenuConfirmAndCancel(t *testing.T) {
	items := sampleItems()
	m := New("Menu", items)

	// Confirm
	mConf, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := mConf.(Model)
	if !m2.IsConfirmed() || m2.IsCancelled() {
		t.Fatalf("expected confirmed=true, cancelled=false")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}

	// Cancel
	mCanc, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m3 := mCanc.(Model)
	if !m3.IsCancelled() || m3.IsConfirmed() {
		t.Fatalf("expected cancelled=true, confirmed=false")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
}

func TestMenuViewOutput(t *testing.T) {
	items := sampleItems()
	items[0].Selected = true
	m := New("Clean Menu", items, WithMultiSelect(), WithSizes(), WithWidth(80))

	view := m.View()
	if !strings.Contains(view, "Clean Menu") {
		t.Fatalf("view should contain title")
	}
	if !strings.Contains(view, "1/4 selected") {
		t.Fatalf("view should contain selection count")
	}
	if !strings.Contains(view, "✓") {
		t.Fatalf("view should contain checkmark for selected item")
	}
	if !strings.Contains(view, "🔒") {
		t.Fatalf("view should contain lock for protected item")
	}
	if !strings.Contains(view, "50.0 MB") || !strings.Contains(view, "5.0 GB") {
		t.Fatalf("view should contain formatted sizes")
	}
	if !strings.Contains(view, "Developer") {
		t.Fatalf("view should contain category header")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024 * 5, "5.0 MB"},
		{1024 * 1024 * 1024 * 3, "3.0 GB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
