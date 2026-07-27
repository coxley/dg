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

func TestEncoderReusesScratch(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	newNodeAt(t, geo, "node", layout.Point{})
	require.NoError(t, geo.Build())

	var encoder Encoder
	frame, err := encoder.EncodeFrame(nil, geo)
	require.NoError(t, err)
	cells := &encoder.grid.Cells[0]
	labels := &encoder.labels[0]
	continuations := &encoder.continuations[0]

	frame, err = encoder.EncodeFrame(frame.Text[:0], geo)
	require.NoError(t, err)
	require.Same(t, cells, &encoder.grid.Cells[0])
	require.Same(t, labels, &encoder.labels[0])
	require.Same(t, continuations, &encoder.continuations[0])
	require.Equal(
		t, ""+
			"┌──────┐\n"+
			"│ node │\n"+
			"└──────┘\n",
		string(frame.Text),
	)
}

func TestUnicodeWideLabel(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	newNodeAt(t, geo, "A界", layout.Point{})
	require.NoError(t, geo.Build())
	got, err := Unicode(geo)
	require.NoError(t, err)
	want := "" +
		"┌─────┐\n" +
		"│ A界 │\n" +
		"└─────┘\n"
	require.Equal(t, want, got)
}

func TestUnicodeMultilineAutoSize(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	newNodeAt(t, geo, "one\nthree", layout.Point{})
	require.NoError(t, geo.Build())

	got, err := Unicode(geo)
	require.NoError(t, err)
	require.Equal(
		t,
		"┌───────┐\n"+
			"│ one   │\n"+
			"│ three │\n"+
			"└───────┘\n",
		got,
	)
}

func TestUnicodeWrapsAndClipsExplicitNodeWithoutChangingLabel(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	nodeID := newNodeAt(t, geo, "one two three", layout.Point{})
	require.NoError(t, geo.SetNodeSize(nodeID, layout.Size{Width: 8, Height: 4}))
	require.NoError(t, geo.Build())

	got, err := Unicode(geo)
	require.NoError(t, err)
	require.Equal(
		t,
		"┌──────┐\n"+
			"│ one  │\n"+
			"│ two  │\n"+
			"└──────┘\n",
		got,
	)
	require.Equal(t, "one two three", geo.Label(nodeID))
}

func TestEncodeFrameIncludesBounds(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	newNodeAt(t, geo, "node", layout.NewPoint(4, 7))
	require.NoError(t, geo.Build())

	frame, err := EncodeFrame(nil, geo)
	require.NoError(t, err)
	require.Equal(t, layout.Rect{
		Min:  layout.NewPoint(4, 7),
		Size: layout.Size{Width: 8, Height: 3},
	}, frame.Bounds)
	require.Equal(
		t, ""+
			"┌──────┐\n"+
			"│ node │\n"+
			"└──────┘\n",
		string(frame.Text),
	)
}

func TestUnicodeSharedEndpoint(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	sink := newNodeAt(t, geo, "sinks", layout.NewPoint(7, 6))
	geo.ConnectNodes(
		newNodeAt(t, geo, "foo", layout.NewPoint(4, 0)),
		ir.Bottom,
		ir.Top,
		sink,
	)
	geo.ConnectNodes(
		newNodeAt(t, geo, "bar", layout.NewPoint(12, 0)),
		ir.Bottom,
		ir.Top,
		sink,
	)
	require.NoError(t, geo.Build())
	got, err := Unicode(geo)
	require.NoError(t, err)
	want := "" +
		"┌─────┐ ┌─────┐\n" +
		"│ foo │ │ bar │\n" +
		"└──┬──┘ └──┬──┘\n" +
		"   │       │   \n" +
		"   │       │   \n" +
		"   └───┬───┘   \n" +
		"   ┌───┴───┐   \n" +
		"   │ sinks │   \n" +
		"   └───────┘   \n"
	require.Equal(t, want, got)
}

func TestUnicodeOmitsDeletedNode(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	kept := newNodeAt(t, geo, "kept", layout.Point{})
	deleted := newNodeAt(t, geo, "deleted", layout.Point{X: 12})
	geo.ConnectNodes(kept, ir.RightSide, ir.LeftSide, deleted)
	require.NoError(t, geo.DeleteNode(deleted))
	require.NoError(t, geo.Build())

	got, err := Unicode(geo)
	require.NoError(t, err)
	require.Equal(
		t, ""+
			"┌──────┐\n"+
			"│ kept │\n"+
			"└──────┘\n",
		got,
	)
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
