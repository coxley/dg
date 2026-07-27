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
	updateModel(t, model, keyPress('X', "X"))
	beforePaste := string(model.editBuffer)
	updateModel(t, model, tea.PasteMsg{Content: "two\nlines"})
	require.Equal(t, beforePaste, string(model.editBuffer))
	require.Equal(t, "labels currently support one line", model.status)

	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.Equal(t, modeNavigate, model.mode)
	require.Equal(t, "nodX", model.geo.Label(nodeID))
	require.Empty(t, model.editBuffer)
}

func TestModelDeletesNode(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, keyPress('d', "d"))

	require.False(t, model.geo.NodeExists(nodeID))
	require.Empty(t, model.frame.Text)
	require.Empty(t, model.hits)
}

func TestModelViewTracksWindowAndCursor(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 12, Height: 6})
	view := model.View()

	require.Equal(t, 6, strings.Count(view.Content, "\n"))
	require.NotNil(t, view.Cursor)
	require.Equal(t, int(model.cursor.X-model.viewport.X), view.Cursor.X)
	require.Equal(t, int(model.cursor.Y-model.viewport.Y), view.Cursor.Y)
	require.True(t, view.AltScreen)
}

func TestAppendViewportRowClipsWideGrapheme(t *testing.T) {
	t.Parallel()

	got := appendViewportRow(nil, []byte("A界B"), 10, 12, 2)
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
	t.Helper()

	right := model.geo.Nodes[nodeID].Rect.Max().X - 1
	for i, port := range model.geo.Ports {
		if port.Anchor.X == right {
			return uint32(i)
		}
	}
	require.FailNow(t, "right-side port not found")
	return 0
}
