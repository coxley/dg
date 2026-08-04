package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/internal/tui/chrome"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
	"github.com/coxley/dg/layout"
)

type preferenceState struct {
	baseline      preferenceDialogValue
	draft         preferenceDialogValue
	defaultRouter layout.Router
}

type preferenceDialogValue = preferencesview.Value

type preferencePreviewMsg struct {
	Value preferenceDialogValue
}

type preferenceSaveMsg struct {
	Value preferenceDialogValue
}

type preferenceCancelMsg struct{}

type dialogCancelMsg struct{}

type preferenceDialogBody struct {
	model  *preferencesview.Model
	bounds chrome.Rect
}

func newPreferenceDialogBody(
	value preferenceDialogValue,
	width int,
	styles preferencesview.Styles,
) *preferenceDialogBody {
	body := &preferenceDialogBody{}
	body.model = preferencesview.New(
		value,
		width,
		0,
		styles,
		preferencesview.WithTints(
			tintOptions(darkTints),
			tintOptions(lightTints),
		),
		preferencesview.WithKeybindActions(keybindActions()),
	)
	return body
}

func (b *preferenceDialogBody) Reset(value preferenceDialogValue) {
	b.model.Reset(value)
	b.SetBounds(b.bounds)
}

func (b *preferenceDialogBody) Context() string {
	if b.model.DirectoryOpen() {
		return "directory picker"
	}
	return string(scopePreferences)
}

func (*preferenceDialogBody) PreferredWidth() int {
	return minimumSettingsModalWidth
}

func (b *preferenceDialogBody) Scopes() []chrome.ScopeID {
	if b.model.DirectoryOpen() {
		return []chrome.ScopeID{scopeDirectory, scopePreferences, scopeGlobal}
	}
	return []chrome.ScopeID{scopePreferences, scopeGlobal}
}

func (*preferenceDialogBody) TextEntry() bool {
	return false
}

func (b *preferenceDialogBody) CapturesKey() bool {
	return b.model.CapturesKey()
}

func (b *preferenceDialogBody) PointerOccupied(point chrome.Point) bool {
	return b.model.PointerOccupied(point.X, point.Y)
}

func (b *preferenceDialogBody) ActiveTab() preferencesview.Tab {
	return b.model.ActiveTab()
}

func (b *preferenceDialogBody) SetTab(tab preferencesview.Tab) {
	b.model.SetTab(tab)
}

func (b *preferenceDialogBody) SetBounds(bounds chrome.Rect) {
	if b.bounds == bounds {
		return
	}
	b.bounds = bounds
	b.model.SetBounds(bounds.Width, bounds.Height)
}

func (b *preferenceDialogBody) Update(message tea.Msg) dialogBodyResult {
	before := b.model.Value()
	switch message := message.(type) {
	case dialogClickMsg:
		return b.update(preferencesview.ClickMsg{
			X: message.Point.X,
			Y: message.Point.Y,
		}, before)
	case dialogWheelMsg:
		var delta int
		switch message.Mouse.Button {
		case tea.MouseWheelUp:
			delta = -1
		case tea.MouseWheelDown:
			delta = 1
		default:
			return dialogBodyResult{}
		}
		return b.update(preferencesview.ScrollMsg{Delta: delta}, before)
	case dialogBackMsg:
		if !b.model.DirectoryOpen() {
			return dialogBodyResult{}
		}
		return b.update(
			tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}),
			before,
		)
	case dialogCloseMsg:
		return dialogBodyResult{
			message: preferenceCancelMsg{},
			handled: true,
		}
	default:
		return b.update(message, before)
	}
}

func (b *preferenceDialogBody) update(
	message tea.Msg,
	before preferenceDialogValue,
) dialogBodyResult {
	model, command := b.model.Update(message)
	b.model = model.(*preferencesview.Model)
	if action, completed := b.model.TakeCompleted(); completed {
		switch action {
		case preferencesview.ActionCancel:
			return dialogBodyResult{
				message: preferenceCancelMsg{},
				command: command,
				handled: true,
			}
		case preferencesview.ActionSave:
			return dialogBodyResult{
				message: preferenceSaveMsg{Value: b.model.Value()},
				command: command,
				handled: true,
			}
		case preferencesview.ActionNone:
		}
	}
	if value := b.model.Value(); !preferencesview.Equal(value, before) {
		return dialogBodyResult{
			message: preferencePreviewMsg{Value: value},
			command: command,
			handled: true,
		}
	}
	return dialogBodyResult{command: command, handled: true}
}

func (b *preferenceDialogBody) SubmitSave() dialogBodyResult {
	before := b.model.Value()
	b.model.SubmitSave()
	return b.update(nil, before)
}

func (b *preferenceDialogBody) View() string {
	return b.model.View().Content
}

func (b *preferenceDialogBody) SetStyles(styles preferencesview.Styles) {
	b.model.SetStyles(styles)
}

func (m *Model) applySettingsSnapshot(snapshot settings.Snapshot) {
	darkTint, lightTint := normalizeTintIDs(snapshot.DarkTint, snapshot.LightTint)
	bindings := configuredBindings(snapshot)
	m.preferences.baseline = preferenceDialogValue{
		Router:        m.geo.Router(),
		SaveDirectory: snapshot.SaveDirectory,
		CommentPrefix: preferencesview.NormalizeCommentPrefix(
			snapshot.CommentPrefix,
		),
		Theme:            preferencesview.NormalizeTheme(string(snapshot.Theme)),
		Keybinds:         keybindValues(bindings),
		OpaqueBackground: snapshot.OpaqueBackground,
		DarkTint:         darkTint,
		LightTint:        lightTint,
	}
	m.preferences.draft = m.preferences.baseline
	m.preferences.defaultRouter = snapshot.Router
	if m.preferences.defaultRouter == (layout.Router{}) {
		m.preferences.defaultRouter = layout.DefaultRouter()
	}
	m.bindings.SetBindings(bindings)
	m.theme = themeForTints(
		m.preferredDark(m.preferences.baseline.Theme),
		darkTint,
		lightTint,
	)
}

func (m *Model) openHelp() {
	m.helpInspector.toggle()
	m.status = ""
	m.syncWorkspace()
}

func (m *Model) openPreferences() {
	if m.dialogs.ActiveID() == surfacePreferences {
		return
	}
	if m.dialogs.ActiveID() != surfaceNone || !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	m.beginPreferenceEdit()
	m.dialogs.OpenPreferences(m.preferenceValue())
	m.syncWorkspace()
}

func (m *Model) preferenceValue() preferenceDialogValue {
	if m.preferenceEdit {
		return m.preferences.draft
	}
	return m.preferences.baseline
}

func (m *Model) beginPreferenceEdit() {
	if m.preferenceEdit {
		return
	}
	m.preferenceEdit = true
	m.preferences.baseline.Router = m.geo.Router()
	m.preferences.draft = m.preferences.baseline
	m.beginTransaction(transactionPreferences)
}

func (m *Model) cancelPreferences() {
	var err error
	if m.preferenceEdit {
		hadTransaction := m.interaction.transaction.open()
		err = m.cancelTransaction()
		if !hadTransaction {
			m.geo.SetRouter(m.preferences.baseline.Router)
			err = errors.Join(err, m.geo.Build())
		}
		m.preferences.draft = m.preferences.baseline
		m.bindings.SetBindings(bindingsFromValues(m.preferences.baseline.Keybinds))
		m.applyTheme(themeForTints(
			m.preferredDark(m.preferences.baseline.Theme),
			m.preferences.baseline.DarkTint,
			m.preferences.baseline.LightTint,
		))
		m.syncSidebarShortcut()
		err = errors.Join(err, m.render())
	}
	m.preferenceEdit = false
	m.dialogs.CloseWithoutMessage()
	if err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) previewPreferences(value preferenceDialogValue) {
	value.DarkTint, value.LightTint = normalizeTintIDs(
		value.DarkTint,
		value.LightTint,
	)
	previous := m.preferences.draft
	m.preferences.draft = value
	m.bindings.SetBindings(bindingsFromValues(value.Keybinds))
	m.syncSidebarShortcut()
	if value.Theme != previous.Theme ||
		value.DarkTint != previous.DarkTint ||
		value.LightTint != previous.LightTint {
		m.applyTheme(themeForTints(
			m.preferredDark(value.Theme),
			value.DarkTint,
			value.LightTint,
		))
	}
	if value.Router == previous.Router {
		return
	}
	m.geo.SetRouter(value.Router)
	if err := m.rebuild(); err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) savePreferences(message preferenceSaveMsg) tea.Cmd {
	m.previewPreferences(message.Value)
	draft := m.preferences.draft
	snapshot := settings.Snapshot{
		Router:           draft.Router,
		SaveDirectory:    draft.SaveDirectory,
		CommentPrefix:    draft.CommentPrefix,
		Theme:            settings.Theme(draft.Theme),
		Keybinds:         settingsKeybinds(draft.Keybinds),
		DarkTint:         draft.DarkTint,
		LightTint:        draft.LightTint,
		OpaqueBackground: draft.OpaqueBackground,
	}
	if err := m.settingsStore.Save(snapshot); err != nil {
		m.setError("save preferences: " + err.Error())
		return nil
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return nil
	}
	m.preferences.baseline = draft
	m.preferences.defaultRouter = draft.Router
	m.preferenceEdit = false
	m.status = ""
	return m.showNotice("Preferences saved", surfaceNone)
}

func (m *Model) preferredDark(theme string) bool {
	switch preferencesview.NormalizeTheme(theme) {
	case preferencesview.ThemeDark:
		return true
	case preferencesview.ThemeLight:
		return false
	default:
		return m.terminalDark
	}
}
