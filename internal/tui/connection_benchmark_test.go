package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

const (
	connectionBenchmarkClusters = 200
	connectionBenchmarkColumns  = 20
	connectionBenchmarkWidth    = 24
	connectionBenchmarkHeight   = 16
)

func BenchmarkModelConnectionPreviewHighWater(b *testing.B) {
	for _, fixture := range []struct {
		name   string
		stress bool
		shrink bool
	}{
		{name: "fresh"},
		{name: "active_stress", stress: true},
		{name: "shrunk_from_stress", stress: true, shrink: true},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			model, source, destination := newConnectionBenchmarkModel(
				b,
				fixture.stress,
				fixture.shrink,
			)
			updateModel(b, model, tea.WindowSizeMsg{Width: 120, Height: 40})
			updateModel(b, model, keyPress('l', "l"))
			sourcePoint := model.geo.Ports[source].Anchor
			updateModel(b, model, tea.MouseClickMsg{
				X:      int(sourcePoint.X),
				Y:      int(sourcePoint.Y),
				Button: tea.MouseLeft,
			})

			destinationPoint := model.geo.Ports[destination].Anchor
			middle := layout.NewPoint(
				(sourcePoint.X+destinationPoint.X)/2,
				(sourcePoint.Y+destinationPoint.Y)/2+2,
			)
			messages := [...]tea.MouseMotionMsg{
				{
					X:      int(middle.X),
					Y:      int(middle.Y),
					Button: tea.MouseLeft,
				},
				{
					X:      int(destinationPoint.X),
					Y:      int(destinationPoint.Y),
					Button: tea.MouseLeft,
				},
			}
			for _, message := range messages {
				updateModel(b, model, message)
			}
			require.NotEmpty(b, model.interaction.render.connectionPreview)

			iteration := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				model.Update(messages[iteration%len(messages)])
				benchmarkView = model.View()
				iteration++
			}
			b.StopTimer()
			b.ReportMetric(float64(len(model.geo.Nodes)), "node-slots")
			b.ReportMetric(float64(len(model.geo.Ports)), "port-slots")
			b.ReportMetric(float64(len(model.geo.Edges)), "edge-slots")
		})
	}
}

func BenchmarkModelHorizontalScrollHighWater(b *testing.B) {
	model, _, _ := newConnectionBenchmarkModel(b, true, false)
	updateModel(b, model, tea.WindowSizeMsg{Width: 120, Height: 40})
	model.viewport.X = connectionBenchmarkOrigin(
		connectionBenchmarkClusters - 1,
	).X
	messages := [...]tea.MouseWheelMsg{
		{Button: tea.MouseWheelLeft},
		{Button: tea.MouseWheelRight},
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

func BenchmarkModelSelectionHighWater(b *testing.B) {
	b.Run("select_all", func(b *testing.B) {
		model, _, _ := newConnectionBenchmarkModel(b, true, false)
		updateModel(b, model, tea.WindowSizeMsg{Width: 120, Height: 40})

		b.ReportAllocs()
		for b.Loop() {
			b.StopTimer()
			model.clearSelection()
			b.StartTimer()

			model.expandSelection()
			benchmarkView = model.View()
		}
	})
	b.Run("deselect_all", func(b *testing.B) {
		model, _, _ := newConnectionBenchmarkModel(b, true, false)
		updateModel(b, model, tea.WindowSizeMsg{Width: 120, Height: 40})
		blank := layout.NewPoint(0, 14)
		click := tea.MouseClickMsg{
			X:      int(blank.X),
			Y:      int(blank.Y),
			Button: tea.MouseLeft,
		}
		release := tea.MouseReleaseMsg(click)

		b.ReportAllocs()
		for b.Loop() {
			b.StopTimer()
			model.geo.Selection().SelectAll()
			b.StartTimer()

			model.Update(click)
			model.Update(release)
			benchmarkView = model.View()
		}
	})
}

func BenchmarkModelDragAllHighWater(b *testing.B) {
	model, _, _ := newConnectionBenchmarkModel(b, true, false)
	updateModel(b, model, tea.WindowSizeMsg{Width: 120, Height: 40})
	model.geo.Selection().SelectAll()
	source := model.geo.Nodes[0].Rect.Min
	updateModel(b, model, tea.MouseClickMsg{
		X:      int(source.X),
		Y:      int(source.Y),
		Button: tea.MouseLeft,
	})
	require.True(b, model.interaction.movingRigidly())
	messages := [...]tea.MouseMotionMsg{
		{
			X:      int(source.X + 1),
			Y:      int(source.Y),
			Button: tea.MouseLeft,
		},
		{
			X:      int(source.X),
			Y:      int(source.Y),
			Button: tea.MouseLeft,
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

func newConnectionBenchmarkModel(
	tb testing.TB,
	stress, shrink bool,
) (*Model, uint32, uint32) {
	tb.Helper()

	geo, err := layout.New()
	require.NoError(tb, err)
	foo, err := geo.NewNodeAt("foo", layout.NewPoint(4, 0))
	require.NoError(tb, err)
	bar, err := geo.NewNodeAt("bar", layout.NewPoint(12, 0))
	require.NoError(tb, err)
	sink, err := geo.NewNodeAt("sinks", layout.NewPoint(7, 6))
	require.NoError(tb, err)
	geo.ConnectNodes(foo, ir.Bottom, ir.Top, sink)
	geo.ConnectNodes(bar, ir.Bottom, ir.Top, sink)
	require.NoError(tb, geo.Build())

	if stress {
		require.True(tb, geo.Selection().SelectOnly(layout.Hit{
			ID:   foo,
			Kind: layout.HitNode,
		}))
		require.True(tb, geo.Selection().Toggle(layout.Hit{
			ID:   bar,
			Kind: layout.HitNode,
		}))
		require.True(tb, geo.Selection().Toggle(layout.Hit{
			ID:   sink,
			Kind: layout.HitNode,
		}))
		for cluster := 1; cluster < connectionBenchmarkClusters; cluster++ {
			previous := connectionBenchmarkOrigin(cluster - 1)
			current := connectionBenchmarkOrigin(cluster)
			require.NoError(tb, geo.DuplicateSelection(
				int64(current.X)-int64(previous.X),
				int64(current.Y)-int64(previous.Y),
			))
		}
		geo.Selection().Clear()
	}
	if shrink {
		for nodeID := len(geo.Nodes) - 1; nodeID >= 3; nodeID-- {
			require.NoError(tb, geo.DeleteNode(uint32(nodeID)))
		}
		require.NoError(tb, geo.Build())
	}

	model, err := New(geo, testModelSettings())
	require.NoError(tb, err)
	return model,
		portExiting(tb, model, foo, 1),
		portExiting(tb, model, bar, -1)
}

func connectionBenchmarkOrigin(cluster int) layout.Point {
	return layout.NewPoint(
		uint32(cluster%connectionBenchmarkColumns*connectionBenchmarkWidth),
		uint32(cluster/connectionBenchmarkColumns*connectionBenchmarkHeight),
	)
}
