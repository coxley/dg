package layout

import (
	"math"
	"slices"
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
			require.ErrorIs(t, err, test.wantErr)
			require.Equal(t, test.want, got)
		})
	}
}

func TestNewRect(t *testing.T) {
	t.Parallel()

	origin := Point{X: 2, Y: 3}
	limit := origin.Add(5, 7)
	got, err := NewRect(origin, limit)
	require.NoError(t, err)
	want := Rect{
		Min:  origin,
		Size: Size{Width: 5, Height: 7},
	}
	require.Equal(t, want, got)
	require.False(t, got.Contains(limit))
}

func TestNodeRect(t *testing.T) {
	t.Parallel()

	got, err := NodeRect(
		Point{X: 2, Y: 3},
		Size{Width: 6, Height: 1},
		Padding{Top: 1, Right: 2, Bottom: 3, Left: 4},
	)
	require.NoError(t, err)
	want := Rect{
		Min:  Point{X: 2, Y: 3},
		Size: Size{Width: 14, Height: 7},
	}
	require.Equal(t, want, got)
	require.True(t, got.Contains(Point{X: 15, Y: 9}))
	require.False(t, got.Contains(got.Max()))
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
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestResolvePortAtCoordinateBoundary(t *testing.T) {
	t.Parallel()

	rect := Rect{Size: Size{Width: 5, Height: 3}}
	for _, side := range []ir.Side{ir.Top, ir.LeftSide} {
		port, err := ResolvePort(rect, side, 0.5)
		require.NoError(t, err)
		require.Equal(t, port.Anchor, port.Exit)
	}
}

func TestBuildPreservesIRIndices(t *testing.T) {
	t.Parallel()

	got, err := New()
	require.NoError(t, err)
	left, err := got.NewNode("left")
	require.NoError(t, err)
	right, err := got.NewNodeAt("right", Point{X: 14, Y: 4})
	require.NoError(t, err)
	edgeID := got.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)

	require.Len(t, got.Nodes, len(got.graph.Nodes))
	require.Len(t, got.Ports, len(got.graph.Ports))
	require.Len(t, got.Edges, len(got.graph.Edges))
	require.Equal(t, Point{X: 14, Y: 4}, got.Nodes[right].Rect.Min)
	require.NoError(t, got.Build())
	require.GreaterOrEqual(t, len(got.Edges[edgeID].Points), 2)
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
	require.NoError(t, err)
	router.Costs.Crossing = 0

	require.Equal(t, Padding{Top: 3, Right: 2, Bottom: 3, Left: 2}, got.padding)
	require.Equal(t, uint32(42), got.router.Costs.Crossing)
	require.Equal(t, uint8(3), got.router.ReroutePasses)
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	got, err := New()
	require.NoError(t, err)
	require.Equal(t, Padding{Right: 1, Left: 1}, got.padding)
	require.Equal(t, DefaultRouter(), got.router)
}

func TestPlaceNodeUpdatesGeometry(t *testing.T) {
	t.Parallel()

	l, err := New()
	require.NoError(t, err)
	nodeID, err := l.NewNode("node")
	require.NoError(t, err)
	portID, ok := l.graph.PickCenterPort(nodeID, ir.RightSide)
	require.True(t, ok)
	oldPort := l.Ports[portID]

	want := Point{X: 7, Y: 9}
	require.NoError(t, l.PlaceNode(nodeID, want))
	require.Equal(t, want, l.Nodes[nodeID].Rect.Min)
	obstacles := slices.Collect(l.Obstacles())
	require.Equal(t, l.Nodes[nodeID].Rect, obstacles[nodeID])
	require.Equal(t, oldPort.Anchor.Add(want.X, want.Y), l.Ports[portID].Anchor)
}

func TestSetNodeLabelUpdatesGeometry(t *testing.T) {
	t.Parallel()

	l, err := New()
	require.NoError(t, err)
	nodeID, err := l.NewNodeAt("old", Point{X: 4, Y: 3})
	require.NoError(t, err)
	rightPortID, ok := l.graph.PickCenterPort(nodeID, ir.RightSide)
	require.True(t, ok)
	oldRightPort := l.Ports[rightPortID]

	require.NoError(t, l.SetNodeLabel(nodeID, "longer"))
	require.Equal(t, "longer", l.Label(nodeID))
	require.Equal(t, Rect{
		Min:  Point{X: 4, Y: 3},
		Size: Size{Width: 10, Height: 3},
	}, l.Nodes[nodeID].Rect)
	require.NotEqual(t, oldRightPort, l.Ports[rightPortID])
}

func TestSetNodeLabelIsTransactional(t *testing.T) {
	t.Parallel()

	l, err := New()
	require.NoError(t, err)
	nodeID, err := l.NewNode("old")
	require.NoError(t, err)
	beforeNode := l.Nodes[nodeID]
	beforePorts := slices.Clone(l.Ports)

	require.ErrorIs(t, l.SetNodeLabel(nodeID, "first\nsecond"), ErrMultilineLabel)
	require.Equal(t, "old", l.Label(nodeID))
	require.Equal(t, beforeNode, l.Nodes[nodeID])
	require.Equal(t, beforePorts, l.Ports)
}

func TestDeleteEdgeReusesID(t *testing.T) {
	t.Parallel()

	l, err := New()
	require.NoError(t, err)
	left, err := l.NewNodeAt("left", Point{})
	require.NoError(t, err)
	right, err := l.NewNodeAt("right", Point{X: 16})
	require.NoError(t, err)
	edgeID := l.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, l.Build())
	require.NotEmpty(t, l.Edges[edgeID].Points)

	require.NoError(t, l.DeleteEdge(edgeID))
	require.False(t, l.EdgeExists(edgeID))
	require.True(t, l.Edges[edgeID].Empty())
	require.NoError(t, l.Build())

	reusedID := l.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.Equal(t, edgeID, reusedID)
	require.NoError(t, l.Build())
	require.NotEmpty(t, l.Edges[reusedID].Points)
}

func TestDeleteNodeCascadesAndReusesIDs(t *testing.T) {
	t.Parallel()

	l, err := New()
	require.NoError(t, err)
	left, err := l.NewNodeAt("left", Point{})
	require.NoError(t, err)
	middle, err := l.NewNodeAt("middle", Point{X: 12})
	require.NoError(t, err)
	right, err := l.NewNodeAt("right", Point{X: 26})
	require.NoError(t, err)
	edgeA := l.ConnectNodes(left, ir.RightSide, ir.LeftSide, middle)
	edgeB := l.ConnectNodes(middle, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, l.Build())
	middlePorts := slices.Clone(l.graph.Nodes[middle].Ports)
	nodeCount := len(l.Nodes)
	portCount := len(l.Ports)

	require.NoError(t, l.DeleteNode(middle))
	require.False(t, l.NodeExists(middle))
	require.True(t, l.Nodes[middle].Empty())
	require.False(t, l.EdgeExists(edgeA))
	require.False(t, l.EdgeExists(edgeB))
	require.True(t, l.Edges[edgeA].Empty())
	require.True(t, l.Edges[edgeB].Empty())
	for _, portID := range middlePorts {
		require.Equal(t, Port{}, l.Ports[portID])
	}
	for hit := range l.Hits(Point{}) {
		if hit.Kind == HitPort {
			require.NotContains(t, middlePorts, hit.ID)
		}
	}
	require.ErrorIs(t, l.PlaceNode(middle, Point{}), ir.ErrNodeNotFound)
	require.ErrorIs(t, l.SetNodeLabel(middle, "deleted"), ir.ErrNodeNotFound)
	require.NoError(t, l.Build())
	_, err = Rasterize(l)
	require.NoError(t, err)

	replacement, err := l.NewNodeAt("replacement", Point{X: 12})
	require.NoError(t, err)
	require.Equal(t, middle, replacement)
	require.Len(t, l.Nodes, nodeCount)
	require.Len(t, l.Ports, portCount)
	require.ElementsMatch(t, middlePorts, l.graph.Nodes[replacement].Ports)
	for _, portID := range middlePorts {
		require.NotEqual(t, Port{}, l.Ports[portID])
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
			require.Equal(t, test.want, got)
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
	require.NoError(t, err)
	graph.Nodes[left].Label = "changed"
	graph.Nodes[left].Ports[0] = math.MaxUint32

	require.Equal(t, "left", got.Label(left))
	require.Len(t, got.Nodes, len(graph.Nodes))
	require.Len(t, got.Ports, len(graph.Ports))
	require.Len(t, got.Edges, len(graph.Edges))
	require.Equal(t, Size{Width: 10, Height: 5}, got.Nodes[left].Rect.Size)

	require.NoError(t, got.PlaceNode(right, Point{X: 16, Y: 4}))
	require.NoError(t, got.Build())
	require.GreaterOrEqual(t, len(got.Edges[edgeID].Points), 2)
}

func TestWithGraphReturnsValidationError(t *testing.T) {
	t.Parallel()

	_, err := New(WithGraph(ir.Graph{
		Nodes: []ir.Node{{Ports: []uint32{0}}},
	}))
	require.Error(t, err)
}

func TestWithGraphRestoresFreeLists(t *testing.T) {
	t.Parallel()

	var graph ir.Graph
	deleted := graph.NewNode("deleted")
	graph.NewNode("live")
	require.NoError(t, graph.DeleteNode(deleted))

	l, err := New(WithGraph(graph))
	require.NoError(t, err)
	require.False(t, l.NodeExists(deleted))

	replacement, err := l.NewNode("replacement")
	require.NoError(t, err)
	require.Equal(t, deleted, replacement)
}

func TestNewNodeReturnsGeometryError(t *testing.T) {
	t.Parallel()

	got, err := New()
	require.NoError(t, err)
	_, err = got.NewNode("first\nsecond")
	require.ErrorIs(t, err, ErrMultilineLabel)
	require.Empty(t, got.Nodes)
	require.Empty(t, got.graph.Nodes)
}

func TestPlaceNodeReturnsGeometryError(t *testing.T) {
	t.Parallel()

	got, err := New()
	require.NoError(t, err)
	nodeID, err := got.NewNode("node")
	require.NoError(t, err)
	before := got.Nodes[nodeID]

	require.Error(t, got.PlaceNode(nodeID, Point{X: math.MaxUint32}))
	require.Equal(t, before, got.Nodes[nodeID])
}
