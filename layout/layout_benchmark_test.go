package layout

import (
	"math"
	"runtime"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

const (
	benchmarkClusterCount   = 200
	benchmarkClusterColumns = 20
	benchmarkClusterWidth   = 24
	benchmarkClusterHeight  = 16
	benchmarkClusterNodes   = 3
)

var (
	benchmarkHitCount   int
	benchmarkLabelLines []LabelLine
	benchmarkPreview    []Point
	benchmarkRaster     []RasterCell
)

func BenchmarkAppendLabelLines(b *testing.B) {
	const label = "the quick brown fox\njumps over the lazy dog"

	lines := AppendLabelLines(nil, label, 12)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		lines = AppendLabelLines(lines[:0], label, 12)
	}
	benchmarkLabelLines = lines
}

func BenchmarkLayoutBuild(b *testing.B) {
	geo, _ := newBenchmarkLayout(b)

	b.ReportAllocs()
	for b.Loop() {
		if err := geo.Build(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLayoutBuildAttachment(b *testing.B) {
	geo, err := New()
	require.NoError(b, err)
	source, err := geo.NewNodeAt("source", NewPoint(10, 10))
	require.NoError(b, err)
	destination, err := geo.NewNodeAt("destination", NewPoint(80, 10))
	require.NoError(b, err)
	node, err := geo.NewNodeAt("tag", NewPoint(40, 30))
	require.NoError(b, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(b, geo.Build())
	points := geo.Edges[edge].Points
	point, ok := routePointAtDistance(points, pathLength(points)/2)
	require.True(b, ok)
	require.NoError(b, geo.PlaceNode(node, NewPoint(point.X-1, point.Y-1)))
	require.NoError(b, geo.AttachNode(node, edge, point))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := geo.Build(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLayoutBuildSmartArrows(b *testing.B) {
	geo, _ := newBenchmarkLayout(b)
	for edgeID := range geo.Edges {
		require.NoError(b, geo.SetEdgeStyle(uint32(edgeID), EdgeStyle{
			PortBArrow: ArrowFilled,
		}))
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := geo.Build(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLayoutMoveAndBuild(b *testing.B) {
	geo, nodeID := newBenchmarkLayout(b)
	require.NoError(b, geo.Build())

	points := [...]Point{NewPoint(3, 0), NewPoint(4, 0)}
	iteration := 0
	b.ReportAllocs()
	for b.Loop() {
		if err := geo.PlaceNode(nodeID, points[iteration%len(points)]); err != nil {
			b.Fatal(err)
		}
		if err := geo.Build(); err != nil {
			b.Fatal(err)
		}
		iteration++
	}
}

func BenchmarkLayoutEditLabelAndBuild(b *testing.B) {
	geo, nodeID := newBenchmarkLayout(b)
	labels := [...]string{"foo", "longer"}
	for _, label := range labels {
		require.NoError(b, geo.SetNodeLabel(nodeID, label))
		require.NoError(b, geo.Build())
	}

	iteration := 0
	b.ReportAllocs()
	for b.Loop() {
		if err := geo.SetNodeLabel(nodeID, labels[iteration%len(labels)]); err != nil {
			b.Fatal(err)
		}
		if err := geo.Build(); err != nil {
			b.Fatal(err)
		}
		iteration++
	}
}

func BenchmarkLayoutDeleteAndConnectEdge(b *testing.B) {
	geo, nodeID := newBenchmarkLayout(b)
	edgeID := uint32(0)
	require.NoError(b, geo.DeleteEdge(edgeID))
	require.Equal(b, edgeID, geo.ConnectNodes(nodeID, ir.Bottom, ir.Top, 0))

	b.ReportAllocs()
	for b.Loop() {
		if err := geo.DeleteEdge(edgeID); err != nil {
			b.Fatal(err)
		}
		if got := geo.ConnectNodes(nodeID, ir.Bottom, ir.Top, 0); got != edgeID {
			b.Fatalf("ConnectNodes() ID = %d, want %d", got, edgeID)
		}
	}
}

func BenchmarkLayoutDeleteAndCreateNode(b *testing.B) {
	geo, nodeID := newBenchmarkLayout(b)
	point := geo.Nodes[nodeID].Rect.Min
	require.NoError(b, geo.DeleteNode(nodeID))
	reusedID, err := geo.NewNodeAt("foo", point)
	require.NoError(b, err)
	require.Equal(b, nodeID, reusedID)

	b.ReportAllocs()
	for b.Loop() {
		if err := geo.DeleteNode(nodeID); err != nil {
			b.Fatal(err)
		}
		reusedID, err := geo.NewNodeAt("foo", point)
		if err != nil {
			b.Fatal(err)
		}
		if reusedID != nodeID {
			b.Fatalf("NewNodeAt() ID = %d, want %d", reusedID, nodeID)
		}
	}
}

func BenchmarkLayoutHits(b *testing.B) {
	geo, _ := newBenchmarkLayout(b)
	require.NoError(b, geo.Build())

	tests := []struct {
		name  string
		point Point
	}{
		{name: "node", point: geo.Nodes[0].LabelPoint},
		{name: "edge", point: geo.Edges[0].Points[1]},
		{name: "miss", point: Point{X: math.MaxUint32, Y: math.MaxUint32}},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			hits := 0
			b.ReportAllocs()
			for b.Loop() {
				for range geo.Hits(test.point) {
					hits++
				}
			}
			benchmarkHitCount = hits
		})
	}
}

func BenchmarkPreviewRoute(b *testing.B) {
	geo, _ := newBenchmarkLayout(b)
	require.NoError(b, geo.Build())
	_, sourcePort, err := geo.EdgePorts(0)
	require.NoError(b, err)
	destination := NewPoint(25, 10)
	preview, err := geo.PreviewRouteWithoutEdge(
		nil,
		sourcePort,
		destination,
		0,
	)
	require.NoError(b, err)

	b.ReportAllocs()
	for b.Loop() {
		preview, err = geo.PreviewRouteWithoutEdge(
			preview[:0],
			sourcePort,
			destination,
			0,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
	benchmarkPreview = preview
}

func BenchmarkRasterizePreviewEdge(b *testing.B) {
	geo, _ := newBenchmarkLayout(b)
	require.NoError(b, geo.Build())
	base, err := RasterizeOwnedInto(nil, nil, geo)
	require.NoError(b, err)
	portA, portB, err := geo.EdgePorts(0)
	require.NoError(b, err)
	edge := RasterEdge{
		Points: geo.Edges[0].Points,
		PortA:  portA,
		PortB:  portB,
	}
	raster, err := RasterizeEdgeInto(nil, &base, geo, edge)
	require.NoError(b, err)

	b.ReportAllocs()
	for b.Loop() {
		raster, err = RasterizeEdgeInto(raster[:0], &base, geo, edge)
		if err != nil {
			b.Fatal(err)
		}
	}
	benchmarkRaster = raster
}

func BenchmarkLayoutStress(b *testing.B) {
	b.Run("footprint", func(b *testing.B) {
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)

		geo := newStressBenchmarkLayout(b)

		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(geo)

		liveBytes := uint64(0)
		if after.HeapAlloc > before.HeapAlloc {
			liveBytes = after.HeapAlloc - before.HeapAlloc
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			runtime.KeepAlive(geo)
		}
		b.ReportMetric(float64(liveBytes), "live-B")
	})

	b.Run("connect_clusters", func(b *testing.B) {
		geo := newStressBenchmarkLayout(b)
		source := stressNodeID(0, 2)
		destination := stressNodeID(1, 0)
		sourcePort, ok := geo.graph.PickCenterPort(source, ir.RightSide)
		require.True(b, ok)
		destinationPort, ok := geo.graph.PickCenterPort(
			destination,
			ir.LeftSide,
		)
		require.True(b, ok)
		destinationPoint := geo.Ports[destinationPort].Anchor
		middle := NewPoint(
			(geo.Ports[sourcePort].Anchor.X+destinationPoint.X)/2,
			(geo.Ports[sourcePort].Anchor.Y+destinationPoint.Y)/2,
		)
		cursors := [...]Point{
			geo.Ports[sourcePort].Exit,
			middle,
			destinationPoint,
		}
		var preview []Point

		connect := func() uint32 {
			for _, cursor := range cursors {
				var err error
				preview, err = geo.PreviewRoute(
					preview[:0],
					sourcePort,
					cursor,
				)
				require.NoError(b, err)
			}
			edgeID, err := geo.ConnectPorts(sourcePort, destinationPort)
			require.NoError(b, err)
			require.NoError(b, geo.Build())
			return edgeID
		}

		edgeID := connect()
		require.NoError(b, geo.DeleteEdge(edgeID))

		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			edgeID = connect()
			b.StopTimer()
			require.NoError(b, geo.DeleteEdge(edgeID))
			b.StartTimer()
		}
		runtime.KeepAlive(geo)
		benchmarkPreview = preview
	})

	b.Run("select_and_move_cluster", func(b *testing.B) {
		geo := newStressBenchmarkLayout(b)
		nodeID := stressNodeID(benchmarkClusterCount/2, 0)
		dx := int64(1)

		move := func() {
			require.True(b, geo.Selection().SelectOnly(Hit{
				ID:   nodeID,
				Kind: HitNode,
			}))
			geo.Selection().Expand()
			require.NoError(b, geo.MoveSelection(dx, 0))
			require.NoError(b, geo.BuildSelection())
			dx = -dx
		}
		move()
		move()

		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			move()
		}
		runtime.KeepAlive(geo)
	})

	b.Run("attach_new_node", func(b *testing.B) {
		geo := newStressBenchmarkLayout(b)
		const edgeID = uint32(0)
		points := geo.Edges[edgeID].Points
		point, ok := routePointAtDistance(points, pathLength(points)/4)
		require.True(b, ok)
		origin := NewPoint(point.X-1, point.Y-1)

		attach := func() uint32 {
			nodeID, err := geo.NewNodeAt("x", origin)
			require.NoError(b, err)
			require.NoError(b, geo.AttachNode(nodeID, edgeID, point))
			return nodeID
		}
		nodeID := attach()
		require.NoError(b, geo.DeleteNode(nodeID))

		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			nodeID = attach()
			b.StopTimer()
			require.NoError(b, geo.DeleteNode(nodeID))
			b.StartTimer()
		}
		runtime.KeepAlive(geo)
	})

	b.Run("edit_label_character", func(b *testing.B) {
		geo := newStressBenchmarkLayout(b)
		const original = "foo"
		const edited = "foo edited"
		nodeID := stressNodeID(0, 0)
		labels := make([]string, 0, 2*(len(edited)-len(original)))
		for end := len(original) + 1; end <= len(edited); end++ {
			labels = append(labels, edited[:end])
		}
		for end := len(edited) - 1; end >= len(original); end-- {
			labels = append(labels, edited[:end])
		}

		require.NoError(b, geo.SetNodeLabel(nodeID, edited))
		require.NoError(b, geo.Build())
		require.NoError(b, geo.SetNodeLabel(nodeID, original))
		require.NoError(b, geo.Build())

		index := 0
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			require.NoError(b, geo.SetNodeLabel(nodeID, labels[index]))
			require.NoError(b, geo.Build())
			index = (index + 1) % len(labels)
		}
		runtime.KeepAlive(geo)
	})
}

func BenchmarkLayoutHighWaterConnect(b *testing.B) {
	for _, fixture := range []struct {
		name  string
		setup func(testing.TB) *Layout
	}{
		{name: "fresh_small", setup: newSmallConnectBenchmarkLayout},
		{name: "shrunk_from_nodes", setup: newNodesHighWaterConnectBenchmarkLayout},
		{name: "shrunk_from_stress", setup: newStressHighWaterConnectBenchmarkLayout},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			geo := fixture.setup(b)

			sourcePort, ok := geo.graph.PickCenterPort(0, ir.RightSide)
			require.True(b, ok)
			destinationPort, ok := geo.graph.PickCenterPort(1, ir.LeftSide)
			require.True(b, ok)
			destination := geo.Ports[destinationPort].Anchor
			middle := NewPoint(
				(geo.Ports[sourcePort].Anchor.X+destination.X)/2,
				(geo.Ports[sourcePort].Anchor.Y+destination.Y)/2,
			)
			cursors := [...]Point{
				geo.Ports[sourcePort].Exit,
				middle,
				destination,
			}
			var preview []Point
			connect := func() uint32 {
				for _, cursor := range cursors {
					var err error
					preview, err = geo.PreviewRoute(
						preview[:0],
						sourcePort,
						cursor,
					)
					require.NoError(b, err)
				}
				edgeID, err := geo.ConnectPorts(sourcePort, destinationPort)
				require.NoError(b, err)
				require.NoError(b, geo.Build())
				return edgeID
			}

			edgeID := connect()
			require.NoError(b, geo.DeleteEdge(edgeID))

			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			liveBytes := uint64(0)
			if after.HeapAlloc > before.HeapAlloc {
				liveBytes = after.HeapAlloc - before.HeapAlloc
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				edgeID = connect()
				require.NoError(b, geo.DeleteEdge(edgeID))
			}
			runtime.KeepAlive(geo)
			benchmarkPreview = preview
			b.ReportMetric(float64(liveBytes), "live-B")
			b.ReportMetric(float64(len(geo.Nodes)), "node-slots")
			b.ReportMetric(float64(len(geo.Ports)), "port-slots")
			b.ReportMetric(float64(len(geo.Edges)), "edge-slots")
		})
	}
}

func TestLayoutBuildReusesScratch(t *testing.T) {
	geo, _ := newBenchmarkLayout(t)
	require.NoError(t, geo.Build())

	allocations := testing.AllocsPerRun(10, func() {
		require.NoError(t, geo.Build())
	})
	require.Zero(t, allocations)
}

func newBenchmarkLayout(tb testing.TB) (*Layout, uint32) {
	tb.Helper()

	geo, err := New()
	require.NoError(tb, err)
	sink, err := geo.NewNodeAt("sinks", NewPoint(7, 6))
	require.NoError(tb, err)
	moving, err := geo.NewNodeAt("foo", NewPoint(4, 0))
	require.NoError(tb, err)
	geo.ConnectNodes(
		moving,
		ir.Bottom,
		ir.Top,
		sink,
	)
	bar, err := geo.NewNodeAt("bar", NewPoint(12, 0))
	require.NoError(tb, err)
	geo.ConnectNodes(
		bar,
		ir.Bottom,
		ir.Top,
		sink,
	)
	return geo, moving
}

func newStressBenchmarkLayout(tb testing.TB) *Layout {
	tb.Helper()

	geo, err := New()
	require.NoError(tb, err)
	foo, err := geo.NewNodeAt("foo", NewPoint(4, 0))
	require.NoError(tb, err)
	bar, err := geo.NewNodeAt("bar", NewPoint(12, 0))
	require.NoError(tb, err)
	sink, err := geo.NewNodeAt("sinks", NewPoint(7, 6))
	require.NoError(tb, err)
	geo.ConnectNodes(foo, ir.Bottom, ir.Top, sink)
	geo.ConnectNodes(bar, ir.Bottom, ir.Top, sink)
	require.NoError(tb, geo.Build())

	require.True(tb, geo.Selection().SelectOnly(Hit{ID: foo, Kind: HitNode}))
	require.True(tb, geo.Selection().Toggle(Hit{ID: bar, Kind: HitNode}))
	require.True(tb, geo.Selection().Toggle(Hit{ID: sink, Kind: HitNode}))
	for cluster := 1; cluster < benchmarkClusterCount; cluster++ {
		previous := stressClusterOrigin(cluster - 1)
		current := stressClusterOrigin(cluster)
		require.NoError(tb, geo.DuplicateSelection(
			int64(current.X)-int64(previous.X),
			int64(current.Y)-int64(previous.Y),
		))
	}
	geo.Selection().Clear()

	require.Len(
		tb,
		geo.Nodes,
		benchmarkClusterCount*benchmarkClusterNodes,
	)
	require.Len(tb, geo.Edges, benchmarkClusterCount*2)
	return geo
}

func newSmallConnectBenchmarkLayout(tb testing.TB) *Layout {
	tb.Helper()

	geo, err := New()
	require.NoError(tb, err)
	_, err = geo.NewNodeAt("foo", NewPoint(4, 0))
	require.NoError(tb, err)
	_, err = geo.NewNodeAt("bar", NewPoint(12, 0))
	require.NoError(tb, err)
	require.NoError(tb, geo.Build())
	return geo
}

func newNodesHighWaterConnectBenchmarkLayout(tb testing.TB) *Layout {
	tb.Helper()

	geo := newSmallConnectBenchmarkLayout(tb)
	for nodeID := 2; nodeID < benchmarkClusterCount*benchmarkClusterNodes; nodeID++ {
		index := nodeID - 2
		_, err := geo.NewNodeAt("x", NewPoint(
			4+uint32(index%40)*12,
			10+uint32(index/40)*9,
		))
		require.NoError(tb, err)
	}
	require.NoError(tb, geo.Build())
	return shrinkConnectBenchmarkLayout(tb, geo)
}

func newStressHighWaterConnectBenchmarkLayout(tb testing.TB) *Layout {
	tb.Helper()

	geo := newStressBenchmarkLayout(tb)
	return shrinkConnectBenchmarkLayout(tb, geo)
}

func shrinkConnectBenchmarkLayout(
	tb testing.TB,
	geo *Layout,
) *Layout {
	tb.Helper()

	for nodeID := len(geo.Nodes) - 1; nodeID >= 2; nodeID-- {
		require.NoError(tb, geo.DeleteNode(uint32(nodeID)))
	}
	require.True(tb, geo.NodeExists(0))
	require.True(tb, geo.NodeExists(1))
	require.NoError(tb, geo.Build())
	return geo
}

func stressClusterOrigin(cluster int) Point {
	return NewPoint(
		uint32(cluster%benchmarkClusterColumns)*benchmarkClusterWidth,
		uint32(cluster/benchmarkClusterColumns)*benchmarkClusterHeight,
	)
}

func stressNodeID(cluster, offset int) uint32 {
	return uint32(cluster*benchmarkClusterNodes + offset)
}
