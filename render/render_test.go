package render

import (
	"slices"
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
	owners := &encoder.grid.Owners[0]
	labels := &encoder.labels[0]
	symbols := &encoder.symbols[0]
	endpoints := &encoder.endpoints[0]
	continuations := &encoder.continuations[0]

	frame, err = encoder.EncodeFrame(frame.Text[:0], geo)
	require.NoError(t, err)
	require.Same(t, cells, &encoder.grid.Cells[0])
	require.Same(t, owners, &encoder.grid.Owners[0])
	require.Same(t, labels, &encoder.labels[0])
	require.Same(t, symbols, &encoder.symbols[0])
	require.Same(t, endpoints, &encoder.endpoints[0])
	require.Same(t, continuations, &encoder.continuations[0])
	require.Equal(
		t, ""+
			"┌──────┐\n"+
			"│ node │\n"+
			"└──────┘\n",
		string(frame.Text),
	)
}

func TestUnicodeNodeBorderStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		style  layout.BorderStyle
		output string
	}{
		{
			name:  "rounded",
			style: layout.BorderRounded,
			output: "" +
				"╭──────╮\n" +
				"│ node │\n" +
				"╰──────╯\n",
		},
		{
			name:  "none",
			style: layout.BorderNone,
			output: "" +
				"        \n" +
				"  node  \n" +
				"        \n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			geo := newLayout(t)
			nodeID := newNodeAt(t, geo, "node", layout.Point{})
			require.NoError(t, geo.SetNodeStyle(nodeID, layout.NodeStyle{
				Border: test.style,
			}))
			require.NoError(t, geo.Build())

			got, err := Unicode(geo)
			require.NoError(t, err)
			require.Equal(t, test.output, got)
		})
	}
}

func TestUnicodeEdgeArrowsDoNotMutateRoute(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	source := newNodeAt(t, geo, "source", layout.Point{})
	sink := newNodeAt(t, geo, "sink", layout.Point{X: 18})
	edgeID := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, sink)
	require.NoError(t, geo.SetEdgeStyle(edgeID, layout.EdgeStyle{
		PortAArrow: layout.ArrowOpen,
		PortBArrow: layout.ArrowFilled,
	}))
	require.NoError(t, geo.Build())
	route := slices.Clone(geo.Edges[edgeID].Points)

	got, err := Unicode(geo)
	require.NoError(t, err)
	require.Contains(t, got, "│ source │◁──────▶│ sink │")
	require.Equal(t, route, geo.Edges[edgeID].Points)
}

func TestEdgeArrowsPointTowardTheirEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		points []layout.Point
		start  bool
		point  layout.Point
		glyph  rune
	}{
		{
			name:   "left endpoint",
			points: []layout.Point{{X: 2, Y: 4}, {X: 8, Y: 4}},
			start:  true,
			point:  layout.Point{X: 3, Y: 4},
			glyph:  '◀',
		},
		{
			name:   "right endpoint",
			points: []layout.Point{{X: 2, Y: 4}, {X: 8, Y: 4}},
			point:  layout.Point{X: 7, Y: 4},
			glyph:  '▶',
		},
		{
			name:   "top endpoint",
			points: []layout.Point{{X: 5, Y: 2}, {X: 5, Y: 8}},
			start:  true,
			point:  layout.Point{X: 5, Y: 3},
			glyph:  '▲',
		},
		{
			name:   "bottom endpoint",
			points: []layout.Point{{X: 5, Y: 2}, {X: 5, Y: 8}},
			point:  layout.Point{X: 5, Y: 7},
			glyph:  '▼',
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			point, anchor, ok := edgeArrowPoint(test.points, test.start)
			require.True(t, ok)
			require.Equal(t, test.point, point)
			require.Equal(t, test.glyph, arrowGlyph(
				layout.ArrowFilled,
				point,
				anchor,
			))
		})
	}
}

func TestUnicodeOccludesLowerNode(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	back := newNodeAt(t, geo, "back", layout.Point{})
	front := newNodeAt(t, geo, "front", layout.Point{})
	size := layout.Size{Width: 9, Height: 3}
	require.NoError(t, geo.SetNodeSize(back, size))
	require.NoError(t, geo.SetNodeSize(front, size))
	require.NoError(t, geo.Build())

	got, err := Unicode(geo)
	require.NoError(t, err)
	require.Equal(t, "┌───────┐\n│ front │\n└───────┘\n", got)

	require.NoError(t, geo.SendToBack(layout.Hit{
		ID:   front,
		Kind: layout.HitNode,
	}))
	got, err = Unicode(geo)
	require.NoError(t, err)
	require.Equal(t, "┌───────┐\n│ back  │\n└───────┘\n", got)
}

func TestUnicodeJoinsSharedNodeBorder(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	newNodeAt(t, geo, "foo", layout.Point{})
	newNodeAt(t, geo, "bar", layout.NewPoint(6, 0))
	require.NoError(t, geo.Build())

	got, err := Unicode(geo)
	require.NoError(t, err)
	require.Equal(
		t,
		"┌─────┬─────┐\n"+
			"│ foo │ bar │\n"+
			"└─────┴─────┘\n",
		got,
	)
}

func TestUnicodeJoinsPartiallySharedNodeBorder(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	left := newNodeAt(t, geo, "foo", layout.Point{})
	right := newNodeAt(t, geo, "bar", layout.NewPoint(6, 1))
	require.NoError(t, geo.SetNodeSize(left, layout.Size{Width: 7, Height: 4}))
	require.NoError(t, geo.SetNodeSize(right, layout.Size{Width: 7, Height: 4}))
	require.NoError(t, geo.Build())

	got, err := Unicode(geo)
	require.NoError(t, err)
	require.Equal(
		t,
		"┌─────┐      \n"+
			"│ foo ├─────┐\n"+
			"│     │ bar │\n"+
			"└─────┤     │\n"+
			"      └─────┘\n",
		got,
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

func TestUnicodeSharedEndpointArrowTerminatesBeforeNode(t *testing.T) {
	t.Parallel()

	geo := newLayout(t)
	sink := newNodeAt(t, geo, "sinks", layout.NewPoint(7, 7))
	geo.ConnectNodes(
		newNodeAt(t, geo, "foo", layout.NewPoint(4, 0)),
		ir.Bottom,
		ir.Top,
		sink,
	)
	arrowEdge := geo.ConnectNodes(
		newNodeAt(t, geo, "bar", layout.NewPoint(12, 0)),
		ir.Bottom,
		ir.Top,
		sink,
	)
	require.NoError(t, geo.SetEdgeStyle(arrowEdge, layout.EdgeStyle{
		PortBArrow: layout.ArrowFilled,
	}))
	require.NoError(t, geo.Build())

	got, err := Unicode(geo)
	require.NoError(t, err)
	require.Equal(
		t,
		"┌─────┐ ┌─────┐\n"+
			"│ foo │ │ bar │\n"+
			"└──┬──┘ └──┬──┘\n"+
			"   │       │   \n"+
			"   └───┬───┘   \n"+
			"       │       \n"+
			"       ▼       \n"+
			"   ┌───────┐   \n"+
			"   │ sinks │   \n"+
			"   └───────┘   \n",
		got,
	)
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

func BenchmarkEncoderEncode(b *testing.B) {
	for _, styled := range []bool{false, true} {
		name := "plain"
		if styled {
			name = "styled"
		}
		b.Run(name, func(b *testing.B) {
			geo := newLayout(b)
			source := newNodeAt(b, geo, "source", layout.Point{})
			sink := newNodeAt(b, geo, "sink", layout.Point{X: 18, Y: 6})
			edgeID := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, sink)
			if styled {
				require.NoError(b, geo.SetNodeStyle(source, layout.NodeStyle{
					Border: layout.BorderRounded,
				}))
				require.NoError(b, geo.SetEdgeStyle(edgeID, layout.EdgeStyle{
					PortAArrow: layout.ArrowOpen,
					PortBArrow: layout.ArrowFilled,
				}))
			}
			require.NoError(b, geo.Build())

			var encoder Encoder
			var dst []byte
			b.ReportAllocs()
			for b.Loop() {
				frame, err := encoder.EncodeFrame(dst[:0], geo)
				if err != nil {
					b.Fatal(err)
				}
				dst = frame.Text
			}
		})
	}
}
