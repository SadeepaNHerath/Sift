package engine

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"golang.org/x/sys/unix"
)

// SystemMetrics represents the complete system health and resource utilization snapshot.
type SystemMetrics struct {
	Hostname     string         `json:"hostname"`
	OSVersion    string         `json:"os_version"`
	Uptime       uint64         `json:"uptime"`
	UptimeStr    string         `json:"uptime_str,omitempty"`
	Arch         string         `json:"arch"`
	HealthScore  int            `json:"health_score"`
	CPU          CPUMetrics     `json:"cpu"`
	Memory       MemoryMetrics  `json:"memory"`
	Disk         DiskMetrics    `json:"disk"`
	Network      NetworkMetrics `json:"network"`
	Battery      BatteryMetrics `json:"battery"`
	TopProcesses []ProcessInfo  `json:"top_processes"`
	Timestamp    time.Time      `json:"timestamp"`
}

// CPUMetrics contains overall and per-core processor statistics and load averages.
type CPUMetrics struct {
	TotalPercent   float64   `json:"total_percent"`
	PerCorePercent []float64 `json:"per_core_percent"`
	Load1          float64   `json:"load1"`
	Load5          float64   `json:"load5"`
	Load15         float64   `json:"load15"`
}

// MemoryMetrics contains memory allocation breakdown and macOS pressure classification.
type MemoryMetrics struct {
	Total         uint64 `json:"total"`
	Used          uint64 `json:"used"`
	Free          uint64 `json:"free"`
	Available     uint64 `json:"available"`
	Wired         uint64 `json:"wired"`
	Active        uint64 `json:"active"`
	Compressed    uint64 `json:"compressed"`
	PressureLevel string `json:"pressure_level"` // "Normal", "Warning", "Critical"
}

// DiskMetrics contains storage capacity, utilization, and I/O throughput rates.
type DiskMetrics struct {
	Total          uint64  `json:"total"`
	Used           uint64  `json:"used"`
	Free           uint64  `json:"free"`
	UsedPercent    float64 `json:"used_percent"`
	ReadBytesRate  float64 `json:"read_bytes_rate"`  // Bytes per second
	WriteBytesRate float64 `json:"write_bytes_rate"` // Bytes per second
}

// NetworkMetrics contains network I/O throughput rates, history, and active interface name.
type NetworkMetrics struct {
	BytesSentRate   float64   `json:"bytes_sent_rate"` // Bytes per second
	BytesRecvRate   float64   `json:"bytes_recv_rate"` // Bytes per second
	SparklinePoints []float64 `json:"sparkline_points"`
	InterfaceName   string    `json:"interface_name"`
}

// BatteryMetrics represents battery health, charging status, cycle count, and temperature.
type BatteryMetrics struct {
	Present     bool    `json:"present"`
	Percent     float64 `json:"percent"`
	Charging    bool    `json:"charging"`
	Health      string  `json:"health"`
	CycleCount  int     `json:"cycle_count"`
	Temperature float64 `json:"temperature"`
}

// ProcessInfo holds runtime resource usage for an active process.
type ProcessInfo struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
	MemBytes   uint64  `json:"mem_bytes"`
	IsZombie   bool    `json:"is_zombie"`
}

const (
	maxSparklinePoints = 20
)

// stateTracker maintains previous sampling data for differential rate calculations.
type stateTracker struct {
	mu            sync.Mutex
	lastSample    time.Time
	lastDiskRead  uint64
	lastDiskWrite uint64
	lastNetSent   uint64
	lastNetRecv   uint64
	sparkline     []float64
	initialized   bool
}

var globalTracker = &stateTracker{
	sparkline: make([]float64, 0, maxSparklinePoints),
}

// CollectMetrics gathers all system metrics using gopsutil, Darwin sysctl, and statfs.
func CollectMetrics() (*SystemMetrics, error) {
	hostname, osVersion, uptime, arch := collectHostInfo()

	cpuMetrics, err := collectCPU()
	if err != nil {
		cpuMetrics = CPUMetrics{
			PerCorePercent: make([]float64, runtime.NumCPU()),
		}
	}

	memMetrics, err := collectMemory()
	if err != nil {
		memMetrics = MemoryMetrics{
			PressureLevel: "Normal",
		}
	}

	diskMetrics, netMetrics := collectDiskAndNetwork()
	batteryMetrics := collectBattery()
	topProcesses := collectTopProcesses(10)

	metrics := &SystemMetrics{
		Hostname:     hostname,
		OSVersion:    osVersion,
		Uptime:       uptime,
		UptimeStr:    FormatUptime(uptime),
		Arch:         arch,
		CPU:          cpuMetrics,
		Memory:       memMetrics,
		Disk:         diskMetrics,
		Network:      netMetrics,
		Battery:      batteryMetrics,
		TopProcesses: topProcesses,
		Timestamp:    time.Now(),
	}

	metrics.HealthScore = CalculateHealthScore(metrics)
	return metrics, nil
}

// CalculateHealthScore computes a composite health score (0-100) factoring CPU load,
// memory pressure, disk fullness, and zombie processes.
func CalculateHealthScore(metrics *SystemMetrics) int {
	if metrics == nil {
		return 0
	}

	score := 100.0

	// 1. CPU load penalty
	// Usage above 60% starts penalizing, up to 30 points
	if metrics.CPU.TotalPercent > 60.0 {
		cpuPenalty := (metrics.CPU.TotalPercent - 60.0) * 0.75
		if cpuPenalty > 30.0 {
			cpuPenalty = 30.0
		}
		score -= cpuPenalty
	}

	// High load average relative to core count (load > cores * 1.5)
	cores := len(metrics.CPU.PerCorePercent)
	if cores <= 0 {
		cores = runtime.NumCPU()
	}
	if cores > 0 && metrics.CPU.Load1 > float64(cores)*1.5 {
		loadExcess := metrics.CPU.Load1 - (float64(cores) * 1.5)
		loadPenalty := loadExcess * 3.0
		if loadPenalty > 15.0 {
			loadPenalty = 15.0
		}
		score -= loadPenalty
	}

	// 2. Memory pressure penalty
	switch strings.ToLower(metrics.Memory.PressureLevel) {
	case "critical":
		score -= 35.0
	case "warning":
		score -= 15.0
	default:
		// Normal, check used percent if total > 0
		if metrics.Memory.Total > 0 {
			usedPct := float64(metrics.Memory.Used) / float64(metrics.Memory.Total) * 100.0
			if usedPct > 90.0 {
				score -= 10.0
			} else if usedPct > 80.0 {
				score -= 5.0
			}
		}
	}

	// 3. Disk fullness penalty
	if metrics.Disk.UsedPercent >= 95.0 {
		score -= 35.0
	} else if metrics.Disk.UsedPercent >= 90.0 {
		score -= 20.0
	} else if metrics.Disk.UsedPercent >= 80.0 {
		score -= 10.0
	} else if metrics.Disk.UsedPercent >= 70.0 {
		score -= 5.0
	}

	// 4. Zombie processes penalty
	for _, proc := range metrics.TopProcesses {
		if proc.IsZombie {
			score -= 5.0
			break
		}
	}

	// Clamp to 0..100
	finalScore := int(score + 0.5)
	if finalScore < 0 {
		finalScore = 0
	}
	if finalScore > 100 {
		finalScore = 100
	}
	return finalScore
}

// FormatUptime converts seconds into a human-readable duration (e.g. "2d 4h 15m").
func FormatUptime(seconds uint64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	secs := seconds % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// collectHostInfo retrieves the system hostname, OS version, uptime, and architecture.
func collectHostInfo() (hostname, osVersion string, uptime uint64, arch string) {
	arch = runtime.GOARCH

	h, err := os.Hostname()
	if err == nil && h != "" {
		hostname = h
	} else {
		hostname = "mac"
	}

	// OS version from sw_vers
	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		prodVer := strings.TrimSpace(string(out))
		if prodVer != "" {
			osVersion = fmt.Sprintf("macOS %s", prodVer)
		}
	}
	if osVersion == "" {
		if hi, err := host.Info(); err == nil && hi.PlatformVersion != "" {
			osVersion = fmt.Sprintf("macOS %s", hi.PlatformVersion)
		} else {
			osVersion = "macOS"
		}
	}

	// Darwin sysctl kern.boottime for uptime calculation
	if ut, err := host.Uptime(); err == nil && ut > 0 {
		uptime = ut
	} else {
		tv, err := unix.SysctlTimeval("kern.boottime")
		if err == nil && tv.Sec > 0 {
			uptime = uint64(time.Now().Unix() - tv.Sec)
		}
	}

	return
}

// collectCPU gathers overall CPU utilization, per-core utilization, and load averages.
func collectCPU() (CPUMetrics, error) {
	res := CPUMetrics{}

	// Load averages
	if l, err := load.Avg(); err == nil {
		res.Load1 = l.Load1
		res.Load5 = l.Load5
		res.Load15 = l.Load15
	}

	// CPU utilization
	// If first call, a brief sampling interval ensures non-zero metrics on one-shot CLI queries.
	globalTracker.mu.Lock()
	firstCall := !globalTracker.initialized
	globalTracker.mu.Unlock()

	var sampleDur time.Duration
	if firstCall {
		sampleDur = 60 * time.Millisecond
	} else {
		sampleDur = 0
	}

	totalPcts, err := cpu.Percent(sampleDur, false)
	if err == nil && len(totalPcts) > 0 {
		res.TotalPercent = totalPcts[0]
	}

	corePcts, err := cpu.Percent(0, true)
	if err == nil {
		res.PerCorePercent = corePcts
	} else {
		res.PerCorePercent = make([]float64, runtime.NumCPU())
	}

	return res, nil
}

// collectMemory gathers memory stats via gopsutil, Darwin sysctl, and vm_stat.
func collectMemory() (MemoryMetrics, error) {
	res := MemoryMetrics{
		PressureLevel: "Normal",
	}

	// 1. gopsutil virtual memory
	v, err := mem.VirtualMemory()
	if err == nil && v != nil {
		res.Total = v.Total
		res.Used = v.Used
		res.Free = v.Free
		res.Available = v.Available
		res.Wired = v.Wired
		res.Active = v.Active
	} else {
		// Fallback to sysctl hw.memsize
		memSize, sysErr := unix.SysctlUint64("hw.memsize")
		if sysErr == nil {
			res.Total = memSize
		}
	}

	// 2. Parse vm_stat for compressed memory and accurate page breakdown
	vmPages, pageSize, vmErr := parseVMStat()
	if vmErr == nil && pageSize > 0 {
		if compPages, ok := vmPages["Pages occupied by compressor"]; ok {
			res.Compressed = compPages * pageSize
		} else if compPages, ok := vmPages["Pages stored in compressor"]; ok {
			res.Compressed = compPages * pageSize
		}

		if res.Wired == 0 {
			if wiredPages, ok := vmPages["Pages wired down"]; ok {
				res.Wired = wiredPages * pageSize
			}
		}
		if res.Active == 0 {
			if activePages, ok := vmPages["Pages active"]; ok {
				res.Active = activePages * pageSize
			}
		}
		if res.Free == 0 {
			if freePages, ok := vmPages["Pages free"]; ok {
				res.Free = freePages * pageSize
			}
		}
	}

	// If available was not calculated or is 0, estimate it
	if res.Available == 0 && res.Total > 0 {
		if res.Free+res.Active < res.Total {
			res.Available = res.Free
		}
	}

	// 3. Darwin memory pressure level via sysctl kern.memorystatus_vm_pressure_level
	// 1 = Normal, 2 = Warning, 4 = Critical
	pressureVal, pErr := unix.SysctlUint32("kern.memorystatus_vm_pressure_level")
	if pErr == nil {
		switch pressureVal {
		case 1:
			res.PressureLevel = "Normal"
		case 2:
			res.PressureLevel = "Warning"
		case 4:
			res.PressureLevel = "Critical"
		default:
			if pressureVal > 2 {
				res.PressureLevel = "Critical"
			} else {
				res.PressureLevel = "Normal"
			}
		}
	} else {
		// Fallback based on available memory ratio
		if res.Total > 0 {
			availRatio := float64(res.Available) / float64(res.Total)
			if availRatio < 0.08 {
				res.PressureLevel = "Critical"
			} else if availRatio < 0.18 {
				res.PressureLevel = "Warning"
			} else {
				res.PressureLevel = "Normal"
			}
		}
	}

	return res, nil
}

// parseVMStat executes and parses the macOS vm_stat utility.
func parseVMStat() (map[string]uint64, uint64, error) {
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return nil, 0, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	pages := make(map[string]uint64)
	var pageSize uint64 = 4096 // standard fallback

	pageSizeRegex := regexp.MustCompile(`page size of (\d+) bytes`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if match := pageSizeRegex.FindStringSubmatch(line); len(match) > 1 {
			if ps, pErr := strconv.ParseUint(match[1], 10, 64); pErr == nil && ps > 0 {
				pageSize = ps
			}
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) == 2 {
			key := strings.Trim(parts[0], `"`)
			valStr := strings.Trim(parts[1], " .")
			if count, parseErr := strconv.ParseUint(valStr, 10, 64); parseErr == nil {
				pages[key] = count
			}
		}
	}

	return pages, pageSize, nil
}

// collectDiskAndNetwork queries storage statfs and network I/O counters and computes rates.
func collectDiskAndNetwork() (DiskMetrics, NetworkMetrics) {
	diskRes := DiskMetrics{}
	netRes := NetworkMetrics{
		InterfaceName: "en0",
	}

	now := time.Now()

	// 1. Root volume storage using statfs
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		bsize := uint64(stat.Bsize)
		diskRes.Total = stat.Blocks * bsize
		diskRes.Free = stat.Bavail * bsize
		diskRes.Used = diskRes.Total - (stat.Bfree * bsize)
		if diskRes.Total > 0 {
			diskRes.UsedPercent = (float64(diskRes.Used) / float64(diskRes.Total)) * 100.0
		}
	}

	// 2. Query total Disk I/O bytes
	var currDiskRead, currDiskWrite uint64
	if diskCounters, err := disk.IOCounters(); err == nil {
		for _, counter := range diskCounters {
			currDiskRead += counter.ReadBytes
			currDiskWrite += counter.WriteBytes
		}
	}

	// 3. Query Network I/O bytes
	var currNetSent, currNetRecv uint64
	if netCounters, err := net.IOCounters(true); err == nil {
		var maxTraffic uint64
		for _, counter := range netCounters {
			// Skip loopback
			if strings.HasPrefix(counter.Name, "lo") {
				continue
			}
			currNetSent += counter.BytesSent
			currNetRecv += counter.BytesRecv

			traffic := counter.BytesSent + counter.BytesRecv
			if traffic > maxTraffic {
				maxTraffic = traffic
				netRes.InterfaceName = counter.Name
			}
		}
	}

	// 4. Update differential tracker for rates and sparkline
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()

	if globalTracker.initialized && !globalTracker.lastSample.IsZero() {
		deltaSec := now.Sub(globalTracker.lastSample).Seconds()
		if deltaSec > 0.1 {
			if currDiskRead >= globalTracker.lastDiskRead {
				diskRes.ReadBytesRate = float64(currDiskRead-globalTracker.lastDiskRead) / deltaSec
			}
			if currDiskWrite >= globalTracker.lastDiskWrite {
				diskRes.WriteBytesRate = float64(currDiskWrite-globalTracker.lastDiskWrite) / deltaSec
			}

			if currNetSent >= globalTracker.lastNetSent {
				netRes.BytesSentRate = float64(currNetSent-globalTracker.lastNetSent) / deltaSec
			}
			if currNetRecv >= globalTracker.lastNetRecv {
				netRes.BytesRecvRate = float64(currNetRecv-globalTracker.lastNetRecv) / deltaSec
			}

			// Network throughput in KB/s for sparkline point
			throughputKB := (netRes.BytesSentRate + netRes.BytesRecvRate) / 1024.0
			globalTracker.sparkline = append(globalTracker.sparkline, throughputKB)
			if len(globalTracker.sparkline) > maxSparklinePoints {
				globalTracker.sparkline = globalTracker.sparkline[len(globalTracker.sparkline)-maxSparklinePoints:]
			}
		}
	} else {
		globalTracker.initialized = true
	}

	globalTracker.lastSample = now
	globalTracker.lastDiskRead = currDiskRead
	globalTracker.lastDiskWrite = currDiskWrite
	globalTracker.lastNetSent = currNetSent
	globalTracker.lastNetRecv = currNetRecv

	// Return copy of sparkline points
	netRes.SparklinePoints = make([]float64, len(globalTracker.sparkline))
	copy(netRes.SparklinePoints, globalTracker.sparkline)

	return diskRes, netRes
}

// collectBattery queries battery percentage, charging state, cycle count, and temperature.
func collectBattery() BatteryMetrics {
	res := BatteryMetrics{
		Health: "Normal",
	}

	// 1. Quick check via pmset -g batt
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err == nil {
		str := string(out)
		if strings.Contains(str, "InternalBattery") || strings.Contains(str, "drawing from 'Battery Power'") {
			res.Present = true

			// Extract percentage
			pctRe := regexp.MustCompile(`(\d+)%`)
			if m := pctRe.FindStringSubmatch(str); len(m) > 1 {
				if p, pErr := strconv.ParseFloat(m[1], 64); pErr == nil {
					res.Percent = p
				}
			}

			// Charging status
			lower := strings.ToLower(str)
			if strings.Contains(lower, "charging") && !strings.Contains(lower, "discharging") && !strings.Contains(lower, "not charging") {
				res.Charging = true
			}
		}
	}

	// 2. Query ioreg for AppleSmartBattery details (cycle count, max capacity, temperature)
	if res.Present {
		ioregOut, ioErr := exec.Command("ioreg", "-r", "-c", "AppleSmartBattery").Output()
		if ioErr == nil {
			ioStr := string(ioregOut)

			// Cycle count
			ccRe := regexp.MustCompile(`"CycleCount"\s*=\s*(\d+)`)
			if m := ccRe.FindStringSubmatch(ioStr); len(m) > 1 {
				if cc, cErr := strconv.Atoi(m[1]); cErr == nil {
					res.CycleCount = cc
				}
			}

			// Health from MaxCapacity
			mcRe := regexp.MustCompile(`"MaxCapacity"\s*=\s*(\d+)`)
			if m := mcRe.FindStringSubmatch(ioStr); len(m) > 1 {
				if mc, mErr := strconv.Atoi(m[1]); mErr == nil {
					if mc >= 80 {
						res.Health = fmt.Sprintf("Good (%d%%)", mc)
					} else {
						res.Health = fmt.Sprintf("Service (%d%%)", mc)
					}
				}
			}

			// Temperature (ioreg reports in centi-degrees Celsius, e.g. 3000 = 30.0 C)
			tempRe := regexp.MustCompile(`"Temperature"\s*=\s*(\d+)`)
			if m := tempRe.FindStringSubmatch(ioStr); len(m) > 1 {
				if tVal, tErr := strconv.ParseFloat(m[1], 64); tErr == nil {
					if tVal > 1000 {
						res.Temperature = tVal / 100.0
					} else {
						res.Temperature = tVal
					}
				}
			}
		}
	}

	return res
}

// collectTopProcesses lists top active processes sorted by CPU usage using ps.
func collectTopProcesses(limit int) []ProcessInfo {
	if limit <= 0 {
		limit = 10
	}

	out, err := exec.Command("ps", "-A", "-o", "pid,%cpu,rss,state,comm", "-r").Output()
	if err != nil {
		return nil
	}

	var results []ProcessInfo
	scanner := bufio.NewScanner(bytes.NewReader(out))

	// Skip header
	if scanner.Scan() {
		_ = scanner.Text()
	}

	for scanner.Scan() && len(results) < limit {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		cpuPct, _ := strconv.ParseFloat(fields[1], 64)
		rssKB, _ := strconv.ParseUint(fields[2], 10, 64)
		state := fields[3]
		isZombie := strings.Contains(state, "Z")

		// Recombine the executable path in case of spaces
		rawComm := strings.Join(fields[4:], " ")
		baseName := filepath.Base(rawComm)
		if strings.HasSuffix(baseName, ".app") {
			baseName = strings.TrimSuffix(baseName, ".app")
		}

		results = append(results, ProcessInfo{
			PID:        pid,
			Name:       baseName,
			CPUPercent: cpuPct,
			MemBytes:   rssKB * 1024,
			IsZombie:   isZombie,
		})
	}

	return results
}
