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

type saveDocumentMsg struct {
	Directory string
	Name      string
}

type saveDialogBody struct {
	form      *chrome.Form
	picker    *directorypicker.Model
	directory string
	name      string
	styles    saveStyles
	bounds    chrome.Rect
}

func newSaveDialogBody(styles saveStyles) *saveDialogBody {
	body := &saveDialogBody{styles: styles}
	body.Reset(".")
	return body
}

func (b *saveDialogBody) Reset(directory string) {
	if directory == "" {
		directory = "."
	}
	b.directory = directory
	b.name = defaultSaveName
	b.form = b.newForm(directory, b.name)
	b.picker = directorypicker.New(directorypicker.Config{
		Title: "Directory",
		Value: directory,
	}, b.styles.Picker)
	b.SetBounds(b.bounds)
}

func (b *saveDialogBody) newForm(directory, name string) *chrome.Form {
	return chrome.NewForm(chrome.FormDeclaration{
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
	}, b.styles.Form)
}

func (*saveDialogBody) Context() string {
	return string(commandSave)
}

func (*saveDialogBody) PreferredWidth() int {
	return 68
}

func (b *saveDialogBody) Scopes() []chrome.ScopeID {
	if b.picker.Opened() {
		return []chrome.ScopeID{scopeDirectory, scopeModal, scopeGlobal}
	}
	return []chrome.ScopeID{scopeModal, scopeGlobal}
}

func (*saveDialogBody) TextEntry() bool {
	return true
}

func (b *saveDialogBody) SetBounds(bounds chrome.Rect) {
	b.bounds = bounds
	b.form.SetBounds(bounds)
	b.picker.SetBounds(bounds.Width, bounds.Height)
}

func (b *saveDialogBody) Update(message tea.Msg) dialogBodyResult {
	switch message := message.(type) {
	case dialogClickMsg:
		if b.picker.Opened() {
			return b.updatePicker(tea.MouseClickMsg(message.Mouse))
		}
		command := b.form.Click(message.Point)
		if command == nil {
			return dialogBodyResult{handled: true}
		}
		return b.Update(command())
	case dialogWheelMsg:
		if !b.picker.Opened() {
			return dialogBodyResult{}
		}
		return b.updatePicker(tea.MouseWheelMsg(message.Mouse))
	case dialogBackMsg:
		if !b.picker.Opened() {
			return dialogBodyResult{}
		}
		b.closePicker()
		return dialogBodyResult{handled: true}
	case dialogCloseMsg:
		b.picker.Close()
		return dialogBodyResult{handled: true}
	}
	if b.picker.Opened() {
		return b.updatePicker(message)
	}
	switch message := message.(type) {
	case chrome.FormActivateMsg:
		if message.ID == saveDirectoryField {
			b.picker.SetValue(b.directory)
			b.picker.Open()
		}
		return dialogBodyResult{handled: true}
	case chrome.FormSubmitMsg:
		switch message.ID {
		case saveConfirmAction:
			b.sync()
			return dialogBodyResult{
				message: saveDocumentMsg{
					Directory: b.directory,
					Name:      b.name,
				},
				handled: true,
			}
		case saveCancelAction:
			return dialogBodyResult{
				message: dialogCancelMsg{},
				handled: true,
			}
		}
		return dialogBodyResult{}
	default:
		form, command := b.form.Update(message)
		b.form = form.(*chrome.Form)
		b.sync()
		return dialogBodyResult{command: command, handled: true}
	}
}

func (b *saveDialogBody) View() string {
	if b.picker.Opened() {
		return b.picker.View().Content
	}
	return b.form.View().Content
}

func (b *saveDialogBody) SetStyles(styles saveStyles) {
	b.styles = styles
	b.form.SetStyles(styles.Form)
	b.picker.SetStyles(styles.Picker)
}

func (b *saveDialogBody) FocusID() chrome.ID {
	return b.form.FocusID()
}

func (b *saveDialogBody) AccessibleLines() []string {
	return b.form.AccessibleLines()
}

func (b *saveDialogBody) PickerOpen() bool {
	return b.picker.Opened()
}

func (b *saveDialogBody) SetValue(directory, name string) {
	b.Reset(directory)
	b.name = name
	b.form = b.newForm(directory, name)
	b.SetBounds(b.bounds)
}

func (b *saveDialogBody) updatePicker(message tea.Msg) dialogBodyResult {
	picker, command := b.picker.Update(message)
	b.picker = picker.(*directorypicker.Model)
	b.directory = b.picker.Value()
	if !b.picker.Opened() {
		b.closePicker()
	}
	return dialogBodyResult{command: command, handled: true}
}

func (b *saveDialogBody) closePicker() {
	b.picker.Close()
	b.directory = b.picker.Value()
	b.form.SetDirectory(saveDirectoryField, b.directory)
}

func (b *saveDialogBody) sync() {
	b.directory, _ = b.form.Directory(saveDirectoryField)
	b.name, _ = b.form.Text(saveNameField)
}

func (m *Model) requestSave() {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	if m.path == "" {
		m.dialogs.OpenSave(m.preferenceValue().SaveDirectory)
		m.status = ""
		return
	}
	m.save(m.path)
}

func (m *Model) saveFromDialog(message saveDocumentMsg) {
	name := strings.TrimSpace(message.Name)
	if name == "" {
		m.setError("enter a file name")
		return
	}
	if filepath.Base(name) != name {
		m.setError("file name must not contain a directory")
		return
	}
	path := filepath.Join(message.Directory, name)
	if info, err := os.Stat(message.Directory); err == nil && !info.IsDir() {
		path = message.Directory
		if name != defaultSaveName {
			path = filepath.Join(filepath.Dir(path), name)
		}
	}
	if !m.save(path) {
		return
	}
	m.dialogs.CloseWithoutMessage()
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
		if err := m.history.Save(path); err != nil {
			m.status += fmt.Sprintf(" (undo history: %v)", err)
			m.statusError = m.status
		}
	}
	return true
}
