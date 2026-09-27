package tui

import (
	"runtime"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
)

const (
	labelCommitControlChord = "ctrl+enter"
	labelCommitSuperChord   = "super+enter"
	paletteDefaultChord     = "super+/"

	scopeCanvas      chrome.ScopeID = "canvas"
	scopeGlobal      chrome.ScopeID = "global"
	scopeLabel       chrome.ScopeID = "label"
	scopeModal       chrome.ScopeID = "modal"
	scopePalette     chrome.ScopeID = "palette"
	scopePreferences chrome.ScopeID = "preferences"
	scopeDirectory   chrome.ScopeID = "directory"
	scopeSidebar     chrome.ScopeID = "sidebar"

	commandActivate        chrome.CommandID = "activate"
	commandArrowEnd        chrome.CommandID = "arrow-end"
	commandArrowStart      chrome.CommandID = "arrow-start"
	commandArrange         chrome.CommandID = "arrange"
	commandBack            chrome.CommandID = "back"
	commandBorder          chrome.CommandID = "border"
	commandCancel          chrome.CommandID = "cancel"
	commandCopy            chrome.CommandID = "copy"
	commandCopyCanvasPath  chrome.CommandID = "copy-canvas-path"
	commandCommitLabel     chrome.CommandID = "commit-label"
	commandDashed          chrome.CommandID = "dashed"
	commandDelete          chrome.CommandID = "delete"
	commandDuplicate       chrome.CommandID = "duplicate"
	commandEditLabel       chrome.CommandID = "edit-label"
	commandExpand          chrome.CommandID = "expand-selection"
	commandFocusNext       chrome.CommandID = "focus-next"
	commandFocusPrevious   chrome.CommandID = "focus-previous"
	commandGroup           chrome.CommandID = "group"
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
	commandPalette         chrome.CommandID = "palette"
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

type actionBinding struct {
	Scope  chrome.ScopeID
	Chords []chrome.Chord
	Label  string
}

type action struct {
	Command     chrome.CommandID
	Label       string
	Bindings    []actionBinding
	HidePalette bool
}

var applicationActions = []action{
	{
		Command: commandBack, Label: "Go Back", HidePalette: true,
		Bindings: []actionBinding{
			{Scope: scopeSidebar, Chords: chrome.Keys("esc", "q"), Label: "return to canvas"},
			{Scope: scopeDirectory, Chords: chrome.Keys("esc", "q"), Label: "close picker"},
			{Scope: scopePreferences, Chords: chrome.Keys("esc", "q"), Label: "cancel preferences"},
			{Scope: scopeModal, Chords: chrome.Keys("esc"), Label: "close"},
		},
	},
	{Command: commandSidebarNext, Label: "Next Item", Bindings: []actionBinding{{Scope: scopeSidebar, Chords: chrome.Keys("tab", "down", "j"), Label: "next item"}}},
	{Command: commandSidebarPrevious, Label: "Previous Item", Bindings: []actionBinding{{Scope: scopeSidebar, Chords: chrome.Keys("shift+tab", "up", "k"), Label: "previous item"}}},
	{Command: commandSidebarActivate, Label: "Open Item", Bindings: []actionBinding{{Scope: scopeSidebar, Chords: chrome.Keys("enter"), Label: "open item"}}},
	{Command: commandSidebarTabNext, Label: "Next Tab", Bindings: []actionBinding{{Scope: scopeSidebar, Chords: chrome.Keys("right", "l"), Label: "next tab"}}},
	{Command: commandSidebarTabPrev, Label: "Previous Tab", Bindings: []actionBinding{{Scope: scopeSidebar, Chords: chrome.Keys("left", "h"), Label: "previous tab"}}},
	{Command: commandSidebarDelete, Label: "Delete Canvas", Bindings: []actionBinding{{Scope: scopeSidebar, Chords: chrome.Keys("backspace", "delete"), Label: "delete canvas"}}},
	{Command: commandMoveUp, Label: "Navigate Up", HidePalette: true, Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("up"), Label: "move up"}}},
	{Command: commandMoveRight, Label: "Navigate Right", HidePalette: true, Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("right"), Label: "move right"}}},
	{Command: commandMoveDown, Label: "Navigate Down", HidePalette: true, Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("down"), Label: "move down"}}},
	{Command: commandMoveLeft, Label: "Navigate Left", HidePalette: true, Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("left"), Label: "move left"}}},
	{Command: commandFocusNext, Label: "Focus Next Node", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("tab"), Label: "next node"}}},
	{Command: commandFocusPrevious, Label: "Focus Previous Node", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("shift+tab"), Label: "previous node"}}},
	{Command: commandActivate, Label: "Complete Connection", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("enter"), Label: "complete connection"}}},
	{Command: commandEditLabel, Label: "Edit Label", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("e"), Label: "edit label"}}},
	{Command: commandNewNode, Label: "New Node", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("n"), Label: "new node"}}},
	{
		Command: commandNewCanvas, Label: "New Canvas",
		Bindings: []actionBinding{
			{Scope: scopeSidebar, Chords: chrome.Keys("ctrl+n"), Label: "new canvas"},
			{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+n"), Label: "new canvas"},
		},
	},
	{Command: commandRectangle, Label: "Rectangle Tool", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("r"), Label: string(commandRectangle)}}},
	{Command: commandLine, Label: "Line Tool", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("l"), Label: "line"}}},
	{Command: commandArrange, Label: "Arrange Selection", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("shift+l"), Label: "arrange"}}},
	{Command: commandBorder, Label: "Cycle Border", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("b"), Label: "border"}}},
	{Command: commandPadding, Label: "Cycle Padding", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("p"), Label: "padding"}}},
	{Command: commandDashed, Label: "Toggle Dashed Stroke", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("-"), Label: "dashed"}}},
	{Command: commandArrowEnd, Label: "Cycle End Arrow", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("a"), Label: "end arrow"}}},
	{Command: commandArrowStart, Label: "Cycle Start Arrow", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("shift+a"), Label: "start arrow"}}},
	{Command: commandTextHorizontal, Label: "Cycle Horizontal Alignment", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("t"), Label: "horizontal text"}}},
	{Command: commandTextVertical, Label: "Cycle Vertical Alignment", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("shift+t"), Label: "vertical text"}}},
	{Command: commandDuplicate, Label: "Duplicate Selection", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("d"), Label: "duplicate"}}},
	{Command: commandDelete, Label: "Delete Selection", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("backspace", "delete"), Label: "delete"}}},
	{Command: commandLayerBackward, Label: "Send Backward", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("["), Label: "send backward"}}},
	{Command: commandLayerForward, Label: "Bring Forward", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("]"), Label: "bring forward"}}},
	{Command: commandLayerBack, Label: "Send to Back", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("{", "shift+["), Label: "send to back"}}},
	{Command: commandLayerFront, Label: "Bring to Front", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("}", "shift+]"), Label: "bring to front"}}},
	{Command: commandUndo, Label: "Undo", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("u", "ctrl+z"), Label: "undo"}}},
	{Command: commandRedo, Label: "Redo", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+r", "ctrl+y", "ctrl+shift+z"), Label: "redo"}}},
	{Command: commandExpand, Label: "Expand Selection", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: primaryKeys("a"), Label: "expand selection"}}},
	{Command: commandGroup, Label: "Group / Ungroup", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: primaryKeys("g"), Label: "group / ungroup"}}},
	{Command: commandCopy, Label: "Copy Selection", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("ctrl+c", "super+c"), Label: "copy"}}},
	{
		Command: commandCancel, Label: "Cancel Tool",
		Bindings: []actionBinding{
			{Scope: scopeLabel, Chords: chrome.Keys("esc"), Label: "finish label"},
			{Scope: scopeCanvas, Chords: chrome.Keys("esc"), Label: "cancel tool"},
		},
	},
	{Command: commandCommitLabel, Label: "Commit Label", Bindings: []actionBinding{{Scope: scopeLabel, Chords: chrome.Keys(labelCommitControlChord, labelCommitSuperChord), Label: "commit label"}}},
	{Command: commandQuit, Label: "Quit", Bindings: []actionBinding{{Scope: scopeCanvas, Chords: chrome.Keys("q"), Label: "cursor / quit"}}},
	{Command: commandHelp, Label: "Toggle Help", Bindings: []actionBinding{{Scope: scopeGlobal, Chords: chrome.Keys("?"), Label: "toggle help"}}},
	{Command: commandCopyCanvasPath, Label: "Copy Path to Canvas", Bindings: []actionBinding{{Scope: scopeGlobal}}},
	{Command: commandPreferences, Label: "Open Preferences", Bindings: []actionBinding{{Scope: scopeGlobal, Chords: primaryKeys("p"), Label: string(scopePreferences)}}},
	{Command: commandSave, Label: "Save Canvas", Bindings: []actionBinding{{Scope: scopeGlobal, Chords: primaryKeys("s"), Label: string(commandSave)}}},
	{Command: commandSidebar, Label: "Toggle Sidebar", Bindings: []actionBinding{{Scope: scopeGlobal, Chords: primaryKeys("b"), Label: "toggle sidebar"}}},
	{Command: commandPalette, Label: "Open Command Palette", HidePalette: true, Bindings: []actionBinding{{Scope: scopeGlobal, Chords: chrome.Keys(paletteDefaultChord), Label: "open command palette"}}},
}

var applicationBindings = flattenActionBindings(applicationActions)

func flattenActionBindings(actions []action) []chrome.Binding {
	var bindings []chrome.Binding
	for _, action := range actions {
		for _, binding := range action.Bindings {
			bindings = append(bindings, chrome.Binding{
				Scope: binding.Scope, Chords: binding.Chords,
				Command: action.Command, Label: binding.Label,
			})
		}
	}
	return bindings
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
		commandArrange,
		commandCommitLabel,
		commandDelete,
		commandDuplicate,
		commandEditLabel,
		commandExpand,
		commandGroup,
		commandRedo,
		commandUndo:
		return m.updateEditCommand(message.Command)
	case commandBack,
		commandCopyCanvasPath,
		commandHelp,
		commandNewCanvas,
		commandPalette,
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
		commandArrange,
		commandCommitLabel,
		commandDelete,
		commandDuplicate,
		commandEditLabel,
		commandExpand,
		commandGroup,
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
	case commandArrange:
		return m.arrangeSelectionAvailable()
	case commandCommitLabel:
		return m.interaction.session.kind == sessionLabelEdit
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
	case commandGroup:
		directNodes, groups, selectedEdges := m.geo.Selection().LogicalCounts()
		return selectedEdges == 0 && (directNodes+groups >= 2 || directNodes == 0 && groups == 1)
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
	case commandCommitLabel:
		return m.interaction.session.kind == sessionLabelEdit
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
	case commandArrange:
		m.toggleArrange()
	case commandCopy:
		if m.dialogs.ActiveID() == surfaceNone && m.interaction.idle() {
			return m.copySelection()
		}
	case commandCommitLabel:
		m.commitAndAdvanceLabelEdit()
	case commandDelete:
		m.deleteActive()
	case commandDuplicate:
		m.duplicateSelectionDefault()
	case commandEditLabel:
		m.beginLabelEdit()
	case commandExpand:
		m.expandSelection()
	case commandGroup:
		m.groupSelection()
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
	case commandCopyCanvasPath:
		return m.copyCanvasPath()
	case commandNewCanvas:
		m.newCanvas()
	case commandPalette:
		return m.openPalette()
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
