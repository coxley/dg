package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	keybinding "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	modalview "github.com/coxley/dg/internal/tui/modal"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
	"github.com/coxley/dg/layout"
)

type preferencesFile struct {
	Router        layout.Router `json:"router"`
	ApplyToFuture bool          `json:"apply_to_future"`
	SaveDirectory string        `json:"save_directory,omitempty"`
	CommentPrefix string        `json:"comment_prefix,omitempty"`
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
	path                  string
}

type componentKind uint8

const (
	exportComponent componentKind = iota
	saveComponent
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
	m.preferences.commentPrefix = normalizeCommentPrefix(preferences.CommentPrefix)
}

// PreferredRouter returns the persisted router for newly created diagrams.
func PreferredRouter() (layout.Router, bool) {
	preferences, _, err := readPreferences()
	return preferences.Router, err == nil && preferences.ApplyToFuture
}

func (m *Model) openHelp() {
	if m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	m.resetSettingsTabs(modalHelp)
	m.openModal(modalHelp)
	m.preferenceEdit = false
	m.status = ""
}

func (m *Model) openPreferences() {
	fromHelp := m.modal == modalHelp
	if !m.preferenceEdit {
		m.resetSettingsTabs(modalPreferences)
	} else {
		m.selectSettingsTab(modalPreferences)
	}
	m.beginPreferenceEdit()
	if !fromHelp {
		m.openModal(modalPreferences)
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
	m.beginTransaction()
}

func (m *Model) updateModal(message tea.KeyPressMsg) tea.Cmd {
	key := message.Key()
	if m.settingsModalCloses(key) {
		m.closeSettingsModal()
		return nil
	}
	switch m.modal {
	case modalNone:
		return nil
	case modalHelp:
		switch {
		case key.Code == tea.KeyEscape ||
			keybinding.Matches(message, m.keys.help) ||
			key.Code == tea.KeyEnter:
			m.closeSettingsModal()
			return nil
		case key.Code == 'p' && key.Mod == 0:
			m.openPreferences()
			return nil
		}
	case modalPreferences:
		if key.Code == tea.KeyEscape {
			m.closeSettingsModal()
			return nil
		}
	case modalExport:
		if key.Code == tea.KeyEscape {
			m.modal = modalNone
			m.exportText = ""
			return nil
		}
		return m.updateExportForm(message)
	case modalSave:
		if key.Code == tea.KeyEscape {
			m.closeSaveForm()
			return nil
		}
		if key.Code == 's' && key.Mod == tea.ModCtrl {
			m.commitSaveForm()
			return nil
		}
		return m.updateSaveForm(message)
	case modalNotice:
		return nil
	}
	if key.Code == tea.KeyTab && (key.Mod == 0 || key.Mod == tea.ModShift) {
		if m.modal == modalHelp {
			return modalview.SwitchTab(modalview.TabID(modalPreferences))
		}
		return modalview.SwitchTab(modalview.TabID(modalHelp))
	} else if m.modal == modalHelp && key.Code == '1' && key.Mod == 0 {
		return modalview.SwitchTab(modalview.TabID(modalHelp))
	} else if m.modal == modalHelp && key.Code == '2' && key.Mod == 0 {
		return modalview.SwitchTab(modalview.TabID(modalPreferences))
	}
	return m.updateSettingsTabs(message)
}

func (m *Model) settingsModalCloses(key tea.Key) bool {
	return (m.modal == modalHelp || m.modal == modalPreferences) &&
		key.Code == 'q' && key.Mod == 0
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
		err = errors.Join(err, m.render())
	}
	m.preferenceEdit = false
	m.modal = modalNone
	if err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) applyPreferences() tea.Cmd {
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return nil
	}
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
	}, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(m.preferences.path), 0o700)
	}
	if err == nil {
		err = os.WriteFile(m.preferences.path, append(data, '\n'), 0o600)
	}
	m.resetSettingsTabs(modalHelp)
	if err != nil {
		m.setError("save preferences: " + err.Error())
		m.modal = modalHelp
		return nil
	}
	m.status = ""
	return m.showNotice("Preferences saved", modalHelp)
}

func (m *Model) resetSettingsTabs(active modal) {
	m.preferenceForm = preferencesview.New(
		preferencesview.Value{
			Router:        m.geo.Router(),
			ApplyToFuture: m.preferences.applyToFuture,
			SaveDirectory: m.preferences.saveDirectory,
			CommentPrefix: m.preferences.commentPrefix,
		},
		preferenceModalWidth-4,
		m.preferenceFormHeight(),
		m.theme.preferenceStyles(),
	)
	m.modal = active
}

func (m *Model) selectSettingsTab(tab modal) {
	m.modal = tab
}

func (m *Model) updateSettingsTabs(message tea.Msg) tea.Cmd {
	if m.modal != modalPreferences {
		return nil
	}
	form, command := m.preferenceForm.Update(message)
	m.preferenceForm = form.(*preferencesview.Model)
	m.syncPreferenceForm()
	if save, completed := m.preferenceForm.Completed(); completed {
		if !save {
			m.closeSettingsModal()
			return command
		}
		return tea.Batch(command, m.applyPreferences())
	}
	return command
}

func (m *Model) updateSettingsWheel(message tea.MouseWheelMsg) tea.Cmd {
	var code rune
	switch message.Mouse().Button {
	case tea.MouseWheelUp:
		code = tea.KeyUp
	case tea.MouseWheelDown:
		code = tea.KeyDown
	default:
		return nil
	}
	return m.updateSettingsTabs(tea.KeyPressMsg(tea.Key{Code: code}))
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
	m.preferences.applyToFuture = value.ApplyToFuture
	m.preferences.saveDirectory = value.SaveDirectory
	m.preferences.commentPrefix = value.CommentPrefix
	if router == m.preferences.router {
		return
	}
	m.preferences.router = router
	m.geo.SetRouter(router)
	if err := m.rebuild(); err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) preferenceFormHeight() int {
	const (
		preferenceCount = 9
		modalFrameRows  = 4
	)
	if m.height == 0 {
		return preferenceCount + 1
	}
	return min(preferenceCount+1, max(m.diagramHeight()-modalFrameRows, 1))
}

func (m *Model) resizePreferenceForm() {
	if m.preferenceForm != nil {
		m.preferenceForm.SetHeight(m.preferenceFormHeight())
	}
}
