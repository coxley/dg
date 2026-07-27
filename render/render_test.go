package render

import (
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestUnicode(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	source := newNodeAt(t, geo, "source", layout.Point{})
	sink := newNodeAt(t, geo, "sink", layout.Point{X: 18, Y: 6})
	geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, sink)
	require.NoError(t, geo.Build())
	got, err := Unicode(geo)
	require.NoError(t, err)
	want := "" +
		"┌────────┐                \n" +
		"│ source ├───────┐        \n" +
		"└────────┘       │        \n" +
		"                 │        \n" +
		"                 │        \n" +
		"                 │        \n" +
		"                 │┌──────┐\n" +
		"                 └┤ sink │\n" +
		"                  └──────┘\n"
	require.Equal(t, want, got)
}

func TestUnicodeWideLabel(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	newNodeAt(t, geo, "A界", layout.Point{})
	require.NoError(t, geo.Build())
	got, err := Unicode(geo)
	require.NoError(t, err)
	want := "┌─────┐\n│ A界 │\n└─────┘\n"
	require.Equal(t, want, got)
}

func TestUnicodeSharedEndpoint(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	sink := newNodeAt(t, geo, "sinks", layout.NewPoint(6, 6))
	geo.ConnectNodes(
		newNodeAt(t, geo, "foo", layout.NewPoint(4, 0)),
		ir.Bottom,
		ir.Top,
		sink,
	)
	geo.ConnectNodes(
		newNodeAt(t, geo, "bar", layout.NewPoint(10, 0)),
		ir.Bottom,
		ir.Top,
		sink,
	)
	require.NoError(t, geo.Build())
	got, err := Unicode(geo)
	require.NoError(t, err)
	want := "" +
		"┌─────┬─────┐\n" +
		"│ foo │ bar │\n" +
		"└──┬──┴──┬──┘\n" +
		"   │     │   \n" +
		"   │     │   \n" +
		"   └──┬──┘   \n" +
		"  ┌───┴───┐  \n" +
		"  │ sinks │  \n" +
		"  └───────┘  \n"
	require.Equal(t, want, got)
}

func newLayout(t testing.TB) *layout.Layout {
	t.Helper()

	geo, err := layout.New()
	require.NoError(t, err)
	return geo
}

func newNodeAt(
	t testing.TB,
	geo *layout.Layout,
	label string,
	point layout.Point,
) uint32 {
	t.Helper()

	nodeID, err := geo.NewNodeAt(label, point)
	require.NoError(t, err)
	return nodeID
}

func BenchmarkUnicode(b *testing.B) {
	geo := newLayout(b)
	source := newNodeAt(b, geo, "source", layout.Point{})
	sink := newNodeAt(b, geo, "sink", layout.Point{X: 18, Y: 6})
	geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, sink)
	require.NoError(b, geo.Build())

	for b.Loop() {
		_, err := Unicode(geo)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	geo := newLayout(b)
	source := newNodeAt(b, geo, "source", layout.Point{})
	sink := newNodeAt(b, geo, "sink", layout.Point{X: 18, Y: 6})
	geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, sink)
	require.NoError(b, geo.Build())

	var dst []byte
	var err error
	for b.Loop() {
		dst, err = Encode(dst[:0], geo)
		if err != nil {
			b.Fatal(err)
		}
	}
}
