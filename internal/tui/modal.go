package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	clipboardview "github.com/coxley/dg/internal/tui/clipboard"
	modalview "github.com/coxley/dg/internal/tui/modal"
)

const minimumSettingsModalWidth = 84

type dialogBodyResult struct {
	message tea.Msg
	command tea.Cmd
	handled bool
}

type dialogBody interface {
	Context() string
	PreferredWidth() int
	Scopes() []chrome.ScopeID
	TextEntry() bool
	SetBounds(chrome.Rect)
	Update(tea.Msg) dialogBodyResult
	View() string
}

type dialogClickMsg struct {
	Point chrome.Point
	Mouse tea.Mouse
}

type dialogWheelMsg struct {
	Mouse tea.Mouse
}

type dialogBackMsg struct{}

type dialogCloseMsg struct{}

type dialogSpec struct {
	ID             chrome.SurfaceID
	Variant        modalview.Variant
	DismissOutside bool
	DismissAnyKey  bool
}

var dialogSpecs = [...]dialogSpec{
	{ID: surfacePreferences, DismissOutside: true},
	{ID: surfaceSave, DismissOutside: true},
	{ID: surfaceExport, DismissOutside: true},
	{ID: surfaceNotice, Variant: modalview.Notice, DismissAnyKey: true},
	{ID: surfaceConfirmation, DismissOutside: true},
}

type dialogPlan struct {
	overlay modalview.Overlay
	body    chrome.Rect
}

// dialogController owns dialog selection, shell geometry, and body routing.
type dialogController struct {
	active       chrome.SurfaceID
	shell        modalview.Model
	preferences  *preferenceDialogBody
	save         *saveDialogBody
	export       exportDialogBody
	notice       noticeDialogBody
	confirmation *confirmationDialogBody
	plan         dialogPlan
	screen       chrome.Size
	avoidTop     int
}

func newDialogController(
	theme Theme,
	clipboard *clipboardview.Model,
	preferenceValue preferenceDialogValue,
) dialogController {
	preferenceWidth := minimumSettingsModalWidth -
		theme.Modal.Container.GetHorizontalFrameSize() -
		theme.Modal.Body.GetHorizontalFrameSize()
	return dialogController{
		shell: modalview.New(theme.Modal),
		preferences: newPreferenceDialogBody(
			preferenceValue,
			preferenceWidth,
			theme.Preferences,
		),
		save:         newSaveDialogBody(theme.Save),
		export:       exportDialogBody{clipboard: clipboard},
		confirmation: newConfirmationDialogBody(theme.Confirmation),
	}
}

func (d *dialogController) ActiveID() chrome.SurfaceID {
	return d.active
}

func (d *dialogController) OpenPreferences(value preferenceDialogValue) {
	d.preferences.Reset(value)
	d.open(surfacePreferences)
}

func (d *dialogController) OpenSave() {
	d.save.Reset()
	d.open(surfaceSave)
}

func (d *dialogController) OpenExport() {
	d.open(surfaceExport)
}

func (d *dialogController) OpenNotice(
	text string,
	returnTo chrome.SurfaceID,
) uint64 {
	d.notice.Reset(text, returnTo)
	d.open(surfaceNotice)
	return d.notice.Generation()
}

func (d *dialogController) OpenConfirmation(title, message, confirm string, result tea.Msg) {
	d.confirmation.Reset(title, message, confirm, result)
	d.open(surfaceConfirmation)
}

func (d *dialogController) open(id chrome.SurfaceID) {
	d.active = id
	d.shell.Hide()
	d.plan = dialogPlan{}
}

func (d *dialogController) Restore(id chrome.SurfaceID) {
	if id == surfaceNone {
		d.hide()
		return
	}
	d.open(id)
}

func (d *dialogController) Arrange(
	screenWidth, screenHeight, avoidTop int,
) modalview.Overlay {
	spec, ok := d.activeSpec()
	body := d.activeBody()
	if !ok || body == nil || screenWidth < 2 {
		d.shell.Hide()
		d.plan = dialogPlan{}
		return modalview.Overlay{}
	}
	width := min(body.PreferredWidth(), screenWidth)
	bounds := chrome.Rect{Width: width}
	screen := chrome.Size{Width: screenWidth, Height: screenHeight}
	if d.screen == screen && d.avoidTop == avoidTop &&
		d.plan.body.Width != 0 && d.plan.body.Height != 0 {
		bounds.Width = d.plan.body.Width
		bounds.Height = d.plan.body.Height
	}
	for range 4 {
		body.SetBounds(bounds)
		d.configure(
			screenWidth,
			screenHeight,
			avoidTop,
			width,
			body.View(),
			spec,
		)
		next := chrome.Rect{
			Width: d.shell.BodyWidth(), Height: d.shell.BodyHeight(),
		}
		if next == bounds {
			break
		}
		bounds = next
	}
	x, y := d.shell.BodyOrigin()
	d.plan = dialogPlan{
		overlay: d.shell.Overlay(),
		body: chrome.Rect{
			X: x, Y: y,
			Width: bounds.Width, Height: bounds.Height,
		},
	}
	d.screen = screen
	d.avoidTop = avoidTop
	return d.plan.overlay
}

func (d *dialogController) configure(
	screenWidth, screenHeight, avoidTop, width int,
	content string,
	spec dialogSpec,
) {
	d.shell.Configure(
		screenWidth,
		screenHeight,
		avoidTop,
		width,
		strings.TrimSuffix(content, "\n"),
		spec.Variant,
		nil,
		0,
	)
}

func (d *dialogController) Overlay() modalview.Overlay {
	return d.plan.overlay
}

func (d *dialogController) Fullscreen() bool {
	return d.shell.Fullscreen()
}

func (d *dialogController) Resizing() bool {
	return d.shell.Resizing()
}

func (d *dialogController) CapturesPointer() bool {
	return d.shell.CapturesPointer()
}

func (d *dialogController) Context() string {
	if body := d.activeBody(); body != nil {
		return body.Context()
	}
	return ""
}

func (d *dialogController) Scopes() []chrome.ScopeID {
	if body := d.activeBody(); body != nil {
		return body.Scopes()
	}
	return nil
}

func (d *dialogController) TextEntry() bool {
	if body := d.activeBody(); body != nil {
		return body.TextEntry()
	}
	return false
}

func (d *dialogController) DismissAnyKey() bool {
	spec, ok := d.activeSpec()
	return ok && spec.DismissAnyKey
}

func (d *dialogController) Update(message tea.Msg) dialogBodyResult {
	body := d.activeBody()
	if body == nil {
		return dialogBodyResult{}
	}
	return body.Update(message)
}

func (d *dialogController) Click(mouse tea.Mouse) dialogBodyResult {
	var command tea.Cmd
	d.shell, command = d.shell.Update(tea.MouseClickMsg(mouse))
	if command != nil || d.shell.CapturesPointer() || mouse.Button != tea.MouseLeft {
		return dialogBodyResult{command: command, handled: true}
	}
	return d.Update(dialogClickMsg{
		Point: chrome.Point{
			X: mouse.X - d.plan.body.X,
			Y: mouse.Y - d.plan.body.Y,
		},
		Mouse: localDialogMouse(mouse, d.plan.body),
	})
}

func (d *dialogController) Motion(mouse tea.Mouse) dialogBodyResult {
	wasCaptured := d.shell.CapturesPointer()
	d.shell, _ = d.shell.Update(tea.MouseMotionMsg(mouse))
	if wasCaptured || d.shell.CapturesPointer() {
		return dialogBodyResult{handled: true}
	}
	return d.Update(tea.MouseMotionMsg(localDialogMouse(mouse, d.plan.body)))
}

func (d *dialogController) Release(message tea.MouseReleaseMsg) {
	d.shell, _ = d.shell.Update(message)
}

func (d *dialogController) Wheel(message tea.MouseWheelMsg) dialogBodyResult {
	return d.Update(dialogWheelMsg{
		Mouse: localDialogMouse(message.Mouse(), d.plan.body),
	})
}

func (d *dialogController) Back() dialogBodyResult {
	return d.Update(dialogBackMsg{})
}

func (d *dialogController) SubmitSave() dialogBodyResult {
	if d.active != surfaceSave {
		return dialogBodyResult{}
	}
	return d.save.Update(chrome.FormSubmitMsg{ID: saveConfirmAction})
}

func (d *dialogController) Close() dialogBodyResult {
	result := d.Update(dialogCloseMsg{})
	d.hide()
	return result
}

func (d *dialogController) CloseWithoutMessage() {
	d.hide()
}

func (d *dialogController) hide() {
	d.active = surfaceNone
	d.shell.Hide()
	d.plan = dialogPlan{}
}

func (d *dialogController) SetStyles(theme Theme) {
	d.shell.SetStyles(theme.Modal)
	d.preferences.SetStyles(theme.Preferences)
	d.save.SetStyles(theme.Save)
	d.export.SetStyles(theme.ExportForm)
	d.confirmation.SetStyles(theme.Confirmation)
}

func (d *dialogController) activeBody() dialogBody {
	switch d.active {
	case surfacePreferences:
		return d.preferences
	case surfaceSave:
		return d.save
	case surfaceExport:
		return &d.export
	case surfaceNotice:
		return &d.notice
	case surfaceConfirmation:
		return d.confirmation
	default:
		return nil
	}
}

func (d *dialogController) activeSpec() (dialogSpec, bool) {
	return dialogSpecFor(d.active)
}

func dialogSpecFor(id chrome.SurfaceID) (dialogSpec, bool) {
	for _, spec := range dialogSpecs {
		if spec.ID == id {
			return spec, true
		}
	}
	return dialogSpec{}, false
}

func localDialogMouse(mouse tea.Mouse, body chrome.Rect) tea.Mouse {
	mouse.X -= body.X
	mouse.Y -= body.Y
	return mouse
}

type exportDialogBody struct {
	clipboard *clipboardview.Model
}

func (*exportDialogBody) Context() string {
	return "export"
}

func (*exportDialogBody) PreferredWidth() int {
	return 50
}

func (*exportDialogBody) Scopes() []chrome.ScopeID {
	return []chrome.ScopeID{scopeModal, scopeGlobal}
}

func (*exportDialogBody) TextEntry() bool {
	return false
}

func (b *exportDialogBody) SetBounds(bounds chrome.Rect) {
	b.clipboard.SetBounds(bounds.Width, bounds.Height)
}

func (b *exportDialogBody) Update(message tea.Msg) dialogBodyResult {
	switch message := message.(type) {
	case dialogClickMsg:
		return dialogBodyResult{
			command: b.clipboard.Click(message.Point),
			handled: true,
		}
	case dialogWheelMsg:
		return dialogBodyResult{
			command: updateClipboardModel(
				b.clipboard,
				tea.MouseWheelMsg(message.Mouse),
			),
			handled: true,
		}
	case dialogBackMsg:
		return dialogBodyResult{}
	case dialogCloseMsg:
		b.clipboard.CancelExport()
		return dialogBodyResult{handled: true}
	default:
		return dialogBodyResult{
			command: updateClipboardModel(b.clipboard, message),
			handled: true,
		}
	}
}

func (b *exportDialogBody) View() string {
	return b.clipboard.View().Content
}

func (b *exportDialogBody) SetStyles(styles chrome.FormStyles) {
	b.clipboard.SetStyles(styles)
}

type noticeDismissedMsg struct {
	ReturnTo chrome.SurfaceID
}

type noticeDialogBody struct {
	generation uint64
	text       string
	returnTo   chrome.SurfaceID
}

func (b *noticeDialogBody) Reset(text string, returnTo chrome.SurfaceID) {
	b.generation++
	b.text = text
	b.returnTo = returnTo
}

func (b *noticeDialogBody) Generation() uint64 {
	return b.generation
}

func (b *noticeDialogBody) ReturnTo() chrome.SurfaceID {
	return b.returnTo
}

func (b *noticeDialogBody) Clear() {
	b.text = ""
	b.returnTo = surfaceNone
}

func (*noticeDialogBody) Context() string {
	return "notice"
}

func (b *noticeDialogBody) PreferredWidth() int {
	return max(28, displayWidth([]byte(b.text))+4)
}

func (*noticeDialogBody) Scopes() []chrome.ScopeID {
	return []chrome.ScopeID{scopeModal, scopeGlobal}
}

func (*noticeDialogBody) TextEntry() bool {
	return false
}

func (*noticeDialogBody) SetBounds(chrome.Rect) {}

func (b *noticeDialogBody) Update(message tea.Msg) dialogBodyResult {
	switch message.(type) {
	case dialogCloseMsg:
		return dialogBodyResult{
			message: noticeDismissedMsg{ReturnTo: b.returnTo},
			handled: true,
		}
	default:
		return dialogBodyResult{}
	}
}

func (b *noticeDialogBody) View() string {
	return " " + b.text
}

func updateClipboardModel(model *clipboardview.Model, message tea.Msg) tea.Cmd {
	updated, command := model.Update(message)
	if updated != model {
		panic("clipboard model replaced")
	}
	return command
}

func (m *Model) updateDialog(message tea.Msg) tea.Cmd {
	return m.handleDialogResult(m.dialogs.Update(message))
}

func (m *Model) updateDialogMouseClick(mouse tea.Mouse) tea.Cmd {
	return m.handleDialogResult(m.dialogs.Click(mouse))
}

func (m *Model) updateDialogMouseMotion(mouse tea.Mouse) tea.Cmd {
	return m.handleDialogResult(m.dialogs.Motion(mouse))
}

func (m *Model) updateDialogWheel(message tea.MouseWheelMsg) tea.Cmd {
	return m.handleDialogResult(m.dialogs.Wheel(message))
}

func (m *Model) dismissDialog() tea.Cmd {
	return m.handleDialogResult(m.dialogs.Close())
}

func (m *Model) handleDialogResult(result dialogBodyResult) tea.Cmd {
	var effect tea.Cmd
	switch message := result.message.(type) {
	case nil:
	case preferencePreviewMsg:
		m.previewPreferences(message.Value)
	case preferenceSaveMsg:
		effect = m.savePreferences(message)
	case preferenceCancelMsg:
		m.cancelPreferences()
	case saveDocumentMsg:
		m.saveFromDialog(message)
	case dialogCancelMsg:
		m.dialogs.CloseWithoutMessage()
	case noticeDismissedMsg:
		m.dialogs.notice.Clear()
		m.dialogs.Restore(message.ReturnTo)
	case clearDraftsMsg:
		m.clearDrafts()
	default:
		panic("unhandled dialog message")
	}
	return batchCommands(result.command, effect)
}

func batchCommands(commands ...tea.Cmd) tea.Cmd {
	filtered := commands[:0]
	for _, command := range commands {
		if command != nil {
			filtered = append(filtered, command)
		}
	}
	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		return tea.Batch(filtered...)
	}
}
