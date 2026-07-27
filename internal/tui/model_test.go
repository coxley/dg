package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/document"
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
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	require.Equal(t, 1, model.active)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
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
	require.NotEmpty(t, model.frame.Text)

	updateModel(t, model, keyPress('u', "u"))
	require.Equal(t, before, model.geo.Nodes[nodeID].Rect.Min)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'r', Mod: tea.ModCtrl}))
	require.Equal(t, after, model.geo.Nodes[nodeID].Rect.Min)
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
	beforePaste := string(model.editBuffer)
	updateModel(t, model, tea.PasteMsg{Content: "two\nlines"})
	require.Equal(t, beforePaste, string(model.editBuffer))
	require.Equal(t, "labels currently support one line", model.status)

	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.Equal(t, modeNavigate, model.mode)
	require.Equal(t, "nodX", model.geo.Label(nodeID))
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
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
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

	updateModel(t, model, tea.MouseReleaseMsg{
		X:      int(destination.Anchor.X),
		Y:      int(destination.Anchor.Y),
		Button: tea.MouseLeft,
	})
	require.Equal(t, modeNavigate, model.mode)
	require.True(t, model.geo.EdgeExists(0))
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
	require.Zero(t, model.connectPreviewLen)

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
	require.NotEqual(t, ' ', frameRuneAt(t, model.frame, near))
	require.Equal(t, ' ', frameRuneAt(t, model.connectFrame, near))
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
	updateModel(t, model, keyPress('d', "d"))

	require.False(t, model.geo.NodeExists(nodeID))
	require.Empty(t, model.frame.Text)
	require.Empty(t, model.hits)
}

func TestModelViewTracksWindowWithoutCursor(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 12, Height: 6})
	view := model.View()

	require.Equal(t, 6, strings.Count(view.Content, "\n"))
	require.Nil(t, view.Cursor)
	require.True(t, view.AltScreen)
	require.Equal(t, tea.MouseModeCellMotion, view.MouseMode)
	require.Contains(t, view.Content, selectionStart)
	require.Contains(t, view.Content, selectionEnd)
	require.False(t, model.highlightedPoint(model.geo.Nodes[nodeID].LabelPoint))
}

func TestModelViewShowsCursorWhileEditing(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 12, Height: 6})
	updateModel(t, model, keyPress('e', "e"))
	view := model.View()

	require.NotNil(t, view.Cursor)
	require.Equal(t, int(model.cursor.X-model.viewport.X), view.Cursor.X)
	require.Equal(t, int(model.cursor.Y-model.viewport.Y), view.Cursor.Y)
	require.NotSame(t, view.Cursor, model.View().Cursor)
}

func TestModelViewShowsSavePathPrompt(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 6})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	updateModel(t, model, tea.PasteMsg{Content: "diagram.json"})
	view := model.View()

	require.Contains(t, view.Content, "save path: diagram.json")
	require.NotNil(t, view.Cursor)
	require.Equal(t, len("save path: diagram.json"), view.Cursor.X)
	require.Equal(t, model.diagramHeight(), view.Cursor.Y)
}

func TestModelSavePathShortcuts(t *testing.T) {
	t.Parallel()

	const path = "界面/one/two"
	model, _ := newTestModel(t)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	updateModel(t, model, tea.PasteMsg{Content: path})

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'b', Mod: tea.ModAlt}))
	require.Equal(t, len("界面/one/"), model.editCaret)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'w', Mod: tea.ModCtrl}))
	require.Equal(t, "界面/two", string(model.editBuffer))
	require.Equal(t, len("界面/"), model.editCaret)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'e', Mod: tea.ModCtrl}))
	require.Equal(t, len("界面/two"), model.editCaret)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	require.Zero(t, model.editCaret)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'e', Mod: tea.ModCtrl}))
	require.Equal(t, len("界面/two"), model.editCaret)

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	require.Empty(t, model.editBuffer)
	require.Zero(t, model.editCaret)
}

func TestModelSavesWithPathPromptAndReusesPath(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	path := filepath.Join(t.TempDir(), "diagram.json")

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	require.Equal(t, modeSavePath, model.mode)
	updateModel(t, model, tea.PasteMsg{Content: path})
	updateModel(t, model, keyPress(tea.KeyEnter, ""))

	require.Equal(t, modeNavigate, model.mode)
	require.Equal(t, path, model.path)
	require.Equal(t, "saved "+path, model.status)
	requireSavedLabel(t, path, "node")

	require.NoError(t, model.geo.SetNodeLabel(nodeID, "updated"))
	require.NoError(t, model.rebuild())
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))

	require.Equal(t, modeNavigate, model.mode)
	requireSavedLabel(t, path, "updated")
}

func TestModelCompletesSavePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "diagram-one.json"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "diagram-two.json"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o700))

	model, _ := newTestModel(t)
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	updateModel(t, model, tea.PasteMsg{Content: filepath.Join(dir, "dia")})
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	require.Equal(t, filepath.Join(dir, "diagram-"), string(model.editBuffer))
	require.Contains(t, model.saveHint, "diagram-one.json")
	require.Contains(t, model.saveHint, "diagram-two.json")

	updateModel(t, model, keyPress('o', "o"))
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	require.Equal(t, filepath.Join(dir, "diagram-one.json"), string(model.editBuffer))
	require.Empty(t, model.saveHint)

	model.editBuffer = append(model.editBuffer[:0], filepath.Join(dir, "nes")...)
	model.editCaret = len(model.editBuffer)
	updateModel(t, model, keyPress(tea.KeyTab, ""))
	require.Equal(
		t,
		filepath.Join(dir, "nested")+string(filepath.Separator),
		string(model.editBuffer),
	)
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
	require.Contains(t, model.View().Content, selectionStart+" ")

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
	updateModel(t, model, keyPress('d', "d"))
	require.False(t, model.geo.NodeExists(left))
	require.False(t, model.geo.NodeExists(connected))
	require.True(t, model.geo.NodeExists(isolated))
	require.False(t, model.geo.EdgeExists(edgeID))

	updateModel(t, model, keyPress('u', "u"))
	require.True(t, model.geo.NodeExists(left))
	require.True(t, model.geo.NodeExists(connected))
	require.True(t, model.geo.EdgeExists(edgeID))
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

func updateModel(t testing.TB, model *Model, message tea.Msg) {
	t.Helper()

	got, command := model.Update(message)
	require.Same(t, model, got)
	require.Nil(t, command)
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
