package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	modalview "github.com/coxley/dg/internal/tui/modal"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
)

const (
	minimumSettingsModalWidth = 84
)

func (m *Model) currentModalOverlay() modalview.Overlay {
	if m.modal == modalNone || m.width < 2 {
		m.dialog.Hide()
		return modalview.Overlay{}
	}
	width := min(minimumSettingsModalWidth, m.width)
	var (
		content string
		variant modalview.Variant
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
	case modalPreferences:
		width = min(minimumSettingsModalWidth, m.width)
		bodyWidth := max(
			width-
				m.theme.Modal.Container.GetHorizontalFrameSize()-
				m.theme.Modal.Body.GetHorizontalFrameSize(),
			0,
		)
		if m.dialog.Overlay().Width != 0 {
			bodyWidth = m.dialog.BodyWidth()
		}
		content = m.preferenceBody(bodyWidth)
	case modalNone:
	}
	m.dialog.Configure(
		m.width,
		m.height,
		m.nav.Bounds().Bottom(),
		width,
		strings.TrimSuffix(content, "\n"),
		variant,
		nil,
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
	case modalNone, modalNotice:
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
	case modalNone, modalNotice:
		return nil
	}
	return nil
}

func (m *Model) closeModal() {
	switch m.modal {
	case modalPreferences:
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

func (m *Model) preferenceBody(width int) string {
	preferenceHeight := 0
	bodyHeight := 0
	if m.preferenceForm != nil {
		m.preferenceForm.SetWidth(width)
		if m.dialog.Overlay().Height != 0 {
			bodyHeight = m.dialog.BodyHeight()
		}
		m.preferenceForm.SetHeight(bodyHeight)
		preferenceHeight = m.preferenceForm.NaturalHeight()
	}

	if m.modal != modalPreferences || m.preferenceForm == nil {
		return ""
	}
	content := m.preferenceForm.View().Content
	height := lipgloss.Height(content)
	frameHeight := m.theme.Modal.Container.GetVerticalFrameSize() +
		m.theme.Modal.Body.GetVerticalFrameSize()
	if bodyHeight > 0 {
		height = bodyHeight
	} else if preferenceHeight+frameHeight+m.nav.Bounds().Bottom() <= m.height {
		height = preferenceHeight
	}
	return m.theme.SettingsContent.
		Height(height).
		MaxHeight(height).
		Render(content)
}
