package layout

import (
	"errors"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestMeasureLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		want    Size
		wantErr error
	}{
		{
			name: "ASCII",
			text: "source",
			want: Size{Width: 6, Height: 1},
		},
		{
			name: "wide rune",
			text: "A界",
			want: Size{Width: 3, Height: 1},
		},
		{
			name:    "newline",
			text:    "first\nsecond",
			wantErr: ErrMultilineLabel,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := MeasureLabel(test.text)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("MeasureLabel(%q) error = %v, want %v", test.text, err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("MeasureLabel(%q) = %+v, want %+v", test.text, got, test.want)
			}
		})
	}
}

func TestNewRect(t *testing.T) {
	t.Parallel()

	origin := Point{X: 2, Y: 3}
	limit := origin.Add(5, 7)
	got, err := NewRect(origin, limit)
	if err != nil {
		t.Fatal(err)
	}
	want := Rect{
		Min:  origin,
		Size: Size{Width: 5, Height: 7},
	}
	if got != want {
		t.Errorf("NewRect() = %+v, want %+v", got, want)
	}
	if got.Contains(limit) {
		t.Errorf("NewRect() contains exclusive limit %+v", limit)
	}
}

func TestNodeRect(t *testing.T) {
	t.Parallel()

	got, err := NodeRect(
		Point{X: 2, Y: 3},
		Size{Width: 6, Height: 1},
		Padding{Top: 1, Right: 2, Bottom: 3, Left: 4},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := Rect{
		Min:  Point{X: 2, Y: 3},
		Size: Size{Width: 14, Height: 7},
	}
	if got != want {
		t.Errorf("NodeRect() = %+v, want %+v", got, want)
	}
	if !got.Contains(Point{X: 15, Y: 9}) {
		t.Error("NodeRect() does not contain its final cell")
	}
	if got.Contains(got.Max()) {
		t.Error("NodeRect() contains its exclusive maximum")
	}
}

func TestResolvePort(t *testing.T) {
	t.Parallel()

	rect := Rect{
		Min:  Point{X: 10, Y: 20},
		Size: Size{Width: 5, Height: 3},
	}
	tests := []struct {
		name   string
		side   ir.Side
		offset float32
		want   Port
	}{
		{
			name:   "top left",
			side:   ir.Top,
			offset: 0,
			want:   Port{Anchor: Point{X: 10, Y: 20}, Exit: Point{X: 10, Y: 19}},
		},
		{
			name:   "right center",
			side:   ir.RightSide,
			offset: 0.5,
			want:   Port{Anchor: Point{X: 14, Y: 21}, Exit: Point{X: 15, Y: 21}},
		},
		{
			name:   "bottom right",
			side:   ir.Bottom,
			offset: 1,
			want:   Port{Anchor: Point{X: 14, Y: 22}, Exit: Point{X: 14, Y: 23}},
		},
		{
			name:   "left center",
			side:   ir.LeftSide,
			offset: 0.5,
			want:   Port{Anchor: Point{X: 10, Y: 21}, Exit: Point{X: 9, Y: 21}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolvePort(rect, test.side, test.offset)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("ResolvePort() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestResolvePortAtCoordinateBoundary(t *testing.T) {
	t.Parallel()

	rect := Rect{Size: Size{Width: 5, Height: 3}}
	for _, side := range []ir.Side{ir.Top, ir.LeftSide} {
		port, err := ResolvePort(rect, side, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		if port.Exit != port.Anchor {
			t.Errorf("ResolvePort(%v) exit = %+v, want boundary point %+v", side, port.Exit, port.Anchor)
		}
	}
}

func TestBuildPreservesIRIndices(t *testing.T) {
	t.Parallel()

	var got Layout
	got.Padding = Padding{Left: 1, Right: 1}
	left := got.NewNode("left")
	right := got.NewNodeAt("right", Point{X: 14, Y: 4})
	edgeID := got.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)

	if err := got.Build(); err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != len(got.graph.Nodes) ||
		len(got.Ports) != len(got.graph.Ports) ||
		len(got.Edges) != len(got.graph.Edges) {
		t.Fatalf(
			"Build() lengths = (%d, %d, %d), want (%d, %d, %d)",
			len(got.Nodes),
			len(got.Ports),
			len(got.Edges),
			len(got.graph.Nodes),
			len(got.graph.Ports),
			len(got.graph.Edges),
		)
	}
	if len(got.Edges[edgeID].Points) < 2 {
		t.Errorf("Build() edge %d has fewer than two points", edgeID)
	}
	if got.Nodes[right].Rect.Min != (Point{X: 14, Y: 4}) {
		t.Errorf("Build() node origin = %+v, want {14 4}", got.Nodes[right].Rect.Min)
	}
}

func TestPlaceNodeInvalidatesGeometry(t *testing.T) {
	t.Parallel()

	var l Layout
	nodeID := l.NewNode("node")
	if err := l.Build(); err != nil {
		t.Fatal(err)
	}

	want := Point{X: 7, Y: 9}
	l.PlaceNode(nodeID, want)
	require.Len(t, l.Nodes, 0, "PlaceNode() retained stale geometry")
	require.Len(t, l.Ports, 0, "PlaceNode() retained stale geometry")
	require.Len(t, l.Edges, 0, "PlaceNode() retained stale geometry")
	if err := l.Build(); err != nil {
		t.Fatal(err)
	}
	if got := l.Nodes[nodeID].Rect.Min; got != want {
		t.Errorf("Build() node origin = %+v, want %+v", got, want)
	}
}
