package dedup

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func sampleGroups() []engine.DuplicateGroup {
	now := time.Now()
	return []engine.DuplicateGroup{
		{
			Hash:        "aabbccddeeff1122",
			Size:        1024 * 1024,
			Reclaimable: 1024 * 1024,
			Files: []engine.DuplicateFile{
				{
					Path:    "/tmp/file1.png",
					Size:    1024 * 1024,
					ModTime: now.Add(-2 * time.Hour),
					Dev:     1,
					Inode:   100,
				},
				{
					Path:    "/tmp/file2.png",
					Size:    1024 * 1024,
					ModTime: now.Add(-1 * time.Hour),
					Dev:     1,
					Inode:   101,
				},
			},
		},
		{
			Hash:        "1122334455667788",
			Size:        2048 * 1024,
			Reclaimable: 4096 * 1024,
			Files: []engine.DuplicateFile{
				{
					Path:    "/tmp/doc_v1.zip",
					Size:    2048 * 1024,
					ModTime: now.Add(-3 * time.Hour),
					Dev:     1,
					Inode:   200,
				},
				{
					Path:    "/tmp/doc_v2.zip",
					Size:    2048 * 1024,
					ModTime: now.Add(-2 * time.Hour),
					Dev:     1,
					Inode:   201,
				},
				{
					Path:    "/tmp/doc_v3.zip",
					Size:    2048 * 1024,
					ModTime: now.Add(-1 * time.Hour),
					Dev:     1,
					Inode:   202,
				},
			},
		},
	}
}

func updateModel(m Model, msg tea.Msg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestDedupModel_Initialization(t *testing.T) {
	groups := sampleGroups()
	m := New(groups)

	if len(m.items) != 5 {
		t.Fatalf("expected 5 items across groups, got %d", len(m.items))
	}

	// Verify default selection: oldest is unselected, others are selected
	if m.items[0].Selected {
		t.Errorf("group 1 oldest should not be selected by default")
	}
	if !m.items[1].Selected {
		t.Errorf("group 1 newer copy should be selected by default")
	}
	if m.items[2].Selected {
		t.Errorf("group 2 oldest should not be selected by default")
	}
	if !m.items[3].Selected || !m.items[4].Selected {
		t.Errorf("group 2 newer copies should be selected by default")
	}
}

func TestDedupModel_NavigationAndScrolling(t *testing.T) {
	m := New(sampleGroups())
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Navigate down
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.cursor)
	}

	// Navigate up
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.cursor)
	}
}

func TestDedupModel_SpaceTogglingSafety(t *testing.T) {
	m := New(sampleGroups())

	// Cursor is at index 0 (the oldest and only unselected copy in group 1)
	if m.items[0].Selected {
		t.Fatalf("initial item 0 should be unselected")
	}

	// Pressing space on index 0 would select all copies in group 1!
	// Safety check must prevent this and show a warning
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeySpace})

	if m.items[0].Selected {
		t.Errorf("safety violation: model allowed selecting 100%% of copies in group 1")
	}
	if m.statusMsg == "" {
		t.Errorf("expected warning status message when attempting to select all copies")
	}

	// Now move cursor to index 1 (selected) and unselect it
	m.cursor = 1
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.items[1].Selected {
		t.Errorf("expected index 1 to be unselected")
	}

	// Now index 0 CAN be selected because index 1 is kept!
	m.cursor = 0
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.items[0].Selected {
		t.Errorf("expected index 0 to be selectable when index 1 is preserved")
	}
}

func TestDedupModel_KeepOldestAndNewest(t *testing.T) {
	m := New(sampleGroups())

	// Press 'n' (keep newest)
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.items[0].Selected != true { // group 1 oldest should now be selected
		t.Errorf("expected oldest in group 1 to be selected when keeping newest")
	}
	if m.items[1].Selected != false { // group 1 newest should be kept (unselected)
		t.Errorf("expected newest in group 1 to be kept")
	}

	// Press 'o' (keep oldest)
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if m.items[0].Selected != false { // group 1 oldest should be kept
		t.Errorf("expected oldest in group 1 to be kept when keeping oldest")
	}
	if m.items[1].Selected != true {
		t.Errorf("expected newest in group 1 to be selected")
	}
}

func TestDedupModel_ToggleAllInvariant(t *testing.T) {
	m := New(sampleGroups())

	// Press 'a'
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	// Must never select the oldest
	for _, itm := range m.items {
		if itm.IsOldest && itm.Selected {
			t.Errorf("toggle all selected oldest copy in group %d", itm.GroupIndex)
		}
	}

	paths := m.SelectedPaths()
	if len(paths) == len(m.items) {
		t.Errorf("SelectedPaths returned 100%% of files, violating safety invariant")
	}
}

func TestDedupModel_ConfirmAndCancel(t *testing.T) {
	m := New(sampleGroups())

	// Test Cancel
	mQuit, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !mQuit.IsCancelled() {
		t.Errorf("expected IsCancelled=true")
	}
	if cmd == nil {
		t.Errorf("expected quit cmd")
	}

	// Test Confirm
	m = New(sampleGroups())
	mConf, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !mConf.IsConfirmed() {
		t.Errorf("expected IsConfirmed=true")
	}
	if cmd == nil {
		t.Errorf("expected quit cmd")
	}
}

func TestDedupModel_ViewRendering(t *testing.T) {
	m := New(sampleGroups())
	m.width = 100
	m.height = 30

	view := m.View()
	if !strings.Contains(view, "Sift Duplicate File Finder") {
		t.Errorf("view missing title")
	}
	if !strings.Contains(view, "Duplicate Set #1") {
		t.Errorf("view missing group header")
	}
	if !strings.Contains(view, "Reclaimable:") {
		t.Errorf("view missing reclaimable summary")
	}
}
