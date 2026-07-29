package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
	"github.com/coxley/dg/layout"
)

type preferencesFile struct {
	Router        layout.Router     `json:"router"`
	ApplyToFuture bool              `json:"apply_to_future"`
	SaveDirectory string            `json:"save_directory,omitempty"`
	CommentPrefix string            `json:"comment_prefix,omitempty"`
	KeyProfile    chrome.KeyProfile `json:"key_profile,omitempty"`
}

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
	path                  string
}

type componentKind uint8

const (
	saveComponent componentKind = iota
)

type componentMsg struct {
	kind    componentKind
	message tea.Msg
}

func preferencesPath() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	name := "dg"
	if runtime.GOOS == "darwin" {
		name = "org.coxley.dg"
	}
	return filepath.Join(cache, name, "preferences.json"), nil
}

func readPreferences() (preferencesFile, string, error) {
	path, err := preferencesPath()
	if err != nil {
		return preferencesFile{}, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return preferencesFile{}, path, nil
	}
	if err != nil {
		return preferencesFile{}, path, err
	}
	var preferences preferencesFile
	if err := json.Unmarshal(data, &preferences); err != nil {
		return preferencesFile{}, path, err
	}
	return preferences, path, nil
}

func (m *Model) loadPreferences() {
	preferences, path, err := readPreferences()
	if err != nil {
		m.setError("load preferences: " + err.Error())
		return
	}
	m.preferences.path = path
	m.preferences.applyToFuture = preferences.ApplyToFuture
	m.preferences.saveDirectory = preferences.SaveDirectory
	m.preferences.commentPrefix = preferencesview.NormalizeCommentPrefix(
		preferences.CommentPrefix,
	)
	m.preferences.keyProfile = preferencesview.NormalizeKeyProfile(preferences.KeyProfile)
	m.bindings.SetProfile(m.preferences.keyProfile)
}

// PreferredRouter returns the persisted router for newly created diagrams.
func PreferredRouter() (layout.Router, bool) {
	preferences, _, err := readPreferences()
	return preferences.Router, err == nil && preferences.ApplyToFuture
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
	if m.preferences.path == "" {
		path, err := preferencesPath()
		if err != nil {
			m.setError(err.Error())
			return nil
		}
		m.preferences.path = path
	}
	data, err := json.MarshalIndent(preferencesFile{
		Router:        m.preferences.router,
		ApplyToFuture: m.preferences.applyToFuture,
		SaveDirectory: m.preferences.saveDirectory,
		CommentPrefix: m.preferences.commentPrefix,
		KeyProfile:    m.preferences.keyProfile,
	}, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(m.preferences.path), 0o700)
	}
	if err == nil {
		err = os.WriteFile(m.preferences.path, append(data, '\n'), 0o600)
	}
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

func componentCommand(kind componentKind, command tea.Cmd) tea.Cmd {
	if command == nil {
		return nil
	}
	return func() tea.Msg {
		message := command()
		if message == nil {
			return nil
		}
		if batch, ok := message.(tea.BatchMsg); ok {
			for i := range batch {
				batch[i] = componentCommand(kind, batch[i])
			}
			return batch
		}
		return componentMsg{kind: kind, message: message}
	}
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
