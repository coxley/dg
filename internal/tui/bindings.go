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
	scopeSidebar     chrome.ScopeID = "sidebar"

	commandBack        chrome.CommandID = "back"
	commandHelp        chrome.CommandID = "help"
	commandPreferences chrome.CommandID = "preferences"
	commandSave        chrome.CommandID = "save"
	commandSidebar     chrome.CommandID = "sidebar"
)

var applicationBindings = []chrome.Binding{
	{Scope: scopeSidebar, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "return to canvas"},
	{Scope: scopeDirectory, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "close picker"},
	{Scope: scopePreferences, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "cancel preferences"},
	{Scope: scopeModal, Chords: chrome.Keys("esc"), Command: commandBack, Label: "close"},
	{Scope: scopeGlobal, Chords: chrome.Keys("?"), Command: commandHelp, Label: "toggle help"},
	{Scope: scopeGlobal, Chords: []chrome.Chord{chrome.Primary(",")}, Command: commandPreferences, Label: string(scopePreferences)},
	{Scope: scopeGlobal, Chords: chrome.Keys("ctrl+s"), Command: commandSave, Label: string(commandSave)},
	{Scope: scopeGlobal, Chords: chrome.Keys("ctrl+b"), Command: commandSidebar, Label: "toggle sidebar"},
}

var (
	canvasBindingScopes  = [...]chrome.ScopeID{scopeCanvas, scopeGlobal}
	sidebarBindingScopes = [...]chrome.ScopeID{scopeSidebar, scopeGlobal}
)

func (m *Model) activeBindingScopes() []chrome.ScopeID {
	if spec, ok := m.activeDialogSpec(); ok {
		return spec.Scopes(m)
	}
	if m.sidebar.focused {
		return sidebarBindingScopes[:]
	}
	return canvasBindingScopes[:]
}

func (m *Model) updateSemanticCommand(message chrome.CommandMsg) tea.Cmd {
	switch message.Command {
	case commandBack:
		if m.sidebar.focused && m.sidebar.placement == sidebarDocked {
			m.sidebar.blur()
			return nil
		}
		if spec, ok := m.activeDialogSpec(); ok && spec.Back != nil {
			if command, handled := spec.Back(m); handled {
				return command
			}
		}
		if id, ok := m.workspace.Back(); ok {
			return m.dismissSurface(id)
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
	case commandSidebar:
		return m.toggleSidebar()
	}
	return nil
}
