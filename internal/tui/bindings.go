package tui

import (
	"runtime"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
)

const (
	scopeCanvas      chrome.ScopeID = "canvas"
	scopeGlobal      chrome.ScopeID = "global"
	scopeLabel       chrome.ScopeID = "label"
	scopeModal       chrome.ScopeID = "modal"
	scopePreferences chrome.ScopeID = "preferences"
	scopeDirectory   chrome.ScopeID = "directory"
	scopeSidebar     chrome.ScopeID = "sidebar"

	commandActivate        chrome.CommandID = "activate"
	commandArrowEnd        chrome.CommandID = "arrow-end"
	commandArrowStart      chrome.CommandID = "arrow-start"
	commandBack            chrome.CommandID = "back"
	commandBorder          chrome.CommandID = "border"
	commandCancel          chrome.CommandID = "cancel"
	commandCopy            chrome.CommandID = "copy"
	commandDashed          chrome.CommandID = "dashed"
	commandDelete          chrome.CommandID = "delete"
	commandDuplicate       chrome.CommandID = "duplicate"
	commandEditLabel       chrome.CommandID = "edit-label"
	commandExpand          chrome.CommandID = "expand-selection"
	commandFocusNext       chrome.CommandID = "focus-next"
	commandFocusPrevious   chrome.CommandID = "focus-previous"
	commandHelp            chrome.CommandID = "help"
	commandLayerBack       chrome.CommandID = "layer-back"
	commandLayerBackward   chrome.CommandID = "layer-backward"
	commandLayerForward    chrome.CommandID = "layer-forward"
	commandLayerFront      chrome.CommandID = "layer-front"
	commandLine            chrome.CommandID = "line"
	commandMoveDown        chrome.CommandID = "move-down"
	commandMoveLeft        chrome.CommandID = "move-left"
	commandMoveRight       chrome.CommandID = "move-right"
	commandMoveUp          chrome.CommandID = "move-up"
	commandNewCanvas       chrome.CommandID = "new-canvas"
	commandNewNode         chrome.CommandID = "new-node"
	commandPadding         chrome.CommandID = "padding"
	commandPreferences     chrome.CommandID = "preferences"
	commandQuit            chrome.CommandID = "quit"
	commandRectangle       chrome.CommandID = "rectangle"
	commandRedo            chrome.CommandID = "redo"
	commandSave            chrome.CommandID = "save"
	commandSidebar         chrome.CommandID = "sidebar"
	commandSidebarNext     chrome.CommandID = "sidebar-next"
	commandSidebarPrevious chrome.CommandID = "sidebar-previous"
	commandSidebarActivate chrome.CommandID = "sidebar-activate"
	commandSidebarTabNext  chrome.CommandID = "sidebar-tab-next"
	commandSidebarTabPrev  chrome.CommandID = "sidebar-tab-previous"
	commandSidebarDelete   chrome.CommandID = "sidebar-delete"
	commandTextHorizontal  chrome.CommandID = "text-horizontal"
	commandTextVertical    chrome.CommandID = "text-vertical"
	commandUndo            chrome.CommandID = "undo"
)

var applicationBindings = []chrome.Binding{
	{Scope: scopeSidebar, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "return to canvas"},
	{Scope: scopeSidebar, Chords: chrome.Keys("tab", "down", "j"), Command: commandSidebarNext, Label: "next item"},
	{Scope: scopeSidebar, Chords: chrome.Keys("shift+tab", "up", "k"), Command: commandSidebarPrevious, Label: "previous item"},
	{Scope: scopeSidebar, Chords: chrome.Keys("enter"), Command: commandSidebarActivate, Label: "open item"},
	{Scope: scopeSidebar, Chords: chrome.Keys("right", "l"), Command: commandSidebarTabNext, Label: "next tab"},
	{Scope: scopeSidebar, Chords: chrome.Keys("left", "h"), Command: commandSidebarTabPrev, Label: "previous tab"},
	{Scope: scopeSidebar, Chords: chrome.Keys("backspace", "delete"), Command: commandSidebarDelete, Label: "delete canvas"},
	{Scope: scopeSidebar, Chords: chrome.Keys("ctrl+n"), Command: commandNewCanvas, Label: "new canvas"},
	{Scope: scopeDirectory, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "close picker"},
	{Scope: scopePreferences, Chords: chrome.Keys("esc", "q"), Command: commandBack, Label: "cancel preferences"},
	{Scope: scopeModal, Chords: chrome.Keys("esc"), Command: commandBack, Label: "close"},
	{Scope: scopeLabel, Chords: chrome.Keys("esc", "ctrl+enter"), Command: commandCancel, Label: "finish label"},
	{Scope: scopeCanvas, Chords: chrome.Keys("up"), Command: commandMoveUp, Label: "move up"},
	{Scope: scopeCanvas, Chords: chrome.Keys("right"), Command: commandMoveRight, Label: "move right"},
	{Scope: scopeCanvas, Chords: chrome.Keys("down"), Command: commandMoveDown, Label: "move down"},
	{Scope: scopeCanvas, Chords: chrome.Keys("left"), Command: commandMoveLeft, Label: "move left"},
	{Scope: scopeCanvas, Chords: chrome.Keys("tab"), Command: commandFocusNext, Label: "next node"},
	{Scope: scopeCanvas, Chords: chrome.Keys("shift+tab"), Command: commandFocusPrevious, Label: "previous node"},
	{Scope: scopeCanvas, Chords: chrome.Keys("enter"), Command: commandActivate, Label: "complete connection"},
	{Scope: scopeCanvas, Chords: chrome.Keys("e"), Command: commandEditLabel, Label: "edit label"},
	{Scope: scopeCanvas, Chords: chrome.Keys("n"), Command: commandNewNode, Label: "new node"},
	{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+n"), Command: commandNewCanvas, Label: "new canvas"},
	{Scope: scopeCanvas, Chords: chrome.Keys("r"), Command: commandRectangle, Label: string(commandRectangle)},
	{Scope: scopeCanvas, Chords: chrome.Keys("l"), Command: commandLine, Label: "line"},
	{Scope: scopeCanvas, Chords: chrome.Keys("b"), Command: commandBorder, Label: "border"},
	{Scope: scopeCanvas, Chords: chrome.Keys("p"), Command: commandPadding, Label: "padding"},
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
	{Scope: scopeCanvas, Chords: primaryKeys("a"), Command: commandExpand, Label: "expand selection"},
	{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+c", "super+c"), Command: commandCopy, Label: "copy"},
	{Scope: scopeCanvas, Chords: chrome.Keys("esc"), Command: commandCancel, Label: "cancel tool"},
	{Scope: scopeCanvas, Chords: chrome.Keys("q"), Command: commandQuit, Label: "cursor / quit"},
	{Scope: scopeGlobal, Chords: chrome.Keys("?"), Command: commandHelp, Label: "toggle help"},
	{Scope: scopeGlobal, Chords: primaryKeys("p"), Command: commandPreferences, Label: string(scopePreferences)},
	{Scope: scopeGlobal, Chords: primaryKeys("s"), Command: commandSave, Label: string(commandSave)},
	{Scope: scopeGlobal, Chords: primaryKeys("b"), Command: commandSidebar, Label: "toggle sidebar"},
}

func primaryKeys(key string) []chrome.Chord {
	control := chrome.NormalizeChord("ctrl+" + key)
	command := chrome.NormalizeChord("super+" + key)
	if runtime.GOOS == "darwin" {
		return []chrome.Chord{command, control}
	}
	return []chrome.Chord{control, command}
}

var (
	canvasBindingScopes  = [...]chrome.ScopeID{scopeCanvas, scopeGlobal}
	labelBindingScopes   = [...]chrome.ScopeID{scopeLabel, scopeGlobal}
	sidebarBindingScopes = [...]chrome.ScopeID{scopeSidebar, scopeGlobal}
)

func (m *Model) activeBindingScopes() []chrome.ScopeID {
	if m.dialogs.ActiveID() != surfaceNone {
		return m.dialogs.Scopes()
	}
	if m.sidebar.focused {
		return sidebarBindingScopes[:]
	}
	if m.interaction.session.kind == sessionLabelEdit {
		return labelBindingScopes[:]
	}
	return canvasBindingScopes[:]
}

func (m *Model) updateSemanticCommand(message chrome.CommandMsg) tea.Cmd {
	switch message.Command {
	case commandActivate,
		commandCancel,
		commandFocusNext,
		commandFocusPrevious,
		commandLine,
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
		commandPadding,
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
		commandNewCanvas,
		commandPreferences,
		commandQuit,
		commandSave,
		commandSidebar,
		commandSidebarActivate,
		commandSidebarDelete,
		commandSidebarNext,
		commandSidebarPrevious,
		commandSidebarTabNext,
		commandSidebarTabPrev:
		return m.updateChromeCommand(message.Command)
	default:
		panic("unhandled semantic command " + message.Command)
	}
	return nil
}

func (m *Model) updateMovementCommand(command chrome.CommandID) {
	switch command {
	case commandActivate:
		return
	case commandCancel:
		m.cancelMode()
	case commandFocusNext:
		m.focusNode(1)
	case commandFocusPrevious:
		m.focusNode(-1)
	case commandLine:
		m.activateTool(modeConnect)
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

func (m *Model) contextualHelpBindings() []chrome.EffectiveBinding {
	scopes := m.activeBindingScopes()
	bindings := m.bindings.Effective(scopes)
	context := m.helpContext()
	if context != string(surfaceCanvas) && context != "label editor" {
		return bindings
	}
	return slices.DeleteFunc(bindings, func(binding chrome.EffectiveBinding) bool {
		if _, ok := m.bindings.Resolve(
			string(binding.Chord),
			scopes,
			m.textEntryActive(),
		); !ok {
			return true
		}
		return !m.canvasCommandAvailable(binding.Command)
	})
}

func (m *Model) canvasCommandAvailable(command chrome.CommandID) bool {
	if !m.interaction.idle() {
		return m.interactionCommandAvailable(command)
	}
	switch command {
	case commandActivate,
		commandCancel,
		commandFocusNext,
		commandFocusPrevious,
		commandLine,
		commandMoveDown,
		commandMoveLeft,
		commandMoveRight,
		commandMoveUp,
		commandNewNode,
		commandRectangle:
		return m.canvasMovementCommandAvailable(command)
	case commandArrowEnd,
		commandArrowStart,
		commandBorder,
		commandDashed,
		commandLayerBack,
		commandLayerBackward,
		commandLayerForward,
		commandLayerFront,
		commandPadding,
		commandTextHorizontal,
		commandTextVertical:
		return m.canvasAppearanceCommandAvailable(command)
	case commandCopy,
		commandDelete,
		commandDuplicate,
		commandEditLabel,
		commandExpand,
		commandRedo,
		commandUndo:
		return m.canvasEditCommandAvailable(command)
	case commandNewCanvas, commandSave:
		return m.canvasStore != nil
	default:
		return true
	}
}

func (m *Model) canvasMovementCommandAvailable(command chrome.CommandID) bool {
	nodes, _ := m.selectedCounts()
	switch command {
	case commandActivate, commandCancel:
		return false
	case commandMoveUp, commandMoveRight, commandMoveDown, commandMoveLeft:
		return nodes != 0
	case commandFocusNext, commandFocusPrevious:
		return m.nodeFocusCommandAvailable()
	default:
		return true
	}
}

func (m *Model) canvasAppearanceCommandAvailable(command chrome.CommandID) bool {
	nodes, edges := m.selectedCounts()
	hit, hasHit := m.activeHit()
	hasSelection := nodes != 0 || edges != 0
	hasNode := nodes != 0 || hasHit && hit.Kind == layout.HitNode
	switch command {
	case commandArrowEnd, commandArrowStart:
		return edges != 0
	case commandBorder, commandPadding, commandTextHorizontal, commandTextVertical:
		return hasNode
	case commandDashed:
		return hasSelection || hasHit &&
			(hit.Kind == layout.HitNode || hit.Kind == layout.HitEdge)
	case commandLayerBack, commandLayerBackward, commandLayerForward, commandLayerFront:
		return hasSelection && m.layerCommandAvailable(command)
	default:
		return false
	}
}

func (m *Model) canvasEditCommandAvailable(command chrome.CommandID) bool {
	nodes, edges := m.selectedCounts()
	hit, hasHit := m.activeHit()
	hasSelection := nodes != 0 || edges != 0
	hasNode := nodes != 0 || hasHit && hit.Kind == layout.HitNode
	switch command {
	case commandDelete:
		return hasSelection || hasHit && hit.Kind != layout.HitPort
	case commandDuplicate:
		if nodes != 0 {
			return m.geo.SelectionMovesRigidly()
		}
		return hasHit &&
			hit.Kind == layout.HitNode &&
			!m.nodeHasIncidentEdge(hit.ID)
	case commandEditLabel:
		return hasNode
	case commandExpand:
		if hasSelection {
			return nodes+edges < m.liveObjectCount()
		}
		return hasHit && hit.Kind != layout.HitPort
	case commandCopy:
		return hasSelection
	case commandUndo:
		return m.history != nil && m.history.CanUndo()
	case commandRedo:
		return m.history != nil && m.history.CanRedo()
	default:
		return false
	}
}

func (m *Model) nodeHasIncidentEdge(nodeID uint32) bool {
	for edgeID := range m.geo.Edges {
		id := uint32(edgeID)
		if !m.geo.EdgeExists(id) {
			continue
		}
		nodeA, nodeB, err := m.geo.EdgeNodes(id)
		if err != nil || nodeA == nodeID || nodeB == nodeID {
			return true
		}
	}
	return false
}

func (m *Model) interactionCommandAvailable(command chrome.CommandID) bool {
	switch command {
	case commandCancel:
		return m.cancelCommandRelevant()
	case commandHelp, commandQuit:
		return true
	}
	if m.interaction.gesture.kind != gestureNone {
		return false
	}
	switch m.interaction.tool {
	case toolRectangle:
		return command == commandLine
	case toolConnect:
		if command == commandRectangle {
			return true
		}
		if m.interaction.session.kind != sessionConnection {
			if !m.lineToolEdgeEditReady() {
				return false
			}
			switch command {
			case commandArrowEnd, commandArrowStart, commandDashed:
				return m.canvasAppearanceCommandAvailable(command)
			case commandDelete:
				return m.canvasEditCommandAvailable(command)
			default:
				return false
			}
		}
		switch command {
		case commandActivate:
			hit, ok := m.activeHit()
			return ok &&
				hit.Kind == layout.HitPort &&
				hit.ID != m.interaction.session.connection.source &&
				m.geo.PortUsable(hit.ID)
		case commandMoveUp,
			commandMoveRight,
			commandMoveDown,
			commandMoveLeft:
			return true
		default:
			return false
		}
	case toolNavigate:
		return false
	default:
		return false
	}
}

func (m *Model) cancelCommandRelevant() bool {
	if m.interaction.tool != toolNavigate ||
		m.interaction.session.kind != sessionNone {
		return true
	}
	switch m.interaction.gesture.kind {
	case gestureRectangle,
		gestureDuplicatePending,
		gestureDuplicate,
		gestureAreaSelection,
		gestureConnectionPending,
		gestureConnection:
		return true
	default:
		return false
	}
}

func (m *Model) layerCommandAvailable(command chrome.CommandID) bool {
	hit, ok := m.selectedLayer()
	if !ok {
		return false
	}
	order := slices.Collect(m.geo.DrawOrder())
	index := slices.Index(order, hit)
	switch command {
	case commandLayerBack, commandLayerBackward:
		return index > 0
	case commandLayerForward, commandLayerFront:
		return index >= 0 && index+1 < len(order)
	default:
		return false
	}
}

func (m *Model) nodeFocusCommandAvailable() bool {
	count := 0
	for nodeID := range m.geo.Nodes {
		if m.geo.NodeExists(uint32(nodeID)) {
			count++
		}
	}
	_, focused := m.focusedNode()
	return count > 1 || count == 1 && !focused
}

func (m *Model) liveObjectCount() int {
	return len(slices.Collect(m.geo.DrawOrder()))
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
	case commandPadding:
		m.cyclePadding()
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
		if m.dialogs.ActiveID() == surfaceNone && m.interaction.idle() {
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
		if m.dialogs.ActiveID() != surfaceNone {
			if result := m.dialogs.Back(); result.handled {
				return m.handleDialogResult(result)
			}
		}
		if id, ok := m.workspace.Back(); ok {
			return m.dismissSurface(id)
		}
	case commandHelp:
		m.openHelp()
	case commandNewCanvas:
		m.newCanvas()
	case commandPreferences:
		if m.dialogs.ActiveID() == surfacePreferences {
			return m.dismissDialog()
		}
		m.openPreferences()
	case commandQuit:
		if m.cancelCommandRelevant() {
			m.cancelMode()
			return nil
		}
		m.interruptInteraction()
		return m.handleFlushRequest(flushRequestMsg{quit: true})
	case commandSave:
		switch m.dialogs.ActiveID() {
		case surfaceSave:
			return m.handleDialogResult(m.dialogs.SubmitSave())
		case surfacePreferences:
			return m.handleDialogResult(m.dialogs.SubmitPreferences())
		case surfaceNone:
			m.requestSave()
		}
	case commandSidebar:
		return m.toggleSidebar()
	case commandSidebarNext:
		m.sidebar.moveFocus(1)
	case commandSidebarPrevious:
		m.sidebar.moveFocus(-1)
	case commandSidebarActivate:
		return m.activateSidebar()
	case commandSidebarTabNext:
		return m.switchSidebarTab(1)
	case commandSidebarTabPrev:
		return m.switchSidebarTab(-1)
	case commandSidebarDelete:
		m.deleteFocusedCanvas()
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
