package layout

import (
	"math"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
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
