package startup

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func sampleServices() []engine.LaunchService {
	return []engine.LaunchService{
		{
			Label:      "com.example.useragent",
			Level:      engine.LevelUserAgent,
			PlistPath:  "/Users/test/Library/LaunchAgents/com.example.useragent.plist",
			BinaryPath: "/usr/local/bin/useragent",
			Loaded:     true,
			PID:        1234,
			IsZombie:   false,
		},
		{
			Label:      "com.example.zombieagent",
			Level:      engine.LevelUserAgent,
			PlistPath:  "/Users/test/Library/LaunchAgents/com.example.zombieagent.plist",
			BinaryPath: "/Applications/DeletedApp.app/Contents/MacOS/agent",
			Loaded:     false,
			IsZombie:   true,
		},
		{
			Label:      "com.example.globalagent",
			Level:      engine.LevelGlobalAgent,
			PlistPath:  "/Library/LaunchAgents/com.example.globalagent.plist",
			BinaryPath: "/usr/local/bin/globalagent",
			Loaded:     false,
			IsZombie:   false,
		},
		{
			Label:      "com.example.systemdaemon",
			Level:      engine.LevelSystemDaemon,
			PlistPath:  "/Library/LaunchDaemons/com.example.systemdaemon.plist",
			BinaryPath: "/usr/libexec/sysdaemon",
			Loaded:     true,
			PID:        5678,
			IsZombie:   false,
		},
	}
}

func updateModel(m Model, msg tea.Msg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestStartupModel_Initialization(t *testing.T) {
	services := sampleServices()
	m := New(services)

	if len(m.filtered) != 4 {
		t.Fatalf("expected 4 filtered services initially, got %d", len(m.filtered))
	}
	if m.filterMode != "all" {
		t.Errorf("expected default filter mode 'all', got %q", m.filterMode)
	}
}

func TestStartupModel_Filtering(t *testing.T) {
	m := New(sampleServices())

	// Filter Zombies
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 zombie service, got %d", len(m.filtered))
	}
	if !m.filtered[0].IsZombie {
		t.Errorf("expected filtered item to be zombie")
	}

	// Filter User
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if len(m.filtered) != 2 {
		t.Fatalf("expected 2 user services, got %d", len(m.filtered))
	}

	// Filter Global
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 global service, got %d", len(m.filtered))
	}

	// Filter System
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 system daemon, got %d", len(m.filtered))
	}

	// Filter All
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if len(m.filtered) != 4 {
		t.Fatalf("expected 4 services after resetting to all, got %d", len(m.filtered))
	}
}

func TestStartupModel_Navigation(t *testing.T) {
	m := New(sampleServices())
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Down
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.cursor)
	}

	// Up
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.cursor)
	}
}

func TestStartupModel_AsyncToggleAndMessages(t *testing.T) {
	m := New(sampleServices())

	// Press Space to toggle service
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeySpace})
	if cmd == nil {
		t.Errorf("expected tea.Cmd returned for async toggle")
	}

	// Simulate toggle response
	m, _ = updateModel(m, toggleServiceMsg{
		label:  "com.example.useragent",
		loaded: false,
		err:    nil,
	})

	if m.allServices[0].Loaded {
		t.Errorf("expected service to be marked unloaded after toggle msg")
	}
	if !strings.Contains(m.statusMsg, "toggled") {
		t.Errorf("expected statusMsg to indicate toggled state")
	}
}

func TestStartupModel_ZombieRemoval(t *testing.T) {
	m := New(sampleServices())

	// Move cursor to zombie (index 1)
	m.cursor = 1

	// Press 'x' to trigger remove
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Errorf("expected tea.Cmd returned for async zombie removal")
	}

	// Simulate remove response
	m, _ = updateModel(m, removeZombieMsg{
		label:     "com.example.zombieagent",
		plistPath: "/Users/test/Library/LaunchAgents/com.example.zombieagent.plist",
		err:       nil,
	})

	if len(m.allServices) != 3 {
		t.Errorf("expected 3 services remaining after zombie removal, got %d", len(m.allServices))
	}
	if !strings.Contains(m.statusMsg, "Trashed zombie plist") {
		t.Errorf("expected statusMsg to confirm zombie removal")
	}
}

func TestStartupModel_View(t *testing.T) {
	m := New(sampleServices())
	m.width = 100
	m.height = 30

	view := m.View()
	if !strings.Contains(view, "Sift Startup & Background Service Manager") {
		t.Errorf("view missing header")
	}
	if !strings.Contains(view, "com.example.useragent") {
		t.Errorf("view missing service label")
	}
	if !strings.Contains(view, "ZOMBIE") {
		t.Errorf("view missing zombie badge")
	}
}
