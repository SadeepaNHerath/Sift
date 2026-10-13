package status

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the complete status dashboard UI.
func (m Model) View() string {
	if m.Quitting {
		return ""
	}

	if m.Metrics == nil {
		if m.Err != nil {
			return fmt.Sprintf("\n  Error collecting system metrics: %v\n  Press 'q' to quit.\n", m.Err)
		}
		return "\n  Gathering system telemetry...\n"
	}

	width := m.Width
	if width < 80 {
		width = 80
	}

	var b strings.Builder

	// 1. Header with System Identity, Health Score Badge, and Cat Mascot
	b.WriteString(m.renderHeader(width))
	b.WriteString("\n")

	// 2. Main resource panels
	halfWidth := (width - 4) / 2
	if halfWidth < 38 {
		halfWidth = 38
	}

	if width >= 90 {
		// Side-by-side 2-column layout
		cpuPanel := m.renderCPU(halfWidth)
		memPanel := m.renderMemory(halfWidth)
		row1 := lipgloss.JoinHorizontal(lipgloss.Top, cpuPanel, " ", memPanel)
		b.WriteString(row1)
		b.WriteString("\n")

		diskPanel := m.renderDisk(halfWidth)
		netPanel := m.renderNetworkAndBattery(halfWidth)
		row2 := lipgloss.JoinHorizontal(lipgloss.Top, diskPanel, " ", netPanel)
		b.WriteString(row2)
		b.WriteString("\n")
	} else {
		// Single-column stacked layout for narrow terminals
		b.WriteString(m.renderCPU(width - 2))
		b.WriteString("\n")
		b.WriteString(m.renderMemory(width - 2))
		b.WriteString("\n")
		b.WriteString(m.renderDisk(width - 2))
		b.WriteString("\n")
		b.WriteString(m.renderNetworkAndBattery(width - 2))
		b.WriteString("\n")
	}

	// 3. Top Processes Table
	b.WriteString(m.renderProcesses(width - 2))
	b.WriteString("\n")

	// 4. Footer Help bar
	b.WriteString(m.renderFooter(width))

	return b.String()
}

// renderHeader renders system identity, uptime, composite health badge, and mascot.
func (m Model) renderHeader(width int) string {
	sys := m.Metrics

	// Health score badge
	score := sys.HealthScore
	var healthColor lipgloss.Color
	var healthDesc string
	var healthIcon string

	switch {
	case score >= 90:
		healthColor = m.Styles.GreenColor
		healthDesc = "OPTIMAL"
		healthIcon = "🟢"
	case score >= 75:
		healthColor = m.Styles.YellowColor
		healthDesc = "GOOD"
		healthIcon = "🟡"
	case score >= 50:
		healthColor = lipgloss.Color("#FFB86C") // Orange
		healthDesc = "FAIR"
		healthIcon = "🟠"
	default:
		healthColor = m.Styles.RedColor
		healthDesc = "DEGRADED"
		healthIcon = "🔴"
	}

	healthBadge := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#1E1F29")).
		Background(healthColor).
		Padding(0, 1).
		Render(fmt.Sprintf("%s %d/100 %s", healthIcon, score, healthDesc))

	title := lipgloss.NewStyle().Bold(true).Foreground(m.Styles.TitleColor).Render("⚡ SIFT SYSTEM STATUS")

	infoLine := fmt.Sprintf(
		"%s • %s • %s • Uptime: %s",
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8F8F2")).Render(sys.Hostname),
		sys.OSVersion,
		sys.Arch,
		lipgloss.NewStyle().Foreground(m.Styles.CyanColor).Render(sys.UptimeStr),
	)

	leftContent := lipgloss.JoinVertical(lipgloss.Left, title, infoLine)

	var headerContent string
	if m.ShowCat {
		catAscii := m.getCatMascot()
		headerContent = lipgloss.JoinHorizontal(lipgloss.Center, leftContent, "   ", healthBadge, "   ", catAscii)
	} else {
		headerContent = lipgloss.JoinHorizontal(lipgloss.Center, leftContent, "   ", healthBadge)
	}

	return m.Styles.HeaderBox.Width(width - 2).Render(headerContent)
}

// getCatMascot returns styled animated ASCII cat companion frames.
func (m Model) getCatMascot() string {
	frame := (m.CatFrame) % 4
	var face string
	var statusMsg string

	switch frame {
	case 0:
		face = "( o.o )"
		statusMsg = "Purring smoothly"
	case 1:
		face = "( -.- )"
		statusMsg = "All systems green"
	case 2:
		face = "( ^.^ )"
		statusMsg = "Optimal condition"
	default:
		face = "( =.= )"
		statusMsg = "Watching resources"
	}

	catStyle := lipgloss.NewStyle().Foreground(m.Styles.AccentColor).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(m.Styles.DimColor).Italic(true)

	lines := []string{
		catStyle.Render(" /\\_/\\   ฅ(•ᆺ•ฅ)"),
		catStyle.Render(face) + "  " + textStyle.Render(statusMsg),
		catStyle.Render(" > ^ <"),
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderCPU builds the CPU utilization panel with overall gauge and per-core breakdown.
func (m Model) renderCPU(panelWidth int) string {
	cpu := m.Metrics.CPU
	innerW := panelWidth - 4
	if innerW < 30 {
		innerW = 30
	}

	coreCount := len(cpu.PerCorePercent)
	displayCores := coreCount
	coreNotice := "all"
	if m.CoreLimit > 0 && m.CoreLimit < coreCount {
		displayCores = m.CoreLimit
		coreNotice = fmt.Sprintf("%d cores", m.CoreLimit)
	}

	header := fmt.Sprintf(
		"%s %s",
		m.Styles.Title.Render("⚙  CPU UTILIZATION"),
		lipgloss.NewStyle().Foreground(m.Styles.DimColor).Render(fmt.Sprintf("(%s, [c] cycles)", coreNotice)),
	)

	// Overall Gauge
	barW := innerW - 14
	if barW > 25 {
		barW = 25
	}
	if barW < 10 {
		barW = 10
	}

	totalGauge := renderBar(cpu.TotalPercent, barW, m.getPercentColor(cpu.TotalPercent), lipgloss.Color("#44475A"))
	overallLine := fmt.Sprintf("Overall: %s", totalGauge)

	// Load Average
	loadLine := fmt.Sprintf(
		"Load Avg: %s (1m)  %s (5m)  %s (15m)",
		m.Styles.Value.Render(fmt.Sprintf("%.2f", cpu.Load1)),
		m.Styles.Value.Render(fmt.Sprintf("%.2f", cpu.Load5)),
		m.Styles.Value.Render(fmt.Sprintf("%.2f", cpu.Load15)),
	)

	// Per-core bars
	var coreLines []string
	miniBarW := (innerW/2 - 14)
	if miniBarW > 12 {
		miniBarW = 12
	}
	if miniBarW < 6 {
		miniBarW = 6
	}

	for i := 0; i < displayCores; i += 2 {
		var leftCore, rightCore string
		pct1 := cpu.PerCorePercent[i]
		leftCore = fmt.Sprintf("C%-2d %s", i+1, renderMiniBar(pct1, miniBarW, m.getPercentColor(pct1)))

		if i+1 < displayCores {
			pct2 := cpu.PerCorePercent[i+1]
			rightCore = fmt.Sprintf("C%-2d %s", i+2, renderMiniBar(pct2, miniBarW, m.getPercentColor(pct2)))
		}

		coreLines = append(coreLines, fmt.Sprintf("%-18s  %s", leftCore, rightCore))
	}

	content := []string{
		header,
		overallLine,
		loadLine,
	}
	if len(coreLines) > 0 {
		content = append(content, strings.Join(coreLines, "\n"))
	}

	return m.Styles.PanelBox.Width(panelWidth).Render(strings.Join(content, "\n"))
}

// renderMemory builds the memory metrics panel with pressure gauge and breakdown.
func (m Model) renderMemory(panelWidth int) string {
	mem := m.Metrics.Memory
	innerW := panelWidth - 4
	if innerW < 30 {
		innerW = 30
	}

	// Pressure Badge
	var pressureBadge string
	switch strings.ToLower(mem.PressureLevel) {
	case "critical":
		pressureBadge = lipgloss.NewStyle().Bold(true).Foreground(m.Styles.RedColor).Render("CRITICAL 🔴")
	case "warning":
		pressureBadge = lipgloss.NewStyle().Bold(true).Foreground(m.Styles.YellowColor).Render("WARNING 🟡")
	default:
		pressureBadge = lipgloss.NewStyle().Bold(true).Foreground(m.Styles.GreenColor).Render("NORMAL 🟢")
	}

	header := fmt.Sprintf(
		"%s   Pressure: %s",
		m.Styles.Title.Render("🧠 MEMORY ALLOCATION"),
		pressureBadge,
	)

	var usedPct float64
	if mem.Total > 0 {
		usedPct = (float64(mem.Used) / float64(mem.Total)) * 100.0
	}

	barW := innerW - 14
	if barW > 25 {
		barW = 25
	}
	if barW < 10 {
		barW = 10
	}

	gauge := renderBar(usedPct, barW, m.getPercentColor(usedPct), lipgloss.Color("#44475A"))
	utilLine := fmt.Sprintf(
		"Used: %s / %s (%s)",
		formatBytes(mem.Used),
		formatBytes(mem.Total),
		gauge,
	)

	// Memory breakdown
	gridLine1 := fmt.Sprintf(
		"Wired:      %-10s   Active:     %-10s",
		m.Styles.Value.Render(formatBytes(mem.Wired)),
		m.Styles.Value.Render(formatBytes(mem.Active)),
	)
	gridLine2 := fmt.Sprintf(
		"Compressed: %-10s   Free:       %-10s",
		m.Styles.Value.Render(formatBytes(mem.Compressed)),
		m.Styles.Value.Render(formatBytes(mem.Free)),
	)
	availLine := fmt.Sprintf(
		"Available:  %s",
		m.Styles.Value.Render(formatBytes(mem.Available)),
	)

	content := strings.Join([]string{
		header,
		utilLine,
		gridLine1,
		gridLine2,
		availLine,
	}, "\n")

	return m.Styles.PanelBox.Width(panelWidth).Render(content)
}

// renderDisk builds the disk capacity and throughput rates panel.
func (m Model) renderDisk(panelWidth int) string {
	d := m.Metrics.Disk
	innerW := panelWidth - 4
	if innerW < 30 {
		innerW = 30
	}

	header := m.Styles.Title.Render("💾 STORAGE & DISK I/O")

	barW := innerW - 14
	if barW > 25 {
		barW = 25
	}
	if barW < 10 {
		barW = 10
	}

	gauge := renderBar(d.UsedPercent, barW, m.getPercentColor(d.UsedPercent), lipgloss.Color("#44475A"))
	usageLine := fmt.Sprintf(
		"Root (/): %s / %s (%s)",
		formatBytes(d.Used),
		formatBytes(d.Total),
		gauge,
	)

	ioLine := fmt.Sprintf(
		"Read:  %-12s  │  Write: %-12s",
		m.Styles.Value.Render(formatRate(d.ReadBytesRate)),
		m.Styles.Value.Render(formatRate(d.WriteBytesRate)),
	)

	freeLine := fmt.Sprintf(
		"Free Storage: %s",
		lipgloss.NewStyle().Bold(true).Foreground(m.Styles.GreenColor).Render(formatBytes(d.Free)),
	)

	content := strings.Join([]string{
		header,
		usageLine,
		ioLine,
		freeLine,
	}, "\n")

	return m.Styles.PanelBox.Width(panelWidth).Render(content)
}

// renderNetworkAndBattery combines network I/O sparkline history and battery hardware telemetry.
func (m Model) renderNetworkAndBattery(panelWidth int) string {
	net := m.Metrics.Network
	batt := m.Metrics.Battery
	innerW := panelWidth - 4
	if innerW < 30 {
		innerW = 30
	}

	netHeader := fmt.Sprintf(
		"%s %s",
		m.Styles.Title.Render("🌐 NETWORK"),
		lipgloss.NewStyle().Foreground(m.Styles.DimColor).Render(fmt.Sprintf("[%s]", net.InterfaceName)),
	)

	netRates := fmt.Sprintf(
		"▲ Sent: %-11s  │  ▼ Recv: %-11s",
		m.Styles.Value.Render(formatRate(net.BytesSentRate)),
		m.Styles.Value.Render(formatRate(net.BytesRecvRate)),
	)

	sparkline := renderSparkline(net.SparklinePoints, innerW-12)
	sparkLine := fmt.Sprintf("Activity: %s", sparkline)

	// Battery / Hardware section
	battHeader := m.Styles.Title.Render("🔋 BATTERY & HARDWARE")
	var battDetails string
	if batt.Present {
		var stateStr string
		if batt.Charging {
			stateStr = lipgloss.NewStyle().Bold(true).Foreground(m.Styles.GreenColor).Render("Charging ⚡")
		} else {
			stateStr = lipgloss.NewStyle().Foreground(m.Styles.YellowColor).Render("Discharging 🔋")
		}

		tempStr := "N/A"
		if batt.Temperature > 0 {
			tempStr = fmt.Sprintf("%.1f°C", batt.Temperature)
		}

		battBar := renderMiniBar(batt.Percent, 10, m.getPercentColor(batt.Percent))
		battDetails = fmt.Sprintf(
			"Level: %s (%s)  Cycles: %s  Temp: %s",
			battBar,
			stateStr,
			m.Styles.Value.Render(fmt.Sprintf("%d", batt.CycleCount)),
			m.Styles.Value.Render(tempStr),
		)
	} else {
		battDetails = lipgloss.NewStyle().Foreground(m.Styles.DimColor).Render("Desktop Mac (AC Powered) • Thermal: Optimal")
	}

	content := strings.Join([]string{
		netHeader,
		netRates,
		sparkLine,
		battHeader,
		battDetails,
	}, "\n")

	return m.Styles.PanelBox.Width(panelWidth).Render(content)
}

// renderProcesses renders a concise table of top CPU and memory consumers.
func (m Model) renderProcesses(width int) string {
	procs := m.Metrics.TopProcesses

	header := m.Styles.Title.Render("⚡ TOP PROCESSES (CPU & MEMORY)")

	colHeader := fmt.Sprintf(
		"  %-7s  %-26s  %8s  %12s  %s",
		"PID", "NAME", "CPU %", "MEMORY", "STATUS",
	)
	colHeaderStyled := lipgloss.NewStyle().Bold(true).Foreground(m.Styles.DimColor).Render(colHeader)

	var rows []string
	maxRows := 6
	if len(procs) < maxRows {
		maxRows = len(procs)
	}

	for i := 0; i < maxRows; i++ {
		p := procs[i]
		name := p.Name
		if len(name) > 24 {
			name = name[:21] + "..."
		}

		statusStr := lipgloss.NewStyle().Foreground(m.Styles.GreenColor).Render("Running")
		if p.IsZombie {
			statusStr = lipgloss.NewStyle().Bold(true).Foreground(m.Styles.RedColor).Render("Zombie 💀")
		}

		cpuStr := fmt.Sprintf("%6.1f%%", p.CPUPercent)
		if p.CPUPercent > 50.0 {
			cpuStr = lipgloss.NewStyle().Bold(true).Foreground(m.Styles.RedColor).Render(cpuStr)
		} else if p.CPUPercent > 10.0 {
			cpuStr = lipgloss.NewStyle().Bold(true).Foreground(m.Styles.YellowColor).Render(cpuStr)
		}

		row := fmt.Sprintf(
			"  %-7d  %-26s  %8s  %12s  %s",
			p.PID,
			name,
			cpuStr,
			formatBytes(p.MemBytes),
			statusStr,
		)
		rows = append(rows, row)
	}

	all := append([]string{header, colHeaderStyled}, rows...)
	return m.Styles.PanelBox.Width(width).Render(strings.Join(all, "\n"))
}

// renderFooter renders the keyboard navigation cheat sheet.
func (m Model) renderFooter(width int) string {
	keys := []string{
		"[q] Quit",
		"[c] Cycle Cores",
		"[k] Toggle Mascot 🐱",
		"[r] Refresh",
		"• Sampling 1s",
	}
	return m.Styles.HelpBar.Render("  " + strings.Join(keys, "   "))
}

// getPercentColor returns a contextual color based on utilization threshold.
func (m Model) getPercentColor(pct float64) lipgloss.Color {
	switch {
	case pct >= 85.0:
		return m.Styles.RedColor
	case pct >= 65.0:
		return m.Styles.YellowColor
	default:
		return m.Styles.GreenColor
	}
}

// renderBar generates a progress gauge with percentage label: [████░░░░] 50.0%.
func renderBar(pct float64, barWidth int, filledColor, emptyColor lipgloss.Color) string {
	if barWidth < 4 {
		barWidth = 4
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	filledLen := int(math.Round((pct / 100.0) * float64(barWidth)))
	if filledLen > barWidth {
		filledLen = barWidth
	}
	emptyLen := barWidth - filledLen

	filledStr := lipgloss.NewStyle().Foreground(filledColor).Render(strings.Repeat("█", filledLen))
	emptyStr := lipgloss.NewStyle().Foreground(emptyColor).Render(strings.Repeat("░", emptyLen))

	return fmt.Sprintf("[%s%s] %5.1f%%", filledStr, emptyStr, pct)
}

// renderMiniBar generates a compact bar: [██░░] 45%.
func renderMiniBar(pct float64, barWidth int, filledColor lipgloss.Color) string {
	if barWidth < 3 {
		barWidth = 3
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	filledLen := int(math.Round((pct / 100.0) * float64(barWidth)))
	if filledLen > barWidth {
		filledLen = barWidth
	}
	emptyLen := barWidth - filledLen

	filledStr := lipgloss.NewStyle().Foreground(filledColor).Render(strings.Repeat("■", filledLen))
	emptyStr := lipgloss.NewStyle().Foreground(lipgloss.Color("#44475A")).Render(strings.Repeat("□", emptyLen))

	return fmt.Sprintf("[%s%s] %3.0f%%", filledStr, emptyStr, pct)
}

// renderSparkline converts a series of values into a Unicode sparkline.
func renderSparkline(points []float64, width int) string {
	if len(points) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render("──────────")
	}

	sparkChars := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// Determine min and max
	var maxVal float64 = 0.001
	for _, p := range points {
		if p > maxVal {
			maxVal = p
		}
	}

	// Truncate to width if necessary
	sub := points
	if len(sub) > width && width > 0 {
		sub = sub[len(sub)-width:]
	}

	var sb strings.Builder
	for _, val := range sub {
		idx := int((val / maxVal) * float64(len(sparkChars)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkChars) {
			idx = len(sparkChars) - 1
		}
		sb.WriteRune(sparkChars[idx])
	}

	return lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(sb.String())
}

// formatBytes returns human-readable binary byte formats (B, KB, MB, GB, TB).
func formatBytes(b uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)

	switch {
	case b >= tb:
		return fmt.Sprintf("%.2f TB", float64(b)/float64(tb))
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// formatRate returns human-readable throughput rates (B/s, KB/s, MB/s, GB/s).
func formatRate(bps float64) string {
	const (
		kb = 1024.0
		mb = kb * 1024.0
		gb = mb * 1024.0
	)

	switch {
	case bps >= gb:
		return fmt.Sprintf("%.2f GB/s", bps/gb)
	case bps >= mb:
		return fmt.Sprintf("%.1f MB/s", bps/mb)
	case bps >= kb:
		return fmt.Sprintf("%.1f KB/s", bps/kb)
	default:
		return fmt.Sprintf("%.0f B/s", bps)
	}
}
