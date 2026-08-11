package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	canvasstore "github.com/coxley/dg/store"
)

const (
	saveSectionField  chrome.ID = "save-section"
	saveNameField     chrome.ID = "save-name"
	saveConfirmAction chrome.ID = "save-confirm"
	saveCancelAction  chrome.ID = "save-cancel"
	saveTextWidth               = 24
)

type saveDocumentMsg struct {
	Section string
	Name    string
}

type saveDialogBody struct {
	form     *chrome.Form
	section  string
	name     string
	styles   saveStyles
	bounds   chrome.Rect
	renaming bool
}

func newSaveDialogBody(styles saveStyles) *saveDialogBody {
	body := &saveDialogBody{styles: styles}
	body.Reset()
	return body
}

func (b *saveDialogBody) Reset() {
	b.section = ""
	b.name = ""
	b.renaming = false
	b.form = b.newForm()
	b.SetBounds(b.bounds)
}

func (b *saveDialogBody) newForm() *chrome.Form {
	confirmLabel := "Name Canvas"
	if b.renaming {
		confirmLabel = "Rename Canvas"
	}
	return chrome.NewForm(chrome.FormDeclaration{
		DefaultAction: saveConfirmAction,
		Fields: []chrome.FormField{
			{
				ID: saveNameField, Label: "Canvas name", Kind: chrome.TextField,
				Text: b.name, Placeholder: "Canvas title", TextWidth: saveTextWidth,
			},
			{
				ID: saveSectionField, Label: "Section (optional)", Kind: chrome.TextField,
				Text: b.section, Placeholder: "Section name", TextWidth: saveTextWidth,
			},
		},
		Spacer: chrome.FormSpacer{ID: "save-spacer", Grow: 1},
		Actions: chrome.ButtonListDeclaration{
			ID: "save-actions",
			Buttons: []chrome.Button{
				{ID: saveConfirmAction, Label: confirmLabel},
				{ID: saveCancelAction, Label: "Cancel"},
			},
		},
	}, b.styles.Form)
}

func (*saveDialogBody) Context() string     { return "name canvas" }
func (*saveDialogBody) PreferredWidth() int { return 52 }
func (*saveDialogBody) Scopes() []chrome.ScopeID {
	return []chrome.ScopeID{scopeModal, scopeGlobal}
}
func (*saveDialogBody) TextEntry() bool { return true }

func (b *saveDialogBody) SetBounds(bounds chrome.Rect) {
	b.bounds = bounds
	b.form.SetBounds(bounds)
}

func (b *saveDialogBody) Update(message tea.Msg) dialogBodyResult {
	switch message := message.(type) {
	case dialogClickMsg:
		return dialogBodyResult{command: b.form.Click(message.Point), handled: true}
	case dialogBackMsg:
		return dialogBodyResult{}
	case dialogCloseMsg:
		return dialogBodyResult{handled: true}
	case chrome.FormSubmitMsg:
		switch message.ID {
		case saveCancelAction:
			return dialogBodyResult{message: dialogCancelMsg{}, handled: true}
		case saveConfirmAction:
			b.sync()
			return dialogBodyResult{
				message: saveDocumentMsg{Section: b.section, Name: b.name},
				handled: true,
			}
		}
	}
	updated, command := b.form.Update(message)
	b.form = updated.(*chrome.Form)
	b.sync()
	return dialogBodyResult{command: command, handled: true}
}

func (b *saveDialogBody) View() string { return b.form.View().Content }

func (b *saveDialogBody) SetStyles(styles saveStyles) {
	b.styles = styles
	b.form.SetStyles(styles.Form)
}

func (b *saveDialogBody) SetValue(section, name string) {
	b.section = section
	b.name = name
	b.form = b.newForm()
	b.SetBounds(b.bounds)
}

func (b *saveDialogBody) SetRenaming(renaming bool) {
	b.renaming = renaming
	b.form = b.newForm()
	b.SetBounds(b.bounds)
}

func (b *saveDialogBody) sync() {
	b.section, _ = b.form.Text(saveSectionField)
	b.name, _ = b.form.Text(saveNameField)
}

func (m *Model) requestSave() {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	if m.canvasStore == nil {
		m.setError("no canvas store configured")
		return
	}
	m.dialogs.OpenSave()
	if m.entry != nil && m.entry.Name != "" {
		m.dialogs.save.SetValue(m.entry.Section, m.entry.Name)
		m.dialogs.save.SetRenaming(!m.entry.Draft)
	}
	m.status = ""
}

func (m *Model) saveFromDialog(message saveDocumentMsg) {
	name := strings.TrimSpace(message.Name)
	section := strings.TrimSpace(message.Section)
	if name == "" {
		m.setError("enter a canvas name")
		return
	}
	if m.entry == nil {
		if err := m.persistActive(); err != nil {
			m.setError(err.Error())
			return
		}
	}
	if m.dirty != m.saved {
		if err := m.persistActive(); err != nil {
			m.setError(err.Error())
			return
		}
	}
	previous := *m.entry
	var entry canvasstore.Entry
	var err error
	if previous.Draft {
		entry, err = m.canvasStore.Name(previous, section, name)
	} else if previous.Section == section && previous.Name == name {
		entry = previous
	} else {
		entry, err = m.canvasStore.Move(previous, section, name)
	}
	if err != nil {
		if errors.Is(err, canvasstore.ErrEntryExists) {
			m.setError("a canvas with that name already exists")
			return
		}
		m.setError(fmt.Sprintf("name canvas: %v", err))
		return
	}
	m.setActiveEntry(entry)
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	verb := "named "
	if !previous.Draft {
		verb = "renamed "
	}
	m.status = verb + canvasTitle(entry)
	m.statusError = ""
	m.dialogs.CloseWithoutMessage()
}
