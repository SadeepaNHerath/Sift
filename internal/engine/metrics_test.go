package engine

import (
	"runtime"
	"testing"
	"time"
)

func TestCollectMetrics(t *testing.T) {
	metrics, err := CollectMetrics()
	if err != nil {
		t.Fatalf("CollectMetrics() failed: %v", err)
	}

	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}

	if metrics.Hostname == "" {
		t.Errorf("expected non-empty hostname")
	}

	if metrics.OSVersion == "" {
		t.Errorf("expected non-empty OS version")
	}

	if metrics.Uptime == 0 {
		t.Errorf("expected positive uptime")
	}

	if metrics.Arch != runtime.GOARCH {
		t.Errorf("expected arch %s, got %s", runtime.GOARCH, metrics.Arch)
	}

	if metrics.HealthScore < 0 || metrics.HealthScore > 100 {
		t.Errorf("expected health score between 0 and 100, got %d", metrics.HealthScore)
	}

	if len(metrics.CPU.PerCorePercent) == 0 {
		t.Errorf("expected per-core CPU percent slice to be non-empty")
	}

	if metrics.Memory.Total == 0 {
		t.Errorf("expected total memory > 0")
	}

	if metrics.Disk.Total == 0 {
		t.Errorf("expected total disk > 0")
	}

	if len(metrics.TopProcesses) == 0 {
		t.Logf("top processes list is empty (may happen in isolated test sandbox)")
	} else {
		first := metrics.TopProcesses[0]
		if first.PID <= 0 && first.Name == "" {
			t.Errorf("invalid first process info: %+v", first)
		}
	}
}

func TestCalculateHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		metrics  *SystemMetrics
		minScore int
		maxScore int
	}{
		{
			name: "Ideal Healthy System",
			metrics: &SystemMetrics{
				CPU: CPUMetrics{
					TotalPercent:   5.0,
					PerCorePercent: []float64{5.0, 5.0, 5.0, 5.0},
					Load1:          0.5,
				},
				Memory: MemoryMetrics{
					Total:         16 * 1024 * 1024 * 1024,
					Used:          4 * 1024 * 1024 * 1024,
					Available:     12 * 1024 * 1024 * 1024,
					PressureLevel: "Normal",
				},
				Disk: DiskMetrics{
					Total:       500 * 1024 * 1024 * 1024,
					Used:        100 * 1024 * 1024 * 1024,
					UsedPercent: 20.0,
				},
			},
			minScore: 95,
			maxScore: 100,
		},
		{
			name: "High CPU Load",
			metrics: &SystemMetrics{
				CPU: CPUMetrics{
					TotalPercent:   95.0,
					PerCorePercent: []float64{95.0, 95.0},
					Load1:          10.0,
				},
				Memory: MemoryMetrics{
					Total:         16 * 1024 * 1024 * 1024,
					Used:          8 * 1024 * 1024 * 1024,
					PressureLevel: "Normal",
				},
				Disk: DiskMetrics{
					UsedPercent: 30.0,
				},
			},
			minScore: 50,
			maxScore: 85,
		},
		{
			name: "Memory Warning and High Disk",
			metrics: &SystemMetrics{
				CPU: CPUMetrics{
					TotalPercent:   20.0,
					PerCorePercent: []float64{20.0, 20.0},
					Load1:          1.0,
				},
				Memory: MemoryMetrics{
					Total:         16 * 1024 * 1024 * 1024,
					Used:          14 * 1024 * 1024 * 1024,
					PressureLevel: "Warning",
				},
				Disk: DiskMetrics{
					UsedPercent: 92.0,
				},
			},
			minScore: 55,
			maxScore: 70,
		},
		{
			name: "Critical System Under Extreme Stress",
			metrics: &SystemMetrics{
				CPU: CPUMetrics{
					TotalPercent:   100.0,
					PerCorePercent: []float64{100.0, 100.0},
					Load1:          20.0,
				},
				Memory: MemoryMetrics{
					Total:         16 * 1024 * 1024 * 1024,
					Used:          16 * 1024 * 1024 * 1024,
					PressureLevel: "Critical",
				},
				Disk: DiskMetrics{
					UsedPercent: 99.0,
				},
				TopProcesses: []ProcessInfo{
					{PID: 1234, Name: "bad_proc", IsZombie: true},
				},
			},
			minScore: 0,
			maxScore: 15,
		},
		{
			name:     "Nil Metrics",
			metrics:  nil,
			minScore: 0,
			maxScore: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := CalculateHealthScore(tt.metrics)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("CalculateHealthScore() = %d, expected between %d and %d", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		seconds  uint64
		expected string
	}{
		{30, "30s"},
		{90, "1m 30s"},
		{3665, "1h 1m 5s"},
		{172800 + 3600*5 + 60*12, "2d 5h 12m"},
	}

	for _, c := range cases {
		got := FormatUptime(c.seconds)
		if got != c.expected {
			t.Errorf("FormatUptime(%d) = %q, expected %q", c.seconds, got, c.expected)
		}
	}
}

func TestDifferentialSampling(t *testing.T) {
	// Sample once
	_, err := CollectMetrics()
	if err != nil {
		t.Fatalf("first CollectMetrics failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	// Sample twice
	m2, err := CollectMetrics()
	if err != nil {
		t.Fatalf("second CollectMetrics failed: %v", err)
	}

	if m2.Network.SparklinePoints == nil {
		t.Errorf("expected non-nil sparkline points slice")
	}
}
