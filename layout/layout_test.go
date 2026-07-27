package layout

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
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

	got, err := New()
	if err != nil {
		t.Fatal(err)
	}
	left, err := got.NewNode("left")
	if err != nil {
		t.Fatal(err)
	}
	right, err := got.NewNodeAt("right", Point{X: 14, Y: 4})
	if err != nil {
		t.Fatal(err)
	}
	edgeID := got.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)

	if len(got.Nodes) != len(got.graph.Nodes) ||
		len(got.Ports) != len(got.graph.Ports) ||
		len(got.Edges) != len(got.graph.Edges) {
		t.Fatalf(
			"geometry lengths = (%d, %d, %d), want (%d, %d, %d)",
			len(got.Nodes),
			len(got.Ports),
			len(got.Edges),
			len(got.graph.Nodes),
			len(got.graph.Ports),
			len(got.graph.Edges),
		)
	}
	if got.Nodes[right].Rect.Min != (Point{X: 14, Y: 4}) {
		t.Errorf("node origin = %+v, want {14 4}", got.Nodes[right].Rect.Min)
	}
	if err := got.Build(); err != nil {
		t.Fatal(err)
	}
	if len(got.Edges[edgeID].Points) < 2 {
		t.Errorf("Build() edge %d has fewer than two points", edgeID)
	}
}

func TestNewOptions(t *testing.T) {
	t.Parallel()

	router := DefaultRouter()
	router.Costs.Crossing = 42
	router.ReroutePasses = 3
	got, err := New(
		WithPadding(2, 3),
		WithRouter(router),
	)
	if err != nil {
		t.Fatal(err)
	}
	router.Costs.Crossing = 0

	if got.padding != (Padding{Top: 3, Right: 2, Bottom: 3, Left: 2}) {
		t.Errorf("New() padding = %+v", got.padding)
	}
	if got.router.Costs.Crossing != 42 || got.router.ReroutePasses != 3 {
		t.Errorf("New() router = %+v", got.router)
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	got, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if got.padding != (Padding{Right: 1, Left: 1}) {
		t.Errorf("New() padding = %+v", got.padding)
	}
	if got.router != DefaultRouter() {
		t.Errorf("New() router = %+v, want %+v", got.router, DefaultRouter())
	}
}

func TestPlaceNodeUpdatesGeometry(t *testing.T) {
	t.Parallel()

	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := l.NewNode("node")
	if err != nil {
		t.Fatal(err)
	}
	portID, ok := l.graph.PickCenterPort(nodeID, ir.RightSide)
	if !ok {
		t.Fatal("node has no right port")
	}
	oldPort := l.Ports[portID]

	want := Point{X: 7, Y: 9}
	if err := l.PlaceNode(nodeID, want); err != nil {
		t.Fatal(err)
	}
	if got := l.Nodes[nodeID].Rect.Min; got != want {
		t.Errorf("PlaceNode() node origin = %+v, want %+v", got, want)
	}
	obstacles := slices.Collect(l.Obstacles())
	if got := obstacles[nodeID]; got != l.Nodes[nodeID].Rect {
		t.Errorf("PlaceNode() obstacle = %+v, want %+v", got, l.Nodes[nodeID].Rect)
	}
	if got := l.Ports[portID].Anchor; got != oldPort.Anchor.Add(want.X, want.Y) {
		t.Errorf("PlaceNode() port anchor = %+v", got)
	}
}

func TestLayoutHits(t *testing.T) {
	t.Parallel()

	l := Layout{
		Nodes: []Node{{Rect: Rect{
			Min:  Point{X: 1, Y: 1},
			Size: Size{Width: 3, Height: 3},
		}}},
		Ports: []Port{{Anchor: Point{X: 1, Y: 2}}},
		Edges: []Edge{
			{Points: []Point{{X: 0, Y: 2}, {X: 2, Y: 2}, {X: 2, Y: 4}}},
			{Points: []Point{{X: 2, Y: 2}, {X: 4, Y: 2}}},
		},
	}
	tests := []struct {
		name  string
		point Point
		want  []Hit
	}{
		{
			name:  "overlapping node port and edge",
			point: Point{X: 1, Y: 2},
			want: []Hit{
				{ID: 0, Kind: HitNode},
				{ID: 0, Kind: HitPort},
				{ID: 0, Kind: HitEdge},
			},
		},
		{
			name:  "bend and shared endpoint",
			point: Point{X: 2, Y: 2},
			want: []Hit{
				{ID: 0, Kind: HitNode},
				{ID: 0, Kind: HitEdge},
				{ID: 1, Kind: HitEdge},
			},
		},
		{
			name:  "outside",
			point: Point{X: 8, Y: 8},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := slices.Collect(l.Hits(test.point))
			if !slices.Equal(got, test.want) {
				t.Errorf("Hits(%+v) = %+v, want %+v", test.point, got, test.want)
			}
		})
	}
}

func TestWithGraphCopiesAndInitializes(t *testing.T) {
	t.Parallel()

	var graph ir.Graph
	left := graph.NewNode("left")
	right := graph.NewNode("right")
	edgeID := graph.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)

	got, err := New(
		WithGraph(graph),
		WithPadding(2, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	graph.Nodes[left].Label = "changed"
	graph.Nodes[left].Ports[0] = math.MaxUint32

	if got.Label(left) != "left" {
		t.Errorf("WithGraph() label = %q, want left", got.Label(left))
	}
	if len(got.Nodes) != len(graph.Nodes) ||
		len(got.Ports) != len(graph.Ports) ||
		len(got.Edges) != len(graph.Edges) {
		t.Fatalf(
			"WithGraph() lengths = (%d, %d, %d), want (%d, %d, %d)",
			len(got.Nodes),
			len(got.Ports),
			len(got.Edges),
			len(graph.Nodes),
			len(graph.Ports),
			len(graph.Edges),
		)
	}
	if got.Nodes[left].Rect.Size != (Size{Width: 10, Height: 5}) {
		t.Errorf("WithGraph() node size = %+v, want {10 5}", got.Nodes[left].Rect.Size)
	}

	if err := got.PlaceNode(right, Point{X: 16, Y: 4}); err != nil {
		t.Fatal(err)
	}
	if err := got.Build(); err != nil {
		t.Fatal(err)
	}
	if len(got.Edges[edgeID].Points) < 2 {
		t.Errorf("Build() edge %d has fewer than two points", edgeID)
	}
}

func TestWithGraphReturnsValidationError(t *testing.T) {
	t.Parallel()

	_, err := New(WithGraph(ir.Graph{
		Nodes: []ir.Node{{Ports: []uint32{0}}},
	}))
	if err == nil {
		t.Fatal("New() error = nil, want invalid graph error")
	}
}

func TestNewNodeReturnsGeometryError(t *testing.T) {
	t.Parallel()

	got, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := got.NewNode("first\nsecond"); !errors.Is(err, ErrMultilineLabel) {
		t.Errorf("NewNode() error = %v, want %v", err, ErrMultilineLabel)
	}
	if len(got.Nodes) != 0 || len(got.graph.Nodes) != 0 {
		t.Error("NewNode() retained a node after returning an error")
	}
}

func TestPlaceNodeReturnsGeometryError(t *testing.T) {
	t.Parallel()

	got, err := New()
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := got.NewNode("node")
	if err != nil {
		t.Fatal(err)
	}
	before := got.Nodes[nodeID]

	if err := got.PlaceNode(nodeID, Point{X: math.MaxUint32}); err == nil {
		t.Fatal("PlaceNode() error = nil, want coordinate overflow")
	}
	if got.Nodes[nodeID] != before {
		t.Errorf("PlaceNode() geometry = %+v after error, want %+v", got.Nodes[nodeID], before)
	}
}
