package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	modalview "github.com/coxley/dg/internal/tui/modal"
	"github.com/google/uuid"
)

const (
	confirmationAccept chrome.ID = "confirmation-accept"
	confirmationCancel chrome.ID = "confirmation-cancel"
)

type clearDraftsMsg struct{}

type confirmationDialogBody struct {
	confirmation *modalview.Confirmation
	styles       modalview.ConfirmationStyles
	message      tea.Msg
	bounds       chrome.Rect
}

func newConfirmationDialogBody(styles modalview.ConfirmationStyles) *confirmationDialogBody {
	return &confirmationDialogBody{styles: styles}
}

func (b *confirmationDialogBody) Reset(title, message, confirm string, result tea.Msg) {
	b.message = result
	b.confirmation = modalview.NewConfirmation(
		modalview.ConfirmationDeclaration{
			ID:      "confirmation",
			Title:   title,
			Message: message,
			Confirm: chrome.Button{ID: confirmationAccept, Label: confirm},
			Cancel:  chrome.Button{ID: confirmationCancel, Label: "Cancel"},
		},
		b.styles,
	)
	b.confirmation.SetBounds(b.bounds)
}

func (*confirmationDialogBody) Context() string     { return "confirmation" }
func (*confirmationDialogBody) PreferredWidth() int { return 44 }
func (*confirmationDialogBody) Scopes() []chrome.ScopeID {
	return []chrome.ScopeID{scopeModal, scopeGlobal}
}
func (*confirmationDialogBody) TextEntry() bool { return false }

func (b *confirmationDialogBody) SetBounds(bounds chrome.Rect) {
	if bounds.Height == 0 {
		bounds.Height = 8
	}
	b.bounds = bounds
	if b.confirmation != nil {
		b.confirmation.SetBounds(bounds)
	}
}

func (b *confirmationDialogBody) Update(message tea.Msg) dialogBodyResult {
	if b.confirmation == nil {
		return dialogBodyResult{}
	}
	switch message := message.(type) {
	case dialogClickMsg:
		return dialogBodyResult{command: b.confirmation.Click(message.Point), handled: true}
	case dialogBackMsg:
		return dialogBodyResult{message: dialogCancelMsg{}, handled: true}
	case dialogCloseMsg:
		return dialogBodyResult{handled: true}
	case chrome.FormSubmitMsg:
		if message.ID == confirmationAccept {
			return dialogBodyResult{message: b.message, handled: true}
		}
		return dialogBodyResult{message: dialogCancelMsg{}, handled: true}
	}
	updated, command := b.confirmation.Update(message)
	b.confirmation = updated.(*modalview.Confirmation)
	return dialogBodyResult{command: command, handled: true}
}

func (b *confirmationDialogBody) View() string {
	if b.confirmation == nil {
		return ""
	}
	return b.confirmation.View().Content
}

func (b *confirmationDialogBody) SetStyles(styles modalview.ConfirmationStyles) {
	b.styles = styles
	if b.confirmation != nil {
		b.confirmation.SetStyles(styles)
	}
}

func (m *Model) openClearDraftsConfirmation() {
	count := 0
	for _, entry := range m.catalog {
		if entry.Draft && (m.entry == nil || !sameCanvas(*m.entry, entry)) {
			count++
		}
	}
	if count == 0 {
		m.status = "no other drafts to delete"
		return
	}
	m.dialogs.OpenConfirmation(
		"Clear Drafts",
		fmt.Sprintf("Delete %d canvases?", count),
		"Delete",
		clearDraftsMsg{},
	)
}

func (m *Model) clearDrafts() {
	preserve := uuid.Nil
	if m.entry != nil && m.entry.Draft {
		preserve = m.entry.ID
	}
	count, err := m.canvasStore.ClearDrafts(preserve)
	if err != nil {
		m.setError(fmt.Sprintf("clear drafts: %v", err))
		return
	}
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.dialogs.CloseWithoutMessage()
	m.status = fmt.Sprintf("deleted %d canvases", count)
	m.statusError = ""
}
