package canvas

import (
	"runtime"
	"strings"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
	"github.com/stretchr/testify/require"
)

func TestModelRetainsAndClearsFrames(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	_, err = geo.NewNodeAt("node", layout.NewPoint(2, 2))
	require.NoError(t, err)
	require.NoError(t, geo.Build())

	var model Model
	require.NoError(t, model.Render(BaseFrame, geo))
	require.NotEmpty(t, model.Frame(BaseFrame).Text)
	require.NotEmpty(t, model.Rows(BaseFrame))

	model.Clear(BaseFrame)
	require.Empty(t, model.Frame(BaseFrame).Text)
	require.Empty(t, model.Rows(BaseFrame))
}

func TestModelRowStartsAtCachedGraphemeBoundary(t *testing.T) {
	t.Parallel()

	const origin = uint32(10)
	text := strings.Repeat("a", 31) + "界" + strings.Repeat("b", 40) + "\n"
	var model Model
	model.retain(BaseFrame, render.Frame{
		Bounds: layout.Rect{
			Min:  layout.NewPoint(origin, 0),
			Size: layout.Size{Width: 73, Height: 1},
		},
		Text: []byte(text),
	})

	row, rowOrigin := model.Row(BaseFrame, 0, origin+32)
	require.Equal(t, origin, rowOrigin)
	require.Equal(t, strings.TrimSuffix(text, "\n"), string(row))

	row, rowOrigin = model.Row(BaseFrame, 0, origin+65)
	require.Equal(t, origin+65, rowOrigin)
	require.Equal(t, strings.Repeat("b", 8), string(row))
}

func BenchmarkModelRenderHighWater(b *testing.B) {
	for _, fixture := range []struct {
		name  string
		setup func(testing.TB) (*layout.Layout, *Model)
	}{
		{name: "fresh_small", setup: newSmallRenderBenchmark},
		{name: "active_many_nodes", setup: newActiveNodesRenderBenchmark},
		{name: "active_stress", setup: newActiveStressRenderBenchmark},
		{name: "shrunk_from_nodes", setup: newShrunkNodesRenderBenchmark},
		{name: "shrunk_from_stress", setup: newShrunkRenderBenchmark},
		{name: "sparse_large_bounds", setup: newSparseRenderBenchmark},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			geo, model := fixture.setup(b)

			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			liveBytes := uint64(0)
			if after.HeapAlloc > before.HeapAlloc {
				liveBytes = after.HeapAlloc - before.HeapAlloc
			}

			bounds := model.Frame(BaseFrame).Bounds
			gridCells := uint64(bounds.Size.Width) * uint64(bounds.Size.Height)
			frameCapacity := cap(model.frames[BaseFrame].frame.Text)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := model.Render(BaseFrame, geo); err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(geo)
			runtime.KeepAlive(model)
			b.ReportMetric(float64(liveBytes), "live-B")
			b.ReportMetric(float64(len(geo.Nodes)), "node-slots")
			b.ReportMetric(float64(len(geo.Edges)), "edge-slots")
			b.ReportMetric(float64(gridCells), "grid-cells")
			b.ReportMetric(float64(frameCapacity), "frame-cap-B")
		})
	}
}

func newSmallRenderBenchmark(tb testing.TB) (*layout.Layout, *Model) {
	tb.Helper()

	geo, err := layout.New()
	require.NoError(tb, err)
	_, err = geo.NewNodeAt("foo", layout.NewPoint(4, 0))
	require.NoError(tb, err)
	_, err = geo.NewNodeAt("bar", layout.NewPoint(12, 0))
	require.NoError(tb, err)
	require.NoError(tb, geo.Build())

	model := new(Model)
	require.NoError(tb, model.Render(BaseFrame, geo))
	return geo, model
}

func newActiveNodesRenderBenchmark(
	tb testing.TB,
) (*layout.Layout, *Model) {
	tb.Helper()

	geo := newCanvasNodesBenchmarkLayout(tb)
	model := new(Model)
	require.NoError(tb, model.Render(BaseFrame, geo))
	return geo, model
}

func newActiveStressRenderBenchmark(
	tb testing.TB,
) (*layout.Layout, *Model) {
	tb.Helper()

	geo := newCanvasStressBenchmarkLayout(tb)
	model := new(Model)
	require.NoError(tb, model.Render(BaseFrame, geo))
	return geo, model
}

func newShrunkNodesRenderBenchmark(tb testing.TB) (*layout.Layout, *Model) {
	tb.Helper()

	geo, model := newActiveNodesRenderBenchmark(tb)
	return shrinkRenderBenchmark(tb, geo, model)
}

func newShrunkRenderBenchmark(tb testing.TB) (*layout.Layout, *Model) {
	tb.Helper()

	geo, model := newActiveStressRenderBenchmark(tb)
	return shrinkRenderBenchmark(tb, geo, model)
}

func shrinkRenderBenchmark(
	tb testing.TB,
	geo *layout.Layout,
	model *Model,
) (*layout.Layout, *Model) {
	tb.Helper()

	for nodeID := len(geo.Nodes) - 1; nodeID >= 2; nodeID-- {
		require.NoError(tb, geo.DeleteNode(uint32(nodeID)))
	}
	require.True(tb, geo.NodeExists(0))
	require.True(tb, geo.NodeExists(1))
	require.NoError(tb, geo.Build())
	require.NoError(tb, model.Render(BaseFrame, geo))
	return geo, model
}

func newCanvasNodesBenchmarkLayout(tb testing.TB) *layout.Layout {
	tb.Helper()

	const nodeCount = 600
	geo, err := layout.New()
	require.NoError(tb, err)
	_, err = geo.NewNodeAt("foo", layout.NewPoint(4, 0))
	require.NoError(tb, err)
	_, err = geo.NewNodeAt("bar", layout.NewPoint(12, 0))
	require.NoError(tb, err)
	for nodeID := 2; nodeID < nodeCount; nodeID++ {
		index := nodeID - 2
		_, err = geo.NewNodeAt("x", layout.NewPoint(
			4+uint32(index%40)*12,
			10+uint32(index/40)*9,
		))
		require.NoError(tb, err)
	}
	require.NoError(tb, geo.Build())
	require.Len(tb, geo.Nodes, nodeCount)
	return geo
}

func newSparseRenderBenchmark(tb testing.TB) (*layout.Layout, *Model) {
	tb.Helper()

	geo, err := layout.New()
	require.NoError(tb, err)
	_, err = geo.NewNodeAt("foo", layout.NewPoint(4, 0))
	require.NoError(tb, err)
	_, err = geo.NewNodeAt("bar", layout.NewPoint(468, 148))
	require.NoError(tb, err)
	require.NoError(tb, geo.Build())

	model := new(Model)
	require.NoError(tb, model.Render(BaseFrame, geo))
	return geo, model
}

func newCanvasStressBenchmarkLayout(tb testing.TB) *layout.Layout {
	tb.Helper()

	const (
		clusterCount   = 200
		clusterColumns = 20
		clusterWidth   = 24
		clusterHeight  = 16
	)
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
	for cluster := 1; cluster < clusterCount; cluster++ {
		previousX := uint32((cluster - 1) % clusterColumns * clusterWidth)
		previousY := uint32((cluster - 1) / clusterColumns * clusterHeight)
		currentX := uint32(cluster % clusterColumns * clusterWidth)
		currentY := uint32(cluster / clusterColumns * clusterHeight)
		require.NoError(tb, geo.DuplicateSelection(
			int64(currentX)-int64(previousX),
			int64(currentY)-int64(previousY),
		))
	}
	geo.Selection().Clear()
	require.Len(tb, geo.Nodes, clusterCount*3)
	require.Len(tb, geo.Edges, clusterCount*2)
	return geo
}
