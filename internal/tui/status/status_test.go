package status

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SadeepaNHerath/Sift/internal/engine"
)

func sampleMetrics() *engine.SystemMetrics {
	return &engine.SystemMetrics{
		Hostname:    "MacBook-Pro.local",
		OSVersion:   "macOS 15.3",
		Uptime:      3600 * 5,
		UptimeStr:   "5h 0m 0s",
		Arch:        "arm64",
		HealthScore: 95,
		CPU: engine.CPUMetrics{
			TotalPercent:   25.4,
			PerCorePercent: []float64{30.0, 20.0, 15.0, 35.0},
			Load1:          1.5,
			Load5:          1.2,
			Load15:         1.0,
		},
		Memory: engine.MemoryMetrics{
			Total:         16 * 1024 * 1024 * 1024,
			Used:          8 * 1024 * 1024 * 1024,
			Free:          4 * 1024 * 1024 * 1024,
			Available:     8 * 1024 * 1024 * 1024,
			Wired:         2 * 1024 * 1024 * 1024,
			Active:        4 * 1024 * 1024 * 1024,
			Compressed:    1 * 1024 * 1024 * 1024,
			PressureLevel: "Normal",
		},
		Disk: engine.DiskMetrics{
			Total:          500 * 1024 * 1024 * 1024,
			Used:           150 * 1024 * 1024 * 1024,
			Free:           350 * 1024 * 1024 * 1024,
			UsedPercent:    30.0,
			ReadBytesRate:  1024 * 100,
			WriteBytesRate: 1024 * 200,
		},
		Network: engine.NetworkMetrics{
			BytesSentRate:   1024 * 50,
			BytesRecvRate:   1024 * 150,
			SparklinePoints: []float64{10, 20, 50, 100, 80, 40},
			InterfaceName:   "en0",
		},
		Battery: engine.BatteryMetrics{
			Present:     true,
			Percent:     92.0,
			Charging:    true,
			Health:      "Good (100%)",
			CycleCount:  120,
			Temperature: 29.5,
		},
		TopProcesses: []engine.ProcessInfo{
			{PID: 101, Name: "WindowServer", CPUPercent: 5.5, MemBytes: 250 * 1024 * 1024, IsZombie: false},
			{PID: 202, Name: "Dock", CPUPercent: 1.2, MemBytes: 80 * 1024 * 1024, IsZombie: false},
		},
		Timestamp: time.Now(),
	}
}

func TestModelUpdateKeys(t *testing.T) {
	m := NewModel()
	m.Metrics = sampleMetrics()

	// 1. Test 'k' toggles cat mascot
	initialCat := m.ShowCat
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updatedModel.(Model)
	if m.ShowCat == initialCat {
		t.Errorf("expected ShowCat to toggle from %v, got %v", initialCat, m.ShowCat)
	}

	// 2. Test 'c' cycles CPU core limit
	m.CoreLimit = 2
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updatedModel.(Model)
	if m.CoreLimit != 4 {
		t.Errorf("expected CoreLimit to cycle to 4, got %d", m.CoreLimit)
	}

	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updatedModel.(Model)
	if m.CoreLimit != 8 {
		t.Errorf("expected CoreLimit to cycle to 8, got %d", m.CoreLimit)
	}

	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updatedModel.(Model)
	if m.CoreLimit != 0 {
		t.Errorf("expected CoreLimit to cycle to 0 (all), got %d", m.CoreLimit)
	}

	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updatedModel.(Model)
	if m.CoreLimit != 2 {
		t.Errorf("expected CoreLimit to cycle back to 2, got %d", m.CoreLimit)
	}

	// 3. Test 'q' triggers quit
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updatedModel.(Model)
	if !m.Quitting {
		t.Errorf("expected Quitting to be true on 'q'")
	}
	if cmd == nil {
		t.Errorf("expected non-nil tea.Quit cmd on 'q'")
	}
}

func TestViewRendering(t *testing.T) {
	m := NewModel()
	m.Width = 100
	m.Height = 40
	m.Metrics = sampleMetrics()
	m.ShowCat = true

	viewOutput := m.View()
	if viewOutput == "" {
		t.Fatal("expected non-empty view output")
	}

	// Check for key components in view output
	expectedStrings := []string{
		"SIFT SYSTEM STATUS",
		"MacBook-Pro.local",
		"CPU UTILIZATION",
		"MEMORY ALLOCATION",
		"STORAGE & DISK I/O",
		"NETWORK",
		"BATTERY & HARDWARE",
		"TOP PROCESSES",
		"WindowServer",
		"[q] Quit",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(viewOutput, s) {
			t.Errorf("view output missing expected string: %q", s)
		}
	}
}

func TestBarAndSparkline(t *testing.T) {
	bar := renderBar(50.0, 10, "#50FA7B", "#44475A")
	if !strings.Contains(bar, "50.0%") {
		t.Errorf("expected bar to contain '50.0%%', got %s", bar)
	}

	spark := renderSparkline([]float64{1, 2, 5, 8, 10}, 10)
	if spark == "" {
		t.Errorf("expected non-empty sparkline")
	}

	emptySpark := renderSparkline(nil, 10)
	if emptySpark == "" {
		t.Errorf("expected non-empty placeholder for empty sparkline")
	}
}
