package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/internal/tui/chrome"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
)

type preferenceState struct {
	baseline              preferenceDialogValue
	draft                 preferenceDialogValue
	applyToFuture         bool
	baselineApplyToFuture bool
}

type preferenceDialogValue = preferencesview.Value

type preferencePreviewMsg struct {
	Value preferenceDialogValue
}

type preferenceSaveMsg struct {
	Value        preferenceDialogValue
	SaveDefaults bool
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

func (b *preferenceDialogBody) SetBounds(bounds chrome.Rect) {
	b.bounds = bounds
	b.model.SetWidth(bounds.Width)
	b.model.SetHeight(bounds.Height)
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
		case preferencesview.ActionSaveDefaults:
			return dialogBodyResult{
				message: preferenceSaveMsg{
					Value:        b.model.Value(),
					SaveDefaults: true,
				},
				command: command,
				handled: true,
			}
		case preferencesview.ActionNone:
		}
	}
	if value := b.model.Value(); value != before {
		return dialogBodyResult{
			message: preferencePreviewMsg{Value: value},
			command: command,
			handled: true,
		}
	}
	return dialogBodyResult{command: command, handled: true}
}

func (b *preferenceDialogBody) View() string {
	return b.model.View().Content
}

func (b *preferenceDialogBody) SetStyles(styles preferencesview.Styles) {
	b.model.SetStyles(styles)
}

func (m *Model) applySettingsSnapshot(snapshot settings.Snapshot) {
	darkTint, lightTint := normalizeTintIDs(snapshot.DarkTint, snapshot.LightTint)
	m.preferences.baseline = preferenceDialogValue{
		Router:        m.geo.Router(),
		SaveDirectory: snapshot.SaveDirectory,
		CommentPrefix: preferencesview.NormalizeCommentPrefix(
			snapshot.CommentPrefix,
		),
		KeyProfile:       keyProfile(snapshot.ShortcutStyle),
		OpaqueBackground: snapshot.OpaqueBackground,
		DarkTint:         darkTint,
		LightTint:        lightTint,
	}
	m.preferences.draft = m.preferences.baseline
	m.preferences.applyToFuture = snapshot.ApplyToFuture
	m.bindings.SetProfile(m.preferences.baseline.KeyProfile)
	m.theme = themeForTints(true, darkTint, lightTint)
}

func keyProfile(style settings.ShortcutStyle) chrome.KeyProfile {
	switch style {
	case settings.ShortcutMac:
		return chrome.ProfileMac
	case settings.ShortcutStandard:
		return chrome.ProfileStandard
	case settings.ShortcutAuto, "":
		return chrome.ProfileAuto
	default:
		return chrome.ProfileAuto
	}
}

func shortcutStyle(profile chrome.KeyProfile) settings.ShortcutStyle {
	switch profile {
	case chrome.ProfileMac:
		return settings.ShortcutMac
	case chrome.ProfileStandard:
		return settings.ShortcutStandard
	case chrome.ProfileAuto:
		return settings.ShortcutAuto
	default:
		return settings.ShortcutAuto
	}
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
	m.preferences.baselineApplyToFuture = m.preferences.applyToFuture
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
		m.preferences.applyToFuture = m.preferences.baselineApplyToFuture
		m.bindings.SetProfile(m.preferences.baseline.KeyProfile)
		m.applyTheme(themeForTints(
			m.theme.Dark,
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
	m.bindings.SetProfile(value.KeyProfile)
	m.syncSidebarShortcut()
	if value.DarkTint != previous.DarkTint ||
		value.LightTint != previous.LightTint {
		m.applyTheme(themeForTints(m.theme.Dark, value.DarkTint, value.LightTint))
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
		ApplyToFuture:    message.SaveDefaults,
		SaveDirectory:    draft.SaveDirectory,
		CommentPrefix:    draft.CommentPrefix,
		ShortcutStyle:    shortcutStyle(draft.KeyProfile),
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
	m.preferences.applyToFuture = message.SaveDefaults
	m.preferenceEdit = false
	m.status = ""
	return m.showNotice("Preferences saved", surfaceNone)
}
