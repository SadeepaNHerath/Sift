package analyze

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SadeepaNHerath/Spacefold/internal/engine"
)

func createTestModel(t *testing.T) Model {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "file_a.txt")
	f2 := filepath.Join(tempDir, "file_b.txt")
	d1 := filepath.Join(tempDir, "sub_dir")

	_ = os.WriteFile(f1, []byte("aaa"), 0644)
	_ = os.WriteFile(f2, []byte("bbbbbb"), 0644)
	_ = os.Mkdir(d1, 0755)

	m := New(tempDir)
	// Populate items manually for deterministic unit tests
	m.Items = []engine.FileEntry{
		{
			Path:          d1,
			Name:          "sub_dir",
			Size:          500,
			IsDir:         true,
			ModTime:       time.Now(),
			ChildrenCount: 2,
		},
		{
			Path:          f2,
			Name:          "file_b.txt",
			Size:          6,
			IsDir:         false,
			ModTime:       time.Now(),
			ChildrenCount: 0,
		},
		{
			Path:          f1,
			Name:          "file_a.txt",
			Size:          3,
			IsDir:         false,
			ModTime:       time.Now(),
			ChildrenCount: 0,
		},
	}
	m.Scanning = false
	m.updateFiltered()
	m.Width = 100
	m.Height = 30
	return m
}

func TestModelInitialization(t *testing.T) {
	m := createTestModel(t)
	if len(m.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(m.Items))
	}
	if len(m.Filtered) != 3 {
		t.Fatalf("expected 3 filtered items, got %d", len(m.Filtered))
	}
	if m.Cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.Cursor)
	}
}

func TestNavigationKeys(t *testing.T) {
	m := createTestModel(t)

	// Move down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.Cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.Cursor)
	}

	// Move down with 'j'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.Cursor != 2 {
		t.Errorf("expected cursor 2, got %d", m.Cursor)
	}

	// Move down again (should clamp at 2)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.Cursor != 2 {
		t.Errorf("expected clamped cursor 2, got %d", m.Cursor)
	}

	// Move up with 'k'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(Model)
	if m.Cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.Cursor)
	}

	// Move up with Up arrow
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.Cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.Cursor)
	}
}

func TestMultiSelection(t *testing.T) {
	m := createTestModel(t)

	// Space to select first item
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if !m.Selected[m.Items[0].Path] {
		t.Errorf("expected item 0 to be selected")
	}

	// Space again to deselect
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if m.Selected[m.Items[0].Path] {
		t.Errorf("expected item 0 to be deselected")
	}
}

func TestSearchFilter(t *testing.T) {
	m := createTestModel(t)

	// Enter search mode with '/'
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	if !m.Searching {
		t.Fatalf("expected searching to be true")
	}

	// Type 'file'
	for _, ch := range "file" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = updated.(Model)
	}

	if m.SearchQuery != "file" {
		t.Fatalf("expected query 'file', got %q", m.SearchQuery)
	}

	// Should match file_b.txt and file_a.txt (2 items)
	if len(m.Filtered) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(m.Filtered))
	}

	// Press Backspace: query becomes 'fil'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if m.SearchQuery != "fil" {
		t.Errorf("expected query 'fil', got %q", m.SearchQuery)
	}

	// Press Esc: clears search query and exits search mode
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.Searching {
		t.Errorf("expected searching to be false after Esc")
	}
	if m.SearchQuery != "" {
		t.Errorf("expected search query to be cleared, got %q", m.SearchQuery)
	}
	if len(m.Filtered) != 3 {
		t.Errorf("expected all 3 items after Esc, got %d", len(m.Filtered))
	}
}

func TestDrillDownAndGoUp(t *testing.T) {
	m := createTestModel(t)
	origPath := m.CurrentPath

	// Cursor is on item 0 ("sub_dir", which is a directory)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if len(m.History) != 1 {
		t.Fatalf("expected history length 1, got %d", len(m.History))
	}
	if m.History[0].Path != origPath {
		t.Errorf("expected previous path in history %s, got %s", origPath, m.History[0].Path)
	}

	// Navigate up with 'h'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(Model)

	if m.CurrentPath != origPath {
		t.Errorf("expected path to return to %s, got %s", origPath, m.CurrentPath)
	}
	if len(m.History) != 0 {
		t.Errorf("expected history stack to be empty, got %d", len(m.History))
	}
}

func TestTopFilesToggle(t *testing.T) {
	m := createTestModel(t)

	// Press 't'
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)
	if !m.ShowTopFiles {
		t.Errorf("expected ShowTopFiles to be true")
	}
	if cmd == nil {
		t.Errorf("expected command to be scheduled for top files scan")
	}

	// Press 't' again
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)
	if m.ShowTopFiles {
		t.Errorf("expected ShowTopFiles to be false")
	}
	if cmd == nil {
		t.Errorf("expected command to be scheduled for dir scan")
	}
}

func TestDeleteModal(t *testing.T) {
	m := createTestModel(t)

	// Press 'delete'
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	m = updated.(Model)
	if !m.ConfirmingDelete {
		t.Fatalf("expected ConfirmingDelete to be true")
	}
	if len(m.DeleteTargets) != 1 {
		t.Fatalf("expected 1 delete target, got %d", len(m.DeleteTargets))
	}

	// Press 'n' to cancel
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.ConfirmingDelete {
		t.Errorf("expected ConfirmingDelete to be false after cancel")
	}
}

func TestViewRendering(t *testing.T) {
	m := createTestModel(t)
	viewOutput := m.View()

	if !strings.Contains(viewOutput, "sub_dir") {
		t.Errorf("expected view to contain 'sub_dir'")
	}
	if !strings.Contains(viewOutput, "file_a.txt") {
		t.Errorf("expected view to contain 'file_a.txt'")
	}
	if !strings.Contains(viewOutput, "Relative Size") {
		t.Errorf("expected view to contain 'Relative Size'")
	}
	if !strings.Contains(viewOutput, "Navigate") {
		t.Errorf("expected view to contain keybindings footer")
	}
}
