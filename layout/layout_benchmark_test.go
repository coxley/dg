package layout

import (
	"math"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

var benchmarkHitCount int

func BenchmarkLayoutBuild(b *testing.B) {
	geo, _ := newBenchmarkLayout(b)

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
	sink, err := geo.NewNodeAt("sinks", NewPoint(6, 6))
	require.NoError(tb, err)
	moving, err := geo.NewNodeAt("foo", NewPoint(4, 0))
	require.NoError(tb, err)
	geo.ConnectNodes(
		moving,
		ir.Bottom,
		ir.Top,
		sink,
	)
	bar, err := geo.NewNodeAt("bar", NewPoint(10, 0))
	require.NoError(tb, err)
	geo.ConnectNodes(
		bar,
		ir.Bottom,
		ir.Top,
		sink,
	)
	return geo, moving
}
