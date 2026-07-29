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
	router                layout.Router
	originalRouter        layout.Router
	applyToFuture         bool
	originalApplyToFuture bool
	saveDirectory         string
	originalSaveDirectory string
	commentPrefix         string
	originalCommentPrefix string
	keyProfile            chrome.KeyProfile
	originalKeyProfile    chrome.KeyProfile
	darkTint              string
	lightTint             string
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
	theme  Theme
	bounds chrome.Rect
}

func newPreferenceDialogBody(
	value preferenceDialogValue,
	theme Theme,
) *preferenceDialogBody {
	body := &preferenceDialogBody{theme: theme}
	body.model = preferencesview.New(
		value,
		minimumSettingsModalWidth-
			theme.Modal.Container.GetHorizontalFrameSize()-
			theme.Modal.Body.GetHorizontalFrameSize(),
		0,
		theme.preferenceStyles(),
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

func (b *preferenceDialogBody) SetStyles(theme Theme) {
	b.theme = theme
	b.model.SetStyles(theme.preferenceStyles())
}

func (m *Model) applySettingsSnapshot(snapshot settings.Snapshot) {
	m.preferences.router = m.geo.Router()
	m.preferences.applyToFuture = snapshot.ApplyToFuture
	m.preferences.saveDirectory = snapshot.SaveDirectory
	m.preferences.commentPrefix = preferencesview.NormalizeCommentPrefix(
		snapshot.CommentPrefix,
	)
	m.preferences.keyProfile = keyProfile(snapshot.ShortcutStyle)
	m.preferences.darkTint = snapshot.DarkTint
	m.preferences.lightTint = snapshot.LightTint
	m.bindings.SetProfile(m.preferences.keyProfile)
	m.syncSidebarShortcut()
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
	if m.dialogs.ActiveID() != surfaceNone || m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	m.beginPreferenceEdit()
	m.dialogs.OpenPreferences(m.preferenceValue())
	m.syncWorkspace()
}

func (m *Model) preferenceValue() preferenceDialogValue {
	return preferenceDialogValue{
		Router:        m.geo.Router(),
		SaveDirectory: m.preferences.saveDirectory,
		CommentPrefix: m.preferences.commentPrefix,
		KeyProfile:    m.preferences.keyProfile,
	}
}

func (m *Model) beginPreferenceEdit() {
	if m.preferenceEdit {
		return
	}
	m.preferenceEdit = true
	m.preferences.originalRouter = m.geo.Router()
	m.preferences.router = m.preferences.originalRouter
	m.preferences.originalApplyToFuture = m.preferences.applyToFuture
	m.preferences.originalSaveDirectory = m.preferences.saveDirectory
	m.preferences.originalCommentPrefix = m.preferences.commentPrefix
	m.preferences.originalKeyProfile = m.preferences.keyProfile
	m.beginTransaction()
}

func (m *Model) cancelPreferences() {
	var err error
	if m.preferenceEdit {
		hadTransaction := m.transactionOpen
		err = m.cancelTransaction()
		if !hadTransaction {
			m.geo.SetRouter(m.preferences.originalRouter)
			err = errors.Join(err, m.geo.Build())
		}
		m.preferences.router = m.preferences.originalRouter
		m.preferences.applyToFuture = m.preferences.originalApplyToFuture
		m.preferences.saveDirectory = m.preferences.originalSaveDirectory
		m.preferences.commentPrefix = m.preferences.originalCommentPrefix
		m.preferences.keyProfile = m.preferences.originalKeyProfile
		m.bindings.SetProfile(m.preferences.originalKeyProfile)
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
	m.preferences.saveDirectory = value.SaveDirectory
	m.preferences.commentPrefix = value.CommentPrefix
	m.preferences.keyProfile = value.KeyProfile
	m.bindings.SetProfile(value.KeyProfile)
	m.syncSidebarShortcut()
	if value.Router == m.preferences.router {
		return
	}
	m.preferences.router = value.Router
	m.geo.SetRouter(value.Router)
	if err := m.rebuild(); err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) savePreferences(message preferenceSaveMsg) tea.Cmd {
	m.previewPreferences(message.Value)
	snapshot := settings.Snapshot{
		Router:        message.Value.Router,
		ApplyToFuture: message.SaveDefaults,
		SaveDirectory: message.Value.SaveDirectory,
		CommentPrefix: message.Value.CommentPrefix,
		ShortcutStyle: shortcutStyle(message.Value.KeyProfile),
		DarkTint:      m.preferences.darkTint,
		LightTint:     m.preferences.lightTint,
	}
	if err := m.settingsStore.Save(snapshot); err != nil {
		m.setError("save preferences: " + err.Error())
		return nil
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return nil
	}
	m.preferences.applyToFuture = message.SaveDefaults
	m.preferenceEdit = false
	m.status = ""
	return m.showNotice("Preferences saved", surfaceNone)
}
