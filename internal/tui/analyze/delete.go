package analyze

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
	"github.com/SadeepaNHerath/Spacefold/internal/engine"
)

// prepareDelete populates DeleteTargets from either multi-selected items or the item under cursor.
func (m *Model) prepareDelete() {
	if len(m.Filtered) == 0 {
		return
	}

	var targets []engine.FileEntry
	if len(m.Selected) > 0 {
		for _, it := range m.Items {
			if m.Selected[it.Path] {
				targets = append(targets, it)
			}
		}
	} else {
		currentIdx := m.Filtered[m.Cursor]
		targets = append(targets, m.Items[currentIdx])
	}

	if len(targets) > 0 {
		m.DeleteTargets = targets
		m.ConfirmingDelete = true
	}
}

// executeDelete moves targets to Trash using core.MoveToTrash.
func (m *Model) executeDelete() tea.Cmd {
	targets := make([]engine.FileEntry, len(m.DeleteTargets))
	copy(targets, m.DeleteTargets)

	return func() tea.Msg {
		var totalFreed int64
		var count int
		var firstErr error

		for _, item := range targets {
			err := core.MoveToTrash(item.Path)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else {
				count++
				totalFreed += item.Size
			}
		}

		return deleteFinishedMsg{
			Count: count,
			Freed: totalFreed,
			Err:   firstErr,
		}
	}
}

// renderDeleteModal builds the confirmation modal dialog.
func (m Model) renderDeleteModal() string {
	var totalSize int64
	for _, it := range m.DeleteTargets {
		totalSize += it.Size
	}

	var sb strings.Builder
	sb.WriteString(m.Styles.ModalTitle.Render("⚠️  CONFIRM MOVE TO TRASH"))
	sb.WriteString("\n\n")

	count := len(m.DeleteTargets)
	sb.WriteString(m.Styles.ModalWarn.Render(
		fmt.Sprintf("Are you sure you want to move %d item(s) (%s) to macOS Trash?",
			count, core.FormatBytes(totalSize)),
	))
	sb.WriteString("\n\n")

	sb.WriteString(m.Styles.Normal.Render("Target(s):"))
	sb.WriteString("\n")

	maxPreview := 4
	for i, it := range m.DeleteTargets {
		if i >= maxPreview {
			sb.WriteString(m.Styles.CountText.Render(fmt.Sprintf("  ... and %d more item(s)\n", len(m.DeleteTargets)-maxPreview)))
			break
		}
		sb.WriteString(m.Styles.Normal.Render(fmt.Sprintf("  • %s (%s)\n", it.Name, core.FormatBytes(it.Size))))
	}

	sb.WriteString("\n")
	sb.WriteString(m.Styles.ModalKeys.Render("Press [y] to Confirm   •   [n / Esc] to Cancel"))

	return m.Styles.ModalBox.Render(sb.String())
}
