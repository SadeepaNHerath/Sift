package menu

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the TUI menu into an ANSI-styled string.
func (m Model) View() string {
	var sb strings.Builder

	// 1. Title bar
	selectedCount := 0
	for _, it := range m.items {
		if it.Selected {
			selectedCount++
		}
	}

	var titleText string
	if m.multiSelect {
		titleText = fmt.Sprintf("%s (%d/%d selected)", m.title, selectedCount, len(m.items))
	} else {
		titleText = fmt.Sprintf("%s (%d items)", m.title, len(m.items))
	}

	if m.searchQuery != "" {
		titleText += fmt.Sprintf(" [%d matching]", len(m.filtered))
	}

	sb.WriteString(m.styles.Title.Render(titleText))
	sb.WriteString("\n")

	// 2. Visible items
	if len(m.filtered) == 0 {
		sb.WriteString(m.styles.Help.Render("  (no matching items)"))
		sb.WriteString("\n")
	} else {
		start := m.offset
		end := m.offset + m.height
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		w := m.width
		if w <= 0 {
			w = 80
		}

		for i := start; i < end; i++ {
			itemIdx := m.filtered[i]
			item := m.items[itemIdx]

			// Render category separator if category changes or at start of list
			if item.Category != "" {
				showCategory := false
				if i == 0 {
					showCategory = true
				} else if m.items[m.filtered[i-1]].Category != item.Category {
					showCategory = true
				}

				if showCategory {
					catHeader := fmt.Sprintf("── [ %s ] ", item.Category)
					catWidth := lipgloss.Width(catHeader)
					if w > catWidth {
						catHeader += strings.Repeat("─", w-catWidth)
					}
					sb.WriteString(m.styles.Category.Render(catHeader))
					sb.WriteString("\n")
				}
			}

			// Cursor indicator
			cursorStr := "  "
			if i == m.cursor {
				cursorStr = m.styles.Cursor.Render("▸ ")
			}

			// Selection indicator
			var checkStr string
			if item.Protected {
				checkStr = m.styles.Protected.Render("🔒 ")
			} else if item.Selected {
				checkStr = m.styles.Selected.Render("✓ ")
			} else {
				checkStr = m.styles.Normal.Render("○ ")
			}

			// Title and description
			var titleStr string
			if i == m.cursor {
				titleStr = m.styles.Cursor.Render(item.Title)
			} else {
				titleStr = m.styles.Normal.Render(item.Title)
			}

			var descStr string
			if item.Description != "" {
				descStr = m.styles.Help.Render(" " + item.Description)
			}

			leftPart := cursorStr + checkStr + titleStr + descStr

			// Render right-aligned file size if enabled
			if m.showSizes {
				sizeStr := m.styles.Size.Render(FormatBytes(item.Size))
				leftWidth := lipgloss.Width(leftPart)
				sizeWidth := lipgloss.Width(sizeStr)
				pad := w - leftWidth - sizeWidth
				if pad > 0 {
					sb.WriteString(leftPart + strings.Repeat(" ", pad) + sizeStr)
				} else {
					sb.WriteString(leftPart + "  " + sizeStr)
				}
			} else {
				sb.WriteString(leftPart)
			}
			sb.WriteString("\n")
		}
	}

	// 3. Search bar
	if m.searching {
		sb.WriteString("\n")
		sb.WriteString(m.styles.Search.Render(fmt.Sprintf("Search: /%s█", m.searchQuery)))
		sb.WriteString("\n")
	} else if m.searchQuery != "" {
		sb.WriteString("\n")
		sb.WriteString(m.styles.Help.Render(fmt.Sprintf("Filter active: %q (press / to edit, esc to clear)", m.searchQuery)))
		sb.WriteString("\n")
	}

	// 4. Help footer
	sb.WriteString("\n")
	var helpText string
	if m.searching {
		helpText = "esc: cancel search • enter: done typing • ↑/↓: navigate"
	} else if m.multiSelect {
		helpText = "↑/↓/j/k: navigate • space: select • a: all • i: invert • /: search • enter: confirm • q: quit"
	} else {
		helpText = "↑/↓/j/k: navigate • space: select • /: search • enter: confirm • q: quit"
	}
	sb.WriteString(m.styles.Help.Render(helpText))

	return sb.String()
}
