package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
)

const (
	scopeCanvas chrome.ScopeID = "canvas"
	scopeGlobal chrome.ScopeID = "global"

	commandHelp chrome.CommandID = "help"
	commandSave chrome.CommandID = "save"
)

var applicationBindings = []chrome.Binding{
	{Scope: scopeCanvas, Chords: chrome.Keys("?"), Command: commandHelp, Label: "help"},
	{Scope: scopeGlobal, Chords: chrome.Keys("ctrl+s"), Command: commandSave, Label: "save"},
}

func (m *Model) activeBindingScopes() []chrome.ScopeID {
	if m.modal != modalNone {
		return nil
	}
	return []chrome.ScopeID{scopeCanvas, scopeGlobal}
}

func (m *Model) updateSemanticCommand(message chrome.CommandMsg) tea.Cmd {
	switch message.Command {
	case commandHelp:
		m.openHelp()
	case commandSave:
		m.requestSave()
	}
	return nil
}
