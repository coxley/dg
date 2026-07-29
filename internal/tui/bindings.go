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

	commandActivate         chrome.CommandID = "activate"
	commandArrowEnd         chrome.CommandID = "arrow-end"
	commandArrowStart       chrome.CommandID = "arrow-start"
	commandBack             chrome.CommandID = "back"
	commandBorder           chrome.CommandID = "border"
	commandCancel           chrome.CommandID = "cancel"
	commandCopy             chrome.CommandID = "copy"
	commandCycleHitNext     chrome.CommandID = "cycle-hit-next"
	commandCycleHitPrevious chrome.CommandID = "cycle-hit-previous"
	commandDashed           chrome.CommandID = "dashed"
	commandDelete           chrome.CommandID = "delete"
	commandDuplicate        chrome.CommandID = "duplicate"
	commandEditLabel        chrome.CommandID = "edit-label"
	commandExpand           chrome.CommandID = "expand-selection"
	commandFocusNext        chrome.CommandID = "focus-next"
	commandFocusPrevious    chrome.CommandID = "focus-previous"
	commandHelp             chrome.CommandID = "help"
	commandLayerBack        chrome.CommandID = "layer-back"
	commandLayerBackward    chrome.CommandID = "layer-backward"
	commandLayerForward     chrome.CommandID = "layer-forward"
	commandLayerFront       chrome.CommandID = "layer-front"
	commandLine             chrome.CommandID = "line"
	commandMove             chrome.CommandID = "move"
	commandMoveDown         chrome.CommandID = "move-down"
	commandMoveLeft         chrome.CommandID = "move-left"
	commandMoveRight        chrome.CommandID = "move-right"
	commandMoveUp           chrome.CommandID = "move-up"
	commandNewNode          chrome.CommandID = "new-node"
	commandPreferences      chrome.CommandID = "preferences"
	commandQuit             chrome.CommandID = "quit"
	commandRectangle        chrome.CommandID = "rectangle"
	commandRedo             chrome.CommandID = "redo"
	commandSave             chrome.CommandID = "save"
	commandSidebar          chrome.CommandID = "sidebar"
	commandSidebarNext      chrome.CommandID = "sidebar-next"
	commandSidebarPrevious  chrome.CommandID = "sidebar-previous"
	commandTextHorizontal   chrome.CommandID = "text-horizontal"
	commandTextVertical     chrome.CommandID = "text-vertical"
	commandUndo             chrome.CommandID = "undo"
)

var applicationBindings = []chrome.Binding{
	{Scope: scopeSidebar, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "return to canvas"},
	{Scope: scopeSidebar, Chords: chrome.Keys("tab", "down", "j"), Command: commandSidebarNext, Label: "next item"},
	{Scope: scopeSidebar, Chords: chrome.Keys("shift+tab", "up", "k"), Command: commandSidebarPrevious, Label: "previous item"},
	{Scope: scopeDirectory, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "close picker"},
	{Scope: scopePreferences, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "cancel preferences"},
	{Scope: scopeModal, Chords: chrome.Keys("esc"), Command: commandBack, Label: "close"},
	{Scope: scopeCanvas, Chords: chrome.Keys("up"), Command: commandMoveUp, Label: "move up"},
	{Scope: scopeCanvas, Chords: chrome.Keys("right"), Command: commandMoveRight, Label: "move right"},
	{Scope: scopeCanvas, Chords: chrome.Keys("down"), Command: commandMoveDown, Label: "move down"},
	{Scope: scopeCanvas, Chords: chrome.Keys("left"), Command: commandMoveLeft, Label: "move left"},
	{Scope: scopeCanvas, Chords: chrome.Keys("tab"), Command: commandFocusNext, Label: "next node"},
	{Scope: scopeCanvas, Chords: chrome.Keys("shift+tab"), Command: commandFocusPrevious, Label: "previous node"},
	{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+tab"), Command: commandCycleHitNext, Label: "next hit"},
	{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+shift+tab"), Command: commandCycleHitPrevious, Label: "previous hit"},
	{Scope: scopeCanvas, Chords: chrome.Keys("enter"), Command: commandActivate, Label: "move or connect"},
	{Scope: scopeCanvas, Chords: chrome.Keys("m"), Command: commandMove, Label: "move"},
	{Scope: scopeCanvas, Chords: chrome.Keys("e"), Command: commandEditLabel, Label: "edit label"},
	{Scope: scopeCanvas, Chords: chrome.Keys("n"), Command: commandNewNode, Label: "new node"},
	{Scope: scopeCanvas, Chords: chrome.Keys("r"), Command: commandRectangle, Label: string(commandRectangle)},
	{Scope: scopeCanvas, Chords: chrome.Keys("l"), Command: commandLine, Label: "line"},
	{Scope: scopeCanvas, Chords: chrome.Keys("b"), Command: commandBorder, Label: "border"},
	{Scope: scopeCanvas, Chords: chrome.Keys("-"), Command: commandDashed, Label: "dashed"},
	{Scope: scopeCanvas, Chords: chrome.Keys("a"), Command: commandArrowEnd, Label: "end arrow"},
	{Scope: scopeCanvas, Chords: chrome.Keys("shift+a"), Command: commandArrowStart, Label: "start arrow"},
	{Scope: scopeCanvas, Chords: chrome.Keys("t"), Command: commandTextHorizontal, Label: "horizontal text"},
	{Scope: scopeCanvas, Chords: chrome.Keys("shift+t"), Command: commandTextVertical, Label: "vertical text"},
	{Scope: scopeCanvas, Chords: chrome.Keys("d"), Command: commandDuplicate, Label: "duplicate"},
	{Scope: scopeCanvas, Chords: chrome.Keys("backspace", "delete"), Command: commandDelete, Label: "delete"},
	{Scope: scopeCanvas, Chords: chrome.Keys("["), Command: commandLayerBackward, Label: "send backward"},
	{Scope: scopeCanvas, Chords: chrome.Keys("]"), Command: commandLayerForward, Label: "bring forward"},
	{Scope: scopeCanvas, Chords: chrome.Keys("{", "shift+["), Command: commandLayerBack, Label: "send to back"},
	{Scope: scopeCanvas, Chords: chrome.Keys("}", "shift+]"), Command: commandLayerFront, Label: "bring to front"},
	{Scope: scopeCanvas, Chords: chrome.Keys("u", "ctrl+z"), Command: commandUndo, Label: "undo"},
	{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+r", "ctrl+y", "ctrl+shift+z"), Command: commandRedo, Label: "redo"},
	{Scope: scopeCanvas, Chords: []chrome.Chord{chrome.Primary("a")}, Command: commandExpand, Label: "expand selection"},
	{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+c", "super+c"), Command: commandCopy, Label: "copy"},
	{Scope: scopeCanvas, Chords: chrome.Keys("esc"), Command: commandCancel, Label: "cancel tool"},
	{Scope: scopeCanvas, Chords: chrome.Keys("q"), Command: commandQuit, Label: "quit"},
	{Scope: scopeGlobal, Chords: chrome.Keys("?"), Command: commandHelp, Label: "toggle help"},
	{Scope: scopeGlobal, Chords: []chrome.Chord{chrome.Primary("p")}, Command: commandPreferences, Label: string(scopePreferences)},
	{Scope: scopeGlobal, Chords: []chrome.Chord{chrome.Primary("s")}, Command: commandSave, Label: string(commandSave)},
	{Scope: scopeGlobal, Chords: []chrome.Chord{chrome.Primary("b")}, Command: commandSidebar, Label: "toggle sidebar"},
}

var (
	canvasBindingScopes  = [...]chrome.ScopeID{scopeCanvas, scopeGlobal}
	labelBindingScopes   = [...]chrome.ScopeID{scopeGlobal}
	sidebarBindingScopes = [...]chrome.ScopeID{scopeSidebar, scopeGlobal}
)

func (m *Model) activeBindingScopes() []chrome.ScopeID {
	if spec, ok := m.activeDialogSpec(); ok {
		return spec.Scopes(m)
	}
	if m.sidebar.focused {
		return sidebarBindingScopes[:]
	}
	if m.mode == modeEditLabel {
		return labelBindingScopes[:]
	}
	return canvasBindingScopes[:]
}

func (m *Model) updateSemanticCommand(message chrome.CommandMsg) tea.Cmd {
	switch message.Command {
	case commandActivate,
		commandCancel,
		commandCycleHitNext,
		commandCycleHitPrevious,
		commandFocusNext,
		commandFocusPrevious,
		commandLine,
		commandMove,
		commandMoveDown,
		commandMoveLeft,
		commandMoveRight,
		commandMoveUp,
		commandNewNode,
		commandRectangle:
		m.updateMovementCommand(message.Command)
	case commandArrowEnd,
		commandArrowStart,
		commandBorder,
		commandDashed,
		commandLayerBack,
		commandLayerBackward,
		commandLayerForward,
		commandLayerFront,
		commandTextHorizontal,
		commandTextVertical:
		m.updateAppearanceCommand(message.Command)
	case commandCopy,
		commandDelete,
		commandDuplicate,
		commandEditLabel,
		commandExpand,
		commandRedo,
		commandUndo:
		return m.updateEditCommand(message.Command)
	case commandBack,
		commandHelp,
		commandPreferences,
		commandQuit,
		commandSave,
		commandSidebar,
		commandSidebarNext,
		commandSidebarPrevious:
		return m.updateChromeCommand(message.Command)
	default:
		panic("unhandled semantic command " + message.Command)
	}
	return nil
}

func (m *Model) updateMovementCommand(command chrome.CommandID) {
	switch command {
	case commandActivate:
		if m.mode == modeConnect {
			m.completeConnection()
		} else {
			m.beginMove()
		}
	case commandCancel:
		m.cancelMode()
	case commandCycleHitNext:
		m.cycleHit(1)
	case commandCycleHitPrevious:
		m.cycleHit(-1)
	case commandFocusNext:
		m.focusNode(1)
	case commandFocusPrevious:
		m.focusNode(-1)
	case commandLine:
		m.activateTool(modeConnect)
	case commandMove:
		m.beginMove()
	case commandMoveDown:
		m.move(0, 1)
	case commandMoveLeft:
		m.move(-1, 0)
	case commandMoveRight:
		m.move(1, 0)
	case commandMoveUp:
		m.move(0, -1)
	case commandNewNode:
		m.newNode()
	case commandRectangle:
		m.activateTool(modeRectangle)
	default:
		panic("unhandled movement command " + command)
	}
}

func (m *Model) updateAppearanceCommand(command chrome.CommandID) {
	switch command {
	case commandArrowEnd:
		m.cycleEdgeArrow(false)
	case commandArrowStart:
		m.cycleEdgeArrow(true)
	case commandBorder:
		m.cycleBorder()
	case commandDashed:
		m.toggleStroke()
	case commandLayerBack:
		m.reorderLayer(true, true)
	case commandLayerBackward:
		m.reorderLayer(false, true)
	case commandLayerForward:
		m.reorderLayer(false, false)
	case commandLayerFront:
		m.reorderLayer(true, false)
	case commandTextHorizontal:
		m.cycleTextAlignment(false)
	case commandTextVertical:
		m.cycleTextAlignment(true)
	default:
		panic("unhandled appearance command " + command)
	}
}

func (m *Model) updateEditCommand(command chrome.CommandID) tea.Cmd {
	switch command {
	case commandCopy:
		if m.activeDialog == surfaceNone && m.mode == modeNavigate {
			return m.copySelection()
		}
	case commandDelete:
		m.deleteActive()
	case commandDuplicate:
		m.duplicateSelectionDefault()
	case commandEditLabel:
		m.beginLabelEdit()
	case commandExpand:
		m.expandSelection()
	case commandRedo:
		m.redo()
	case commandUndo:
		m.undo()
	default:
		panic("unhandled edit command " + command)
	}
	return nil
}

func (m *Model) updateChromeCommand(command chrome.CommandID) tea.Cmd {
	switch command {
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
	case commandQuit:
		m.interruptInteraction()
		return tea.Quit
	case commandSave:
		switch m.activeDialog {
		case surfaceSave:
			m.commitSaveForm()
		case surfaceNone:
			m.requestSave()
		}
	case commandSidebar:
		return m.toggleSidebar()
	case commandSidebarNext:
		m.sidebar.moveFocus(1)
	case commandSidebarPrevious:
		m.sidebar.moveFocus(-1)
	default:
		panic("unhandled chrome command " + command)
	}
	return nil
}

func isModifierKey(message tea.KeyPressMsg) bool {
	switch message.Key().Code {
	case tea.KeyLeftShift, tea.KeyRightShift,
		tea.KeyLeftAlt, tea.KeyRightAlt,
		tea.KeyLeftCtrl, tea.KeyRightCtrl,
		tea.KeyLeftMeta, tea.KeyRightMeta,
		tea.KeyLeftHyper, tea.KeyRightHyper,
		tea.KeyLeftSuper, tea.KeyRightSuper:
		return true
	default:
		return false
	}
}
