package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	modalview "github.com/coxley/dg/internal/tui/modal"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
)

const (
	settingsModalWidth = 84
)

var settingsTabs = []modalview.Tab{
	{ID: modalview.TabID(modalHelp), Label: "Shortcuts"},
	{ID: modalview.TabID(modalPreferences), Label: "Preferences"},
}

func (m *Model) currentModalOverlay() modalview.Overlay {
	if m.modal == modalNone || m.width < 2 {
		m.dialog.Hide()
		return modalview.Overlay{}
	}
	width := min(settingsModalWidth, m.width)
	var (
		content string
		variant modalview.Variant
		tabs    []modalview.Tab
	)
	switch m.modal {
	case modalSave:
		width = min(68, m.width)
		content = m.saveForm.View()
	case modalExport:
		width = min(50, m.width)
		content = m.clipboard.View().Content
	case modalNotice:
		width = min(max(28, displayWidth([]byte(m.notice))+4), m.width)
		content = " " + m.notice
		variant = modalview.Notice
	case modalHelp, modalPreferences:
		content = m.settingsBody(width)
		tabs = settingsTabs
	case modalNone:
	}
	m.dialog.Configure(
		m.width,
		m.height,
		toolbarTop+m.nav.Height(),
		width,
		strings.TrimSuffix(content, "\n"),
		variant,
		tabs,
		modalview.TabID(m.modal),
	)
	return m.dialog.Overlay()
}

func (m *Model) openModal(next modal) {
	m.modal = next
	m.dialog.Hide()
}

func (m *Model) updateModalMouseClick(mouse tea.Mouse) tea.Cmd {
	m.currentModalOverlay()
	var command tea.Cmd
	m.dialog, command = m.dialog.Update(tea.MouseClickMsg(mouse))
	if command != nil || m.dialog.CapturesPointer() || mouse.Button != tea.MouseLeft {
		return command
	}
	switch m.modal {
	case modalPreferences:
		x, y := m.dialog.BodyOrigin()
		return m.updateSettingsTabs(preferencesview.ClickMsg{
			X: mouse.X - x,
			Y: mouse.Y - y,
		})
	case modalSave:
		return m.updateSaveForm(tea.MouseClickMsg(mouse))
	case modalExport:
		return m.updateClipboard(tea.MouseClickMsg(mouse))
	case modalNone, modalHelp, modalNotice:
		return nil
	}
	return nil
}

func (m *Model) updateModalMouseMotion(mouse tea.Mouse) tea.Cmd {
	wasCaptured := m.dialog.CapturesPointer()
	m.dialog, _ = m.dialog.Update(tea.MouseMotionMsg(mouse))
	if wasCaptured || m.dialog.CapturesPointer() {
		if m.modal == modalPreferences && m.dialog.Resizing() {
			m.preferenceForm.SetHeight(m.dialog.BodyHeight())
		}
		return nil
	}
	switch m.modal {
	case modalPreferences:
		return m.updateSettingsTabs(tea.MouseMotionMsg(mouse))
	case modalSave:
		return m.updateSaveForm(tea.MouseMotionMsg(mouse))
	case modalExport:
		return m.updateClipboard(tea.MouseMotionMsg(mouse))
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
		m.clipboard.CancelExport()
	case modalNotice:
		m.modal = m.noticeReturn
		m.dismissNotice()
	case modalNone:
	}
	m.dialog.Hide()
}

func (m *Model) settingsBody(width int) string {
	innerWidth := max(width-m.theme.Modal.Container.GetHorizontalFrameSize(), 0)
	m.help.SetWidth(innerWidth)
	help := m.help.View(m.keys)
	preferenceHeight := 0
	if m.preferenceForm != nil {
		preferenceHeight = m.preferenceForm.NaturalHeight()
	}

	var content string
	switch m.modal {
	case modalHelp:
		content = help
	case modalPreferences:
		content = m.preferenceForm.View().Content
	default:
		return ""
	}
	height := lipgloss.Height(content)
	largerHeight := max(lipgloss.Height(help), preferenceHeight)
	frameHeight := m.theme.Modal.Container.GetVerticalFrameSize() +
		m.theme.Modal.Body.GetVerticalFrameSize() + 1
	if largerHeight+frameHeight+toolbarTop+m.nav.Height() <= m.height {
		height = largerHeight
	}
	return m.theme.SettingsContent.
		Height(height).
		MaxHeight(height).
		Render(content)
}
