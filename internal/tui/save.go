package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/internal/tui/directorypicker"
)

const defaultSaveName = "diagram.json"

const (
	saveDirectoryField chrome.ID = "save-directory"
	saveNameField      chrome.ID = "save-name"
	saveConfirmAction  chrome.ID = "save-confirm"
	saveCancelAction   chrome.ID = "save-cancel"
)

func (m *Model) requestSave() {
	if m.mode != modeNavigate {
		m.setError(finishOperation)
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
	m.saveName = defaultSaveName
	m.resetSaveForm()
	m.openDialog(surfaceSave)
	m.status = ""
}

func (m *Model) resetSaveForm() {
	directory := m.saveDirectory
	if directory == "" {
		directory = "."
	}
	name := m.saveName
	if name == "" {
		name = defaultSaveName
	}
	m.saveForm = chrome.NewForm(chrome.FormDeclaration{
		Fields: []chrome.FormField{
			{
				ID: saveDirectoryField, Label: "Directory",
				Kind: chrome.DirectoryField, Directory: directory,
			},
			{
				ID: saveNameField, Label: "File name", Kind: chrome.TextField,
				Text: name, Placeholder: defaultSaveName,
			},
		},
		Spacer: chrome.FormSpacer{ID: "save-spacer", Grow: 1},
		Actions: chrome.ButtonListDeclaration{
			ID: "save-actions",
			Buttons: []chrome.Button{
				{ID: saveConfirmAction, Label: "Save"},
				{ID: saveCancelAction, Label: "Cancel"},
			},
		},
	}, m.theme.formStyles())
	m.savePicker = directorypicker.New(directorypicker.Config{
		Title:      "Directory",
		Value:      directory,
		AllowFiles: true,
		ShowHidden: true,
	}, directorypicker.Styles{Dark: m.theme.Dark})
}

func (m *Model) updateSaveForm(message tea.Msg) tea.Cmd {
	if m.savePicker != nil && m.savePicker.Opened() {
		picker, command := m.savePicker.Update(message)
		m.savePicker = picker.(*directorypicker.Model)
		m.saveDirectory = m.savePicker.Value()
		if !m.savePicker.Opened() {
			m.saveForm.SetDirectory(saveDirectoryField, m.saveDirectory)
		}
		return command
	}
	switch message := message.(type) {
	case chrome.FormActivateMsg:
		if message.ID == saveDirectoryField {
			m.savePicker.SetValue(m.saveDirectory)
			m.savePicker.Open()
		}
		return nil
	case chrome.FormSubmitMsg:
		switch message.ID {
		case saveConfirmAction:
			m.commitSaveForm()
		case saveCancelAction:
			m.closeSaveForm()
		}
		return nil
	default:
		form, command := m.saveForm.Update(message)
		m.saveForm = form.(*chrome.Form)
		m.syncSaveForm()
		return command
	}
}

func (m *Model) commitSaveForm() {
	name := strings.TrimSpace(m.saveName)
	if name == "" {
		m.setError("enter a file name")
		return
	}
	if filepath.Base(name) != name {
		m.setError("file name must not contain a directory")
		return
	}
	path := filepath.Join(m.saveDirectory, name)
	if info, err := os.Stat(m.saveDirectory); err == nil && !info.IsDir() {
		path = m.saveDirectory
		if name != defaultSaveName {
			path = filepath.Join(filepath.Dir(path), name)
		}
	}
	if !m.save(path) {
		return
	}
	m.closeSaveForm()
}

func (m *Model) closeSaveForm() {
	m.activeDialog = surfaceNone
	m.savePicker.Close()
	m.saveDirectory = ""
	m.saveName = ""
}

func (m *Model) syncSaveForm() {
	if m.saveForm == nil {
		return
	}
	m.saveDirectory, _ = m.saveForm.Directory(saveDirectoryField)
	m.saveName, _ = m.saveForm.Text(saveNameField)
}

func (m *Model) save(path string) bool {
	data, err := document.Marshal(m.geo)
	if err != nil {
		m.setError(err.Error())
		return false
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		m.setError(fmt.Sprintf("save diagram: %v", err))
		return false
	}
	m.path = path
	m.status = "saved " + path
	if m.history != nil {
		if err := m.history.Store(path); err != nil {
			m.status += fmt.Sprintf(" (undo history: %v)", err)
			m.statusError = m.status
		}
	}
	return true
}
