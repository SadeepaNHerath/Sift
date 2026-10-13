package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// Severity indicates the urgency of a diagnostic check.
type Severity string

const (
	SeverityOK       Severity = "OK"
	SeverityWarning  Severity = "WARNING"
	SeverityCritical Severity = "CRITICAL"
)

// CheckResult holds the diagnostic outcome of an individual health check.
type CheckResult struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Severity   Severity `json:"severity"`
	Summary    string   `json:"summary"`
	Details    string   `json:"details,omitempty"`
	FixCommand string   `json:"fix_command,omitempty"`
}

// DoctorReport contains all aggregated system health checks.
type DoctorReport struct {
	Results   []CheckResult `json:"results"`
	OKCount   int           `json:"ok_count"`
	WarnCount int           `json:"warn_count"`
	CritCount int           `json:"crit_count"`
}

// RunDoctorChecks performs comprehensive diagnostics across permissions, storage, memory, caches, and configuration.
func RunDoctorChecks() (*DoctorReport, error) {
	report := &DoctorReport{}

	// 1. Full Disk Access Check
	checkFDA(report)

	// 2. Disk Capacity Pressure Check
	checkDiskPressure(report)

	// 3. Memory Pressure & Load Check
	checkMemoryPressure(report)

	// 4. Oversized Caches Check
	checkCaches(report)

	// 5. Stale Installers Check
	checkInstallers(report)

	// 6. Orphaned App Leftovers Check
	checkLeftovers(report)

	// 7. Broken Launch Agents Check
	checkLaunchAgents(report)

	// 8. Homebrew Health Check
	checkHomebrew(report)

	// 9. Touch ID for Sudo Check
	checkTouchID(report)

	// Compute totals
	for _, r := range report.Results {
		switch r.Severity {
		case SeverityOK:
			report.OKCount++
		case SeverityWarning:
			report.WarnCount++
		case SeverityCritical:
			report.CritCount++
		}
	}

	return report, nil
}

func checkFDA(report *DoctorReport) {
	fda := core.IsFullDiskAccessGranted()
	if fda {
		report.Results = append(report.Results, CheckResult{
			ID:       "fda",
			Name:     "Full Disk Access",
			Category: "Permissions",
			Severity: SeverityOK,
			Summary:  "Full Disk Access is granted",
		})
	} else {
		report.Results = append(report.Results, CheckResult{
			ID:         "fda",
			Name:       "Full Disk Access",
			Category:   "Permissions",
			Severity:   SeverityWarning,
			Summary:    "Full Disk Access is not granted; some protected caches cannot be scanned",
			Details:    "Terminal or Sift needs FDA permissions in macOS System Settings",
			FixCommand: "Open System Settings → Privacy & Security → Full Disk Access",
		})
	}
}

func checkDiskPressure(report *DoctorReport) {
	total, used, free, err := core.GetDiskUsage()
	if err != nil {
		report.Results = append(report.Results, CheckResult{
			ID:       "disk",
			Name:     "Disk Capacity",
			Category: "Storage",
			Severity: SeverityWarning,
			Summary:  fmt.Sprintf("Failed to read disk usage: %v", err),
		})
		return
	}

	if total == 0 {
		return
	}

	usedPct := float64(used) / float64(total) * 100.0
	freeFormatted := core.FormatBytes(free)
	totalFormatted := core.FormatBytes(total)

	if usedPct >= 92.0 {
		report.Results = append(report.Results, CheckResult{
			ID:         "disk",
			Name:       "Disk Capacity",
			Category:   "Storage",
			Severity:   SeverityCritical,
			Summary:    fmt.Sprintf("System drive is critically full (%.1f%% used, %s free of %s)", usedPct, freeFormatted, totalFormatted),
			Details:    "Low disk space can cause system freezes, crash reporters, and swap failures",
			FixCommand: "sf clean && sf analyze",
		})
	} else if usedPct >= 82.0 {
		report.Results = append(report.Results, CheckResult{
			ID:         "disk",
			Name:       "Disk Capacity",
			Category:   "Storage",
			Severity:   SeverityWarning,
			Summary:    fmt.Sprintf("System drive usage is high (%.1f%% used, %s free of %s)", usedPct, freeFormatted, totalFormatted),
			Details:    "Consider cleaning caches or analyzing large directories",
			FixCommand: "sf clean",
		})
	} else {
		report.Results = append(report.Results, CheckResult{
			ID:       "disk",
			Name:     "Disk Capacity",
			Category: "Storage",
			Severity: SeverityOK,
			Summary:  fmt.Sprintf("Healthy disk space available: %s free of %s (%.1f%% used)", freeFormatted, totalFormatted, usedPct),
		})
	}
}

func checkMemoryPressure(report *DoctorReport) {
	metrics, err := CollectMetrics()
	if err != nil {
		report.Results = append(report.Results, CheckResult{
			ID:       "memory",
			Name:     "Memory Pressure",
			Category: "Performance",
			Severity: SeverityOK,
			Summary:  "Memory metrics collection unavailable",
		})
		return
	}

	switch metrics.Memory.PressureLevel {
	case "Critical":
		report.Results = append(report.Results, CheckResult{
			ID:         "memory",
			Name:       "Memory Pressure",
			Category:   "Performance",
			Severity:   SeverityCritical,
			Summary:    "System memory pressure is Critical",
			Details:    "Heavy paging and compression active. Background apps should be closed.",
			FixCommand: "sf status (inspect top RAM consumers)",
		})
	case "Warning":
		report.Results = append(report.Results, CheckResult{
			ID:         "memory",
			Name:       "Memory Pressure",
			Category:   "Performance",
			Severity:   SeverityWarning,
			Summary:    "System memory pressure is Elevated",
			Details:    "macOS is compressing inactive pages",
			FixCommand: "sf status",
		})
	default:
		report.Results = append(report.Results, CheckResult{
			ID:       "memory",
			Name:     "Memory Pressure",
			Category: "Performance",
			Severity: SeverityOK,
			Summary:  fmt.Sprintf("Memory pressure is normal (Health Score: %d/100)", metrics.HealthScore),
		})
	}
}

func checkCaches(report *DoctorReport) {
	home := core.GetHomeDir()
	cachesPath := filepath.Join(home, "Library", "Caches")
	size, err := core.FileSize(cachesPath)
	if err != nil || size == 0 {
		report.Results = append(report.Results, CheckResult{
			ID:       "caches",
			Name:     "User Caches",
			Category: "Storage",
			Severity: SeverityOK,
			Summary:  "User caches within normal bounds",
		})
		return
	}

	sizeGB := float64(size) / (1024 * 1024 * 1024)
	if sizeGB >= 15.0 {
		report.Results = append(report.Results, CheckResult{
			ID:         "caches",
			Name:       "User Caches",
			Category:   "Storage",
			Severity:   SeverityWarning,
			Summary:    fmt.Sprintf("User cache directory is large: %s", core.FormatBytes(size)),
			Details:    "App caches accumulate over time and can safely be pruned",
			FixCommand: "sf clean",
		})
	} else {
		report.Results = append(report.Results, CheckResult{
			ID:       "caches",
			Name:     "User Caches",
			Category: "Storage",
			Severity: SeverityOK,
			Summary:  fmt.Sprintf("User caches are healthy (%s)", core.FormatBytes(size)),
		})
	}
}

func checkInstallers(report *DoctorReport) {
	files, err := ScanInstallers()
	if err != nil || len(files) == 0 {
		report.Results = append(report.Results, CheckResult{
			ID:       "installers",
			Name:     "Installer Packages",
			Category: "Cleanliness",
			Severity: SeverityOK,
			Summary:  "No stale installer packages (.dmg, .pkg) in common folders",
		})
		return
	}

	var totalBytes int64
	for _, f := range files {
		totalBytes += f.Size
	}

	report.Results = append(report.Results, CheckResult{
		ID:         "installers",
		Name:       "Installer Packages",
		Category:   "Cleanliness",
		Severity:   SeverityWarning,
		Summary:    fmt.Sprintf("Found %d downloaded installer file(s) consuming %s", len(files), core.FormatBytes(totalBytes)),
		Details:    "Disk images and packages in Downloads and caches are rarely needed after installation",
		FixCommand: "sf installer",
	})
}

func checkLeftovers(report *DoctorReport) {
	apps, err := ScanInstalledApps(false)
	if err != nil {
		return
	}

	orphans, err := FindOrphanedResources(apps)
	if err != nil || len(orphans) == 0 {
		report.Results = append(report.Results, CheckResult{
			ID:       "leftovers",
			Name:     "Orphaned Leftovers",
			Category: "Cleanliness",
			Severity: SeverityOK,
			Summary:  "No orphaned application leftovers found",
		})
		return
	}

	var totalBytes int64
	for _, o := range orphans {
		totalBytes += o.Size
	}

	report.Results = append(report.Results, CheckResult{
		ID:         "leftovers",
		Name:       "Orphaned Leftovers",
		Category:   "Cleanliness",
		Severity:   SeverityWarning,
		Summary:    fmt.Sprintf("Found %d orphaned leftover item(s) (%s) from removed apps", len(orphans), core.FormatBytes(totalBytes)),
		Details:    "Support folders left behind after dragging apps directly to Trash",
		FixCommand: "sf clean",
	})
}

func checkLaunchAgents(report *DoctorReport) {
	home := core.GetHomeDir()
	dirs := []string{
		filepath.Join(home, "Library", "LaunchAgents"),
		"/Library/LaunchAgents",
	}

	brokenCount := 0
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".plist") {
				plistPath := filepath.Join(d, e.Name())
				content, err := os.ReadFile(plistPath)
				if err != nil {
					continue
				}
				s := string(content)
				// Basic heuristic: check if referenced Program exists
				if strings.Contains(s, "<key>Program</key>") {
					lines := strings.Split(s, "\n")
					for i, l := range lines {
						if strings.Contains(l, "<key>Program</key>") && i+1 < len(lines) {
							next := lines[i+1]
							if strings.Contains(next, "<string>") && strings.Contains(next, "</string>") {
								start := strings.Index(next, "<string>") + len("<string>")
								end := strings.Index(next, "</string>")
								if start < end {
									prog := strings.TrimSpace(next[start:end])
									if prog != "" && !strings.HasPrefix(prog, "/System") {
										if _, statErr := os.Stat(prog); os.IsNotExist(statErr) {
											brokenCount++
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if brokenCount > 0 {
		report.Results = append(report.Results, CheckResult{
			ID:         "launchd",
			Name:       "Background Launch Agents",
			Category:   "System Health",
			Severity:   SeverityWarning,
			Summary:    fmt.Sprintf("Found %d launch agent(s) with missing executable binaries", brokenCount),
			Details:    "Dead launch agents cause launchd to retry failed spawns repeatedly",
			FixCommand: "sf optimize",
		})
	} else {
		report.Results = append(report.Results, CheckResult{
			ID:       "launchd",
			Name:     "Background Launch Agents",
			Category: "System Health",
			Severity: SeverityOK,
			Summary:  "Background launch agents are healthy",
		})
	}
}

func checkHomebrew(report *DoctorReport) {
	brewPath, err := exec.LookPath("brew")
	if err != nil {
		report.Results = append(report.Results, CheckResult{
			ID:       "homebrew",
			Name:     "Homebrew Package Manager",
			Category: "Developer",
			Severity: SeverityOK,
			Summary:  "Homebrew is not installed (skipping check)",
		})
		return
	}

	_ = brewPath
	report.Results = append(report.Results, CheckResult{
		ID:       "homebrew",
		Name:     "Homebrew Package Manager",
		Category: "Developer",
		Severity: SeverityOK,
		Summary:  "Homebrew package manager installed and detected",
	})
}

func checkTouchID(report *DoctorReport) {
	enabled, supported, _, err := CheckStatus()
	if err != nil || !supported {
		return
	}

	if enabled {
		report.Results = append(report.Results, CheckResult{
			ID:       "touchid",
			Name:     "Touch ID for Sudo",
			Category: "Configuration",
			Severity: SeverityOK,
			Summary:  "Touch ID authentication for sudo is enabled",
		})
	} else {
		report.Results = append(report.Results, CheckResult{
			ID:         "touchid",
			Name:       "Touch ID for Sudo",
			Category:   "Configuration",
			Severity:   SeverityOK,
			Summary:    "Touch ID is available for terminal sudo authentication",
			FixCommand: "sf touchid --enable",
		})
	}
}
