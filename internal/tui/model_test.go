package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
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

func TestModelMovesNode(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	beforeOrigin := model.geo.Nodes[nodeID].Rect.Min
	beforeCursor := model.cursor

	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.Equal(t, modeMove, model.mode)
	updateModel(t, model, keyPress(tea.KeyRight, ""))

	require.Equal(t, beforeOrigin.Add(1, 0), model.geo.Nodes[nodeID].Rect.Min)
	require.Equal(t, beforeCursor.Add(1, 0), model.cursor)
	require.True(t, model.geo.NodeExists(nodeID))
	require.NotEmpty(t, model.frame.Text)
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

func TestModelEditSupportsCaretAndRollback(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress('e', "e"))
	updateModel(t, model, keyPress(tea.KeyLeft, ""))
	updateModel(t, model, keyPress('X', "X"))
	require.Equal(t, "nodXe", model.geo.Label(nodeID))

	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.Equal(t, modeNavigate, model.mode)
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
			name:      "ctrl-w deletes previous word and whitespace",
			label:     "one two  ",
			caret:     len("one two  "),
			key:       tea.Key{Code: 'w', Mod: tea.ModCtrl},
			wantLabel: "one ",
			wantCaret: len("one "),
		},
		{
			name:      "ctrl-w preserves grapheme boundaries",
			label:     "界面 test ",
			caret:     len("界面 test "),
			key:       tea.Key{Code: 'w', Mod: tea.ModCtrl},
			wantLabel: "界面 ",
			wantCaret: len("界面 "),
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
			name:      "ctrl-a moves to line start",
			label:     oneTwo,
			caret:     len("one "),
			key:       tea.Key{Code: 'a', Mod: tea.ModCtrl},
			wantLabel: oneTwo,
			wantCaret: 0,
		},
		{
			name:      "ctrl-e moves to line end",
			label:     oneTwo,
			caret:     len("one "),
			key:       tea.Key{Code: 'e', Mod: tea.ModCtrl},
			wantLabel: oneTwo,
			wantCaret: len(oneTwo),
		},
		{
			name:      "alt-b moves to previous word",
			label:     "one  two three",
			caret:     len("one  two three"),
			key:       tea.Key{Code: 'b', Mod: tea.ModAlt},
			wantLabel: "one  two three",
			wantCaret: len("one  two "),
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

func TestModelCreatesAndCancelsNodes(t *testing.T) {
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
	cancelled := model.target.ID
	updateModel(t, model, keyPress(tea.KeyEscape, ""))
	require.False(t, model.geo.NodeExists(cancelled))
}

func TestModelConnectsSelectedPorts(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	source := portExiting(t, model, left, 1)
	destination := portExiting(t, model, right, -1)

	selectHit(t, model, layout.Hit{ID: source, Kind: layout.HitPort})
	updateModel(t, model, keyPress('c', "c"))
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
	updateModel(t, model, keyPress('c', "c"))
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

	geo, err := layout.New()
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("node", layout.NewPoint(2, 2))
	require.NoError(t, err)
	model, err := New(geo)
	require.NoError(t, err)
	return model, nodeID
}

func newTwoNodeModel(t testing.TB) (*Model, uint32, uint32) {
	t.Helper()

	geo, err := layout.New()
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(20, 2))
	require.NoError(t, err)
	model, err := New(geo)
	require.NoError(t, err)
	return model, left, right
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
