package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/coxley/dg/document"
)

func (m *Model) requestSave() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	if m.path == "" {
		m.openSaveForm()
		return
	}
	m.save(m.path)
}

func (m *Model) openSaveForm() {
	m.saveDirectory = m.preferences.saveDirectory
	if m.saveDirectory == "" {
		m.saveDirectory = "."
	}
	m.saveName = "diagram.json"
	m.saveForm = huh.NewForm(
		huh.NewGroup(
			huh.NewFilePicker().
				Title("Directory").
				DirAllowed(true).
				FileAllowed(true).
				ShowHidden(true).
				CurrentDirectory(m.saveDirectory).
				Picking(true).
				Value(&m.saveDirectory),
			huh.NewInput().
				Title("File name").
				Placeholder("diagram.json").
				Value(&m.saveName).
				Validate(func(name string) error {
					if name == "" {
						return errors.New("enter a file name")
					}
					if filepath.Base(name) != name {
						return errors.New("file name must not contain a directory")
					}
					return nil
				}),
		),
	).
		WithWidth(64).
		WithHeight(14).
		WithShowHelp(true)
	_ = m.saveForm.Init()
	m.modal = modalSave
	m.status = ""
}

func (m *Model) updateSaveForm(message tea.Msg) tea.Cmd {
	form, command := m.saveForm.Update(message)
	m.saveForm = form.(*huh.Form)
	if m.saveForm.State != huh.StateCompleted {
		return componentCommand(saveComponent, command)
	}
	m.commitSaveForm()
	return nil
}

func (m *Model) commitSaveForm() {
	name := strings.TrimSpace(m.saveName)
	if name == "" {
		m.status = "enter a file name"
		return
	}
	path := filepath.Join(m.saveDirectory, name)
	if info, err := os.Stat(m.saveDirectory); err == nil && !info.IsDir() {
		path = m.saveDirectory
		if name != "diagram.json" {
			path = filepath.Join(filepath.Dir(path), name)
		}
	}
	if !m.save(path) {
		return
	}
	m.closeSaveForm()
}

func (m *Model) closeSaveForm() {
	m.modal = modalNone
	m.saveForm = nil
	m.saveDirectory = ""
	m.saveName = ""
}

func (m *Model) save(path string) bool {
	data, err := document.Marshal(m.geo)
	if err != nil {
		m.status = err.Error()
		return false
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		m.status = fmt.Sprintf("save diagram: %v", err)
		return false
	}
	m.path = path
	m.status = "saved " + path
	if m.history != nil {
		if err := m.history.Store(path); err != nil {
			m.status += fmt.Sprintf(" (undo history: %v)", err)
		}
	}
	return true
}
