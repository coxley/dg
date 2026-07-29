package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
)

const (
	scopeCanvas      chrome.ScopeID = "canvas"
	scopeGlobal      chrome.ScopeID = "global"
	scopeModal       chrome.ScopeID = "modal"
	scopePreferences chrome.ScopeID = "preferences"
	scopeDirectory   chrome.ScopeID = "directory"

	commandBack        chrome.CommandID = "back"
	commandHelp        chrome.CommandID = "help"
	commandPreferences chrome.CommandID = "preferences"
	commandSave        chrome.CommandID = "save"
)

var applicationBindings = []chrome.Binding{
	{Scope: scopeDirectory, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "close picker"},
	{Scope: scopePreferences, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "cancel preferences"},
	{Scope: scopeModal, Chords: chrome.Keys("esc"), Command: commandBack, Label: "close"},
	{Scope: scopeGlobal, Chords: chrome.Keys("?"), Command: commandHelp, Label: "toggle help"},
	{Scope: scopeGlobal, Chords: []chrome.Chord{chrome.Primary(",")}, Command: commandPreferences, Label: "preferences"},
	{Scope: scopeGlobal, Chords: chrome.Keys("ctrl+s"), Command: commandSave, Label: "save"},
}

func (m *Model) activeBindingScopes() []chrome.ScopeID {
	if m.modal == modalPreferences {
		if m.preferenceForm != nil && m.preferenceForm.DirectoryOpen() {
			return []chrome.ScopeID{scopeDirectory, scopePreferences, scopeGlobal}
		}
		return []chrome.ScopeID{scopePreferences, scopeGlobal}
	}
	if m.modal != modalNone {
		return []chrome.ScopeID{scopeModal, scopeGlobal}
	}
	return []chrome.ScopeID{scopeCanvas, scopeGlobal}
}

func (m *Model) updateSemanticCommand(message chrome.CommandMsg) tea.Cmd {
	switch message.Command {
	case commandBack:
		if m.modal == modalPreferences &&
			m.preferenceForm != nil &&
			m.preferenceForm.DirectoryOpen() {
			return m.updateSettingsTabs(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		}
		if id, ok := m.workspace.Back(); ok {
			m.dismissSurface(id)
		} else if m.modal != modalNone {
			m.closeModal()
		}
	case commandHelp:
		m.openHelp()
	case commandPreferences:
		m.openPreferences()
	case commandSave:
		switch m.modal {
		case modalSave:
			m.commitSaveForm()
		case modalNone:
			m.requestSave()
		case modalPreferences, modalExport, modalNotice:
		}
	}
	return nil
}
