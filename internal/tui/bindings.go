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
	{Scope: scopeGlobal, Chords: []chrome.Chord{chrome.Primary(",")}, Command: commandPreferences, Label: string(scopePreferences)},
	{Scope: scopeGlobal, Chords: chrome.Keys("ctrl+s"), Command: commandSave, Label: string(commandSave)},
}

func (m *Model) activeBindingScopes() []chrome.ScopeID {
	if spec, ok := m.activeDialogSpec(); ok {
		return spec.Scopes(m)
	}
	return []chrome.ScopeID{scopeCanvas, scopeGlobal}
}

func (m *Model) updateSemanticCommand(message chrome.CommandMsg) tea.Cmd {
	switch message.Command {
	case commandBack:
		if spec, ok := m.activeDialogSpec(); ok && spec.Back != nil {
			if command, handled := spec.Back(m); handled {
				return command
			}
		}
		if id, ok := m.workspace.Back(); ok {
			m.dismissSurface(id)
		}
	case commandHelp:
		m.openHelp()
	case commandPreferences:
		m.openPreferences()
	case commandSave:
		switch m.activeDialog {
		case surfaceSave:
			m.commitSaveForm()
		case surfaceNone:
			m.requestSave()
		}
	}
	return nil
}
