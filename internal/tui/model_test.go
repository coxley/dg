package tui

import (
	"math/bits"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/document"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	modalview "github.com/coxley/dg/internal/tui/modal"
	"github.com/coxley/dg/internal/tui/nav"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
	"github.com/stretchr/testify/require"
)

var benchmarkView tea.View

func TestModelNavigatesAndCyclesHits(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	rightPort := portOnRightBoundary(t, model, nodeID)
	model.cursor = model.geo.Ports[rightPort].Anchor
	model.refreshHits()
	require.GreaterOrEqual(t, len(model.hits), 2)

	before := model.cursor
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, before.Add(1, 0), model.cursor)

	model.cursor = model.geo.Ports[rightPort].Anchor
	model.refreshHits()
	updateModel(t, model, tea.KeyPressMsg(tea.Key{
		Code: tea.KeyTab,
		Mod:  tea.ModCtrl,
	}))
	require.Equal(t, 1, model.active)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{
		Code: tea.KeyTab,
		Mod:  tea.ModCtrl | tea.ModShift,
	}))
	require.Zero(t, model.active)
}

func TestModelMoveIsOneUndoInteraction(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	before := model.geo.Nodes[nodeID].Rect.Min
	beforeCursor := model.cursor
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.Equal(t, modeMove, model.mode)
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	updateModel(t, model, keyPress(tea.KeyDown, ""))
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	after := model.geo.Nodes[nodeID].Rect.Min
	require.Equal(t, before.Add(2, 1), after)
	require.Equal(t, beforeCursor.Add(2, 1), model.cursor)
	require.True(t, model.geo.NodeExists(nodeID))
	require.NotEmpty(t, model.canvas.Frame(canvasview.BaseFrame).Text)

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, before, model.geo.Nodes[nodeID].Rect.Min)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'r', Mod: tea.ModCtrl}))
	require.Equal(t, after, model.geo.Nodes[nodeID].Rect.Min)
}

func TestModelFocusCyclesNodesAndArrowMovesFocusedNode(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	leftOrigin := model.geo.Nodes[left].Rect.Min

	updateModel(t, model, keyPress(tea.KeyTab, ""))
	require.Equal(t, layout.Hit{ID: left, Kind: layout.HitNode}, model.target)
	require.True(t, model.geo.Selection().Contains(model.target))

	updateModel(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, leftOrigin.Add(1, 0), model.geo.Nodes[left].Rect.Min)

	updateModel(t, model, keyPress(tea.KeyTab, ""))
	require.Equal(t, layout.Hit{ID: right, Kind: layout.HitNode}, model.target)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{
		Code: tea.KeyTab,
		Mod:  tea.ModShift,
	}))
	require.Equal(t, layout.Hit{ID: left, Kind: layout.HitNode}, model.target)
	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, leftOrigin, model.geo.Nodes[left].Rect.Min)
}

func TestModelCreatesExplicitRectangleAsOneInteraction(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	updateModel(t, model, keyPress('r', "r"))
	require.Equal(t, modeRectangle, model.mode)

	updateModel(t, model, tea.MouseClickMsg{
		X:      15,
		Y:      8,
		Button: tea.MouseLeft,
	})
	nodeID := model.target.ID
	updateModel(t, model, tea.MouseMotionMsg{
		X:      22,
		Y:      12,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      22,
		Y:      12,
		Button: tea.MouseLeft,
	})

	require.Equal(t, modeNavigate, model.mode)
	require.True(t, model.geo.NodeExists(nodeID))
	require.Equal(t, "", model.geo.Label(nodeID))
	size, explicit := model.geo.ExplicitNodeSize(nodeID)
	require.True(t, explicit)
	require.Equal(t, layout.Size{Width: 8, Height: 5}, size)
	require.Equal(t, layout.NewPoint(15, 8), model.geo.Nodes[nodeID].Rect.Min)
	require.True(t, model.geo.Selection().Contains(layout.Hit{
		ID:   nodeID,
		Kind: layout.HitNode,
	}))

	updateModel(t, model, keyPress('u', "u"))
	require.False(t, model.geo.NodeExists(nodeID))
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'r', Mod: tea.ModCtrl}))
	require.True(t, model.geo.NodeExists(nodeID))
	require.Equal(t, layout.Size{Width: 8, Height: 5}, model.geo.Nodes[nodeID].Rect.Size)
}

func TestModelSwitchesDirectlyBetweenDrawingTools(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)

	updateModel(t, model, keyPress('r', "r"))
	require.Equal(t, modeRectangle, model.mode)
	updateModel(t, model, keyPress('l', "l"))
	require.Equal(t, modeConnect, model.mode)
	updateModel(t, model, keyPress('r', "r"))
	require.Equal(t, modeRectangle, model.mode)
	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, modeNavigate, model.mode)
}

func TestModelBlurCommitsRectangleAtVisibleSize(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	updateModel(t, model, keyPress('r', "r"))
	updateModel(t, model, tea.MouseClickMsg{
		X:      15,
		Y:      8,
		Button: tea.MouseLeft,
	})
	nodeID := model.target.ID
	updateModel(t, model, tea.MouseMotionMsg{
		X:      22,
		Y:      12,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.BlurMsg{})

	require.Equal(t, modeNavigate, model.mode)
	require.Equal(t, layout.Size{Width: 8, Height: 5}, model.geo.Nodes[nodeID].Rect.Size)
	updateModel(t, model, keyPress('u', "u"))
	require.False(t, model.geo.NodeExists(nodeID))
}

func TestModelInheritsNodeAndEdgeStyles(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	model.target = layout.Hit{ID: left, Kind: layout.HitNode}
	updateModel(t, model, keyPress('b', "b"))
	require.Equal(t, layout.NodeStyle{Border: layout.BorderRounded}, model.nodeStyle)

	updateModel(t, model, keyPress('r', "r"))
	updateModel(t, model, tea.MouseClickMsg{
		X:      35,
		Y:      7,
		Button: tea.MouseLeft,
	})
	created := model.target.ID
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      42,
		Y:      9,
		Button: tea.MouseLeft,
	})
	style, ok := model.geo.NodeStyle(created)
	require.True(t, ok)
	require.Equal(t, model.nodeStyle, style)

	edgeID := model.geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, model.rebuild())
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	edgeHit := layout.Hit{ID: edgeID, Kind: layout.HitEdge}
	model.selectOnly(edgeHit)
	model.target = edgeHit
	require.True(t, model.geo.Selection().Contains(edgeHit))
	updateModel(t, model, keyPress('a', "a"))
	require.Empty(t, model.status)
	require.Equal(t, layout.EdgeStyle{PortBArrow: layout.ArrowFilled}, model.edgeStyle)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'A',
		Mod:  tea.ModShift,
	}))
	require.Equal(t, layout.EdgeStyle{
		PortAArrow: layout.ArrowFilled,
		PortBArrow: layout.ArrowFilled,
	}, model.edgeStyle)

	source := portExiting(t, model, right, 1)
	destination := portExiting(t, model, created, -1)
	model.connectSource = source
	model.connectStarted = true
	model.completeConnectionTo(destination)
	require.Equal(t, layout.HitEdge, model.target.Kind)
	inherited, ok := model.geo.EdgeStyle(model.target.ID)
	require.True(t, ok)
	require.Equal(t, model.edgeStyle, inherited)
}

func TestModelCyclesStylesAcrossSelection(t *testing.T) {
	t.Parallel()

	model, left, middle := newTwoNodeModel(t)
	right, err := model.geo.NewNodeAt("right", layout.NewPoint(38, 2))
	require.NoError(t, err)
	edgeA := model.geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, middle)
	edgeB := model.geo.ConnectNodes(middle, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, model.geo.SetNodeStyle(left, layout.NodeStyle{
		Border: layout.BorderRounded,
	}))
	require.NoError(t, model.geo.SetEdgeStyle(edgeA, layout.EdgeStyle{
		PortBArrow: layout.ArrowFilled,
	}))
	require.NoError(t, model.rebuild())

	leftHit := layout.Hit{ID: left, Kind: layout.HitNode}
	middleHit := layout.Hit{ID: middle, Kind: layout.HitNode}
	model.selectOnly(leftHit)
	require.True(t, model.geo.Selection().Toggle(middleHit))
	model.target = middleHit
	updateModel(t, model, keyPress('b', "b"))
	require.Equal(
		t,
		layout.NodeStyle{Border: layout.BorderDouble},
		mustNodeStyle(t, model, left),
	)
	require.Equal(
		t,
		layout.NodeStyle{Border: layout.BorderRounded},
		mustNodeStyle(t, model, middle),
	)
	require.True(t, model.geo.Selection().Contains(leftHit))
	require.True(t, model.geo.Selection().Contains(middleHit))
	nodes, _ := model.geo.Selection().Counts()
	require.Equal(t, 2, nodes)

	edgeAHit := layout.Hit{ID: edgeA, Kind: layout.HitEdge}
	edgeBHit := layout.Hit{ID: edgeB, Kind: layout.HitEdge}
	model.selectOnly(edgeAHit)
	require.True(t, model.geo.Selection().Toggle(edgeBHit))
	require.True(t, model.geo.Selection().Toggle(leftHit))
	model.target = edgeBHit
	updateModel(t, model, keyPress('a', "a"))
	require.Equal(t, layout.EdgeStyle{
		PortBArrow: layout.ArrowOpen,
	}, mustEdgeStyle(t, model, edgeA))
	require.Equal(t, layout.EdgeStyle{
		PortBArrow: layout.ArrowFilled,
	}, mustEdgeStyle(t, model, edgeB))
	require.True(t, model.geo.Selection().Contains(edgeAHit))
	require.True(t, model.geo.Selection().Contains(edgeBHit))
	require.True(t, model.geo.Selection().Contains(leftHit))

	updateModel(t, model, keyPress('-', "-"))
	require.Equal(t, layout.StrokeDashed, mustNodeStyle(t, model, left).Stroke)
	require.Equal(t, layout.StrokeDashed, mustEdgeStyle(t, model, edgeA).Stroke)
	require.Equal(t, layout.StrokeDashed, mustEdgeStyle(t, model, edgeB).Stroke)
	require.True(t, model.geo.Selection().Contains(edgeAHit))
	require.True(t, model.geo.Selection().Contains(edgeBHit))
	require.True(t, model.geo.Selection().Contains(leftHit))

	updateModel(t, model, keyPress('-', "-"))
	require.Equal(t, layout.StrokeSolid, mustNodeStyle(t, model, left).Stroke)
	require.Equal(t, layout.StrokeSolid, mustEdgeStyle(t, model, edgeA).Stroke)
	require.Equal(t, layout.StrokeSolid, mustEdgeStyle(t, model, edgeB).Stroke)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'A',
		Mod:  tea.ModShift,
	}))
	require.Equal(t, layout.EdgeStyle{
		PortAArrow: layout.ArrowFilled,
		PortBArrow: layout.ArrowOpen,
	}, mustEdgeStyle(t, model, edgeA))
	require.Equal(t, layout.EdgeStyle{
		PortAArrow: layout.ArrowFilled,
		PortBArrow: layout.ArrowFilled,
	}, mustEdgeStyle(t, model, edgeB))
	require.True(t, model.geo.Selection().Contains(edgeAHit))
	require.True(t, model.geo.Selection().Contains(edgeBHit))
	require.True(t, model.geo.Selection().Contains(leftHit))

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, layout.EdgeStyle{
		PortBArrow: layout.ArrowOpen,
	}, mustEdgeStyle(t, model, edgeA))
	require.Equal(t, layout.EdgeStyle{
		PortBArrow: layout.ArrowFilled,
	}, mustEdgeStyle(t, model, edgeB))
}

func TestModelCyclesTextAlignmentAcrossSelection(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	leftHit := layout.Hit{ID: left, Kind: layout.HitNode}
	rightHit := layout.Hit{ID: right, Kind: layout.HitNode}
	model.selectOnly(leftHit)
	require.True(t, model.geo.Selection().Toggle(rightHit))

	updateModel(t, model, keyPress('t', "t"))
	updateModel(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'T',
		Mod:  tea.ModShift,
	}))

	for _, nodeID := range []uint32{left, right} {
		style := mustNodeStyle(t, model, nodeID)
		require.Equal(t, layout.AlignCenter, style.Horizontal)
		require.Equal(t, layout.AlignMiddle, style.Vertical)
	}
	require.True(t, model.geo.Selection().Contains(leftHit))
	require.True(t, model.geo.Selection().Contains(rightHit))
}

func TestModelDuplicatesSelectedNodesAndInternalEdges(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	edgeID := model.geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, model.rebuild())
	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{
		ID:   right,
		Kind: layout.HitNode,
	}))

	updateModel(t, model, keyPress('d', "d"))

	require.Empty(t, model.status)
	nodes, edges := model.geo.Selection().Counts()
	require.Equal(t, 2, nodes)
	require.Equal(t, 1, edges)
	require.True(t, model.geo.EdgeExists(edgeID))
	updateModel(t, model, keyPress('u', "u"))
	require.Len(t, model.geo.Graph().Nodes, 4)
	require.False(t, model.geo.NodeExists(2))
	require.False(t, model.geo.NodeExists(3))
}

func TestModelAltDragPreviewsAndCommitsDuplicate(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	point := model.geo.Nodes[nodeID].LabelPoint
	click := tea.MouseClickMsg{
		X:      int(point.X),
		Y:      int(point.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	}
	updateModel(t, model, click)
	require.True(t, model.duplicatePending)
	require.False(t, model.duplicateDragging)
	require.Len(t, model.geo.Graph().Nodes, 1)

	updateModel(t, model, tea.MouseMotionMsg{
		X:      click.X + 10,
		Y:      click.Y + 4,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	require.True(t, model.duplicateDragging)
	require.NotNil(t, model.duplicateGeo)
	require.NotEmpty(t, model.canvas.Frame(canvasview.DuplicateFrame).Text)
	require.Len(t, model.geo.Graph().Nodes, 1)

	updateModel(t, model, tea.MouseReleaseMsg{
		X:      click.X + 10,
		Y:      click.Y + 4,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	require.False(t, model.duplicateDragging)
	require.Nil(t, model.duplicateGeo)
	require.Len(t, model.geo.Graph().Nodes, 2)
	copied, ok := model.firstSelectedNode()
	require.True(t, ok)
	require.Equal(t, model.geo.Nodes[nodeID].Rect.Min.Add(10, 4), model.geo.Nodes[copied.ID].Rect.Min)

	updateModel(t, model, keyPress('u', "u"))
	require.False(t, model.geo.NodeExists(copied.ID))
}

func TestModelAltClickWithoutDragDoesNotDuplicate(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	point := model.geo.Nodes[nodeID].LabelPoint
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(point.X),
		Y:      int(point.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      int(point.X),
		Y:      int(point.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})

	require.False(t, model.duplicatePending)
	require.Len(t, model.geo.Graph().Nodes, 1)
}

func TestModelAltDragTranslatesPreviewRoutes(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	edgeID := model.geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, model.rebuild())
	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{
		ID:   right,
		Kind: layout.HitNode,
	}))
	require.True(t, model.geo.Selection().Toggle(layout.Hit{
		ID:   edgeID,
		Kind: layout.HitEdge,
	}))
	start := model.geo.Nodes[left].LabelPoint
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(start.X),
		Y:      int(start.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(start.X) + 30,
		Y:      int(start.Y) + 10,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	var previewEdge uint32
	ok := false
	for id := range model.duplicateGeo.Selection().Edges() {
		previewEdge, ok = id, true
		break
	}
	require.True(t, ok)
	before := append([]layout.Point(nil), model.duplicateGeo.Edges[previewEdge].Points...)

	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(start.X) + 31,
		Y:      int(start.Y) + 12,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})

	for i, point := range before {
		require.Equal(t, point.Add(1, 2), model.duplicateGeo.Edges[previewEdge].Points[i])
	}
}

func TestModelHelpAndPreferencesApplyRouterLive(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	updateModel(t, model, keyPress('?', "?"))
	require.Equal(t, modalHelp, model.modal)
	view := ansi.Strip(model.View().Content)
	require.Contains(t, view, "Shortcuts")
	require.Contains(t, view, "Preferences")
	require.Contains(t, view, "Cursor")
	require.Contains(t, view, "node")
	require.Contains(t, view, "?")
	require.Contains(t, view, "help")
	require.Contains(t, view, "backspace")
	require.Contains(t, view, "delete")

	command := updateModelCommand(t, model, keyPress(tea.KeyTab, ""))
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.Equal(t, modalPreferences, model.modal)
	require.Contains(t, ansi.Strip(model.View().Content), "Step cost")
	before := model.geo.Router()
	command = updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, before.Costs.Step+1, model.geo.Router().Costs.Step)
	require.NotNil(t, command)
	command = updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code: tea.KeyTab,
		Mod:  tea.ModShift,
	}))
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.Equal(t, modalHelp, model.modal)
	require.Equal(t, before.Costs.Step+1, model.geo.Router().Costs.Step)
	command = updateModelCommand(t, model, keyPress(tea.KeyTab, ""))
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.Equal(t, modalPreferences, model.modal)
	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, before, model.geo.Router())
	require.Equal(t, modalNone, model.modal)
}

func TestEnhancedQuestionMarkOpensAndClosesHelp(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	question := tea.KeyPressMsg(tea.Key{
		Code:        '/',
		ShiftedCode: '?',
		Text:        "?",
		Mod:         tea.ModShift,
	})

	updateModel(t, model, question)
	require.Equal(t, modalHelp, model.modal)
	updateModel(t, model, question)
	require.Equal(t, modalNone, model.modal)
}

func TestPreferenceModalFitsShortTerminals(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
	model.openPreferences()

	require.LessOrEqual(
		t,
		model.currentModalOverlay().Height,
		model.height,
	)
}

func TestSettingsModalClosesWithQ(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.openHelp()
	updateModel(t, model, keyPress('q', "q"))
	require.Equal(t, modalNone, model.modal)

	model.openPreferences()
	updateModel(t, model, keyPress('q', "q"))
	require.Equal(t, modalNone, model.modal)
}

func TestSettingsTabsAcceptMouseAndWheelInput(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.openHelp()
	overlay := model.currentModalOverlay()
	shortcutsWidth := lipgloss.Width(model.theme.Modal.ActiveTab.Render("Shortcuts"))
	got, command := model.Update(tea.MouseClickMsg{
		X:      overlay.ContentLeft + shortcutsWidth + 1,
		Y:      overlay.ContentTop,
		Button: tea.MouseLeft,
	})
	require.Same(t, model, got)
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.Equal(t, modalPreferences, model.modal)

	overlay = model.currentModalOverlay()
	updateModelCommand(t, model, tea.MouseWheelMsg{
		X:      overlay.ContentLeft + 1,
		Y:      overlay.ContentTop + 2,
		Button: tea.MouseWheelDown,
	})
	_, completed := model.preferenceForm.Completed()
	require.False(t, completed)

	got, command = model.Update(tea.MouseClickMsg{
		X:      overlay.ContentLeft + 1,
		Y:      overlay.ContentTop,
		Button: tea.MouseLeft,
	})
	require.Same(t, model, got)
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.Equal(t, modalHelp, model.modal)
}

func TestSettingsModalKeepsLargerTabSizeWhenItFits(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 50})
	model.openHelp()
	help := model.currentModalOverlay()

	command := updateModelCommand(t, model, keyPress(tea.KeyTab, ""))
	require.NotNil(t, command)
	updateModel(t, model, command())
	preferences := model.currentModalOverlay()

	require.Equal(t, help.Width, preferences.Width)
	require.Equal(t, help.Height, preferences.Height)
}

func TestSettingsModalShowsCompleteHelpWithoutResize(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 180, Height: 50})
	updateModel(t, model, tea.KeyboardEnhancementsMsg{})
	model.openHelp()

	view := ansi.Strip(model.currentModalOverlay().Content)
	fullHelp := model.help
	fullHelp.SetWidth(0)
	require.GreaterOrEqual(
		t,
		model.dialog.BodyWidth(),
		lipgloss.Width(fullHelp.View(model.keys)),
	)
	require.NotContains(t, view, model.help.Ellipsis)
	for _, group := range model.keys.FullHelp() {
		for _, binding := range group {
			if !binding.Enabled() {
				continue
			}
			require.Contains(t, view, binding.Help().Key)
			require.Contains(t, view, binding.Help().Desc)
		}
	}
}

func TestPreferenceActionsAcceptMouseClicks(t *testing.T) {
	t.Parallel()

	t.Run("cancel", func(t *testing.T) {
		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 50})
		model.openPreferences()
		x, y := modalTextPoint(t, model.currentModalOverlay(), "Cancel")

		updateModel(t, model, tea.MouseClickMsg{
			X:      x,
			Y:      y,
			Button: tea.MouseLeft,
		})

		require.Equal(t, modalNone, model.modal)
	})

	t.Run("save as defaults", func(t *testing.T) {
		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 50})
		model.preferences.path = filepath.Join(t.TempDir(), "preferences.json")
		model.openPreferences()
		x, y := modalTextPoint(
			t,
			model.currentModalOverlay(),
			"Save as Defaults",
		)

		command := updateModelCommand(t, model, tea.MouseClickMsg{
			X:      x,
			Y:      y,
			Button: tea.MouseLeft,
		})

		require.NotNil(t, command)
		data, err := os.ReadFile(model.preferences.path)
		require.NoError(t, err)
		require.Contains(t, string(data), `"apply_to_future": true`)
	})
}

func TestPreferenceEscapeClosesDirectoryBeforeModal(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 50})
	model.openPreferences()
	for range 7 {
		model.updateSettingsTabs(huh.NextField())
	}
	updateModelCommand(t, model, keyPress('l', "l"))
	require.True(t, model.preferenceForm.DirectoryOpen())

	updateModelCommand(t, model, keyPress(tea.KeyEscape, ""))
	require.False(t, model.preferenceForm.DirectoryOpen())
	require.Equal(t, modalPreferences, model.modal)

	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, modalNone, model.modal)
}

func TestPreferenceQClosesDirectoryBeforeModal(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 50})
	model.openPreferences()
	for range 7 {
		model.updateSettingsTabs(huh.NextField())
	}
	updateModelCommand(t, model, keyPress('l', "l"))
	require.True(t, model.preferenceForm.DirectoryOpen())

	updateModelCommand(t, model, keyPress('q', "q"))
	require.False(t, model.preferenceForm.DirectoryOpen())
	require.Equal(t, modalPreferences, model.modal)

	updateModel(t, model, keyPress('q', "q"))
	require.Equal(t, modalNone, model.modal)
}

func TestSettingsModalCanMoveAndOutsideClickCancelsPreferences(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
	model.openPreferences()
	before := model.geo.Router()
	updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.NotEqual(t, before, model.geo.Router())

	overlay := model.currentModalOverlay()
	updateModel(t, model, tea.MouseClickMsg{
		X:      overlay.Left + 4,
		Y:      overlay.Top,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      overlay.Left + 7,
		Y:      overlay.Top + 2,
		Button: tea.MouseLeft,
	})
	moved := model.currentModalOverlay()
	require.Equal(t, overlay.Left+3, moved.Left)
	require.Equal(t, overlay.Top+2, moved.Top)

	command := updateModelCommand(t, model, tea.MouseClickMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	})
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.Equal(t, modalNone, model.modal)
	require.Equal(t, before, model.geo.Router())
}

func TestSettingsModalCanResize(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
	model.openPreferences()
	before := model.currentModalOverlay()
	mouse := tea.Mouse{
		X:      before.Left + before.Width - 1,
		Y:      before.Top + before.Height - 1,
		Button: tea.MouseRight,
	}

	updateModel(t, model, tea.MouseClickMsg(mouse))
	mouse.X += 4
	mouse.Y += 2
	updateModel(t, model, tea.MouseMotionMsg(mouse))
	after := model.currentModalOverlay()
	require.Equal(t, before.Width+4, after.Width)
	require.Equal(t, before.Height+2, after.Height)
	lines := strings.Split(ansi.Strip(after.Content), "\n")
	require.Len(t, lines, after.Height)
	require.True(t, strings.HasPrefix(lines[len(lines)-1], "└"))
	require.True(t, strings.HasSuffix(lines[len(lines)-1], "┘"))
	screen := strings.Split(ansi.Strip(model.View().Content), "\n")
	after = model.currentModalOverlay()
	bottom := screen[after.Top+after.Height-1]
	require.Equal(t, "└", ansi.Cut(bottom, after.Left, after.Left+1))
	require.Equal(
		t,
		"┘",
		ansi.Cut(
			bottom,
			after.Left+after.Width-1,
			after.Left+after.Width,
		),
	)

	updateModel(t, model, tea.MouseReleaseMsg(mouse))
	require.False(t, model.dialog.Resizing())
}

func TestPreferenceStepperUsesOnlyArrowKeys(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	model.openPreferences()
	before := model.geo.Router().Costs.Step
	require.Contains(t, ansi.Strip(model.View().Content), "⇽")
	require.Contains(t, ansi.Strip(model.View().Content), "⇾")

	updateModel(t, model, keyPress('9', "9"))
	require.Equal(t, before, model.geo.Router().Costs.Step)
	command := updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, before+1, model.geo.Router().Costs.Step)
	require.Equal(t, 1, model.preferenceForm.FieldFlash(0))
	require.NotNil(t, command)
}

func TestPreferenceFormCanReachSaveAndCancel(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})
	model.openPreferences()
	for range 9 {
		model.updateSettingsTabs(huh.NextField())
	}

	view := ansi.Strip(model.preferenceForm.View().Content)
	require.Contains(t, view, "Save")
	require.Contains(t, view, "Cancel")
}

func TestPreferenceModalInterruptCancelsLiveChanges(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.openHelp()
	command := updateModelCommand(t, model, keyPress(tea.KeyTab, ""))
	require.NotNil(t, command)
	updateModel(t, model, command())
	before := model.geo.Router()
	updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.NotEqual(t, before, model.geo.Router())

	updateModel(t, model, tea.BlurMsg{})

	require.Equal(t, before, model.geo.Router())
	require.Equal(t, modalNone, model.modal)
	require.False(t, model.preferenceEdit)
}

func TestPreferenceModalReopensWithCancelledValues(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	before := model.geo.Router()
	model.openPreferences()
	updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.NotEqual(t, before, model.geo.Router())
	model.closeSettingsModal()

	model.openPreferences()
	require.Equal(t, before, model.preferences.router)
	require.Equal(t, before.Costs.Step, model.preferenceForm.Value().Router.Costs.Step)
}

func TestPreferencesSaveShowsNotice(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.preferences.path = filepath.Join(t.TempDir(), "preferences.json")
	model.openPreferences()

	command := model.applyPreferences(true)

	require.NotNil(t, command)
	require.Equal(t, modalNotice, model.modal)
	require.Equal(t, "Preferences saved", model.notice)
	require.FileExists(t, model.preferences.path)
}

func TestNoticeExpiresOrDismissesOnKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model, _ := newTestModel(t)
		command := model.showNotice("done", modalNone)
		messages := make(chan tea.Msg, 1)
		go func() {
			messages <- command()
		}()

		time.Sleep(noticeDuration - time.Millisecond)
		require.Equal(t, modalNotice, model.modal)
		require.Empty(t, messages)
		time.Sleep(time.Millisecond)
		updateModel(t, model, <-messages)
		require.Equal(t, modalNone, model.modal)
		require.Empty(t, model.notice)

		model.showNotice("done", modalNone)
		updateModel(t, model, keyPress('x', "x"))
		require.Equal(t, modalNone, model.modal)
		require.Empty(t, model.notice)
	})
}

func TestStatusErrorsRenderRed(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 12})
	model.setError("broken")
	require.Contains(t, model.View().Content, model.theme.Canvas.Error.Render("broken"))

	model.status = "ready"
	require.NotContains(t, model.View().Content, model.theme.Canvas.Error.Render("ready"))
}

func TestLineModePortHighlightUsesForegroundOnly(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.mode = modeConnect

	highlight := model.highlightStyle().Render("x")
	require.Contains(t, highlight, "[38;")
	require.NotContains(t, highlight, "[48;")
}

func TestFormCommandScopesComponentMessages(t *testing.T) {
	t.Parallel()

	require.Nil(t, componentCommand(saveComponent, func() tea.Msg { return nil })())

	type opaqueMsg struct{ value int }
	command := componentCommand(saveComponent, tea.Batch(
		func() tea.Msg { return opaqueMsg{value: 1} },
		func() tea.Msg { return opaqueMsg{value: 2} },
	))
	batch, ok := command().(tea.BatchMsg)
	require.True(t, ok)
	require.Len(t, batch, 2)

	for i, command := range batch {
		message, ok := command().(componentMsg)
		require.True(t, ok)
		require.Equal(t, saveComponent, message.kind)
		require.Equal(t, opaqueMsg{value: i + 1}, message.message)
	}
}

func TestModelReordersLayersWithUndo(t *testing.T) {
	t.Parallel()

	model, back, front := newTwoNodeModel(t)
	backHit := layout.Hit{ID: back, Kind: layout.HitNode}
	frontHit := layout.Hit{ID: front, Kind: layout.HitNode}
	model.selectOnly(backHit)

	updateModel(t, model, keyPress(']', "]"))
	require.Equal(
		t,
		[]layout.Hit{frontHit, backHit},
		slices.Collect(model.geo.DrawOrder()),
	)
	updateModel(t, model, keyPress('u', "u"))
	require.Equal(
		t,
		[]layout.Hit{backHit, frontHit},
		slices.Collect(model.geo.DrawOrder()),
	)

	model.selectOnly(frontHit)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{
		Code: '[',
		Mod:  tea.ModShift,
	}))
	require.Equal(
		t,
		[]layout.Hit{frontHit, backHit},
		slices.Collect(model.geo.DrawOrder()),
	)
}

func TestModelBlurCommitsActiveMove(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	before := model.geo.Nodes[nodeID].Rect.Min
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	updateModel(t, model, tea.BlurMsg{})
	require.Equal(t, modeNavigate, model.mode)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'z', Mod: tea.ModCtrl}))
	require.Equal(t, before, model.geo.Nodes[nodeID].Rect.Min)
}

func TestModelEditsLabel(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress('e', "e"))
	require.Equal(t, modeEditLabel, model.mode)

	updateModel(t, model, keyPress(tea.KeyBackspace, ""))
	require.Equal(t, "nod", model.geo.Label(nodeID))
	updateModel(t, model, keyPress('X', "X"))
	require.Equal(t, "nodX", model.geo.Label(nodeID))
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.Equal(t, modeEditLabel, model.mode)
	require.Equal(t, "nodX\n", model.geo.Label(nodeID))
	updateModel(t, model, tea.PasteMsg{Content: "two\nlines"})
	require.Equal(t, "nodX\ntwo\nlines", string(model.editBuffer))

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}))
	require.Equal(t, modeNavigate, model.mode)
	require.Equal(t, "nodX\ntwo\nlines", model.geo.Label(nodeID))
	require.Empty(t, model.editBuffer)
}

func TestModelEscapeCommitsLabelEdit(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress('e', "e"))
	updateModel(t, model, keyPress(tea.KeyLeft, ""))
	updateModel(t, model, keyPress('X', "X"))
	require.Equal(t, "nodXe", model.geo.Label(nodeID))

	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, modeNavigate, model.mode)
	require.Equal(t, "nodXe", model.geo.Label(nodeID))

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'z', Mod: tea.ModCtrl}))
	require.Equal(t, "node", model.geo.Label(nodeID))
}

func TestModelEditMovesByGraphemeWidth(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "A界"))
	require.NoError(t, model.rebuild())
	model.cursor = model.geo.Nodes[nodeID].LabelPoint
	model.refreshHits()

	updateModel(t, model, keyPress('e', "e"))
	updateModel(t, model, keyPress(tea.KeyLeft, ""))
	require.Equal(t, 1, model.editCaret)
	require.Equal(t, model.geo.Nodes[nodeID].LabelPoint.Add(1, 0), model.cursor)
	updateModel(t, model, keyPress(tea.KeyDelete, ""))
	require.Equal(t, "A", model.geo.Label(nodeID))
}

func TestRectangleLabelCursorUsesTextAlignment(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.nodeStyle = layout.NodeStyle{
		Horizontal: layout.AlignCenter,
		Vertical:   layout.AlignMiddle,
	}
	model.startRectangle(layout.NewPoint(20, 10))
	model.resizeNode(layout.NewPoint(31, 16))
	nodeID := model.target.ID
	model.finishRectangle()
	model.startLabelEdit(layout.Hit{ID: nodeID, Kind: layout.HitNode})

	want, visible := model.geo.LabelLinePoint(nodeID, 0, 1, 0)
	require.True(t, visible)
	require.True(t, model.editCaretVisible)
	require.Equal(t, want, model.cursor)

	updateModel(t, model, keyPress('x', "x"))
	want, visible = model.geo.LabelLinePoint(nodeID, 0, 1, 1)
	require.True(t, visible)
	require.Equal(t, want.Add(1, 0), model.cursor)
}

func TestModelEditShortcuts(t *testing.T) {
	t.Parallel()

	const oneTwo = "one two"
	tests := []struct {
		name      string
		label     string
		caret     int
		key       tea.Key
		wantLabel string
		wantCaret int
	}{
		{
			name:      "ctrl-w stops at path boundary",
			label:     "界面/path/to/file.json",
			caret:     len("界面/path/to/file.json"),
			key:       tea.Key{Code: 'w', Mod: tea.ModCtrl},
			wantLabel: "界面/path/to/",
			wantCaret: len("界面/path/to/"),
		},
		{
			name:      "ctrl-u deletes to line start",
			label:     oneTwo,
			caret:     len("one "),
			key:       tea.Key{Code: 'u', Mod: tea.ModCtrl},
			wantLabel: "two",
			wantCaret: 0,
		},
		{
			name:      "alt-b moves to previous word",
			label:     "one  two/three",
			caret:     len("one  two/three"),
			key:       tea.Key{Code: 'b', Mod: tea.ModAlt},
			wantLabel: "one  two/three",
			wantCaret: len("one  two/"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			model, nodeID := newTestModel(t)
			require.NoError(t, model.geo.SetNodeLabel(nodeID, test.label))
			require.NoError(t, model.rebuild())
			model.cursor = model.geo.Nodes[nodeID].LabelPoint
			model.refreshHits()
			updateModel(t, model, keyPress('e', "e"))
			model.editCaret = test.caret
			model.moveCursorToCaret()

			updateModel(t, model, tea.KeyPressMsg(test.key))

			require.Equal(t, test.wantLabel, model.geo.Label(nodeID))
			require.Equal(t, test.wantCaret, model.editCaret)
			require.Equal(
				t,
				model.geo.Nodes[nodeID].LabelPoint.Add(
					uint32(displayWidth([]byte(test.wantLabel[:test.wantCaret]))),
					0,
				),
				model.cursor,
			)
		})
	}
}

func TestModelEditMovesToLineBounds(t *testing.T) {
	t.Parallel()

	const label = "one two"
	model, nodeID := newTestModel(t)
	require.NoError(t, model.geo.SetNodeLabel(nodeID, label))
	require.NoError(t, model.rebuild())
	model.cursor = model.geo.Nodes[nodeID].LabelPoint
	model.refreshHits()
	updateModel(t, model, keyPress('e', "e"))
	model.editCaret = len("one ")
	model.moveCursorToCaret()

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	require.Zero(t, model.editCaret)
	require.Equal(t, model.geo.Nodes[nodeID].LabelPoint, model.cursor)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'e', Mod: tea.ModCtrl}))
	require.Equal(t, len(label), model.editCaret)
	require.Equal(
		t,
		model.geo.Nodes[nodeID].LabelPoint.Add(uint32(displayWidth([]byte(label))), 0),
		model.cursor,
	)
}

func TestModelEditShortcutsUseCurrentMultilineRow(t *testing.T) {
	t.Parallel()

	const label = "one\ntwo three"
	model, nodeID := newTestModel(t)
	require.NoError(t, model.geo.SetNodeLabel(nodeID, label))
	require.NoError(t, model.rebuild())
	model.cursor = model.geo.Nodes[nodeID].LabelPoint
	model.refreshHits()
	updateModel(t, model, keyPress('e', "e"))
	model.editCaret = len("one\ntwo ")
	model.moveCursorToCaret()

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	require.Equal(t, "one\nthree", model.geo.Label(nodeID))
	require.Equal(t, len("one\n"), model.editCaret)
	require.Equal(t, model.geo.Nodes[nodeID].LabelPoint.Add(0, 1), model.cursor)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'e', Mod: tea.ModCtrl}))
	require.Equal(t, len("one\nthree"), model.editCaret)
	require.Equal(t, model.geo.Nodes[nodeID].LabelPoint.Add(5, 1), model.cursor)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	require.Equal(t, len("one\n"), model.editCaret)
	require.Equal(t, model.geo.Nodes[nodeID].LabelPoint.Add(0, 1), model.cursor)
}

func TestModelCreatesNodesWithEnterAndEscape(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, keyPress('n', "n"))
	created := model.target.ID
	require.Equal(t, modeEditLabel, model.mode)
	require.True(t, model.geo.NodeExists(created))
	require.Equal(t, layout.Size{Width: 4, Height: 3}, model.geo.Nodes[created].Rect.Size)

	updateModel(t, model, keyPress('A', "A"))
	require.Equal(t, "A", model.geo.Label(created))
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}))
	require.Equal(t, modeNavigate, model.mode)
	require.True(t, model.geo.NodeExists(created))

	model.cursor = layout.NewPoint(30, 10)
	model.refreshHits()
	updateModel(t, model, keyPress('n', "n"))
	escaped := model.target.ID
	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.True(t, model.geo.NodeExists(escaped))

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'z', Mod: tea.ModCtrl}))
	require.False(t, model.geo.NodeExists(escaped))
}

func TestModelConnectsSelectedPorts(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	source := portExiting(t, model, left, 1)
	destination := portExiting(t, model, right, -1)

	selectHit(t, model, layout.Hit{ID: source, Kind: layout.HitPort})
	updateModel(t, model, keyPress('c', "c"))
	require.Equal(t, modeNavigate, model.mode)
	updateModel(t, model, keyPress('l', "l"))
	require.Equal(t, modeConnect, model.mode)
	require.Equal(t, source, model.connectSource)
	anchors := make(map[layout.Point]struct{})
	for portID := range model.geo.NodePorts(left) {
		if !model.geo.PortUsable(portID) {
			continue
		}
		anchor := model.geo.Ports[portID].Anchor
		anchors[anchor] = struct{}{}
		require.True(t, model.highlightedPoint(anchor))
	}
	require.Len(t, anchors, 6)
	require.False(t, model.highlightedPoint(model.geo.Nodes[left].Rect.Min))

	selectHit(t, model, layout.Hit{ID: destination, Kind: layout.HitPort})
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.Equal(t, modeNavigate, model.mode)
	require.Len(t, model.geo.Edges, 1)
	require.True(t, model.geo.EdgeExists(0))
	require.NotEmpty(t, model.geo.Edges[0].Points)
}

func TestModelMouseDragsLineBetweenPorts(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 50, Height: 15})
	require.NoError(t, model.geo.PlaceNode(left, model.geo.Nodes[left].Rect.Min.Add(0, 6)))
	require.NoError(t, model.geo.PlaceNode(right, model.geo.Nodes[right].Rect.Min.Add(0, 6)))
	require.NoError(t, model.rebuild())
	sourceID := portExiting(t, model, left, 1)
	destinationID := portExiting(t, model, right, -1)
	source := model.geo.Ports[sourceID]
	destination := model.geo.Ports[destinationID]

	updateModel(t, model, keyPress('l', "l"))
	require.Equal(t, modeConnect, model.mode)
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(source.Anchor.X),
		Y:      int(source.Anchor.Y),
		Button: tea.MouseLeft,
	})
	require.True(t, model.connectDragging)
	require.Equal(t, sourceID, model.connectSource)

	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(destination.Anchor.X),
		Y:      int(destination.Anchor.Y),
		Button: tea.MouseLeft,
	})
	middle := layout.NewPoint(
		(source.Exit.X+destination.Anchor.X)/2,
		source.Exit.Y,
	)
	require.False(t, model.highlightedPoint(middle))
	connections, ok := model.connectionPreviewConnections(middle)
	require.True(t, ok)
	require.Equal(t, layout.East|layout.West, connections)
	require.Equal(
		t,
		"─",
		string(appendViewportSpaces(nil, 1, uint64(middle.X), uint64(middle.Y), model)),
	)
	glyph, ok := previewGlyph(
		model,
		uint64(source.Anchor.X),
		uint64(source.Anchor.Y),
	)
	require.True(t, ok)
	require.Equal(t, '├', glyph)
	glyph, ok = previewGlyph(
		model,
		uint64(destination.Anchor.X),
		uint64(destination.Anchor.Y),
	)
	require.True(t, ok)
	require.Equal(t, '┤', glyph)

	updateModel(t, model, tea.MouseReleaseMsg{
		X:      int(destination.Anchor.X),
		Y:      int(destination.Anchor.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, modeNavigate, model.mode)
	require.True(t, model.geo.EdgeExists(0))
}

func TestModelConnectionPreviewTargetsConnectedPort(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	third, err := model.geo.NewNodeAt("third", layout.NewPoint(2, 8))
	require.NoError(t, err)
	existingSource := portExiting(t, model, left, 1)
	destination := portExiting(t, model, right, -1)
	_, err = model.geo.ConnectPorts(existingSource, destination)
	require.NoError(t, err)
	require.NoError(t, model.geo.Build())
	require.NoError(t, model.render())
	updateModel(t, model, tea.WindowSizeMsg{Width: 50, Height: 20})

	source := portExiting(t, model, third, 1)
	updateModel(t, model, keyPress('l', "l"))
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(model.geo.Ports[source].Anchor.X),
		Y:      int(model.geo.Ports[source].Anchor.Y),
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(model.geo.Ports[destination].Anchor.X),
		Y:      int(model.geo.Ports[destination].Anchor.Y),
		Button: tea.MouseLeft,
	})

	require.NotEmpty(t, model.connectPreview)
	require.Equal(t, model.geo.Ports[source].Anchor, model.connectPreview[0])
	require.Equal(
		t,
		model.geo.Ports[destination].Anchor,
		model.connectPreview[len(model.connectPreview)-1],
	)
	var (
		join   layout.Point
		merged layout.Connections
	)
	for _, cell := range model.connectRaster {
		if bits.OnesCount8(uint8(cell.Connections)) == 3 {
			join = cell.Point
			merged = cell.Connections
			break
		}
	}
	require.NotZero(t, merged)
	glyph, ok := previewGlyph(model, uint64(join.X), uint64(join.Y))
	require.True(t, ok)
	require.Equal(t, render.Glyph(merged), glyph)
}

func TestModelConnectionPreviewRoutesAroundNodes(t *testing.T) {
	t.Parallel()

	model, left, obstacle := newTwoNodeModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 20})
	sourceID := portExiting(t, model, left, 1)
	source := model.geo.Ports[sourceID]
	cursor := layout.NewPoint(35, source.Anchor.Y)

	updateModel(t, model, keyPress('l', "l"))
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(source.Anchor.X),
		Y:      int(source.Anchor.Y),
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(cursor.X),
		Y:      int(cursor.Y),
		Button: tea.MouseLeft,
	})

	require.Equal(t, source.Anchor, model.connectPreview[0])
	require.Equal(t, cursor, model.connectPreview[len(model.connectPreview)-1])
	for i := 1; i < len(model.connectPreview); i++ {
		point := model.connectPreview[i-1]
		for point != model.connectPreview[i] {
			point = stepToward(point, model.connectPreview[i])
			require.False(
				t,
				model.geo.Nodes[obstacle].Rect.Contains(point),
				"preview crosses node at %+v: %+v",
				point,
				model.connectPreview,
			)
		}
	}
}

func TestModelReconnectsNearestEdgeEndpoint(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	portA := portExiting(t, model, left, 1)
	portB := portExiting(t, model, right, -1)
	edgeID, err := model.geo.ConnectPorts(portA, portB)
	require.NoError(t, err)
	third, err := model.geo.NewNodeAt("third", layout.NewPoint(35, 8))
	require.NoError(t, err)
	replacement := portExiting(t, model, third, -1)
	require.NoError(t, model.rebuild())

	selectHit(t, model, layout.Hit{ID: edgeID, Kind: layout.HitEdge})
	updateModel(t, model, keyPress('l', "l"))
	require.True(t, model.reconnecting)
	oldPort := model.connectOldPort
	selectHit(t, model, layout.Hit{ID: replacement, Kind: layout.HitPort})
	updateModel(t, model, keyPress(tea.KeyEnter, ""))

	require.Equal(t, modeNavigate, model.mode)
	require.True(t, model.geo.EdgeExists(edgeID))
	gotA, gotB, err := model.geo.EdgePorts(edgeID)
	require.NoError(t, err)
	require.Contains(t, []uint32{gotA, gotB}, replacement)
	require.NotContains(t, []uint32{gotA, gotB}, oldPort)
	require.NotEmpty(t, model.geo.Edges[edgeID].Points)
}

func TestModelMouseDragsNearbyEdgeEndpoint(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	edgeID := model.geo.ConnectNodes(
		left,
		ir.RightSide,
		ir.LeftSide,
		right,
	)
	third, err := model.geo.NewNodeAt("third", layout.NewPoint(35, 8))
	require.NoError(t, err)
	replacement := portExiting(t, model, third, -1)
	require.NoError(t, model.rebuild())
	updateModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 20})

	edge := model.geo.Edges[edgeID]
	near := stepToward(edge.Points[0], edge.Points[1])
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(near.X),
		Y:      int(near.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, modeNavigate, model.mode)
	require.True(t, model.edgeDragPending)
	require.True(t, selectionContains(model, layout.HitEdge, edgeID))
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      int(near.X),
		Y:      int(near.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, modeNavigate, model.mode)
	require.False(t, model.edgeDragPending)
	require.Empty(t, model.connectPreview)

	blank := near.Add(0, 3)
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(near.X),
		Y:      int(near.Y),
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(blank.X),
		Y:      int(blank.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, modeConnect, model.mode)
	require.NotEqual(
		t,
		' ',
		frameRuneAt(t, model.canvas.Frame(canvasview.BaseFrame), near),
	)
	require.Equal(
		t,
		' ',
		frameRuneAt(t, model.canvas.Frame(canvasview.ConnectionFrame), near),
	)
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      int(blank.X),
		Y:      int(blank.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, modeNavigate, model.mode)
	require.False(t, model.connectStarted)

	updateModel(t, model, tea.MouseClickMsg{
		X:      int(near.X),
		Y:      int(near.Y),
		Button: tea.MouseLeft,
	})

	destination := model.geo.Ports[replacement].Anchor
	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(destination.X),
		Y:      int(destination.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, modeConnect, model.mode)
	require.True(t, model.reconnecting)
	require.True(t, model.connectDragging)
	oldPort := model.connectOldPort
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      int(destination.X),
		Y:      int(destination.Y),
		Button: tea.MouseLeft,
	})

	require.Equal(t, modeNavigate, model.mode)
	gotA, gotB, err := model.geo.EdgePorts(edgeID)
	require.NoError(t, err)
	require.Contains(t, []uint32{gotA, gotB}, replacement)
	require.NotContains(t, []uint32{gotA, gotB}, oldPort)
}

func TestModelMousePrioritizesSelectedEdgeAtPort(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	edgeID := model.geo.ConnectNodes(
		left,
		ir.RightSide,
		ir.LeftSide,
		right,
	)
	require.NoError(t, model.rebuild())
	updateModel(t, model, tea.WindowSizeMsg{Width: 50, Height: 15})
	model.selectOnly(layout.Hit{ID: edgeID, Kind: layout.HitEdge})

	portA, _, err := model.geo.EdgePorts(edgeID)
	require.NoError(t, err)
	point := model.geo.Ports[portA].Anchor
	require.GreaterOrEqual(t, len(slices.Collect(model.geo.Hits(point))), 3)
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(point.X),
		Y:      int(point.Y),
		Button: tea.MouseLeft,
	})

	hit, ok := model.activeHit()
	require.True(t, ok)
	require.Equal(t, layout.Hit{ID: edgeID, Kind: layout.HitEdge}, hit)
	require.True(t, model.edgeDragPending)
	require.False(t, model.dragging)
}

func TestModelDeletesNode(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress(tea.KeyBackspace, ""))

	require.False(t, model.geo.NodeExists(nodeID))
	require.Empty(t, model.canvas.Frame(canvasview.BaseFrame).Text)
	require.Empty(t, model.hits)
}

func TestModelViewTracksWindowWithoutCursor(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 12})
	view := model.View()

	require.Equal(t, 12, strings.Count(view.Content, "\n"))
	require.Nil(t, view.Cursor)
	require.True(t, view.AltScreen)
	require.Equal(t, tea.MouseModeAllMotion, view.MouseMode)
	require.Contains(t, view.Content, model.theme.Canvas.Selection.Render("┌──"))
	require.False(t, model.highlightedPoint(model.geo.Nodes[nodeID].LabelPoint))
}

func TestToolbarFloatsOverCanvasAndCentersTools(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 10})
	view := model.View()
	lines := strings.Split(view.Content, "\n")
	require.NotContains(t, lines[0], "╭")
	require.Contains(t, lines[1], "╭───────────────────────────╮")
	require.Contains(t, lines[2], "│                           │")
	tools := ansi.Strip(lines[3])
	require.Contains(t, tools, "│  Cursor  Rectangle  Line  │")
	require.Contains(t, lines[4], "│                           │")
	require.Contains(t, lines[5], "╰───────────────────────────╯")

	point, ok := model.documentPoint(3, 0)
	require.True(t, ok)
	require.Equal(t, layout.NewPoint(3, 0), point)

	start, row, ok := model.nav.Cell(nav.Rectangle)
	require.True(t, ok)
	got, command := model.Update(tea.MouseClickMsg{
		X:      start,
		Y:      row,
		Button: tea.MouseLeft,
	})
	require.Same(t, model, got)
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.Equal(t, modeRectangle, model.mode)
}

func TestToolbarHighlightsHoveredTool(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 10})
	start, row, ok := model.nav.Cell(nav.Rectangle)
	require.True(t, ok)
	updateModel(t, model, tea.MouseMotionMsg{
		X: start + 1,
		Y: row,
	})

	require.Contains(
		t,
		model.View().Content,
		model.theme.Nav.Hover.Render(" Rectangle "),
	)
}

func TestKeyboardEnhancementsAdvertiseSuperCopy(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	require.Equal(t, "ctrl+c", model.keys.copy.Help().Key)
	require.True(t, key.Matches(
		tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModSuper}),
		model.keys.copy,
	))
	updateModel(t, model, tea.KeyboardEnhancementsMsg{})
	require.Equal(t, "super+c / ctrl+c", model.keys.copy.Help().Key)
}

func TestModelViewShowsCursorWhileEditing(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 12, Height: 12})
	updateModel(t, model, keyPress('e', "e"))
	view := model.View()

	require.NotNil(t, view.Cursor)
	require.Equal(t, int(model.cursor.X-model.viewport.X), view.Cursor.X)
	require.Equal(t, int(model.cursor.Y-model.viewport.Y), view.Cursor.Y)
	require.NotSame(t, view.Cursor, model.View().Cursor)
}

func TestModelViewShowsSaveForm(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.preferences.saveDirectory = t.TempDir()
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	view := model.View()

	require.Contains(t, view.Content, "Directory")
	require.Nil(t, view.Cursor)
}

func TestModelSavesWithPathPromptAndReusesPath(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	dir := t.TempDir()
	path := filepath.Join(dir, defaultSaveName)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	require.Equal(t, modalSave, model.modal)
	model.saveDirectory = dir
	model.saveName = defaultSaveName
	model.commitSaveForm()

	require.Equal(t, modeNavigate, model.mode)
	require.Equal(t, modalNone, model.modal)
	require.Equal(t, path, model.path)
	require.Equal(t, "saved "+path, model.status)
	requireSavedLabel(t, path, "node")

	require.NoError(t, model.geo.SetNodeLabel(nodeID, "updated"))
	require.NoError(t, model.rebuild())
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))

	require.Equal(t, modeNavigate, model.mode)
	requireSavedLabel(t, path, "updated")
}

func TestModelSaveFormBrowsesRealPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "diagram-one.json"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "diagram-two.json"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o700))

	model, _ := newTestModel(t)
	model.preferences.saveDirectory = dir
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))

	views := model.View().Content
	updateModelCommand(t, model, keyPress(tea.KeyDown, ""))
	views += model.View().Content
	updateModelCommand(t, model, keyPress(tea.KeyDown, ""))
	views += model.View().Content
	require.Contains(t, views, "diagram-one.json")
	require.Contains(t, views, "diagram-two.json")
	require.Contains(t, views, "nested")
}

func TestModelMouseSelectsAndDragsNode(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 15})
	before := model.geo.Nodes[nodeID].Rect.Min
	click := tea.Mouse{
		X: int(model.cursor.X - model.viewport.X),
		Y: int(model.cursor.Y - model.viewport.Y),
	}
	updateModel(t, model, tea.MouseClickMsg{
		X:      click.X,
		Y:      click.Y,
		Button: tea.MouseLeft,
	})
	require.True(t, model.dragging)

	updateModel(t, model, tea.MouseMotionMsg{
		X:      click.X + 2,
		Y:      click.Y + 1,
		Button: tea.MouseLeft,
	})
	require.Equal(t, before.Add(2, 1), model.geo.Nodes[nodeID].Rect.Min)
	updateModel(t, model, tea.MouseReleaseMsg{Button: tea.MouseLeft})
	require.False(t, model.dragging)
}

func TestModelMouseAreaSelectsIntersectingObjects(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 50, Height: 15})
	updateModel(t, model, tea.MouseClickMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	})
	require.True(t, model.selecting)
	updateModel(t, model, tea.MouseMotionMsg{
		X:      12,
		Y:      6,
		Button: tea.MouseLeft,
	})
	require.True(t, model.highlightedPoint(layout.NewPoint(0, 0)))
	require.True(t, model.highlightedPoint(layout.NewPoint(6, 3)))
	require.True(t, model.highlightedPoint(layout.NewPoint(12, 6)))
	require.False(t, model.highlightedPoint(layout.NewPoint(13, 3)))

	updateModel(t, model, tea.MouseReleaseMsg{
		X:      12,
		Y:      6,
		Button: tea.MouseLeft,
	})
	require.False(t, model.selecting)
	require.True(t, selectionContains(model, layout.HitNode, left))
	require.False(t, selectionContains(model, layout.HitNode, right))
	require.True(t, model.highlightedPoint(model.geo.Nodes[left].Rect.Min))
	require.False(t, model.highlightedPoint(model.geo.Nodes[right].Rect.Min))
}

func TestModelControlAExpandsComponentsThenEverything(t *testing.T) {
	t.Parallel()

	model, left, connected, isolated, edgeID := newComponentModel(t)
	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	require.True(t, selectionContains(model, layout.HitNode, left))
	require.True(t, selectionContains(model, layout.HitNode, connected))
	require.False(t, selectionContains(model, layout.HitNode, isolated))
	require.True(t, selectionContains(model, layout.HitEdge, edgeID))

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	require.True(t, selectionContains(model, layout.HitNode, isolated))
}

func TestModelControlClickTogglesObjects(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 50, Height: 15})
	require.NoError(t, model.geo.PlaceNode(left, model.geo.Nodes[left].Rect.Min.Add(0, 6)))
	require.NoError(t, model.geo.PlaceNode(right, model.geo.Nodes[right].Rect.Min.Add(0, 6)))
	require.NoError(t, model.rebuild())
	for _, nodeID := range []uint32{left, right} {
		point := model.geo.Nodes[nodeID].LabelPoint
		updateModel(t, model, tea.MouseClickMsg{
			X:      int(point.X),
			Y:      int(point.Y),
			Button: tea.MouseLeft,
			Mod:    tea.ModCtrl,
		})
	}
	require.True(t, selectionContains(model, layout.HitNode, left))
	require.True(t, selectionContains(model, layout.HitNode, right))
	require.False(t, model.dragging)

	point := model.geo.Nodes[left].LabelPoint
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(point.X),
		Y:      int(point.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModCtrl,
	})
	require.False(t, selectionContains(model, layout.HitNode, left))
	require.True(t, selectionContains(model, layout.HitNode, right))
}

func TestModelMovesAndDeletesSelectionAsOneInteraction(t *testing.T) {
	t.Parallel()

	model, left, connected, isolated, edgeID := newComponentModel(t)
	leftOrigin := model.geo.Nodes[left].Rect.Min
	connectedOrigin := model.geo.Nodes[connected].Rect.Min
	isolatedOrigin := model.geo.Nodes[isolated].Rect.Min
	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))

	updateModel(t, model, keyPress('m', "m"))
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.Equal(t, leftOrigin.Add(1, 0), model.geo.Nodes[left].Rect.Min)
	require.Equal(t, connectedOrigin.Add(1, 0), model.geo.Nodes[connected].Rect.Min)
	require.Equal(t, isolatedOrigin, model.geo.Nodes[isolated].Rect.Min)

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, leftOrigin, model.geo.Nodes[left].Rect.Min)
	require.Equal(t, connectedOrigin, model.geo.Nodes[connected].Rect.Min)

	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	updateModel(t, model, keyPress(tea.KeyBackspace, ""))
	require.False(t, model.geo.NodeExists(left))
	require.False(t, model.geo.NodeExists(connected))
	require.True(t, model.geo.NodeExists(isolated))
	require.False(t, model.geo.EdgeExists(edgeID))

	updateModel(t, model, keyPress('u', "u"))
	require.True(t, model.geo.NodeExists(left))
	require.True(t, model.geo.NodeExists(connected))
	require.True(t, model.geo.EdgeExists(edgeID))
}

func TestModelArrowMovesSelectionAsOneInteraction(t *testing.T) {
	t.Parallel()

	model, left, connected, _, edgeID := newComponentModel(t)
	leftHit := layout.Hit{ID: left, Kind: layout.HitNode}
	connectedHit := layout.Hit{ID: connected, Kind: layout.HitNode}
	edgeHit := layout.Hit{ID: edgeID, Kind: layout.HitEdge}
	model.selectOnly(leftHit)
	require.True(t, model.geo.Selection().Toggle(connectedHit))
	require.True(t, model.geo.Selection().Toggle(edgeHit))
	model.target = edgeHit
	leftOrigin := model.geo.Nodes[left].Rect.Min
	connectedOrigin := model.geo.Nodes[connected].Rect.Min

	updateModel(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, leftOrigin.Add(1, 0), model.geo.Nodes[left].Rect.Min)
	require.Equal(t, connectedOrigin.Add(1, 0), model.geo.Nodes[connected].Rect.Min)
	require.True(t, model.geo.Selection().Contains(leftHit))
	require.True(t, model.geo.Selection().Contains(connectedHit))
	require.True(t, model.geo.Selection().Contains(edgeHit))

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, leftOrigin, model.geo.Nodes[left].Rect.Min)
	require.Equal(t, connectedOrigin, model.geo.Nodes[connected].Rect.Min)
}

func TestModelMouseDragCommitsNewNodeLabel(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 50, Height: 20})
	model.cursor = layout.NewPoint(30, 10)
	model.refreshHits()
	updateModel(t, model, keyPress('n', "n"))
	nodeID := model.target.ID
	updateModel(t, model, keyPress('N', "N"))
	updateModel(t, model, keyPress('e', "e"))
	updateModel(t, model, keyPress('w', "w"))
	before := model.geo.Nodes[nodeID].Rect.Min

	click := tea.Mouse{
		X: int(model.cursor.X - model.viewport.X),
		Y: int(model.cursor.Y - model.viewport.Y),
	}
	updateModel(t, model, tea.MouseClickMsg{
		X:      click.X,
		Y:      click.Y,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      click.X + 2,
		Y:      click.Y + 1,
		Button: tea.MouseLeft,
	})

	require.Equal(t, modeNavigate, model.mode)
	require.Equal(t, "New", model.geo.Label(nodeID))
	require.Equal(t, before.Add(2, 1), model.geo.Nodes[nodeID].Rect.Min)
	require.True(t, model.dragging)
}

func TestModelMouseCyclesHitsAndScrolls(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 15})
	require.NoError(t, model.geo.PlaceNode(nodeID, layout.NewPoint(2, 8)))
	require.NoError(t, model.rebuild())
	portID := portExiting(t, model, nodeID, 1)
	point := model.geo.Ports[portID].Anchor
	mouse := tea.Mouse{
		X:      int(point.X - model.viewport.X),
		Y:      int(point.Y - model.viewport.Y),
		Button: tea.MouseLeft,
	}
	updateModel(t, model, tea.MouseClickMsg(mouse))
	hit, ok := model.activeHit()
	require.True(t, ok)
	require.Equal(t, layout.HitNode, hit.Kind)
	updateModel(t, model, tea.MouseReleaseMsg{Button: tea.MouseLeft})
	updateModel(t, model, tea.MouseClickMsg(mouse))
	hit, ok = model.activeHit()
	require.True(t, ok)
	require.Equal(t, layout.HitPort, hit.Kind)

	updateModel(t, model, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	require.Equal(t, uint32(3), model.viewport.Y)
	updateModel(t, model, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	require.Zero(t, model.viewport.Y)
}

func TestModelMouseDragCanMovePartiallyOutsideViewportAndBack(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 20})
	require.NoError(t, model.geo.PlaceNode(nodeID, layout.NewPoint(2, 8)))
	staticID, err := model.geo.NewNodeAt("static", layout.NewPoint(20, 8))
	require.NoError(t, err)
	require.NoError(t, model.rebuild())
	model.history.Clear()
	staticBefore := model.geo.Nodes[staticID].Rect.Min

	label := model.geo.Nodes[nodeID].LabelPoint
	click := tea.Mouse{
		X:      int(label.X - model.viewport.X),
		Y:      int(label.Y - model.viewport.Y),
		Button: tea.MouseLeft,
	}
	updateModel(t, model, tea.MouseClickMsg(click))
	updateModel(t, model, tea.MouseMotionMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	})

	require.Equal(t, uint32(2), model.viewport.X)
	require.Equal(t, uint32(1), model.viewport.Y)
	require.Equal(t, layout.Point{}, model.geo.Nodes[nodeID].Rect.Min)
	staticAfter := model.geo.Nodes[staticID].Rect.Min
	require.Equal(t, staticBefore.X, staticAfter.X-model.viewport.X)
	require.Equal(t, staticBefore.Y, staticAfter.Y-model.viewport.Y)

	updateModel(t, model, tea.MouseClickMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      4,
		Y:      4,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      4,
		Y:      4,
		Button: tea.MouseLeft,
	})

	require.Equal(t, layout.NewPoint(4, 4), model.geo.Nodes[nodeID].Rect.Min)
	staticAfter = model.geo.Nodes[staticID].Rect.Min
	require.Equal(t, staticBefore.X, staticAfter.X-model.viewport.X)
	require.Equal(t, staticBefore.Y, staticAfter.Y-model.viewport.Y)
}

func TestModelDoubleClickRestoresAutoNodeSize(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 15})
	explicit := layout.Size{Width: 12, Height: 5}
	require.NoError(t, model.geo.SetNodeSize(nodeID, explicit))
	require.NoError(t, model.rebuild())
	model.history.Clear()

	point := model.geo.Nodes[nodeID].LabelPoint
	mouse := tea.Mouse{
		X:      int(point.X - model.viewport.X),
		Y:      int(point.Y - model.viewport.Y),
		Button: tea.MouseLeft,
	}
	updateModel(t, model, tea.MouseClickMsg(mouse))
	updateModel(t, model, tea.MouseReleaseMsg(mouse))
	updateModel(t, model, tea.MouseClickMsg(mouse))

	_, ok := model.geo.ExplicitNodeSize(nodeID)
	require.False(t, ok)
	require.Equal(t, layout.Size{Width: 8, Height: 3}, model.geo.Nodes[nodeID].Rect.Size)

	updateModel(t, model, keyPress('u', "u"))
	size, ok := model.geo.ExplicitNodeSize(nodeID)
	require.True(t, ok)
	require.Equal(t, explicit, size)
}

func TestModelMouseResizesNodeAsOneInteraction(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 15})
	require.NoError(t, model.geo.PlaceNode(nodeID, layout.NewPoint(2, 8)))
	require.NoError(t, model.rebuild())
	before := model.geo.Nodes[nodeID].Rect.Size
	handle := resizeCornerPoint(model.geo.Nodes[nodeID].Rect, resizeEast|resizeSouth)
	mouse := tea.Mouse{
		X:      int(handle.X - model.viewport.X),
		Y:      int(handle.Y - model.viewport.Y),
		Button: tea.MouseRight,
	}

	updateModel(t, model, tea.MouseClickMsg(mouse))
	require.True(t, model.resizing)
	updateModel(t, model, tea.MouseMotionMsg{
		X:      mouse.X + 4,
		Y:      mouse.Y + 2,
		Button: tea.MouseRight,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      mouse.X + 4,
		Y:      mouse.Y + 2,
		Button: tea.MouseRight,
	})

	require.False(t, model.resizing)
	require.Equal(t, before.Width+4, model.geo.Nodes[nodeID].Rect.Size.Width)
	require.Equal(t, before.Height+2, model.geo.Nodes[nodeID].Rect.Size.Height)
	_, explicit := model.geo.ExplicitNodeSize(nodeID)
	require.True(t, explicit)

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, before, model.geo.Nodes[nodeID].Rect.Size)
	_, explicit = model.geo.ExplicitNodeSize(nodeID)
	require.False(t, explicit)
}

func TestModelMouseResizeUsesNearestCorner(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 15})
	before := model.geo.Nodes[nodeID].Rect
	mouse := tea.Mouse{
		X:      int(before.Min.X - model.viewport.X),
		Y:      int(before.Min.Y - model.viewport.Y),
		Button: tea.MouseRight,
	}

	updateModel(t, model, tea.MouseClickMsg(mouse))
	require.Equal(t, resizeCorner(0), model.resizeCorner)
	require.Equal(
		t,
		layout.NewPoint(before.Max().X-1, before.Max().Y-1),
		model.resizeFixed,
	)
	updateModel(t, model, tea.MouseMotionMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseRight,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseRight,
	})

	require.Equal(t, layout.Point{}, model.geo.Nodes[nodeID].Rect.Min)
	require.Equal(t, before.Max(), model.geo.Nodes[nodeID].Rect.Max())

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, before, model.geo.Nodes[nodeID].Rect)
}

func TestAppendViewportRowClipsWideGrapheme(t *testing.T) {
	t.Parallel()

	got := appendViewportRow(nil, []byte("A界B"), 10, 12, 0, 2, nil)
	require.Equal(t, " B", string(got))
}

func BenchmarkModelMoveAndView(b *testing.B) {
	model, _ := newTestModel(b)
	updateModel(b, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	updateModel(b, model, keyPress(tea.KeyEnter, ""))
	keys := [...]tea.KeyPressMsg{
		keyPress(tea.KeyRight, ""),
		keyPress(tea.KeyLeft, ""),
	}

	iteration := 0
	b.ReportAllocs()
	for b.Loop() {
		model.Update(keys[iteration%len(keys)])
		benchmarkView = model.View()
		iteration++
	}
}

func newTestModel(t testing.TB) (*Model, uint32) {
	t.Helper()

	history, err := layout.NewHistory(layout.WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := layout.New(layout.WithHistory(history))
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("node", layout.NewPoint(2, 2))
	require.NoError(t, err)
	history.Clear()
	model, err := New(geo)
	require.NoError(t, err)
	return model, nodeID
}

func mustNodeStyle(t testing.TB, model *Model, nodeID uint32) layout.NodeStyle {
	t.Helper()
	style, ok := model.geo.NodeStyle(nodeID)
	require.True(t, ok)
	return style
}

func mustEdgeStyle(t testing.TB, model *Model, edgeID uint32) layout.EdgeStyle {
	t.Helper()
	style, ok := model.geo.EdgeStyle(edgeID)
	require.True(t, ok)
	return style
}

func requireSavedLabel(t testing.TB, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	doc, err := document.Unmarshal(data)
	require.NoError(t, err)
	geo, err := doc.Layout()
	require.NoError(t, err)
	require.Equal(t, want, geo.Label(0))
}

func newTwoNodeModel(t testing.TB) (*Model, uint32, uint32) {
	t.Helper()

	history, err := layout.NewHistory(layout.WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := layout.New(layout.WithHistory(history))
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(20, 2))
	require.NoError(t, err)
	history.Clear()
	model, err := New(geo)
	require.NoError(t, err)
	return model, left, right
}

func newComponentModel(t testing.TB) (*Model, uint32, uint32, uint32, uint32) {
	t.Helper()

	history, err := layout.NewHistory(layout.WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := layout.New(layout.WithHistory(history))
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	connected, err := geo.NewNodeAt("connected", layout.NewPoint(20, 2))
	require.NoError(t, err)
	isolated, err := geo.NewNodeAt("isolated", layout.NewPoint(40, 2))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, connected)
	require.NoError(t, geo.Build())
	history.Clear()
	model, err := New(geo)
	require.NoError(t, err)
	return model, left, connected, isolated, edgeID
}

func selectionContains(model *Model, kind layout.HitKind, id uint32) bool {
	return model.geo.Selection().Contains(layout.Hit{ID: id, Kind: kind})
}

func stepToward(point, destination layout.Point) layout.Point {
	switch {
	case point.X < destination.X:
		point.X++
	case point.X > destination.X:
		point.X--
	case point.Y < destination.Y:
		point.Y++
	case point.Y > destination.Y:
		point.Y--
	}
	return point
}

func frameRuneAt(t testing.TB, frame render.Frame, point layout.Point) rune {
	t.Helper()

	require.True(t, frame.Bounds.Contains(point))
	rows := strings.Split(strings.TrimSuffix(string(frame.Text), "\n"), "\n")
	y := int(point.Y - frame.Bounds.Min.Y)
	x := int(point.X - frame.Bounds.Min.X)
	runes := []rune(rows[y])
	require.Less(t, x, len(runes))
	return runes[x]
}

func modalTextPoint(
	t testing.TB,
	overlay modalview.Overlay,
	text string,
) (x, y int) {
	t.Helper()

	for row, line := range strings.Split(ansi.Strip(overlay.Content), "\n") {
		column := strings.Index(line, text)
		if column >= 0 {
			return overlay.Left + ansi.StringWidth(line[:column]) + 1,
				overlay.Top + row
		}
	}
	require.Fail(t, "modal text not rendered", text)
	return 0, 0
}

func updateModel(t testing.TB, model *Model, message tea.Msg) {
	t.Helper()

	switch message := message.(type) {
	case tea.MouseClickMsg:
		got, command := model.Update(message)
		require.Same(t, model, got)
		require.Nil(t, command)
		return
	case tea.MouseMotionMsg:
		got, command := model.Update(message)
		require.Same(t, model, got)
		require.Nil(t, command)
		return
	case tea.MouseReleaseMsg:
		got, command := model.Update(message)
		require.Same(t, model, got)
		require.Nil(t, command)
		return
	}
	got, command := model.Update(message)
	require.Same(t, model, got)
	require.Nil(t, command)
}

func updateModelCommand(t testing.TB, model *Model, message tea.Msg) tea.Cmd {
	t.Helper()

	got, command := model.Update(message)
	require.Same(t, model, got)
	return command
}

func keyPress(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}

func portOnRightBoundary(t testing.TB, model *Model, nodeID uint32) uint32 {
	return portExiting(t, model, nodeID, 1)
}

func portExiting(t testing.TB, model *Model, nodeID uint32, xDirection int) uint32 {
	t.Helper()

	rect := model.geo.Nodes[nodeID].Rect
	for i, port := range model.geo.Ports {
		if !rect.Contains(port.Anchor) {
			continue
		}
		delta := int64(port.Exit.X) - int64(port.Anchor.X)
		if delta == int64(xDirection) {
			return uint32(i)
		}
	}
	require.FailNow(t, "horizontal port not found")
	return 0
}

func selectHit(t testing.TB, model *Model, want layout.Hit) {
	t.Helper()

	switch want.Kind {
	case layout.HitNode:
		model.cursor = model.geo.Nodes[want.ID].LabelPoint
	case layout.HitPort:
		model.cursor = model.geo.Ports[want.ID].Anchor
	case layout.HitEdge:
		model.cursor = model.geo.Edges[want.ID].Points[0]
	}
	model.refreshHits()
	for i, hit := range model.hits {
		if hit == want {
			model.active = i
			return
		}
	}
	require.FailNow(t, "hit not found", "%+v", want)
}
