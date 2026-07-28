package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func BenchmarkModelAltDrag(b *testing.B) {
	model, left, right := newTwoNodeModel(b)
	edgeID := model.geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(b, model.rebuild())
	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(b, model.geo.Selection().Toggle(layout.Hit{
		ID:   right,
		Kind: layout.HitNode,
	}))
	require.True(b, model.geo.Selection().Toggle(layout.Hit{
		ID:   edgeID,
		Kind: layout.HitEdge,
	}))
	updateModel(b, model, tea.WindowSizeMsg{Width: 120, Height: 40})
	start := model.geo.Nodes[left].LabelPoint
	updateModel(b, model, tea.MouseClickMsg{
		X:      int(start.X),
		Y:      int(start.Y),
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	updateModel(b, model, tea.MouseMotionMsg{
		X:      int(start.X) + 30,
		Y:      int(start.Y) + 10,
		Button: tea.MouseLeft,
		Mod:    tea.ModAlt,
	})
	require.True(b, model.duplicateDragging)

	messages := [...]tea.MouseMotionMsg{
		{
			X:      int(start.X) + 31,
			Y:      int(start.Y) + 11 + toolbarHeight,
			Button: tea.MouseLeft,
			Mod:    tea.ModAlt,
		},
		{
			X:      int(start.X) + 30,
			Y:      int(start.Y) + 10 + toolbarHeight,
			Button: tea.MouseLeft,
			Mod:    tea.ModAlt,
		},
	}
	iteration := 0
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		model.Update(messages[iteration%len(messages)])
		benchmarkView = model.View()
		iteration++
	}
}

func BenchmarkModelMoveCommittedDuplicate(b *testing.B) {
	model, left, right := newTwoNodeModel(b)
	edgeID := model.geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(b, model.rebuild())
	model.selectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(b, model.geo.Selection().Toggle(layout.Hit{
		ID:   right,
		Kind: layout.HitNode,
	}))
	require.True(b, model.geo.Selection().Toggle(layout.Hit{
		ID:   edgeID,
		Kind: layout.HitEdge,
	}))
	require.NoError(b, model.geo.DuplicateSelection(30, 10))
	require.NoError(b, model.rebuild())
	updateModel(b, model, tea.WindowSizeMsg{Width: 120, Height: 40})
	model.beginMove()
	require.Equal(b, modeMove, model.mode)

	keys := [...]tea.KeyPressMsg{
		keyPress(tea.KeyRight, ""),
		keyPress(tea.KeyLeft, ""),
	}
	iteration := 0
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		model.Update(keys[iteration%len(keys)])
		benchmarkView = model.View()
		iteration++
	}
}
