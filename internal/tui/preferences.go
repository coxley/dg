package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
)

type preferencesFile struct {
	Router        layout.Router `json:"router"`
	ApplyToFuture bool          `json:"apply_to_future"`
	SaveDirectory string        `json:"save_directory,omitempty"`
}

type preferenceState struct {
	router                layout.Router
	originalRouter        layout.Router
	applyToFuture         bool
	originalApplyToFuture bool
	saveDirectory         string
	originalSaveDirectory string
	path                  string
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
		m.status = "load preferences: " + err.Error()
		return
	}
	m.preferences.path = path
	m.preferences.applyToFuture = preferences.ApplyToFuture
	m.preferences.saveDirectory = preferences.SaveDirectory
}

// PreferredRouter returns the persisted router for newly created diagrams.
func PreferredRouter() (layout.Router, bool) {
	preferences, _, err := readPreferences()
	return preferences.Router, err == nil && preferences.ApplyToFuture
}

func (m *Model) openHelp() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	m.modal = modalHelp
	m.preferenceEdit = false
	m.status = ""
}

func (m *Model) openPreferences() {
	m.modal = modalPreferences
	if m.preferenceEdit {
		return
	}
	m.preferenceEdit = true
	m.preferenceRow = 0
	m.preferences.originalRouter = m.geo.Router()
	m.preferences.router = m.preferences.originalRouter
	m.preferences.originalApplyToFuture = m.preferences.applyToFuture
	m.preferences.originalSaveDirectory = m.preferences.saveDirectory
	m.beginTransaction()
}

func (m *Model) updateModal(message tea.KeyPressMsg) {
	key := message.Key()
	if (m.modal == modalHelp || m.modal == modalPreferences) &&
		key.Code == tea.KeyTab &&
		(key.Mod == 0 || key.Mod == tea.ModShift) {
		if m.modal == modalHelp {
			m.openPreferences()
		} else {
			m.modal = modalHelp
		}
		return
	}
	switch m.modal {
	case modalNone:
	case modalHelp:
		switch {
		case key.Code == tea.KeyEscape || key.Code == '?' || key.Code == tea.KeyEnter:
			m.closeSettingsModal()
		case key.Code == 'p' && key.Mod == 0:
			m.openPreferences()
		}
	case modalPreferences:
		m.updatePreferences(key)
	case modalSave:
		m.updateSavePath(message)
	}
}

func (m *Model) updatePreferences(key tea.Key) {
	const preferenceRows = 8
	if key.Code == tea.KeyEscape {
		m.closeSettingsModal()
		return
	}
	if key.Code == tea.KeyEnter {
		m.applyPreferences()
		return
	}
	switch key.Code {
	case tea.KeyUp:
		m.preferenceRow = (m.preferenceRow - 1 + preferenceRows) % preferenceRows
	case tea.KeyDown:
		m.preferenceRow = (m.preferenceRow + 1) % preferenceRows
	case tea.KeyLeft:
		m.adjustPreference(-1)
	case tea.KeyRight:
		m.adjustPreference(1)
	case ' ':
		if m.preferenceRow == 6 {
			m.preferences.applyToFuture = !m.preferences.applyToFuture
		}
	default:
		if m.preferenceRow == 7 && key.Mod == 0 {
			switch key.Code {
			case tea.KeyBackspace:
				start := previousGraphemeStart(
					[]byte(m.preferences.saveDirectory),
					len(m.preferences.saveDirectory),
				)
				m.preferences.saveDirectory = m.preferences.saveDirectory[:start]
			default:
				m.preferences.saveDirectory += key.Text
			}
		}
	}
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
		err = errors.Join(err, m.render())
	}
	m.preferenceEdit = false
	m.modal = modalNone
	if err != nil {
		m.status = err.Error()
	}
}

func (m *Model) adjustPreference(delta int64) {
	router := &m.preferences.router
	value := func(cost *uint32) {
		if delta < 0 {
			*cost -= min(*cost, uint32(-delta))
		} else {
			*cost += min(^uint32(0)-*cost, uint32(delta))
		}
	}
	switch m.preferenceRow {
	case 0:
		value(&router.Costs.Step)
	case 1:
		value(&router.Costs.SharedStep)
	case 2:
		value(&router.Costs.Bend)
	case 3:
		value(&router.Costs.Crossing)
	case 4:
		value(&router.Costs.EndpointStep)
	case 5:
		if delta < 0 {
			router.ReroutePasses -= min(router.ReroutePasses, uint8(-delta))
		} else {
			router.ReroutePasses += min(^uint8(0)-router.ReroutePasses, uint8(delta))
		}
	case 6:
		m.preferences.applyToFuture = !m.preferences.applyToFuture
	}
	if m.preferenceRow <= 5 {
		m.geo.SetRouter(*router)
		if err := m.rebuild(); err != nil {
			m.status = err.Error()
		}
	}
}

func (m *Model) applyPreferences() {
	if err := m.commitTransaction(); err != nil {
		m.status = err.Error()
		return
	}
	m.preferenceEdit = false
	if m.preferences.path == "" {
		path, err := preferencesPath()
		if err != nil {
			m.status = err.Error()
			return
		}
		m.preferences.path = path
	}
	data, err := json.MarshalIndent(preferencesFile{
		Router:        m.preferences.router,
		ApplyToFuture: m.preferences.applyToFuture,
		SaveDirectory: m.preferences.saveDirectory,
	}, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(m.preferences.path), 0o700)
	}
	if err == nil {
		err = os.WriteFile(m.preferences.path, append(data, '\n'), 0o600)
	}
	m.modal = modalHelp
	if err != nil {
		m.status = "save preferences: " + err.Error()
	}
}
