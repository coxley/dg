package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	modalview "github.com/coxley/dg/internal/tui/modal"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
)

const minimumSettingsModalWidth = 84

type dialogSpec struct {
	ID             chrome.SurfaceID
	Context        string
	Width          func(*Model) int
	Variant        modalview.Variant
	DismissOutside bool
	DismissAnyKey  bool
	TextEntry      bool
	Scopes         func(*Model) []chrome.ScopeID
	Body           func(*Model, int) string
	Update         func(*Model, tea.Msg) tea.Cmd
	Click          func(*Model, tea.Mouse) tea.Cmd
	Wheel          func(*Model, tea.MouseWheelMsg) tea.Cmd
	Resize         func(*Model)
	Back           func(*Model) (tea.Cmd, bool)
	Close          func(*Model)
}

var dialogSpecs = [...]dialogSpec{
	{
		ID:             surfacePreferences,
		Context:        string(scopePreferences),
		Width:          fixedDialogWidth(minimumSettingsModalWidth),
		DismissOutside: true,
		Scopes:         preferenceDialogScopes,
		Body:           preferenceDialogBody,
		Update:         updatePreferenceDialog,
		Click:          clickPreferenceDialog,
		Wheel:          wheelPreferenceDialog,
		Resize:         resizePreferenceDialog,
		Back:           backPreferenceDialog,
		Close:          (*Model).closeSettingsModal,
	},
	{
		ID:             surfaceSave,
		Context:        string(commandSave),
		Width:          fixedDialogWidth(68),
		DismissOutside: true,
		TextEntry:      true,
		Scopes:         saveDialogScopes,
		Body:           saveDialogBody,
		Update:         updateSaveDialog,
		Click:          clickSaveDialog,
		Wheel:          wheelSaveDialog,
		Back:           backSaveDialog,
		Close:          (*Model).closeSaveForm,
	},
	{
		ID:             surfaceExport,
		Context:        "export",
		Width:          fixedDialogWidth(50),
		DismissOutside: true,
		Scopes:         modalDialogScopes,
		Body:           exportDialogBody,
		Update:         updateExportDialog,
		Click:          clickExportDialog,
		Wheel:          wheelExportDialog,
		Close:          closeExportDialog,
	},
	{
		ID:            surfaceNotice,
		Context:       "notice",
		Width:         noticeDialogWidth,
		Variant:       modalview.Notice,
		DismissAnyKey: true,
		Scopes:        modalDialogScopes,
		Body:          noticeDialogBody,
		Close:         closeNoticeDialog,
	},
}

func (m *Model) currentDialogOverlay() modalview.Overlay {
	spec, ok := m.activeDialogSpec()
	if !ok || m.width < 2 {
		m.dialog.Hide()
		return modalview.Overlay{}
	}
	width := min(spec.Width(m), m.width)
	content := spec.Body(m, width)
	m.dialog.Configure(
		m.width,
		m.height,
		m.nav.Bounds().Bottom(),
		width,
		strings.TrimSuffix(content, "\n"),
		spec.Variant,
		nil,
		0,
	)
	return m.dialog.Overlay()
}

func (m *Model) openDialog(id chrome.SurfaceID) {
	m.activeDialog = id
	m.dialog.Hide()
}

func (m *Model) updateDialog(message tea.Msg) tea.Cmd {
	spec, ok := m.activeDialogSpec()
	if !ok || spec.Update == nil {
		return nil
	}
	return spec.Update(m, message)
}

func (m *Model) updateDialogMouseClick(mouse tea.Mouse) tea.Cmd {
	m.currentDialogOverlay()
	var command tea.Cmd
	m.dialog, command = m.dialog.Update(tea.MouseClickMsg(mouse))
	if command != nil || m.dialog.CapturesPointer() || mouse.Button != tea.MouseLeft {
		return command
	}
	spec, ok := m.activeDialogSpec()
	if !ok || spec.Click == nil {
		return nil
	}
	return spec.Click(m, mouse)
}

func (m *Model) updateDialogMouseMotion(mouse tea.Mouse) tea.Cmd {
	wasCaptured := m.dialog.CapturesPointer()
	m.dialog, _ = m.dialog.Update(tea.MouseMotionMsg(mouse))
	spec, ok := m.activeDialogSpec()
	if !ok {
		return nil
	}
	if wasCaptured || m.dialog.CapturesPointer() {
		if m.dialog.Resizing() && spec.Resize != nil {
			spec.Resize(m)
		}
		return nil
	}
	if spec.Update == nil {
		return nil
	}
	return spec.Update(m, tea.MouseMotionMsg(mouse))
}

func (m *Model) updateDialogWheel(message tea.MouseWheelMsg) tea.Cmd {
	spec, ok := m.activeDialogSpec()
	if !ok {
		return nil
	}
	if spec.Wheel != nil {
		return spec.Wheel(m, message)
	}
	return nil
}

func (m *Model) closeDialog() {
	spec, ok := m.activeDialogSpec()
	if ok && spec.Close != nil {
		spec.Close(m)
	}
	m.dialog.Hide()
}

func (m *Model) activeDialogSpec() (dialogSpec, bool) {
	return dialogSpecFor(m.activeDialog)
}

func dialogSpecFor(id chrome.SurfaceID) (dialogSpec, bool) {
	for _, spec := range dialogSpecs {
		if spec.ID == id {
			return spec, true
		}
	}
	return dialogSpec{}, false
}

func fixedDialogWidth(width int) func(*Model) int {
	return func(*Model) int {
		return width
	}
}

func modalDialogScopes(*Model) []chrome.ScopeID {
	return []chrome.ScopeID{scopeModal, scopeGlobal}
}

func preferenceDialogScopes(m *Model) []chrome.ScopeID {
	if m.preferenceForm != nil && m.preferenceForm.DirectoryOpen() {
		return []chrome.ScopeID{scopeDirectory, scopePreferences, scopeGlobal}
	}
	return []chrome.ScopeID{scopePreferences, scopeGlobal}
}

func saveDialogScopes(m *Model) []chrome.ScopeID {
	if m.savePicker != nil && m.savePicker.Opened() {
		return []chrome.ScopeID{scopeDirectory, scopeModal, scopeGlobal}
	}
	return modalDialogScopes(m)
}

func preferenceDialogBody(m *Model, width int) string {
	if m.preferenceForm == nil {
		return ""
	}
	bodyWidth := max(
		width-
			m.theme.Modal.Container.GetHorizontalFrameSize()-
			m.theme.Modal.Body.GetHorizontalFrameSize(),
		0,
	)
	if m.dialog.Overlay().Width != 0 {
		bodyWidth = m.dialog.BodyWidth()
	}
	m.preferenceForm.SetWidth(bodyWidth)
	height := 0
	if m.dialog.Overlay().Height != 0 {
		height = m.dialog.BodyHeight()
	}
	m.preferenceForm.SetHeight(height)
	return m.preferenceForm.View().Content
}

func saveDialogBody(m *Model, width int) string {
	if m.saveForm == nil {
		return ""
	}
	bodyWidth, height := m.dialogBodySize(width)
	if m.savePicker != nil && m.savePicker.Opened() {
		m.savePicker.SetBounds(bodyWidth, height)
		return m.savePicker.View().Content
	}
	m.saveForm.SetBounds(chrome.Rect{Width: bodyWidth, Height: height})
	return m.saveForm.View().Content
}

func exportDialogBody(m *Model, width int) string {
	width, height := m.dialogBodySize(width)
	m.clipboard.SetBounds(width, height)
	return m.clipboard.View().Content
}

func (m *Model) dialogBodySize(width int) (int, int) {
	bodyWidth := max(
		width-
			m.theme.Modal.Container.GetHorizontalFrameSize()-
			m.theme.Modal.Body.GetHorizontalFrameSize(),
		0,
	)
	if m.dialog.Overlay().Width == 0 {
		return bodyWidth, 0
	}
	return m.dialog.BodyWidth(), m.dialog.BodyHeight()
}

func noticeDialogBody(m *Model, _ int) string {
	return " " + m.notice
}

func noticeDialogWidth(m *Model) int {
	return max(28, displayWidth([]byte(m.notice))+4)
}

func updatePreferenceDialog(m *Model, message tea.Msg) tea.Cmd {
	return m.updateSettingsTabs(message)
}

func updateSaveDialog(m *Model, message tea.Msg) tea.Cmd {
	return m.updateSaveForm(message)
}

func updateExportDialog(m *Model, message tea.Msg) tea.Cmd {
	return m.updateClipboard(message)
}

func clickPreferenceDialog(m *Model, mouse tea.Mouse) tea.Cmd {
	x, y := m.dialog.BodyOrigin()
	return m.updateSettingsTabs(preferencesview.ClickMsg{
		X: mouse.X - x,
		Y: mouse.Y - y,
	})
}

func clickSaveDialog(m *Model, mouse tea.Mouse) tea.Cmd {
	x, y := m.dialog.BodyOrigin()
	if m.savePicker != nil && m.savePicker.Opened() {
		mouse.X -= x
		mouse.Y -= y
		return m.updateSaveForm(tea.MouseClickMsg(mouse))
	}
	command := m.saveForm.Click(chrome.Point{X: mouse.X - x, Y: mouse.Y - y})
	if command == nil {
		return nil
	}
	return m.updateSaveForm(command())
}

func clickExportDialog(m *Model, mouse tea.Mouse) tea.Cmd {
	x, y := m.dialog.BodyOrigin()
	return m.clipboard.Click(chrome.Point{X: mouse.X - x, Y: mouse.Y - y})
}

func wheelPreferenceDialog(m *Model, message tea.MouseWheelMsg) tea.Cmd {
	return m.updateSettingsWheel(message)
}

func wheelExportDialog(m *Model, message tea.MouseWheelMsg) tea.Cmd {
	return m.updateClipboard(message)
}

func wheelSaveDialog(m *Model, message tea.MouseWheelMsg) tea.Cmd {
	if m.savePicker == nil || !m.savePicker.Opened() {
		return nil
	}
	x, y := m.dialog.BodyOrigin()
	mouse := message.Mouse()
	mouse.X -= x
	mouse.Y -= y
	return m.updateSaveForm(tea.MouseWheelMsg(mouse))
}

func resizePreferenceDialog(m *Model) {
	m.preferenceForm.SetHeight(m.dialog.BodyHeight())
}

func backPreferenceDialog(m *Model) (tea.Cmd, bool) {
	if m.preferenceForm == nil || !m.preferenceForm.DirectoryOpen() {
		return nil, false
	}
	return m.updateSettingsTabs(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})), true
}

func backSaveDialog(m *Model) (tea.Cmd, bool) {
	if m.savePicker == nil || !m.savePicker.Opened() {
		return nil, false
	}
	m.savePicker.Close()
	m.saveDirectory = m.savePicker.Value()
	m.saveForm.SetDirectory(saveDirectoryField, m.saveDirectory)
	return nil, true
}

func closeExportDialog(m *Model) {
	m.activeDialog = surfaceNone
	m.clipboard.CancelExport()
}

func closeNoticeDialog(m *Model) {
	m.activeDialog = m.noticeReturn
	m.dismissNotice()
}
