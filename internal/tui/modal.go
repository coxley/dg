package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	settingsModalWidth   = 84
	preferenceModalWidth = 68
)

type modalOverlay struct {
	lines []string
	left  int
	top   int
	width int
}

func (m *Model) currentModalOverlay() modalOverlay {
	if m.modal == modalNone || m.width < 2 {
		return modalOverlay{}
	}
	width := min(settingsModalWidth, m.width)
	switch m.modal {
	case modalSave:
		width = min(68, m.width)
	case modalExport:
		width = min(50, m.width)
	case modalPreferences:
		width = min(preferenceModalWidth, m.width)
	case modalNotice:
		width = min(max(28, displayWidth([]byte(m.notice))+4), m.width)
	case modalNone, modalHelp:
	}
	lines := m.modalLines(width)
	height := m.diagramHeight()
	top := max((height-len(lines))/2, 0)
	belowToolbar := toolbarTop + toolbarBoxHeight
	if belowToolbar+len(lines) <= height {
		top = max(top, belowToolbar)
	}
	left := max((m.width-width)/2, 0)
	if m.modalPositioned {
		left = min(max(m.modalLeft, 0), max(m.width-width, 0))
		top = min(max(m.modalTop, 0), max(height-len(lines), 0))
	}
	return modalOverlay{
		lines: lines,
		left:  left,
		top:   top,
		width: width,
	}
}

func (o modalOverlay) contains(x, y int) bool {
	return x >= o.left && x < o.left+o.width &&
		y >= o.top && y < o.top+len(o.lines)
}

func (m *Model) openModal(next modal) {
	m.modal = next
	m.modalPositioned = false
	m.modalDragging = false
}

func (m *Model) updateModalMouseClick(mouse tea.Mouse) tea.Cmd {
	overlay := m.currentModalOverlay()
	if !overlay.contains(mouse.X, mouse.Y) {
		m.closeModal()
		return nil
	}
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	if mouse.Y == overlay.top {
		m.modalDragging = true
		m.modalDragOffsetX = mouse.X - overlay.left
		m.modalDragOffsetY = mouse.Y - overlay.top
		return nil
	}
	if (m.modal == modalHelp || m.modal == modalPreferences) &&
		mouse.Y == overlay.top+1 {
		x := mouse.X - overlay.left - 1
		shortcutsWidth := lipgloss.Width(m.settingsTab("Shortcuts", modalHelp))
		if x >= shortcutsWidth {
			m.openPreferences()
		} else if x >= 0 {
			m.modal = modalHelp
		}
		return nil
	}
	switch m.modal {
	case modalPreferences:
		return m.updateSettingsTabs(tea.MouseClickMsg(mouse))
	case modalSave:
		return m.updateSaveForm(tea.MouseClickMsg(mouse))
	case modalExport:
		return m.updateExportForm(tea.MouseClickMsg(mouse))
	case modalNone, modalHelp, modalNotice:
		return nil
	}
	return nil
}

func (m *Model) updateModalMouseMotion(mouse tea.Mouse) tea.Cmd {
	if m.modalDragging {
		m.modalLeft = mouse.X - m.modalDragOffsetX
		m.modalTop = mouse.Y - m.modalDragOffsetY
		m.modalPositioned = true
		return nil
	}
	switch m.modal {
	case modalPreferences:
		return m.updateSettingsTabs(tea.MouseMotionMsg(mouse))
	case modalSave:
		return m.updateSaveForm(tea.MouseMotionMsg(mouse))
	case modalExport:
		return m.updateExportForm(tea.MouseMotionMsg(mouse))
	case modalNone, modalHelp, modalNotice:
		return nil
	}
	return nil
}

func (m *Model) closeModal() {
	switch m.modal {
	case modalHelp, modalPreferences:
		m.closeSettingsModal()
	case modalSave:
		m.closeSaveForm()
	case modalExport:
		m.modal = modalNone
		m.exportText = ""
	case modalNotice:
		m.modal = m.noticeReturn
		m.dismissNotice()
	case modalNone:
	}
	m.modalDragging = false
}

func (m *Model) modalLines(width int) []string {
	switch m.modal {
	case modalSave:
		return m.componentModalLines(m.saveForm.View(), width)
	case modalExport:
		return m.componentModalLines(m.exportForm.View(), width)
	case modalNotice:
		style := m.theme.Modal.Border(lipgloss.RoundedBorder())
		return strings.Split(style.Width(max(width-2, 0)).Render(" "+m.notice), "\n")
	default:
		return m.settingsModalLines(width)
	}
}

func (m *Model) settingsModalLines(width int) []string {
	return m.componentModalLines(m.settingsView(width-2), width)
}

func (m *Model) settingsView(width int) string {
	m.help.SetWidth(width)
	body := m.help.View(m.keys)
	if m.modal == modalPreferences {
		body = m.preferenceForm.View()
	}
	tabs := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.settingsTab("Shortcuts", modalHelp),
		m.settingsTab("Preferences", modalPreferences),
	)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		tabs,
		lipgloss.NewStyle().PaddingTop(1).MaxWidth(width).Render(body),
	)
}

func (m *Model) settingsTab(label string, tab modal) string {
	style := m.theme.Tab
	if m.modal == tab {
		style = m.theme.TabActive
	}
	return style.Render(label)
}

func (m *Model) componentModalLines(content string, width int) []string {
	rendered := m.theme.Modal.
		Width(max(width-2, 0)).
		MaxWidth(width).
		Render(strings.TrimSuffix(content, "\n"))
	return strings.Split(rendered, "\n")
}
