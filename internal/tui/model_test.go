package tui

import (
	"cmp"
	"fmt"
	"image/color"
	"math/bits"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/settings"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/internal/tui/chrome"
	modalview "github.com/coxley/dg/internal/tui/modal"
	"github.com/coxley/dg/internal/tui/nav"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

var benchmarkView tea.View

func TestNewUsesInjectedSettingsWithoutGlobalConfigLookup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative")
	history, err := layout.NewHistory(layout.WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := layout.New(layout.WithHistory(history))
	require.NoError(t, err)
	store := settings.NewStore(filepath.Join(t.TempDir(), "config.json"))

	model, err := New(geo, WithSettings(settings.Snapshot{
		ApplyToFuture: true,
		SaveDirectory: "/diagrams",
		CommentPrefix: "# ",
		ShortcutStyle: settings.ShortcutMac,
		DarkTint:      defaultDarkTint.ID,
		LightTint:     defaultLightTint.ID,
	}, store))

	require.NoError(t, err)
	require.Same(t, store, model.settingsStore)
	require.True(t, model.preferences.applyToFuture)
	require.Equal(t, "/diagrams", model.preferences.baseline.SaveDirectory)
	require.Equal(t, "# ", model.preferences.baseline.CommentPrefix)
	require.Equal(t, chrome.ProfileMac, model.preferences.baseline.KeyProfile)
	require.Equal(t, defaultDarkTint.ID, model.preferences.baseline.DarkTint)
	require.Equal(t, defaultLightTint.ID, model.preferences.baseline.LightTint)
	require.NotNil(t, model.dialogs.preferences.model)
	require.NotNil(t, model.dialogs.save.form)
	require.NotNil(t, model.dialogs.save.picker)
	require.NotNil(t, model.clipboard)
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
}

func TestNewStartsWithoutSelectionOrFocusHighlight(t *testing.T) {
	t.Parallel()

	history, err := layout.NewHistory(layout.WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := layout.New(layout.WithHistory(history))
	require.NoError(t, err)
	nodeID, err := geo.NewNode("node")
	require.NoError(t, err)
	node := layout.Hit{ID: nodeID, Kind: layout.HitNode}
	require.True(t, geo.Selection().Toggle(node))

	model, err := New(geo, testModelSettings())

	require.NoError(t, err)
	require.True(t, model.geo.Selection().Empty())
	require.Empty(t, model.hits)
	require.False(t, model.highlightedPoint(model.geo.Nodes[nodeID].Rect.Min))
}

func TestModelNavigatesFromHit(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	rightPort := portOnRightBoundary(t, model, nodeID)
	model.cursor = model.geo.Ports[rightPort].Anchor
	model.refreshHits()
	require.GreaterOrEqual(t, len(model.hits), 2)

	before := model.cursor
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, before.Add(1, 0), model.cursor)
}

func TestModelArrowMoveIsUndoable(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	before := model.geo.Nodes[nodeID].Rect.Min
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	beforeCursor := model.cursor
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	updateModel(t, model, keyPress(tea.KeyDown, ""))
	after := model.geo.Nodes[nodeID].Rect.Min
	require.Equal(t, before.Add(2, 1), after)
	require.Equal(t, beforeCursor.Add(2, 1), model.cursor)
	require.True(t, model.geo.NodeExists(nodeID))
	require.NotEmpty(t, model.canvas.Frame(canvasview.BaseFrame).Text)

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, before.Add(2, 0), model.geo.Nodes[nodeID].Rect.Min)
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
	require.Equal(t, modeRectangle, model.interaction.mode())

	updateModel(t, model, tea.MouseClickMsg{
		X:      15,
		Y:      8,
		Button: tea.MouseLeft,
	})
	nodeID := model.target.ID
	require.Equal(t, transactionRectangle, model.interaction.transaction.owner)
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

	require.Equal(t, modeNavigate, model.interaction.mode())
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
	require.Equal(t, modeRectangle, model.interaction.mode())
	updateModel(t, model, keyPress('l', "l"))
	require.Equal(t, modeConnect, model.interaction.mode())
	updateModel(t, model, keyPress('r', "r"))
	require.Equal(t, modeRectangle, model.interaction.mode())
	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, modeNavigate, model.interaction.mode())
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

	require.Equal(t, modeNavigate, model.interaction.mode())
	require.Equal(t, layout.Size{Width: 8, Height: 5}, model.geo.Nodes[nodeID].Rect.Size)
	updateModel(t, model, keyPress('u', "u"))
	require.False(t, model.geo.NodeExists(nodeID))
}

func TestModelCancelDiscardsRectangle(t *testing.T) {
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

	updateModel(t, model, keyPress(tea.KeyEscape, ""))

	require.Equal(t, modeNavigate, model.interaction.mode())
	require.False(t, model.geo.NodeExists(nodeID))
	require.False(t, model.interaction.transaction.open())
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
	model.interaction.session = interactionSession{
		kind: sessionConnection,
		connection: connectionSession{
			source: source,
		},
	}
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
	require.Equal(t, gestureDuplicatePending, model.interaction.gesture.kind)
	require.Len(t, model.geo.Graph().Nodes, 1)

	updateModel(t, model, tea.MouseMotionMsg{
		X:      click.X + 10,
		Y:      click.Y + 4,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	require.Equal(t, gestureDuplicate, model.interaction.gesture.kind)
	require.NotNil(t, model.interaction.render.duplicateLayout)
	require.NotEmpty(t, model.canvas.Frame(canvasview.DuplicateFrame).Text)
	require.Len(t, model.geo.Graph().Nodes, 1)

	updateModel(t, model, tea.MouseReleaseMsg{
		X:      click.X + 10,
		Y:      click.Y + 4,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	require.NotEqual(t, gestureDuplicate, model.interaction.gesture.kind)
	require.Nil(t, model.interaction.render.duplicateLayout)
	require.Len(t, model.geo.Graph().Nodes, 2)
	copied, ok := model.firstSelectedNode()
	require.True(t, ok)
	require.Equal(t, model.geo.Nodes[nodeID].Rect.Min.Add(10, 4), model.geo.Nodes[copied.ID].Rect.Min)

	updateModel(t, model, keyPress('u', "u"))
	require.False(t, model.geo.NodeExists(copied.ID))
}

func TestModelBlurDiscardsDuplicatePreview(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	point := model.geo.Nodes[nodeID].LabelPoint
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(point.X),
		Y:      int(point.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(point.X) + 4,
		Y:      int(point.Y) + 2,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	require.Equal(t, gestureDuplicate, model.interaction.gesture.kind)

	updateModel(t, model, tea.BlurMsg{})

	require.Equal(t, gestureNone, model.interaction.gesture.kind)
	require.Nil(t, model.interaction.render.duplicateLayout)
	require.Empty(t, model.canvas.Frame(canvasview.DuplicateFrame).Text)
	require.Len(t, model.geo.Graph().Nodes, 1)
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

	require.NotEqual(t, gestureDuplicatePending, model.interaction.gesture.kind)
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
	for id := range model.interaction.render.duplicateLayout.Selection().Edges() {
		previewEdge, ok = id, true
		break
	}
	require.True(t, ok)
	before := append(
		[]layout.Point(nil),
		model.interaction.render.duplicateLayout.Edges[previewEdge].Points...,
	)

	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(start.X) + 31,
		Y:      int(start.Y) + 12,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})

	for i, point := range before {
		require.Equal(
			t,
			point.Add(1, 2),
			model.interaction.render.duplicateLayout.Edges[previewEdge].Points[i],
		)
	}
}

func TestModelHelpAndPreferencesApplyRouterLive(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.preferences.baseline.KeyProfile = chrome.ProfileStandard
	model.bindings.SetProfile(chrome.ProfileStandard)
	updateModel(t, model, keyPress('?', "?"))
	require.True(t, model.helpInspector.visible)
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	view := ansi.Strip(model.View().Content)
	require.Contains(t, view, "HELP · canvas")
	require.Contains(t, view, "?")
	effective := model.bindings.Effective(model.activeBindingScopes())
	require.True(t, slices.ContainsFunc(effective, func(binding chrome.EffectiveBinding) bool {
		return binding.Command == commandHelp && binding.Chord == "?"
	}))
	require.True(t, slices.ContainsFunc(effective, func(binding chrome.EffectiveBinding) bool {
		return binding.Command == commandPreferences && binding.Chord == "ctrl+p"
	}))

	updateModel(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'p',
		Text: "p",
		Mod:  tea.ModCtrl,
	}))
	require.Equal(t, surfacePreferences, model.dialogs.ActiveID())
	require.Contains(t, ansi.Strip(model.View().Content), "Step cost")
	require.Contains(
		t,
		ansi.Strip(strings.Join(model.helpInspector.lines(), "\n")),
		"HELP · preferences",
	)
	before := model.geo.Router()
	command := updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, before.Costs.Step+1, model.geo.Router().Costs.Step)
	require.NotNil(t, command)
	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, before, model.geo.Router())
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.True(t, model.helpInspector.visible)
}

func TestPreferenceModalFitsShortTerminals(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
	model.openPreferences()

	require.LessOrEqual(
		t,
		model.dialogs.Overlay().Height,
		model.height,
	)
}

func TestPreferencesCloseWithQWithoutHidingHelp(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.openHelp()
	model.openPreferences()
	resolved, ok := model.bindings.Resolve("q", model.activeBindingScopes(), false)
	require.True(t, ok)
	require.Equal(t, commandBack, resolved.Command)
	back, ok := model.workspace.Back()
	require.True(t, ok)
	require.Equal(t, surfacePreferences, back)
	updateModel(t, model, keyPress('q', "q"))
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.True(t, model.helpInspector.visible)
}

func TestPreferencesPrimaryShortcutClosesAndRestoresPreview(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.preferences.baseline.KeyProfile = chrome.ProfileStandard
	model.bindings.SetProfile(chrome.ProfileStandard)
	preferencesKey := tea.KeyPressMsg(tea.Key{
		Code: 'p',
		Text: "p",
		Mod:  tea.ModCtrl,
	})
	before := model.geo.Router()

	updateModel(t, model, preferencesKey)
	require.Equal(t, surfacePreferences, model.dialogs.ActiveID())
	updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.NotEqual(t, before, model.geo.Router())

	updateModel(t, model, preferencesKey)
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.Equal(t, before, model.geo.Router())
	require.False(t, model.preferenceEdit)
	require.False(t, model.interaction.transaction.open())
}

func TestDialogControllerOwnsDistinctModalSurfacesAndScopes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		id   chrome.SurfaceID
		open func(*Model)
	}{
		{
			name: "preferences dialog",
			id:   surfacePreferences,
			open: (*Model).openPreferences,
		},
		{
			name: "save dialog",
			id:   surfaceSave,
			open: func(model *Model) {
				model.dialogs.OpenSave(model.preferences.baseline.SaveDirectory)
			},
		},
		{
			name: "export",
			id:   surfaceExport,
			open: func(model *Model) {
				model.dialogs.OpenExport()
			},
		},
		{
			name: "notice",
			id:   surfaceNotice,
			open: func(model *Model) {
				model.showNotice("Saved", surfaceNone)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			model, _ := newTestModel(t)
			updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 30})
			test.open(model)
			model.syncWorkspace()

			surface, ok := model.surfacePlan(test.id)
			require.True(t, ok)
			require.Equal(t, chrome.SurfaceModal, surface.Surface.Role)
			require.Equal(t, test.id != surfaceNotice, surface.Surface.DismissOutside)
			back, ok := model.workspace.Back()
			require.True(t, ok)
			require.Equal(t, test.id, back)
			require.Equal(t, model.dialogs.Scopes(), model.activeBindingScopes())

			model.dismissDialog()
			require.Equal(t, surfaceNone, model.dialogs.ActiveID())
		})
	}
}

func TestDialogShellMovesEveryFloatingSurface(t *testing.T) {
	t.Parallel()

	for _, id := range []chrome.SurfaceID{
		surfacePreferences,
		surfaceSave,
		surfaceExport,
		surfaceNotice,
	} {
		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 40})
		switch id {
		case surfacePreferences:
			model.openPreferences()
		case surfaceSave:
			model.dialogs.OpenSave(model.preferences.baseline.SaveDirectory)
		case surfaceExport:
			model.dialogs.OpenExport()
		case surfaceNotice:
			model.showNotice("Saved", surfaceNone)
		}
		model.syncWorkspace()
		before := model.dialogs.Overlay()
		require.False(t, model.dialogs.Fullscreen())

		updateModel(t, model, tea.MouseClickMsg{
			X:      before.Left + 2,
			Y:      before.Top,
			Button: tea.MouseLeft,
		})
		require.Equal(t, id, model.workspace.CaptureID())
		updateModel(t, model, tea.MouseMotionMsg{
			X:      before.Left + 5,
			Y:      before.Top + 2,
			Button: tea.MouseLeft,
		})

		after := model.dialogs.Overlay()
		require.Equal(t, before.Left+3, after.Left)
		require.Equal(t, before.Top+2, after.Top)
		updateModel(t, model, tea.MouseReleaseMsg{Button: tea.MouseLeft})
		require.Empty(t, model.workspace.CaptureID())
	}
}

func TestHelpInspectorMovesWithoutTakingFocus(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.openHelp()
	model.syncWorkspace()
	help, ok := model.surfacePlan(surfaceHelp)
	require.True(t, ok)
	before := help.Rect
	updateModel(t, model, tea.MouseClickMsg{
		X:      before.X + 2,
		Y:      before.Y,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      before.X - 2,
		Y:      before.Y - 2,
		Button: tea.MouseLeft,
	})
	moved, ok := model.surfacePlan(surfaceHelp)
	require.True(t, ok)
	require.Equal(t, before.X-4, moved.Rect.X)
	require.Equal(t, before.Y-2, moved.Rect.Y)
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.Equal(t, surfaceHelp, model.workspace.CaptureID())
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      moved.Rect.X,
		Y:      moved.Rect.Y,
		Button: tea.MouseLeft,
	})
	require.Empty(t, model.workspace.CaptureID())
}

func TestHelpInspectorShowsEffectiveModalBindings(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.openHelp()
	model.openPreferences()
	model.syncWorkspace()

	view := ansi.Strip(strings.Join(model.helpInspector.lines(), "\n"))
	require.Contains(t, view, "HELP · preferences")
	require.Contains(t, view, "cancel preferences")
	require.Contains(t, view, "toggle help")
}

func TestCanvasHelpShowsOnlyAvailableCommands(t *testing.T) {
	t.Parallel()

	hasCommand := func(bindings []chrome.EffectiveBinding, command chrome.CommandID) bool {
		return slices.ContainsFunc(bindings, func(binding chrome.EffectiveBinding) bool {
			return binding.Command == command
		})
	}

	model, _ := newTestModel(t)
	base := model.contextualHelpBindings()
	for _, command := range []chrome.CommandID{
		commandMoveUp,
		commandActivate,
		commandArrowEnd,
		commandLayerForward,
		commandUndo,
		commandRedo,
	} {
		require.False(t, hasCommand(base, command), command)
	}
	require.True(t, hasCommand(base, commandFocusNext))
	require.True(t, hasCommand(base, commandNewNode))
	_, hasMoveBinding := model.bindings.Resolve(
		"m",
		[]chrome.ScopeID{scopeCanvas},
		false,
	)
	require.False(t, hasMoveBinding)

	updateModel(t, model, keyPress(tea.KeyTab, ""))
	selected := model.contextualHelpBindings()
	require.True(t, hasCommand(selected, commandMoveUp))
	require.False(t, hasCommand(selected, commandLayerForward))
	require.True(t, hasCommand(selected, commandNewNode))
	require.False(t, hasCommand(selected, commandArrowEnd))

	updateModel(t, model, keyPress(tea.KeyRight, ""))
	require.True(t, hasCommand(model.contextualHelpBindings(), commandUndo))
	updateModel(t, model, keyPress('u', "u"))
	require.True(t, hasCommand(model.contextualHelpBindings(), commandRedo))

	edgeModel, left, right := newTwoNodeModel(t)
	edgeID := edgeModel.geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, edgeModel.rebuild())
	edgeModel.selectOnly(layout.Hit{ID: edgeID, Kind: layout.HitEdge})
	edgeBindings := edgeModel.contextualHelpBindings()
	require.True(t, hasCommand(edgeBindings, commandArrowEnd))
	require.True(t, hasCommand(edgeBindings, commandArrowStart))
	require.False(t, hasCommand(edgeBindings, commandMoveUp))
}

func TestLabelHelpOnlyShowsExecutableChords(t *testing.T) {
	t.Parallel()

	model := newHelpScenarioModel(t, helpLabelEditing, 1, 1)
	hasBinding := func(chord chrome.Chord, command chrome.CommandID) bool {
		return slices.ContainsFunc(
			model.contextualHelpBindings(),
			func(binding chrome.EffectiveBinding) bool {
				return binding.Chord == chord && binding.Command == command
			},
		)
	}

	require.True(t, hasBinding("esc", commandCancel))
	require.False(t, hasBinding("ctrl+enter", commandCancel))
	require.False(t, hasBinding("?", commandHelp))
	before := string(model.editBuffer)
	updateModel(t, model, keyPress('?', "?"))
	require.Equal(t, before+"?", string(model.editBuffer))

	updateModel(t, model, tea.KeyboardEnhancementsMsg{
		Flags: ansi.KittyDisambiguateEscapeCodes,
	})
	require.True(t, hasBinding("ctrl+enter", commandCancel))
	require.False(t, hasBinding("?", commandHelp))
}

func TestCanvasHelpCommandsArePerformable(t *testing.T) {
	t.Parallel()

	violations := make(map[string]struct{})
	for scenario := helpExistingDiagramBlankCell; scenario < helpScenarioCount; scenario++ {
		for _, enhanced := range []bool{false, true} {
			collectHelpCommandViolations(t, violations, scenario, 1, 1, enhanced)
		}
	}
	rapid.Check(t, func(rt *rapid.T) {
		scenario := helpScenario(
			rapid.IntRange(0, int(helpScenarioCount)-1).Draw(rt, "scenario"),
		)
		dx := rapid.IntRange(1, 4).Draw(rt, "dx")
		dy := rapid.IntRange(1, 3).Draw(rt, "dy")
		enhanced := rapid.Bool().Draw(rt, "keyboard enhancements")
		collectHelpCommandViolations(t, violations, scenario, dx, dy, enhanced)
	})
	if len(violations) == 0 {
		return
	}
	list := make([]string, 0, len(violations))
	for violation := range violations {
		list = append(list, violation)
	}
	slices.Sort(list)
	t.Fatal(strings.Join(list, "\n"))
}

func collectHelpCommandViolations(
	t testing.TB,
	violations map[string]struct{},
	scenario helpScenario,
	dx, dy int,
	enhanced bool,
) {
	t.Helper()

	model := newHelpScenarioModel(t, scenario, dx, dy)
	setHelpKeyboardEnhancements(t, model, enhanced)
	for _, binding := range model.contextualHelpBindings() {
		if !enhanced && helpChordRequiresDisambiguation(binding.Chord) {
			violations[fmt.Sprintf(
				"%s: %s advertises unsupported %s chord",
				scenario,
				binding.Command,
				binding.Chord,
			)] = struct{}{}
			continue
		}
		candidate := newHelpScenarioModel(t, scenario, dx, dy)
		setHelpKeyboardEnhancements(t, candidate, enhanced)
		candidate.status = ""
		candidate.statusError = ""
		before := snapshotHelpCommand(candidate)
		key := helpKeyPress(t, binding.Chord)
		resolved, ok := candidate.bindings.ResolveKey(
			key,
			candidate.activeBindingScopes(),
			candidate.textEntryActive(),
		)
		if !ok || resolved.Command != binding.Command {
			violations[fmt.Sprintf(
				"%s: %s resolves %s as %s",
				scenario,
				binding.Command,
				binding.Chord,
				resolved.Command,
			)] = struct{}{}
			continue
		}
		_, command := candidate.Update(key)

		switch {
		case candidate.statusError != "":
			violations[fmt.Sprintf(
				"%s: %s failed: %s",
				scenario,
				binding.Command,
				candidate.statusError,
			)] = struct{}{}
		case !helpCommandPerformed(
			candidate,
			binding.Command,
			command,
			before,
		):
			violations[fmt.Sprintf(
				"%s: %s had no effect",
				scenario,
				binding.Command,
			)] = struct{}{}
		}
	}
}

func setHelpKeyboardEnhancements(t testing.TB, model *Model, enhanced bool) {
	t.Helper()

	if !enhanced {
		return
	}
	got, command := model.Update(tea.KeyboardEnhancementsMsg{
		Flags: ansi.KittyDisambiguateEscapeCodes,
	})
	require.Same(t, model, got)
	require.Nil(t, command)
}

func helpChordRequiresDisambiguation(chord chrome.Chord) bool {
	value := string(chord)
	return strings.HasPrefix(value, "super+") ||
		value == "ctrl+enter" ||
		strings.Contains(value, "ctrl+shift+")
}

func helpKeyPress(t testing.TB, chord chrome.Chord) tea.KeyPressMsg {
	t.Helper()

	modifiers, name := testChordParts(t, chord)
	special := map[string]rune{
		"backspace": tea.KeyBackspace,
		"delete":    tea.KeyDelete,
		"down":      tea.KeyDown,
		"enter":     tea.KeyEnter,
		"esc":       tea.KeyEscape,
		"left":      tea.KeyLeft,
		"right":     tea.KeyRight,
		tabChord:    tea.KeyTab,
		"up":        tea.KeyUp,
	}
	if code, ok := special[name]; ok {
		return tea.KeyPressMsg(tea.Key{Code: code, Mod: modifiers})
	}
	runes := []rune(name)
	require.Len(t, runes, 1, "unsupported help chord %q", chord)
	text := name
	if modifiers.Contains(tea.ModCtrl) {
		text = ""
	}
	return tea.KeyPressMsg(tea.Key{
		Code: runes[0],
		Text: text,
		Mod:  modifiers,
	})
}

func helpCommandPerformed(
	model *Model,
	commandID chrome.CommandID,
	command tea.Cmd,
	before helpCommandSnapshot,
) bool {
	after := snapshotHelpCommand(model)
	switch commandID {
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
		return helpMovementCommandPerformed(model, commandID, before, after)
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
		return helpAppearanceCommandPerformed(commandID, before, after)
	case commandCopy,
		commandDelete,
		commandDuplicate,
		commandEditLabel,
		commandExpand,
		commandRedo,
		commandUndo:
		return helpEditCommandPerformed(commandID, command, before, after)
	case commandHelp,
		commandPreferences,
		commandQuit,
		commandSave,
		commandSidebar:
		return helpChromeCommandPerformed(commandID, command, before, after)
	default:
		return false
	}
}

func helpMovementCommandPerformed(
	model *Model,
	commandID chrome.CommandID,
	before, after helpCommandSnapshot,
) bool {
	switch commandID {
	case commandActivate:
		return after.edgeCount == before.edgeCount+1 &&
			after.tool == toolNavigate &&
			after.session.kind == sessionNone
	case commandCancel:
		return model.interaction.idle() &&
			after.transaction == transactionNone
	case commandFocusNext:
		return helpFocusChanged(before, after, 1)
	case commandFocusPrevious:
		return helpFocusChanged(before, after, -1)
	case commandLine:
		return after.tool == toolConnect
	case commandMoveDown:
		return helpMoved(before, after, 0, 1)
	case commandMoveLeft:
		return helpMoved(before, after, -1, 0)
	case commandMoveRight:
		return helpMoved(before, after, 1, 0)
	case commandMoveUp:
		return helpMoved(before, after, 0, -1)
	case commandNewNode:
		return after.nodeCount == before.nodeCount+1 &&
			after.session.kind == sessionLabelEdit
	case commandRectangle:
		return after.tool == toolRectangle
	default:
		return false
	}
}

func helpAppearanceCommandPerformed(
	commandID chrome.CommandID,
	before, after helpCommandSnapshot,
) bool {
	switch commandID {
	case commandArrowEnd:
		return helpEdgeStyleChanged(before, after, func(style *layout.EdgeStyle) {
			style.PortBArrow = style.PortBArrow.Next()
		})
	case commandArrowStart:
		return helpEdgeStyleChanged(before, after, func(style *layout.EdgeStyle) {
			style.PortAArrow = style.PortAArrow.Next()
		})
	case commandBorder:
		return helpNodeStyleChanged(before, after, func(style *layout.NodeStyle) {
			style.Border = style.Border.Next()
		})
	case commandDashed:
		return helpStrokeChanged(before, after)
	case commandLayerBack:
		return helpLayerChanged(before, after, true, true)
	case commandLayerBackward:
		return helpLayerChanged(before, after, false, true)
	case commandLayerForward:
		return helpLayerChanged(before, after, false, false)
	case commandLayerFront:
		return helpLayerChanged(before, after, true, false)
	case commandTextHorizontal:
		return helpNodeStyleChanged(before, after, func(style *layout.NodeStyle) {
			style.Horizontal = style.Horizontal.Next()
		})
	case commandTextVertical:
		return helpNodeStyleChanged(before, after, func(style *layout.NodeStyle) {
			style.Vertical = style.Vertical.Next()
		})
	default:
		return false
	}
}

func helpEditCommandPerformed(
	commandID chrome.CommandID,
	command tea.Cmd,
	before, after helpCommandSnapshot,
) bool {
	switch commandID {
	case commandCopy:
		return command != nil
	case commandDelete:
		return after.nodeCount+after.edgeCount < before.nodeCount+before.edgeCount
	case commandDuplicate:
		return after.nodeCount > before.nodeCount
	case commandEditLabel:
		return after.session.kind == sessionLabelEdit &&
			after.target.Kind == layout.HitNode
	case commandExpand:
		return len(after.selection) > len(before.selection)
	case commandRedo:
		return !reflect.DeepEqual(before.document, after.document) && after.canUndo
	case commandUndo:
		return !reflect.DeepEqual(before.document, after.document) && after.canRedo
	default:
		return false
	}
}

func helpChromeCommandPerformed(
	commandID chrome.CommandID,
	command tea.Cmd,
	before, after helpCommandSnapshot,
) bool {
	switch commandID {
	case commandHelp:
		return after.helpVisible != before.helpVisible
	case commandPreferences:
		return after.dialog == surfacePreferences && after.preferenceEdit
	case commandQuit:
		if command == nil {
			return false
		}
		_, ok := command().(tea.QuitMsg)
		return ok
	case commandSave:
		return after.dialog == surfaceSave
	case commandSidebar:
		return after.sidebarOpen != before.sidebarOpen ||
			after.sidebarFocused != before.sidebarFocused
	default:
		return false
	}
}

func helpFocusChanged(before, after helpCommandSnapshot, delta int) bool {
	nodes := make([]layout.Hit, 0, len(before.nodeOrigins))
	for nodeID := range before.nodeOrigins {
		nodes = append(nodes, layout.Hit{ID: nodeID, Kind: layout.HitNode})
	}
	slices.SortFunc(nodes, func(a, b layout.Hit) int {
		pa, pb := before.nodeOrigins[a.ID], before.nodeOrigins[b.ID]
		if order := cmp.Compare(pa.Y, pb.Y); order != 0 {
			return order
		}
		if order := cmp.Compare(pa.X, pb.X); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
	index := -1
	if slices.Contains(before.selection, before.target) {
		index = slices.Index(nodes, before.target)
	}
	if index < 0 && delta < 0 {
		index = 0
	}
	want := nodes[(index+delta+len(nodes))%len(nodes)]
	return after.target == want && slices.Equal(after.selection, []layout.Hit{want})
}

func helpMoved(before, after helpCommandSnapshot, dx, dy int64) bool {
	movedNode := false
	for _, hit := range before.selection {
		if hit.Kind != layout.HitNode {
			continue
		}
		want, ok := movePoint64(before.nodeOrigins[hit.ID], dx, dy)
		if !ok || after.nodeOrigins[hit.ID] != want {
			return false
		}
		movedNode = true
	}
	if movedNode {
		return true
	}
	want, ok := movePoint64(before.cursor, dx, dy)
	return ok && after.cursor == want
}

func helpNodeStyleChanged(
	before, after helpCommandSnapshot,
	change func(*layout.NodeStyle),
) bool {
	for nodeID, style := range before.nodeStyles {
		want := style
		change(&want)
		if after.nodeStyles[nodeID] == want {
			return true
		}
	}
	return false
}

func helpEdgeStyleChanged(
	before, after helpCommandSnapshot,
	change func(*layout.EdgeStyle),
) bool {
	for edgeID, style := range before.edgeStyles {
		want := style
		change(&want)
		if after.edgeStyles[edgeID] == want {
			return true
		}
	}
	return false
}

func helpStrokeChanged(before, after helpCommandSnapshot) bool {
	return helpNodeStyleChanged(before, after, func(style *layout.NodeStyle) {
		style.Stroke = style.Stroke.Toggle()
	}) || helpEdgeStyleChanged(before, after, func(style *layout.EdgeStyle) {
		style.Stroke = style.Stroke.Toggle()
	})
}

func helpLayerChanged(
	before, after helpCommandSnapshot,
	all, backward bool,
) bool {
	var target layout.Hit
	for _, hit := range before.order {
		if slices.Contains(before.selection, hit) {
			target = hit
		}
	}
	beforeIndex := slices.Index(before.order, target)
	afterIndex := slices.Index(after.order, target)
	switch {
	case all && backward:
		return afterIndex == 0
	case all:
		return afterIndex == len(after.order)-1
	case backward:
		return afterIndex == beforeIndex-1
	default:
		return afterIndex == beforeIndex+1
	}
}

type helpScenario uint8

const (
	helpExistingDiagramBlankCell helpScenario = iota
	helpNodeSelected
	helpSingleNodeSelected
	helpIsolatedNodeSelected
	helpEdgeSelected
	helpUndoAvailable
	helpRedoAvailable
	helpAreaSelection
	helpRectangleTool
	helpRectangleDrawing
	helpConnectionTool
	helpConnectionSource
	helpConnectionDestination
	helpNodeMoving
	helpNodeResizing
	helpNodeDuplicating
	helpLabelEditing
	helpScenarioCount
)

func (s helpScenario) String() string {
	switch s {
	case helpExistingDiagramBlankCell:
		return "existing diagram, blank cursor cell"
	case helpNodeSelected:
		return "node selected"
	case helpSingleNodeSelected:
		return "single node selected"
	case helpIsolatedNodeSelected:
		return "isolated node selected"
	case helpEdgeSelected:
		return "edge selected"
	case helpUndoAvailable:
		return "undo available"
	case helpRedoAvailable:
		return "redo available"
	case helpAreaSelection:
		return "area selection"
	case helpRectangleTool:
		return "rectangle tool"
	case helpRectangleDrawing:
		return "rectangle drawing"
	case helpConnectionTool:
		return "connection tool"
	case helpConnectionSource:
		return "connection source"
	case helpConnectionDestination:
		return "connection destination"
	case helpNodeMoving:
		return "node moving"
	case helpNodeResizing:
		return "node resizing"
	case helpNodeDuplicating:
		return "node duplicating"
	case helpLabelEditing:
		return "label editing"
	default:
		return fmt.Sprintf("help scenario %d", s)
	}
}

func newHelpScenarioModel(
	t testing.TB,
	scenario helpScenario,
	dx, dy int,
) *Model {
	t.Helper()

	if scenario == helpSingleNodeSelected {
		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 30})
		updateModel(t, model, keyPress(tea.KeyTab, ""))
		return model
	}

	model, left, _, isolated, edgeID := newComponentModel(t)
	for nodeID := range model.geo.Nodes {
		if !model.geo.NodeExists(uint32(nodeID)) {
			continue
		}
		node := &model.geo.Nodes[nodeID]
		require.NoError(t, model.geo.PlaceNode(
			uint32(nodeID),
			node.Rect.Min.Add(0, 8),
		))
	}
	require.NoError(t, model.rebuild())
	model.history.Clear()
	updateModel(t, model, tea.WindowSizeMsg{Width: 200, Height: 30})
	blank := layout.NewPoint(70, 20)
	focusNode := func() {
		updateModel(t, model, keyPress(tea.KeyTab, ""))
	}
	beginConnection := func() {
		updateModel(t, model, keyPress('l', "l"))
		source := portExiting(t, model, left, -1)
		updateModel(t, model, tea.MouseClickMsg{
			X:      int(model.geo.Ports[source].Anchor.X),
			Y:      int(model.geo.Ports[source].Anchor.Y),
			Button: tea.MouseLeft,
		})
		updateModel(t, model, tea.MouseReleaseMsg{
			X:      int(blank.X),
			Y:      int(blank.Y),
			Button: tea.MouseLeft,
		})
	}

	switch scenario {
	case helpExistingDiagramBlankCell:
	case helpNodeSelected:
		focusNode()
	case helpIsolatedNodeSelected:
		hit := layout.Hit{ID: isolated, Kind: layout.HitNode}
		selectHit(t, model, hit)
		model.selectOnly(hit)
	case helpEdgeSelected:
		hit := layout.Hit{ID: edgeID, Kind: layout.HitEdge}
		selectHit(t, model, hit)
		model.selectOnly(hit)
	case helpUndoAvailable:
		focusNode()
		for range dx {
			updateModel(t, model, keyPress(tea.KeyRight, ""))
		}
	case helpRedoAvailable:
		focusNode()
		updateModel(t, model, keyPress(tea.KeyRight, ""))
		updateModel(t, model, keyPress('u', "u"))
	case helpAreaSelection:
		updateModel(t, model, tea.MouseClickMsg{
			X: int(blank.X), Y: int(blank.Y), Button: tea.MouseLeft,
		})
		updateModel(t, model, tea.MouseMotionMsg{
			X: int(blank.X) + dx, Y: int(blank.Y) + dy, Button: tea.MouseLeft,
		})
	case helpRectangleTool:
		updateModel(t, model, keyPress('r', "r"))
	case helpRectangleDrawing:
		updateModel(t, model, keyPress('r', "r"))
		updateModel(t, model, tea.MouseClickMsg{
			X: int(blank.X), Y: int(blank.Y), Button: tea.MouseLeft,
		})
		updateModel(t, model, tea.MouseMotionMsg{
			X: int(blank.X) + dx, Y: int(blank.Y) + dy, Button: tea.MouseLeft,
		})
	case helpConnectionTool:
		updateModel(t, model, keyPress('l', "l"))
	case helpConnectionSource:
		beginConnection()
	case helpConnectionDestination:
		beginConnection()
		destination := portExiting(t, model, isolated, 1)
		selectHit(t, model, layout.Hit{ID: destination, Kind: layout.HitPort})
		model.refreshConnectionPreview()
	case helpNodeMoving:
		focusNode()
		point := model.geo.Nodes[model.target.ID].LabelPoint
		updateModel(t, model, tea.MouseClickMsg{
			X: int(point.X), Y: int(point.Y), Button: tea.MouseLeft,
		})
		updateModel(t, model, tea.MouseMotionMsg{
			X: int(point.X) + dx, Y: int(point.Y) + dy, Button: tea.MouseLeft,
		})
	case helpNodeResizing:
		focusNode()
		point := resizeCornerPoint(
			model.geo.Nodes[model.target.ID].Rect,
			resizeEast|resizeSouth,
		)
		updateModel(t, model, tea.MouseClickMsg{
			X: int(point.X), Y: int(point.Y), Button: tea.MouseRight,
		})
		updateModel(t, model, tea.MouseMotionMsg{
			X: int(point.X) + dx, Y: int(point.Y) + dy, Button: tea.MouseRight,
		})
	case helpNodeDuplicating:
		focusNode()
		point := model.geo.Nodes[model.target.ID].LabelPoint
		updateModel(t, model, tea.MouseClickMsg{
			X: int(point.X), Y: int(point.Y), Button: tea.MouseLeft, Mod: tea.ModAlt,
		})
		updateModel(t, model, tea.MouseMotionMsg{
			X: int(point.X) + dx, Y: int(point.Y) + dy,
			Button: tea.MouseLeft, Mod: tea.ModAlt,
		})
	case helpLabelEditing:
		focusNode()
		updateModel(t, model, keyPress('e', "e"))
	default:
		require.FailNow(t, "unknown help scenario", "%d", scenario)
	}
	return model
}

type helpCommandSnapshot struct {
	document       document.Document
	selection      []layout.Hit
	order          []layout.Hit
	nodeOrigins    map[uint32]layout.Point
	nodeStyles     map[uint32]layout.NodeStyle
	edgeStyles     map[uint32]layout.EdgeStyle
	nodeCount      int
	edgeCount      int
	cursor         layout.Point
	viewport       layout.Point
	target         layout.Hit
	active         int
	hits           []layout.Hit
	tool           activeTool
	session        interactionSession
	gesture        pointerGesture
	transaction    transactionOwner
	canUndo        bool
	canRedo        bool
	dialog         chrome.SurfaceID
	helpVisible    bool
	sidebarOpen    bool
	sidebarFocused bool
	preferenceEdit bool
}

func snapshotHelpCommand(model *Model) helpCommandSnapshot {
	selection := make([]layout.Hit, 0)
	order := slices.Collect(model.geo.DrawOrder())
	nodeOrigins := make(map[uint32]layout.Point)
	nodeStyles := make(map[uint32]layout.NodeStyle)
	edgeStyles := make(map[uint32]layout.EdgeStyle)
	nodeCount, edgeCount := 0, 0
	for hit := range model.geo.DrawOrder() {
		if model.geo.Selection().Contains(hit) {
			selection = append(selection, hit)
		}
		switch hit.Kind {
		case layout.HitNode:
			nodeCount++
			nodeOrigins[hit.ID] = model.geo.Nodes[hit.ID].Rect.Min
			nodeStyles[hit.ID], _ = model.geo.NodeStyle(hit.ID)
		case layout.HitEdge:
			edgeCount++
			edgeStyles[hit.ID], _ = model.geo.EdgeStyle(hit.ID)
		case layout.HitPort:
		}
	}
	return helpCommandSnapshot{
		document:       document.FromLayout(model.geo),
		selection:      selection,
		order:          order,
		nodeOrigins:    nodeOrigins,
		nodeStyles:     nodeStyles,
		edgeStyles:     edgeStyles,
		nodeCount:      nodeCount,
		edgeCount:      edgeCount,
		cursor:         model.cursor,
		viewport:       model.viewport,
		target:         model.target,
		active:         model.active,
		hits:           slices.Clone(model.hits),
		tool:           model.interaction.tool,
		session:        model.interaction.session,
		gesture:        model.interaction.gesture,
		transaction:    model.interaction.transaction.owner,
		canUndo:        model.history != nil && model.history.CanUndo(),
		canRedo:        model.history != nil && model.history.CanRedo(),
		dialog:         model.dialogs.ActiveID(),
		helpVisible:    model.helpInspector.visible,
		sidebarOpen:    model.sidebar.open,
		sidebarFocused: model.sidebar.focused,
		preferenceEdit: model.preferenceEdit,
	}
}

func TestHelpScrollbarSupportsPointerDrag(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.selectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.openHelp()
	model.syncWorkspace()
	help, ok := model.surfacePlan(surfaceHelp)
	require.True(t, ok)
	plan := model.helpInspector.viewport.Plan()
	require.NotEmpty(t, plan.VerticalThumb)

	x := help.Rect.X + plan.VerticalThumb.X
	y := help.Rect.Y + plan.VerticalThumb.Y
	updateModel(t, model, tea.MouseClickMsg{
		X: x, Y: y, Button: tea.MouseLeft,
	})
	require.Equal(t, surfaceHelp, model.workspace.CaptureID())
	updateModel(t, model, tea.MouseMotionMsg{
		X:      x,
		Y:      help.Rect.Y + plan.VerticalBar.Bottom() - 1,
		Button: tea.MouseLeft,
	})
	require.Positive(t, model.helpInspector.viewport.Plan().Offset.Y)
	updateModel(t, model, tea.MouseReleaseMsg{
		X: x, Y: y, Button: tea.MouseLeft,
	})
	require.Empty(t, model.workspace.CaptureID())
}

func TestPreferenceActionsAcceptMouseClicks(t *testing.T) {
	t.Parallel()

	t.Run("cancel", func(t *testing.T) {
		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 50})
		model.openPreferences()
		x, y := modalTextPoint(t, model.dialogs.Overlay(), "Cancel")

		updateModel(t, model, tea.MouseClickMsg{
			X:      x,
			Y:      y,
			Button: tea.MouseLeft,
		})

		require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	})

	t.Run("save as defaults", func(t *testing.T) {
		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 50})
		path := filepath.Join(t.TempDir(), "config.json")
		model.settingsStore = settings.NewStore(path)
		model.openPreferences()
		x, y := modalTextPoint(
			t,
			model.dialogs.Overlay(),
			"Save as Defaults",
		)

		command := updateModelCommand(t, model, tea.MouseClickMsg{
			X:      x,
			Y:      y,
			Button: tea.MouseLeft,
		})

		require.NotNil(t, command)
		data, err := os.ReadFile(path)
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
		model.updateDialog(keyPress(tea.KeyDown, ""))
	}
	command := updateModelCommand(t, model, keyPress('l', "l"))
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.True(t, model.dialogs.preferences.model.DirectoryOpen())

	updateModelCommand(t, model, keyPress(tea.KeyEscape, ""))
	require.False(t, model.dialogs.preferences.model.DirectoryOpen())
	require.Equal(t, surfacePreferences, model.dialogs.ActiveID())

	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
}

func TestPreferenceQClosesDirectoryBeforeModal(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 50})
	model.openPreferences()
	for range 7 {
		model.updateDialog(keyPress(tea.KeyDown, ""))
	}
	command := updateModelCommand(t, model, keyPress('l', "l"))
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.True(t, model.dialogs.preferences.model.DirectoryOpen())

	updateModelCommand(t, model, keyPress('q', "q"))
	require.False(t, model.dialogs.preferences.model.DirectoryOpen())
	require.Equal(t, surfacePreferences, model.dialogs.ActiveID())

	updateModel(t, model, keyPress('q', "q"))
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
}

func TestSettingsModalCanMoveAndOutsideClickCancelsPreferences(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
	model.openPreferences()
	before := model.geo.Router()
	updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.NotEqual(t, before, model.geo.Router())

	overlay := model.dialogs.Overlay()
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
	moved := model.dialogs.Overlay()
	require.Equal(t, overlay.Left+3, moved.Left)
	require.Equal(t, overlay.Top+2, moved.Top)

	command := updateModelCommand(t, model, tea.MouseClickMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	})
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.Equal(t, before, model.geo.Router())
}

func TestSettingsModalCanResize(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
	model.openPreferences()
	before := model.dialogs.Overlay()
	require.Equal(
		t,
		chrome.Rect{
			Width:  model.dialogs.plan.body.Width,
			Height: model.dialogs.plan.body.Height,
		},
		model.dialogs.preferences.bounds,
	)
	mouse := tea.Mouse{
		X:      before.Left + before.Width - 1,
		Y:      before.Top + before.Height - 1,
		Button: tea.MouseRight,
	}

	updateModel(t, model, tea.MouseClickMsg(mouse))
	mouse.X += 4
	mouse.Y += 2
	updateModel(t, model, tea.MouseMotionMsg(mouse))
	after := model.dialogs.Overlay()
	require.Equal(t, before.Width+4, after.Width)
	require.Equal(t, before.Height+2, after.Height)
	require.Equal(
		t,
		chrome.Rect{
			Width:  model.dialogs.plan.body.Width,
			Height: model.dialogs.plan.body.Height,
		},
		model.dialogs.preferences.bounds,
	)
	plan := model.dialogs.plan
	overlayLines := strings.Split(ansi.Strip(after.Content), "\n")
	require.Len(t, overlayLines, after.Height)
	screen := strings.Split(ansi.Strip(model.View().Content), "\n")
	require.Equal(t, plan, model.dialogs.plan)
	after = model.dialogs.Overlay()
	for row, overlayLine := range overlayLines {
		require.Equal(
			t,
			overlayLine,
			ansi.Cut(
				screen[after.Top+row],
				after.Left,
				after.Left+after.Width,
			),
		)
	}

	updateModel(t, model, tea.MouseReleaseMsg(mouse))
	require.False(t, model.dialogs.Resizing())
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
	require.Equal(t, 1, model.dialogs.preferences.model.FieldFlash(0))
	require.NotNil(t, command)
}

func TestPreferenceFormCanReachSaveAndCancel(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})
	model.openPreferences()
	for range 9 {
		model.updateDialog(keyPress(tea.KeyDown, ""))
	}

	view := ansi.Strip(model.dialogs.preferences.model.View().Content)
	require.Contains(t, view, "Save")
	require.Contains(t, view, "Cancel")
}

func TestShortPreferenceFormRevealsTintAndActions(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
	model.openPreferences()
	for range 9 {
		updateModel(t, model, keyPress(tea.KeyDown, ""))
	}
	require.Equal(t, chrome.ID("dark-tint"), model.dialogs.preferences.model.FocusID())
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	require.Equal(t, chrome.ID("light-tint"), model.dialogs.preferences.model.FocusID())
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	require.Equal(t, chrome.ID("save"), model.dialogs.preferences.model.FocusID())
	require.Contains(t, ansi.Strip(model.View().Content), "Save")
}

func TestPreferenceModalInterruptCancelsLiveChanges(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.openPreferences()
	before := model.geo.Router()
	updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.NotEqual(t, before, model.geo.Router())

	updateModel(t, model, tea.BlurMsg{})

	require.Equal(t, before, model.geo.Router())
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.False(t, model.preferenceEdit)
}

func TestPreferenceModalReopensWithCancelledValues(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	before := model.geo.Router()
	model.openPreferences()
	updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
	require.NotEqual(t, before, model.geo.Router())
	model.cancelPreferences()

	model.openPreferences()
	require.Equal(t, before, model.preferences.draft.Router)
	require.Equal(t, before.Costs.Step, model.dialogs.preferences.model.Value().Router.Costs.Step)
}

func TestPreferenceTintPreviewsRestoreBaselineOnCancel(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.openPreferences()
	require.True(t, model.dialogs.preferences.model.Focus("dark-tint"))
	baseline := model.preferences.baseline

	model.updateDialog(keyPress(tea.KeyRight, ""))

	require.NotEqual(t, baseline.DarkTint, model.preferences.draft.DarkTint)
	require.Equal(t, model.preferences.draft.DarkTint, model.theme.TintID)

	model.cancelPreferences()

	require.Equal(t, baseline, model.preferences.baseline)
	require.Equal(t, baseline, model.preferences.draft)
	require.Equal(t, baseline.DarkTint, model.theme.TintID)
}

func TestBackgroundColorSelectsRegisteredLightTint(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.BackgroundColorMsg{Color: color.White})
	require.Equal(t, model.preferences.baseline.LightTint, model.theme.TintID)

	model.openPreferences()
	require.True(t, model.dialogs.preferences.model.Focus("light-tint"))
	model.updateDialog(keyPress(tea.KeyRight, ""))

	require.NotEqual(
		t,
		model.preferences.baseline.LightTint,
		model.preferences.draft.LightTint,
	)
	require.Equal(t, model.preferences.draft.LightTint, model.theme.TintID)
}

func TestLiveBackgroundCommandsAndFocusReporting(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	raw, ok := model.Init()().(tea.RawMsg)
	require.True(t, ok)
	require.Equal(
		t,
		ansi.SetModeLightDark+ansi.RequestBackgroundColor,
		raw.Msg,
	)

	for _, message := range []tea.Msg{
		uv.DarkColorSchemeEvent{},
		uv.LightColorSchemeEvent{},
		tea.FocusMsg{},
	} {
		command := updateModelCommand(t, model, message)
		require.NotNil(t, command)
		require.Equal(t, tea.RequestBackgroundColor(), command())
	}
	require.True(t, model.View().ReportFocus)

	var cleanup strings.Builder
	require.NoError(t, resetLiveTint(&cleanup))
	require.Equal(t, ansi.ResetModeLightDark, cleanup.String())
}

func TestNodeDragAttachesHighlightsAndDetaches(t *testing.T) {
	t.Parallel()

	model, _, _, node, edge := newComponentModel(t)
	require.NoError(t, model.geo.BringToFront(layout.Hit{
		ID:   node,
		Kind: layout.HitNode,
	}))
	require.NoError(t, model.render())
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 30})
	point := modelEdgeMiddle(model.geo.Edges[edge].Points)
	rect := model.geo.Nodes[node].Rect
	click := model.geo.Nodes[node].LabelPoint
	targetOrigin := layout.NewPoint(point.X-2, point.Y-1)
	dx := int(targetOrigin.X) - int(rect.Min.X)
	dy := int(targetOrigin.Y) - int(rect.Min.Y)

	model.updateMouseClick(tea.Mouse{
		X:      int(click.X),
		Y:      int(click.Y),
		Button: tea.MouseLeft,
	})
	model.updateMouseMotion(tea.Mouse{
		X:      int(click.X) + dx,
		Y:      int(click.Y) + dy,
		Button: tea.MouseLeft,
	})

	require.True(
		t,
		model.interaction.gesture.hasAttachment,
		"gesture=%+v node=%+v edge=%+v point=%+v",
		model.interaction.gesture,
		model.geo.Nodes[node].Rect,
		model.geo.Edges[edge].Points,
		point,
	)
	require.Equal(t, edge, model.interaction.gesture.attachmentEdge)
	require.NotEqual(
		t,
		highlightCandidateEdge,
		model.highlightKindAt(point),
		"the hovering node owns the attachment cell",
	)
	visibleEdgePoint, ok := visibleEdgePointOutside(
		model,
		edge,
		model.geo.Nodes[node].Rect,
	)
	require.True(t, ok)
	require.Equal(
		t,
		highlightCandidateEdge,
		model.highlightKindAt(visibleEdgePoint),
	)
	require.Equal(
		t,
		model.theme.CandidateEdge,
		model.highlightStyle(highlightCandidateEdge),
	)

	model.updateMouseRelease(tea.Mouse{
		X:      int(click.X) + dx,
		Y:      int(click.Y) + dy,
		Button: tea.MouseLeft,
	})
	attachment, attached := model.geo.NodeAttachment(node)
	require.True(t, attached)
	require.Equal(t, edge, attachment.EdgeID)

	attachmentCell := model.geo.Nodes[node].Rect.Min.Add(
		attachment.Anchor.X,
		attachment.Anchor.Y,
	)
	require.True(t, model.geo.Edges[edge].Contains(attachmentCell))
	owner, ok := model.canvas.OwnerAt(canvasview.BaseFrame, attachmentCell)
	require.True(t, ok)
	require.Equal(t, layout.Hit{ID: node, Kind: layout.HitNode}, owner)
	model.selectOnly(layout.Hit{ID: edge, Kind: layout.HitEdge})
	require.False(
		t,
		model.highlightedPoint(attachmentCell),
		"the selected edge must not style an attachment-owned label cell",
	)
	visibleEdgePoint, ok = visibleEdgePointOutside(
		model,
		edge,
		model.geo.Nodes[node].Rect,
	)
	require.True(t, ok)
	require.True(t, model.highlightedPoint(visibleEdgePoint))

	route := slices.Clone(model.geo.Edges[edge].Points)
	click = attachmentCell
	model.updateMouseClick(tea.Mouse{
		X:      int(click.X),
		Y:      int(click.Y),
		Button: tea.MouseLeft,
	})
	model.updateMouseRelease(tea.Mouse{
		X:      int(click.X),
		Y:      int(click.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, attachment, mustModelAttachment(t, model, node))
	require.Equal(t, route, model.geo.Edges[edge].Points)
	require.True(t, model.geo.Selection().Contains(layout.Hit{
		ID:   node,
		Kind: layout.HitNode,
	}))
	require.Equal(t, layout.Hit{ID: node, Kind: layout.HitNode}, model.target)

	click = model.geo.Nodes[node].Rect.Min.Add(1, 0)
	model.updateMouseClick(tea.Mouse{
		X:      int(click.X),
		Y:      int(click.Y),
		Button: tea.MouseLeft,
	})
	model.updateMouseMotion(tea.Mouse{
		X:      int(click.X),
		Y:      int(click.Y) + 10,
		Button: tea.MouseLeft,
	})
	model.updateMouseRelease(tea.Mouse{
		X:      int(click.X),
		Y:      int(click.Y) + 10,
		Button: tea.MouseLeft,
	})
	_, attached = model.geo.NodeAttachment(node)
	require.False(
		t,
		attached,
		"gesture=%+v status=%q node=%+v",
		model.interaction.gesture,
		model.status,
		model.geo.Nodes[node].Rect,
	)
}

func TestAltDragDuplicateAttachesOnRelease(t *testing.T) {
	t.Parallel()

	model, _, _, node, edge := newComponentModel(t)
	require.NoError(t, model.geo.BringToFront(layout.Hit{
		ID:   node,
		Kind: layout.HitNode,
	}))
	require.NoError(t, model.render())
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 30})
	point := modelEdgeMiddle(model.geo.Edges[edge].Points)
	rect := model.geo.Nodes[node].Rect
	click := model.geo.Nodes[node].LabelPoint
	targetOrigin := layout.NewPoint(point.X-2, point.Y-1)
	dx := int(targetOrigin.X) - int(rect.Min.X)
	dy := int(targetOrigin.Y) - int(rect.Min.Y)

	model.updateMouseClick(tea.Mouse{
		X:      int(click.X),
		Y:      int(click.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	model.updateMouseMotion(tea.Mouse{
		X:      int(click.X) + dx,
		Y:      int(click.Y) + dy,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})

	require.Equal(t, gestureDuplicate, model.interaction.gesture.kind)
	require.True(t, model.interaction.gesture.hasAttachment)
	require.Equal(t, edge, model.interaction.gesture.attachmentEdge)

	model.updateMouseRelease(tea.Mouse{
		X:      int(click.X) + dx,
		Y:      int(click.Y) + dy,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})

	duplicate, ok := model.geo.Selection().FirstNode()
	require.True(t, ok)
	require.NotEqual(t, node, duplicate)
	require.Equal(t, edge, mustModelAttachment(t, model, duplicate).EdgeID)
	_, originalAttached := model.geo.NodeAttachment(node)
	require.False(t, originalAttached)

	model.undo()
	require.True(t, model.geo.NodeExists(node))
	require.False(t, model.geo.NodeExists(duplicate))
}

func mustModelAttachment(
	t testing.TB,
	model *Model,
	nodeID uint32,
) layout.Attachment {
	t.Helper()
	attachment, ok := model.geo.NodeAttachment(nodeID)
	require.True(t, ok)
	return attachment
}

func visibleEdgePointOutside(
	model *Model,
	edgeID uint32,
	rect layout.Rect,
) (layout.Point, bool) {
	points := model.geo.Edges[edgeID].Points
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		length := max(a.X, b.X) - min(a.X, b.X) +
			max(a.Y, b.Y) - min(a.Y, b.Y)
		for offset := range length + 1 {
			var point layout.Point
			switch {
			case a.X == b.X && b.Y >= a.Y:
				point = layout.NewPoint(a.X, a.Y+offset)
			case a.X == b.X:
				point = layout.NewPoint(a.X, a.Y-offset)
			case b.X >= a.X:
				point = layout.NewPoint(a.X+offset, a.Y)
			default:
				point = layout.NewPoint(a.X-offset, a.Y)
			}
			if rect.Contains(point) {
				continue
			}
			owner, ok := model.canvas.OwnerAt(canvasview.BaseFrame, point)
			if ok && owner == (layout.Hit{ID: edgeID, Kind: layout.HitEdge}) {
				return point, true
			}
		}
	}
	return layout.Point{}, false
}

func TestRejectedRigidDropsDoNotPoisonUndo(t *testing.T) {
	t.Parallel()

	model, left, right, blocker := newComponentModelWithBlocker(t)
	initial := model.geo.Nodes[left].Rect.Min
	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	model.shiftSelection(1, 0)
	committed := model.geo.Nodes[left].Rect.Min
	require.NotEqual(t, initial, committed)

	for range 2 {
		model.clearSelection()
		model.geo.Selection().Toggle(layout.Hit{ID: left, Kind: layout.HitNode})
		model.geo.Selection().Toggle(layout.Hit{ID: right, Kind: layout.HitNode})
		model.target = layout.Hit{ID: left, Kind: layout.HitNode}
		model.beginTransaction(transactionPointerMove)
		model.interaction.gesture = pointerGesture{
			kind:   gestureMove,
			target: model.target,
			rigid:  true,
		}
		dx := int64(model.geo.Nodes[blocker].Rect.Min.X) -
			int64(model.geo.Nodes[left].Rect.Min.X) + 2
		dy := int64(model.geo.Nodes[blocker].Rect.Min.Y) -
			int64(model.geo.Nodes[left].Rect.Min.Y) + 2
		require.NoError(t, model.geo.MoveSelection(dx, dy))

		model.finishMove()

		require.Equal(t, committed, model.geo.Nodes[left].Rect.Min)
		require.Contains(t, model.status, "placement rejected")
		require.NoError(t, model.geo.Build())
	}

	model.undo()
	require.Equal(t, initial, model.geo.Nodes[left].Rect.Min)
}

func TestPreferenceShortcutStyleUpdatesLiveRestoresAndPersists(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	path := filepath.Join(t.TempDir(), "config.json")
	model.settingsStore = settings.NewStore(path)
	model.openPreferences()
	for range 8 {
		model.updateDialog(keyPress(tea.KeyDown, ""))
	}
	model.updateDialog(keyPress(tea.KeyLeft, ""))
	require.Equal(t, chrome.ProfileMac, model.preferences.draft.KeyProfile)
	chord, ok := model.bindings.ChordFor(scopeGlobal, commandSave)
	require.True(t, ok)
	require.Equal(t, chrome.Chord("super+s"), chord)
	require.Contains(t, model.sidebar.declaration.Footer, "cmd+b")

	model.cancelPreferences()
	require.Equal(t, chrome.ProfileStandard, model.preferences.baseline.KeyProfile)
	chord, ok = model.bindings.ChordFor(scopeGlobal, commandSave)
	require.True(t, ok)
	require.Equal(t, chrome.Chord("ctrl+s"), chord)
	require.Contains(t, model.sidebar.declaration.Footer, "ctrl+b")

	model.openPreferences()
	for range 8 {
		model.updateDialog(keyPress(tea.KeyDown, ""))
	}
	model.updateDialog(keyPress(tea.KeyLeft, ""))
	require.True(t, model.dialogs.preferences.model.Focus("dark-tint"))
	model.updateDialog(keyPress(tea.KeyRight, ""))
	darkTint := model.preferences.draft.DarkTint
	require.NotNil(t, model.savePreferences(preferenceSaveMsg{
		Value:        model.dialogs.preferences.model.Value(),
		SaveDefaults: true,
	}))

	snapshot, err := model.settingsStore.Load()
	require.NoError(t, err)
	require.Equal(t, settings.ShortcutMac, snapshot.ShortcutStyle)
	require.Equal(t, darkTint, snapshot.DarkTint)
	require.Equal(t, model.preferences.baseline, model.preferences.draft)
	require.Equal(t, darkTint, model.preferences.baseline.DarkTint)
}

func TestPreferencesSaveShowsNotice(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	path := filepath.Join(t.TempDir(), "config.json")
	model.settingsStore = settings.NewStore(path)
	model.openPreferences()

	command := model.savePreferences(preferenceSaveMsg{
		Value:        model.dialogs.preferences.model.Value(),
		SaveDefaults: true,
	})

	require.NotNil(t, command)
	require.Equal(t, surfaceNotice, model.dialogs.ActiveID())
	require.Equal(t, "Preferences saved", model.dialogs.notice.text)
	require.FileExists(t, path)
}

func TestPreferencePersistenceFailureKeepsDraftOpen(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	model.settingsStore = settings.NewStore(filepath.Join(blocker, "config.json"))
	model.openPreferences()
	before := model.geo.Router()
	model.updateDialog(keyPress(tea.KeyRight, ""))
	require.True(t, model.dialogs.preferences.model.Focus("dark-tint"))
	model.updateDialog(keyPress(tea.KeyRight, ""))
	draft := model.dialogs.preferences.model.Value()
	require.NotEqual(t, before, draft.Router)
	require.Equal(t, draft.DarkTint, model.theme.TintID)

	command := model.savePreferences(preferenceSaveMsg{
		Value:        draft,
		SaveDefaults: true,
	})

	require.Nil(t, command)
	require.Equal(t, surfacePreferences, model.dialogs.ActiveID())
	require.True(t, model.preferenceEdit)
	require.True(t, model.interaction.transaction.open())
	require.Equal(t, draft, model.dialogs.preferences.model.Value())
	require.Equal(t, draft.Router, model.geo.Router())
	require.Equal(t, draft.DarkTint, model.theme.TintID)
	require.Contains(t, model.status, "save preferences:")

	model.cancelPreferences()
	require.Equal(t, before, model.geo.Router())
	require.Equal(t, model.preferences.baseline.DarkTint, model.theme.TintID)
}

func TestPreferencePersistenceFailurePermitsRetry(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	model.settingsStore = settings.NewStore(filepath.Join(blocker, "config.json"))
	model.openPreferences()
	model.updateDialog(keyPress(tea.KeyRight, ""))
	require.True(t, model.dialogs.preferences.model.Focus("dark-tint"))
	model.updateDialog(keyPress(tea.KeyRight, ""))
	draft := model.preferences.draft

	require.Nil(t, model.savePreferences(preferenceSaveMsg{
		Value:        draft,
		SaveDefaults: true,
	}))
	require.True(t, model.interaction.transaction.open())

	path := filepath.Join(t.TempDir(), "config.json")
	model.settingsStore = settings.NewStore(path)
	command := model.savePreferences(preferenceSaveMsg{
		Value:        draft,
		SaveDefaults: true,
	})

	require.NotNil(t, command)
	require.False(t, model.preferenceEdit)
	require.False(t, model.interaction.transaction.open())
	require.Equal(t, draft, model.preferences.baseline)
	require.Equal(t, draft.DarkTint, model.theme.TintID)
	snapshot, err := model.settingsStore.Load()
	require.NoError(t, err)
	require.Equal(t, draft.Router, snapshot.Router)
	require.Equal(t, draft.DarkTint, snapshot.DarkTint)
}

func TestNoticeExpiresOrDismissesOnKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model, _ := newTestModel(t)
		command := model.showNotice("done", surfaceNone)
		messages := make(chan tea.Msg, 1)
		go func() {
			messages <- command()
		}()

		time.Sleep(noticeDuration - time.Millisecond)
		require.Equal(t, surfaceNotice, model.dialogs.ActiveID())
		require.Empty(t, messages)
		time.Sleep(time.Millisecond)
		updateModel(t, model, <-messages)
		require.Equal(t, surfaceNone, model.dialogs.ActiveID())
		require.Empty(t, model.dialogs.notice.text)

		model.showNotice("done", surfaceNone)
		updateModel(t, model, keyPress('x', "x"))
		require.Equal(t, surfaceNone, model.dialogs.ActiveID())
		require.Empty(t, model.dialogs.notice.text)
	})
}

func TestStatusUsesRootThemeVariants(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 12})
	model.theme.Status.Error = lipgloss.NewStyle().Underline(true)
	model.setError("broken")
	require.Contains(t, model.View().Content, model.theme.Status.Error.Render("b"))

	model.theme.Status.Normal = lipgloss.NewStyle().Bold(true)
	model.status = "ready"
	require.Contains(t, model.View().Content, model.theme.Status.Normal.Render("ready"))
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

func TestModelBlurCommitsLabelEdit(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	updateModel(t, model, keyPress('e', "e"))
	updateModel(t, model, keyPress(tea.KeyBackspace, ""))
	updateModel(t, model, tea.BlurMsg{})

	require.Equal(t, modeNavigate, model.interaction.mode())
	require.Equal(t, "nod", model.geo.Label(nodeID))
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'z', Mod: tea.ModCtrl}))
	require.Equal(t, "node", model.geo.Label(nodeID))
}

func TestModelEditsLabel(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	updateModel(t, model, keyPress('e', "e"))
	require.Equal(t, modeEditLabel, model.interaction.mode())
	require.Equal(t, transactionLabelEdit, model.interaction.transaction.owner)

	updateModel(t, model, keyPress(tea.KeyBackspace, ""))
	require.Equal(t, "nod", model.geo.Label(nodeID))
	updateModel(t, model, keyPress('X', "X"))
	require.Equal(t, "nodX", model.geo.Label(nodeID))
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.Equal(t, modeEditLabel, model.interaction.mode())
	require.Equal(t, "nodX\n", model.geo.Label(nodeID))
	updateModel(t, model, tea.PasteMsg{Content: "two\nlines"})
	require.Equal(t, "nodX\ntwo\nlines", string(model.editBuffer))

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}))
	require.Equal(t, modeNavigate, model.interaction.mode())
	require.Equal(t, "nodX\ntwo\nlines", model.geo.Label(nodeID))
	require.Empty(t, model.editBuffer)
}

func TestModelEscapeCommitsLabelEdit(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	updateModel(t, model, keyPress('e', "e"))
	updateModel(t, model, keyPress(tea.KeyLeft, ""))
	updateModel(t, model, keyPress('X', "X"))
	require.Equal(t, "nodXe", model.geo.Label(nodeID))

	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, modeNavigate, model.interaction.mode())
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
	require.Equal(t, modeEditLabel, model.interaction.mode())
	require.True(t, model.geo.NodeExists(created))
	require.Equal(t, layout.Size{Width: 4, Height: 3}, model.geo.Nodes[created].Rect.Size)

	updateModel(t, model, keyPress('A', "A"))
	require.Equal(t, "A", model.geo.Label(created))
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}))
	require.Equal(t, modeNavigate, model.interaction.mode())
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
	require.Equal(t, modeNavigate, model.interaction.mode())
	updateModel(t, model, keyPress('l', "l"))
	require.Equal(t, modeConnect, model.interaction.mode())
	require.Equal(t, source, model.interaction.session.connection.source)
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
	require.Equal(t, modeNavigate, model.interaction.mode())
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
	require.Equal(t, modeConnect, model.interaction.mode())
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(source.Anchor.X),
		Y:      int(source.Anchor.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, gestureConnection, model.interaction.gesture.kind)
	require.Equal(t, sourceID, model.interaction.session.connection.source)

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
	require.Equal(t, modeNavigate, model.interaction.mode())
	require.True(t, model.geo.EdgeExists(0))
}

func TestModelBlurDiscardsConnectionPreview(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 50, Height: 15})
	source := model.geo.Ports[portExiting(t, model, left, 1)].Anchor
	destination := model.geo.Ports[portExiting(t, model, right, -1)].Anchor

	updateModel(t, model, keyPress('l', "l"))
	updateModel(t, model, tea.MouseClickMsg{
		X:      int(source.X),
		Y:      int(source.Y),
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      int(destination.X),
		Y:      int(destination.Y),
		Button: tea.MouseLeft,
	})
	require.NotEmpty(t, model.interaction.render.connectionPreview)

	updateModel(t, model, tea.BlurMsg{})

	require.Equal(t, modeNavigate, model.interaction.mode())
	require.Empty(t, model.interaction.render.connectionPreview)
	require.Empty(t, model.interaction.render.connectionRaster)
	require.Empty(t, model.geo.Edges)
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

	preview := model.interaction.render.connectionPreview
	require.NotEmpty(t, preview)
	require.Equal(t, model.geo.Ports[source].Anchor, preview[0])
	require.Equal(
		t,
		model.geo.Ports[destination].Anchor,
		preview[len(preview)-1],
	)
	var (
		join   layout.Point
		merged layout.Connections
	)
	for _, cell := range model.interaction.render.connectionRaster {
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

	preview := model.interaction.render.connectionPreview
	require.Equal(t, source.Anchor, preview[0])
	require.Equal(t, cursor, preview[len(preview)-1])
	for i := 1; i < len(preview); i++ {
		point := preview[i-1]
		for point != preview[i] {
			point = stepToward(point, preview[i])
			require.False(
				t,
				model.geo.Nodes[obstacle].Rect.Contains(point),
				"preview crosses node at %+v: %+v",
				point,
				preview,
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
	require.True(t, model.interaction.session.connection.reconnect)
	oldPort := model.interaction.session.connection.oldPort
	selectHit(t, model, layout.Hit{ID: replacement, Kind: layout.HitPort})
	updateModel(t, model, keyPress(tea.KeyEnter, ""))

	require.Equal(t, modeNavigate, model.interaction.mode())
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
	require.Equal(t, modeNavigate, model.interaction.mode())
	require.Equal(t, gestureConnectionPending, model.interaction.gesture.kind)
	require.True(t, selectionContains(model, layout.HitEdge, edgeID))
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      int(near.X),
		Y:      int(near.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, modeNavigate, model.interaction.mode())
	require.NotEqual(t, gestureConnectionPending, model.interaction.gesture.kind)
	require.Empty(t, model.interaction.render.connectionPreview)

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
	require.Equal(t, modeConnect, model.interaction.mode())
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
	require.Equal(t, modeNavigate, model.interaction.mode())
	require.NotEqual(t, sessionConnection, model.interaction.session.kind)

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
	require.Equal(t, modeConnect, model.interaction.mode())
	require.True(t, model.interaction.session.connection.reconnect)
	require.Equal(t, gestureConnection, model.interaction.gesture.kind)
	oldPort := model.interaction.session.connection.oldPort
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      int(destination.X),
		Y:      int(destination.Y),
		Button: tea.MouseLeft,
	})

	require.Equal(t, modeNavigate, model.interaction.mode())
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
	require.Equal(t, gestureConnectionPending, model.interaction.gesture.kind)
}

func TestModelDeletesNode(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress(tea.KeyTab, ""))
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
	require.False(t, model.highlightedPoint(model.geo.Nodes[nodeID].Rect.Min))
	require.False(t, model.highlightedPoint(model.geo.Nodes[nodeID].LabelPoint))
}

func TestToolbarFloatsOverCanvasAndCentersTools(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 10})
	view := model.View()
	lines := strings.Split(view.Content, "\n")
	bounds := model.nav.Bounds()
	require.NotZero(t, bounds.Width)
	require.Equal(t, (model.width-bounds.Width)/2, bounds.X)
	require.Equal(t, 1, bounds.Y)
	surface, ok := model.surfacePlan(surfaceNavigation)
	require.True(t, ok)
	require.Equal(t, bounds, surface.Rect)
	for row, toolbarLine := range model.nav.LinesFor(model.activeTool()) {
		rendered := ansi.Cut(
			lines[bounds.Y+row],
			bounds.X,
			bounds.Right(),
		)
		require.Equal(t, ansi.Strip(toolbarLine), ansi.Strip(rendered))
	}

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
	require.Equal(t, modeRectangle, model.interaction.mode())
}

func TestToolbarHighlightsHoveredTool(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	theme := model.theme
	theme.Navigation.Item = lipgloss.NewStyle()
	theme.Navigation.Hover = lipgloss.NewStyle().Underline(true)
	model.applyTheme(theme)
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
		theme.Navigation.Hover.Render(" Rectangle "),
	)
}

func TestCanvasHostOffsetsRenderingPointerAndCursor(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 15})
	require.NoError(t, model.workspace.SetSurfaces([]chrome.Surface{
		{
			ID: "dock", Role: chrome.SurfaceDock,
			Requested: chrome.Rect{Width: 8}, Visible: true,
		},
	}))
	host := model.workspace.Plan().Canvas
	require.Equal(t, chrome.Rect{X: 8, Width: 32, Height: 14}, host)

	x, y, ok := model.cursorPosition()
	require.True(t, ok)
	require.Equal(t, host.X+int(model.cursor.X-model.viewport.X), x)
	require.Equal(t, host.Y+int(model.cursor.Y-model.viewport.Y), y)
	point, ok := model.documentPoint(host.X+2, host.Y+3)
	require.True(t, ok)
	require.Equal(t, model.viewport.Add(2, 3), point)
	_, ok = model.documentPoint(host.X-1, host.Y)
	require.False(t, ok)

	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	require.Equal(t, strings.Repeat(" ", host.X), ansi.Cut(lines[2], 0, host.X))
	require.Contains(t, ansi.Cut(lines[2], host.X, host.Right()), "┌")
}

func TestKeyboardEnhancementsAdvertiseMacVocabulary(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	model.selectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.preferences.baseline.KeyProfile = chrome.ProfileMac
	model.bindings.SetProfile(chrome.ProfileMac)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.openHelp()
	require.False(t, slices.ContainsFunc(
		model.bindings.Effective(model.activeBindingScopes()),
		func(binding chrome.EffectiveBinding) bool {
			return binding.Command == commandCopy && binding.Chord == "super+c"
		},
	))

	updateModel(t, model, tea.KeyboardEnhancementsMsg{})
	require.False(t, slices.ContainsFunc(
		model.bindings.Effective(model.activeBindingScopes()),
		func(binding chrome.EffectiveBinding) bool {
			return binding.Command == commandCopy && binding.Chord == "super+c"
		},
	))
	updateModel(t, model, tea.KeyboardEnhancementsMsg{
		Flags: ansi.KittyDisambiguateEscapeCodes,
	})

	copyBinding, ok := findEffectiveBinding(
		model.bindings.Effective(model.activeBindingScopes()),
		commandCopy,
		"super+c",
	)
	require.True(t, ok)
	require.Equal(
		t,
		"cmd+c",
		chrome.DisplayChord(copyBinding.Chord, chrome.VocabularyMac),
	)
	model.helpInspector.viewport.Scroll(0, 100)
	help := ansi.Strip(strings.Join(model.helpInspector.lines(), "\n"))
	require.Contains(t, help, "cmd+c")
	require.NotContains(t, help, "super+c")
	resolved, ok := model.bindings.ResolveKey(
		tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModSuper}),
		model.activeBindingScopes(),
		false,
	)
	require.True(t, ok)
	require.Equal(t, commandCopy, resolved.Command)
}

func TestModelViewShowsCursorWhileEditing(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 12, Height: 12})
	updateModel(t, model, keyPress(tea.KeyTab, ""))
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
	model.preferences.baseline.SaveDirectory = t.TempDir()
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
	require.Equal(t, surfaceSave, model.dialogs.ActiveID())
	model.dialogs.save.SetValue(dir, defaultSaveName)
	model.handleDialogResult(model.dialogs.save.Update(chrome.FormSubmitMsg{
		ID: saveConfirmAction,
	}))

	require.Equal(t, modeNavigate, model.interaction.mode())
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.Equal(t, path, model.path)
	require.Equal(t, "saved "+path, model.status)
	requireSavedLabel(t, path, "node")

	require.NoError(t, model.geo.SetNodeLabel(nodeID, "updated"))
	require.NoError(t, model.rebuild())
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))

	require.Equal(t, modeNavigate, model.interaction.mode())
	requireSavedLabel(t, path, "updated")
}

func TestModelSaveFailureKeepsDraftOpen(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	directory := filepath.Join(t.TempDir(), "missing")
	model.dialogs.save.SetValue(directory, "draft.json")

	model.handleDialogResult(model.dialogs.save.Update(chrome.FormSubmitMsg{
		ID: saveConfirmAction,
	}))

	require.Equal(t, surfaceSave, model.dialogs.ActiveID())
	require.Equal(t, directory, model.dialogs.save.directory)
	require.Equal(t, "draft.json", model.dialogs.save.name)
	require.Empty(t, model.path)
	require.Contains(t, model.status, "save diagram:")
}

func TestModelSaveFormBrowsesRealPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "diagram-one.json"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "diagram-two.json"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".hidden"), 0o700))

	model, _ := newTestModel(t)
	model.preferences.baseline.SaveDirectory = dir
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	command := updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.NotNil(t, command)
	updateModel(t, model, command())
	require.True(t, model.dialogs.save.picker.Opened())

	views := model.View().Content
	updateModelCommand(t, model, keyPress(tea.KeyDown, ""))
	views += model.View().Content
	updateModelCommand(t, model, keyPress(tea.KeyDown, ""))
	views += model.View().Content
	require.Contains(t, views, "nested")
	require.NotContains(t, views, "diagram-one.json")
	require.NotContains(t, views, "diagram-two.json")
	require.NotContains(t, views, ".hidden")
}

func TestModelSaveTextInputTypesAndPastesWithoutParentCommands(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	require.Equal(t, saveNameField, model.dialogs.save.form.FocusID())

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	updateModel(t, model, keyPress('q', "q"))
	updateModel(t, model, tea.PasteMsg{Content: "uick.json\n"})
	require.Equal(t, "quick.json", model.dialogs.save.name)
	require.Equal(t, surfaceSave, model.dialogs.ActiveID())
	require.Contains(t, model.dialogs.save.form.AccessibleLines(), "File name: quick.json")
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
	require.Equal(t, gestureMove, model.interaction.gesture.kind)
	require.Equal(t, transactionPointerMove, model.interaction.transaction.owner)

	updateModel(t, model, tea.MouseMotionMsg{
		X:      click.X + 2,
		Y:      click.Y + 1,
		Button: tea.MouseLeft,
	})
	require.Equal(t, before.Add(2, 1), model.geo.Nodes[nodeID].Rect.Min)
	updateModel(t, model, tea.MouseReleaseMsg{Button: tea.MouseLeft})
	require.NotEqual(t, gestureMove, model.interaction.gesture.kind)
}

func TestModelBlurCommitsPointerMove(t *testing.T) {
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
	updateModel(t, model, tea.MouseMotionMsg{
		X:      click.X + 2,
		Y:      click.Y + 1,
		Button: tea.MouseLeft,
	})

	updateModel(t, model, tea.BlurMsg{})

	require.Equal(t, gestureNone, model.interaction.gesture.kind)
	require.Equal(t, before.Add(2, 1), model.geo.Nodes[nodeID].Rect.Min)
	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, before, model.geo.Nodes[nodeID].Rect.Min)
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
	require.Equal(t, gestureAreaSelection, model.interaction.gesture.kind)
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
	require.NotEqual(t, gestureAreaSelection, model.interaction.gesture.kind)
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

func TestModelMovesAndExpandsSingleControlSelectedNode(t *testing.T) {
	t.Parallel()

	model, left, connected, isolated, edgeID := newComponentModel(t)
	for nodeID := range model.geo.Nodes {
		if model.geo.NodeExists(uint32(nodeID)) {
			node := &model.geo.Nodes[nodeID]
			require.NoError(t, model.geo.PlaceNode(
				uint32(nodeID),
				node.Rect.Min.Add(0, 8),
			))
		}
	}
	require.NoError(t, model.rebuild())
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 30})
	point := model.geo.Nodes[isolated].LabelPoint
	host := model.workspace.Geometry().Canvas
	updateModel(t, model, tea.MouseClickMsg{
		X:      host.X + int(point.X-model.viewport.X),
		Y:      host.Y + int(point.Y-model.viewport.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModCtrl,
	})
	require.True(t, selectionContains(model, layout.HitNode, isolated))

	origin := model.geo.Nodes[isolated].Rect.Min
	updateModel(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, origin.Add(1, 0), model.geo.Nodes[isolated].Rect.Min)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	require.True(t, selectionContains(model, layout.HitNode, left))
	require.True(t, selectionContains(model, layout.HitNode, connected))
	require.True(t, selectionContains(model, layout.HitNode, isolated))
	require.True(t, selectionContains(model, layout.HitEdge, edgeID))
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
	require.NotEqual(t, gestureMove, model.interaction.gesture.kind)

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

	updateModel(t, model, keyPress(tea.KeyRight, ""))
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

	require.Equal(t, modeNavigate, model.interaction.mode())
	require.Equal(t, "New", model.geo.Label(nodeID))
	require.Equal(t, before.Add(2, 1), model.geo.Nodes[nodeID].Rect.Min)
	require.Equal(t, gestureMove, model.interaction.gesture.kind)
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
	require.Equal(t, gestureResize, model.interaction.gesture.kind)
	require.Equal(t, transactionResize, model.interaction.transaction.owner)
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

	require.NotEqual(t, gestureResize, model.interaction.gesture.kind)
	require.Equal(t, before.Width+4, model.geo.Nodes[nodeID].Rect.Size.Width)
	require.Equal(t, before.Height+2, model.geo.Nodes[nodeID].Rect.Size.Height)
	_, explicit := model.geo.ExplicitNodeSize(nodeID)
	require.True(t, explicit)

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, before, model.geo.Nodes[nodeID].Rect.Size)
	_, explicit = model.geo.ExplicitNodeSize(nodeID)
	require.False(t, explicit)
}

func TestModelBlurCommitsResize(t *testing.T) {
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
	updateModel(t, model, tea.MouseMotionMsg{
		X:      mouse.X + 4,
		Y:      mouse.Y + 2,
		Button: tea.MouseRight,
	})

	updateModel(t, model, tea.BlurMsg{})

	require.Equal(t, gestureNone, model.interaction.gesture.kind)
	require.Equal(t, before.Width+4, model.geo.Nodes[nodeID].Rect.Size.Width)
	require.Equal(t, before.Height+2, model.geo.Nodes[nodeID].Rect.Size.Height)
	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, before, model.geo.Nodes[nodeID].Rect.Size)
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
	require.Equal(t, resizeCorner(0), model.interaction.gesture.corner)
	require.Equal(
		t,
		layout.NewPoint(before.Max().X-1, before.Max().Y-1),
		model.interaction.gesture.fixed,
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
	updateModel(b, model, keyPress(tea.KeyTab, ""))
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
	model, err := New(geo, testModelSettings())
	require.NoError(t, err)
	return model, nodeID
}

func testModelSettings() Option {
	return WithSettings(settings.Snapshot{
		ShortcutStyle: settings.ShortcutStandard,
	}, nil)
}

func findEffectiveBinding(
	bindings []chrome.EffectiveBinding,
	command chrome.CommandID,
	chord chrome.Chord,
) (chrome.EffectiveBinding, bool) {
	for _, binding := range bindings {
		if binding.Command == command && binding.Chord == chord {
			return binding, true
		}
	}
	return chrome.EffectiveBinding{}, false
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
	model, err := New(geo, testModelSettings())
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
	model, err := New(geo, testModelSettings())
	require.NoError(t, err)
	return model, left, connected, isolated, edgeID
}

func newComponentModelWithBlocker(
	t testing.TB,
) (*Model, uint32, uint32, uint32) {
	t.Helper()

	history, err := layout.NewHistory(layout.WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := layout.New(layout.WithHistory(history))
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(20, 2))
	require.NoError(t, err)
	blocker, err := geo.NewNodeAt("blocker", layout.NewPoint(30, 15))
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeSize(blocker, layout.Size{
		Width:  50,
		Height: 20,
	}))
	geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, geo.Build())
	history.Clear()
	model, err := New(geo, testModelSettings())
	require.NoError(t, err)
	return model, left, right, blocker
}

func modelEdgeMiddle(points []layout.Point) layout.Point {
	var total uint64
	for i := 1; i < len(points); i++ {
		total += pointDistance(points[i-1], points[i])
	}
	distance := total / 2
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		segment := pointDistance(a, b)
		if distance > segment {
			distance -= segment
			continue
		}
		switch {
		case a.X == b.X && b.Y >= a.Y:
			return layout.NewPoint(a.X, a.Y+uint32(distance))
		case a.X == b.X:
			return layout.NewPoint(a.X, a.Y-uint32(distance))
		case b.X >= a.X:
			return layout.NewPoint(a.X+uint32(distance), a.Y)
		default:
			return layout.NewPoint(a.X-uint32(distance), a.Y)
		}
	}
	return points[len(points)-1]
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
