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
	if m.activeDialog == surfacePreferences {
		return
	}
	if m.activeDialog != surfaceNone || m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	m.resetPreferenceForm()
	m.beginPreferenceEdit()
	m.openDialog(surfacePreferences)
	m.syncWorkspace()
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

func (m *Model) closeSettingsModal() {
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
		err = errors.Join(err, m.render())
	}
	m.preferenceEdit = false
	m.activeDialog = surfaceNone
	if err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) applyPreferences(saveDefaults bool) tea.Cmd {
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return nil
	}
	m.preferences.applyToFuture = saveDefaults
	m.preferenceEdit = false
	err := m.settingsStore.Save(settings.Snapshot{
		Router:        m.preferences.router,
		ApplyToFuture: m.preferences.applyToFuture,
		SaveDirectory: m.preferences.saveDirectory,
		CommentPrefix: m.preferences.commentPrefix,
		ShortcutStyle: shortcutStyle(m.preferences.keyProfile),
		DarkTint:      m.preferences.darkTint,
		LightTint:     m.preferences.lightTint,
	})
	if err != nil {
		m.setError("save preferences: " + err.Error())
		m.activeDialog = surfaceNone
		return nil
	}
	m.status = ""
	return m.showNotice("Preferences saved", surfaceNone)
}

func (m *Model) resetPreferenceForm() {
	m.preferenceForm = preferencesview.New(
		preferencesview.Value{
			Router:        m.geo.Router(),
			SaveDirectory: m.preferences.saveDirectory,
			CommentPrefix: m.preferences.commentPrefix,
			KeyProfile:    m.preferences.keyProfile,
		},
		minimumSettingsModalWidth-
			m.theme.Modal.Container.GetHorizontalFrameSize()-
			m.theme.Modal.Body.GetHorizontalFrameSize(),
		0,
		m.theme.preferenceStyles(),
	)
}

func (m *Model) updateSettingsTabs(message tea.Msg) tea.Cmd {
	if m.activeDialog != surfacePreferences {
		return nil
	}
	form, command := m.preferenceForm.Update(message)
	m.preferenceForm = form.(*preferencesview.Model)
	m.syncPreferenceForm()
	if action, completed := m.preferenceForm.Completed(); completed {
		switch action {
		case preferencesview.ActionCancel:
			m.closeSettingsModal()
			return command
		case preferencesview.ActionSave:
			return tea.Batch(command, m.applyPreferences(false))
		case preferencesview.ActionSaveDefaults:
			return tea.Batch(command, m.applyPreferences(true))
		case preferencesview.ActionNone:
		}
	}
	return command
}

func (m *Model) updateSettingsWheel(message tea.MouseWheelMsg) tea.Cmd {
	var delta int
	switch message.Mouse().Button {
	case tea.MouseWheelUp:
		delta = -1
	case tea.MouseWheelDown:
		delta = 1
	default:
		return nil
	}
	return m.updateSettingsTabs(preferencesview.ScrollMsg{Delta: delta})
}

func (m *Model) syncPreferenceForm() {
	if m.preferenceForm == nil {
		return
	}
	value := m.preferenceForm.Value()
	router := value.Router
	m.preferences.saveDirectory = value.SaveDirectory
	m.preferences.commentPrefix = value.CommentPrefix
	m.preferences.keyProfile = value.KeyProfile
	m.bindings.SetProfile(value.KeyProfile)
	if router == m.preferences.router {
		return
	}
	m.preferences.router = router
	m.geo.SetRouter(router)
	if err := m.rebuild(); err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) resizePreferenceForm() {
	if m.preferenceForm == nil {
		return
	}
	height := 0
	if m.dialog.Overlay().Height != 0 {
		height = m.dialog.BodyHeight()
	}
	m.preferenceForm.SetHeight(height)
}
