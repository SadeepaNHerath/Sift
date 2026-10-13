package analyze

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// formatBytes wraps core.FormatBytes for package convenience.
func formatBytes(b int64) string {
	return core.FormatBytes(b)
}

// View renders the entire disk analyzer UI into a styled ANSI string.
func (m Model) View() string {
	if m.ConfirmingDelete {
		return m.renderDeleteModal()
	}

	var sb strings.Builder
	w := m.Width
	if w <= 0 {
		w = 80
	}

	// 1. Breadcrumb Bar & Storage Usage Header
	sb.WriteString(m.renderBreadcrumbBar(w))
	sb.WriteString("\n")

	// 2. Mode Title / Subheader
	sb.WriteString(m.renderSubheader())
	sb.WriteString("\n\n")

	// 3. Tabular Item List
	sb.WriteString(m.renderTable(w))
	sb.WriteString("\n")

	// 4. Search input or Filter notice
	if m.Searching {
		sb.WriteString(m.Styles.SearchPrompt.Render("  Search: /") + m.Styles.SearchQuery.Render(m.SearchQuery+"█") + "\n")
	} else if m.SearchQuery != "" {
		sb.WriteString(m.Styles.Help.Render(fmt.Sprintf("  Filter: %q (press / to edit, Esc to clear)", m.SearchQuery)) + "\n")
	}

	// 5. Status line (if any)
	if m.StatusMsg != "" {
		sb.WriteString("  " + m.Styles.StatusMsg.Render(m.StatusMsg) + "\n")
	}

	// 6. Help footer with keybindings
	sb.WriteString(m.renderFooter())

	return sb.String()
}

// renderBreadcrumbBar formats the current directory path and system disk storage metrics.
func (m Model) renderBreadcrumbBar(totalWidth int) string {
	home := core.GetHomeDir()
	displayPath := m.CurrentPath
	if home != "" && strings.HasPrefix(displayPath, home) {
		displayPath = "~" + strings.TrimPrefix(displayPath, home)
	}

	parts := strings.Split(displayPath, "/")
	var nonEmpties []string
	for _, p := range parts {
		if p != "" {
			nonEmpties = append(nonEmpties, p)
		}
	}

	breadcrumb := strings.Join(nonEmpties, " › ")
	if breadcrumb == "" {
		breadcrumb = "/"
	}
	left := m.Styles.Breadcrumb.Render("📂 " + breadcrumb)

	total, _, free, err := core.GetDiskUsage()
	var right string
	if err == nil && total > 0 {
		pct := float64(total-free) / float64(total) * 100.0
		right = m.Styles.DiskUsage.Render(fmt.Sprintf(" Disk: %s free / %s (%.1f%% used)",
			formatBytes(free), formatBytes(total), pct))
	}

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	pad := totalWidth - leftWidth - rightWidth
	if pad < 2 {
		pad = 2
	}

	return left + strings.Repeat(" ", pad) + right
}

// renderSubheader prints the active view mode and item statistics.
func (m Model) renderSubheader() string {
	if m.Scanning {
		return m.Styles.Help.Render("  ⏳ Scanning files and calculating directory sizes...")
	}

	if m.ShowTopFiles {
		return m.Styles.Title.Render(fmt.Sprintf("  🔥 Top %d Largest Files (%d discovered)",
			m.TopCount, len(m.Items)))
	}

	var totalSize int64
	if m.RootEntry != nil {
		totalSize = m.RootEntry.Size
	}
	return m.Styles.Title.Render(fmt.Sprintf("  📁 Directory Items: %d total (%s)",
		len(m.Items), formatBytes(totalSize)))
}

// renderTable renders the visible rows with icons, names, size bars, sizes, and item counts.
func (m Model) renderTable(totalWidth int) string {
	if len(m.Filtered) == 0 {
		if m.Scanning {
			return m.Styles.Help.Render("     Scanning directory...")
		}
		if m.SearchQuery != "" {
			return m.Styles.Help.Render("     (no items matching search filter)")
		}
		return m.Styles.Help.Render("     (empty directory)")
	}

	visibleRows := m.visibleRowCount()
	start := m.Offset
	end := start + visibleRows
	if end > len(m.Filtered) {
		end = len(m.Filtered)
	}

	// Calculate max size for relative bar graph scaling
	var maxSize int64
	for _, it := range m.Items {
		if it.Size > maxSize {
			maxSize = it.Size
		}
	}
	if maxSize <= 0 {
		maxSize = 1
	}

	var sb strings.Builder

	// Table header
	headerCursor := "  "
	headerSel := "   "
	headerName := "Name"
	headerBar := "Relative Size"
	headerBytes := "Size"
	headerItems := "Items"

	// Width allocations
	const (
		barLen   = 12
		bytesLen = 10
		itemsLen = 12
	)

	// Available width for item name
	// totalWidth - cursor(2) - sel(4) - icon(3) - bar(barLen+2) - bytes(bytesLen) - items(itemsLen) - padding
	fixedWidth := 2 + 4 + 3 + (barLen + 2) + bytesLen + itemsLen + 6
	nameWidth := totalWidth - fixedWidth
	if nameWidth < 16 {
		nameWidth = 16
	}

	headerLine := fmt.Sprintf("%s%s %-*s  %-*s %*s %*s",
		headerCursor,
		headerSel,
		nameWidth, headerName,
		barLen+2, headerBar,
		bytesLen, headerBytes,
		itemsLen, headerItems,
	)
	sb.WriteString(m.Styles.Help.Render(headerLine))
	sb.WriteString("\n")
	sb.WriteString(m.Styles.Help.Render("  " + strings.Repeat("─", totalWidth-4)))
	sb.WriteString("\n")

	for i := start; i < end; i++ {
		idx := m.Filtered[i]
		item := m.Items[idx]

		// Cursor
		cursor := "  "
		if i == m.Cursor {
			cursor = m.Styles.Cursor.Render("▸ ")
		}

		// Selection
		sel := "[ ] "
		if m.Selected[item.Path] {
			sel = m.Styles.Selected.Render("[✓] ")
		}

		// Icon
		var icon string
		if item.IsDir {
			icon = m.Styles.FolderIcon.Render("📁 ")
		} else {
			icon = m.Styles.FileIcon.Render("📄 ")
		}

		// Truncated Name
		displayName := item.Name
		if len(displayName) > nameWidth {
			displayName = displayName[:nameWidth-3] + "..."
		}

		var styledName string
		if i == m.Cursor {
			styledName = m.Styles.Cursor.Render(fmt.Sprintf("%-*s", nameWidth, displayName))
		} else {
			styledName = m.Styles.Normal.Render(fmt.Sprintf("%-*s", nameWidth, displayName))
		}

		// Bar graph
		ratio := float64(item.Size) / float64(maxSize)
		filled := int(math.Round(ratio * float64(barLen)))
		if filled < 0 {
			filled = 0
		} else if filled > barLen {
			filled = barLen
		}

		barStr := fmt.Sprintf("[%s%s]",
			m.Styles.SizeBarFilled.Render(strings.Repeat("█", filled)),
			m.Styles.SizeBarEmpty.Render(strings.Repeat("░", barLen-filled)),
		)

		// Size string
		sizeStr := m.Styles.SizeText.Render(fmt.Sprintf("%*s", bytesLen, formatBytes(item.Size)))

		// Items count string
		var countStr string
		if item.IsDir {
			countStr = m.Styles.CountText.Render(fmt.Sprintf("%*s", itemsLen, fmt.Sprintf("%d items", item.ChildrenCount)))
		} else {
			countStr = m.Styles.CountText.Render(fmt.Sprintf("%*s", itemsLen, "—"))
		}

		row := fmt.Sprintf("%s%s%s%s  %s %s %s",
			cursor,
			sel,
			icon,
			styledName,
			barStr,
			sizeStr,
			countStr,
		)
		sb.WriteString(row)
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderFooter generates the help keybindings text.
func (m Model) renderFooter() string {
	if m.Searching {
		return m.Styles.Help.Render("  Esc: Clear & Exit Search  •  Enter: Apply Filter  •  ↑/↓: Navigate")
	}

	if m.ShowTopFiles {
		return m.Styles.Help.Render("  ↑/↓: Navigate • t: Tree View • Space: Select • o: Open • f: Finder • p: Preview • d/Del: Trash • q: Quit")
	}

	return m.Styles.Help.Render("  ↑/↓/j/k: Navigate • Enter/l: Open • ←/h/Esc: Up • Space: Select • t: Top Files • /: Search • d/Del: Trash • r: Rescan • q: Quit")
}
