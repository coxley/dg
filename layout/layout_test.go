package layout

import (
	"math"
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
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
			name: "empty",
			want: Size{},
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

func TestRectEmpty(t *testing.T) {
	t.Parallel()

	require.True(t, (Rect{}).Empty())
	require.True(t, (Rect{Size: Size{Width: 1}}).Empty())
	require.True(t, (Rect{Size: Size{Height: 1}}).Empty())
	require.False(t, (Rect{Size: Size{Width: 1, Height: 1}}).Empty())
}

func TestConnectionsContains(t *testing.T) {
	t.Parallel()

	connections := North | East
	require.True(t, connections.ContainsAll(North|East))
	require.False(t, connections.ContainsAll(North|South))
	require.True(t, connections.ContainsAny(East|West))
	require.False(t, connections.ContainsAny(South|West))
}

func TestGridIndex(t *testing.T) {
	t.Parallel()

	grid, err := NewGrid(Rect{
		Min:  Point{X: 3, Y: 5},
		Size: Size{Width: 4, Height: 2},
	})
	require.NoError(t, err)

	index, ok := grid.Index(Point{X: 5, Y: 6})
	require.True(t, ok)
	require.Equal(t, 6, index)
	_, ok = grid.Index(Point{X: 2, Y: 6})
	require.False(t, ok)
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
	require.True(t, got.OnBoundary(got.Min))
	require.False(t, got.OnBoundary(got.Min.Add(1, 1)))
	require.False(t, got.OnBoundary(got.Max()))
}

func TestNodeRectReservesAnEmptyLabelRow(t *testing.T) {
	t.Parallel()

	got, err := NodeRect(Point{}, Size{}, Padding{Left: 1, Right: 1})
	require.NoError(t, err)
	require.Equal(t, Size{Width: 4, Height: 3}, got.Size)
}

func TestNodeRectPreservesNaturalWidth(t *testing.T) {
	t.Parallel()

	got, err := NodeRect(Point{}, Size{Width: 5, Height: 1}, Padding{Left: 1, Right: 1})
	require.NoError(t, err)
	require.Equal(t, Size{Width: 9, Height: 3}, got.Size)
}

func TestCenterPortPreservesNaturalWidth(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	sink, err := geo.NewNode("sink")
	require.NoError(t, err)
	sinks, err := geo.NewNode("sinks")
	require.NoError(t, err)

	sinkPort, ok := geo.graph.PickCenterPort(sink, ir.Top)
	require.True(t, ok)
	sinksPort, ok := geo.graph.PickCenterPort(sinks, ir.Top)
	require.True(t, ok)
	require.Equal(t, Size{Width: 8, Height: 3}, geo.Nodes[sink].Rect.Size)
	require.Equal(t, Size{Width: 9, Height: 3}, geo.Nodes[sinks].Rect.Size)
	require.Equal(t, uint32(4), geo.Ports[sinkPort].Anchor.X)
	require.Equal(t, uint32(4), geo.Ports[sinksPort].Anchor.X)
}

func TestNodeRectAddsOnlyExplicitPadding(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		label := Size{
			Width:  rapid.Uint32Range(0, 1<<16).Draw(t, "label width"),
			Height: rapid.Uint32Range(0, 1<<16).Draw(t, "label height"),
		}
		padding := Padding{
			Top:    rapid.Uint8().Draw(t, "top padding"),
			Right:  rapid.Uint8().Draw(t, "right padding"),
			Bottom: rapid.Uint8().Draw(t, "bottom padding"),
			Left:   rapid.Uint8().Draw(t, "left padding"),
		}
		rect, err := NodeRect(Point{}, label, padding)
		require.NoError(t, err)

		labelHeight := label.Height
		if label == (Size{}) {
			labelHeight = 1
		}
		require.Equal(
			t,
			label.Width+uint32(padding.Left)+uint32(padding.Right)+2,
			rect.Size.Width,
		)
		require.Equal(
			t,
			labelHeight+uint32(padding.Top)+uint32(padding.Bottom)+2,
			rect.Size.Height,
		)
	})
}

func TestCenterPortUsesMiddleBoundaryCell(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		label := rapid.StringMatching(`[a-z]{0,64}`).Draw(t, "label")
		geo, err := New()
		require.NoError(t, err)
		nodeID, err := geo.NewNode(label)
		require.NoError(t, err)
		portID, ok := geo.graph.PickCenterPort(nodeID, ir.Top)
		require.True(t, ok)

		rect := geo.Nodes[nodeID].Rect
		position := geo.Ports[portID].Anchor.X - rect.Min.X
		require.Equal(t, rect.Size.Width/2, position)
		require.Equal(t, (rect.Size.Width-1)/2, rect.Size.Width-1-position)
	})
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

func TestPortUsabilityFollowsSideLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		label string
		size  Size
		want  map[ir.Side]int
	}{
		{
			label: "a",
			size:  Size{Width: 5, Height: 3},
			want:  map[ir.Side]int{ir.Top: 1, ir.RightSide: 1, ir.Bottom: 1, ir.LeftSide: 1},
		},
		{
			label: "foo bar",
			size:  Size{Width: 11, Height: 3},
			want:  map[ir.Side]int{ir.Top: 3, ir.RightSide: 1, ir.Bottom: 3, ir.LeftSide: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			t.Parallel()

			geo, err := New()
			require.NoError(t, err)
			nodeID, err := geo.NewNode(test.label)
			require.NoError(t, err)
			require.Equal(t, test.size, geo.Nodes[nodeID].Rect.Size)

			got := make(map[ir.Side]int)
			for portID := range geo.NodePorts(nodeID) {
				if !geo.PortUsable(portID) {
					continue
				}
				port := geo.graph.Ports[portID]
				got[port.Side]++
				require.True(t, geo.Nodes[nodeID].Rect.OnBoundary(geo.Ports[portID].Anchor))
				require.False(t, isCorner(geo.Nodes[nodeID].Rect, geo.Ports[portID].Anchor))
			}
			require.Equal(t, test.want, got)
		})
	}
}

func TestPortUsabilitySupportsArbitraryOffsets(t *testing.T) {
	t.Parallel()

	var graph ir.Graph
	nodeID := graph.NewNode("long custom label")
	usable := uint32(len(graph.Ports))
	graph.Ports = append(graph.Ports, ir.NewPort(nodeID, ir.Top, .4))
	graph.Nodes[nodeID].Ports = append(graph.Nodes[nodeID].Ports, usable)
	tooClose := uint32(len(graph.Ports))
	graph.Ports = append(graph.Ports, ir.NewPort(nodeID, ir.Top, .42))
	graph.Nodes[nodeID].Ports = append(graph.Nodes[nodeID].Ports, tooClose)

	geo, err := New(WithGraph(graph))
	require.NoError(t, err)
	require.True(t, geo.PortUsable(usable))
	require.False(t, geo.PortUsable(tooClose))
}

func isCorner(rect Rect, point Point) bool {
	maxp := rect.Max()
	onHorizontalCorner := point.X == rect.Min.X || point.X == maxp.X-1
	onVerticalCorner := point.Y == rect.Min.Y || point.Y == maxp.Y-1
	return onHorizontalCorner && onVerticalCorner
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

func TestConnectPortsValidatesIDs(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNode("left")
	require.NoError(t, err)
	right, err := geo.NewNode("right")
	require.NoError(t, err)
	third, err := geo.NewNode("third")
	require.NoError(t, err)
	portA, ok := geo.graph.PickCenterPort(left, ir.RightSide)
	require.True(t, ok)
	portB, ok := geo.graph.PickCenterPort(right, ir.LeftSide)
	require.True(t, ok)
	portC, ok := geo.graph.PickCenterPort(third, ir.LeftSide)
	require.True(t, ok)

	edgeID, err := geo.ConnectPorts(portA, portB)
	require.NoError(t, err)
	require.True(t, geo.EdgeExists(edgeID))
	require.Contains(t, slices.Collect(geo.NodePorts(left)), portA)
	require.NoError(t, geo.ReconnectEdge(edgeID, portB, portC))
	gotA, gotB, err := geo.EdgePorts(edgeID)
	require.NoError(t, err)
	require.Equal(t, [2]uint32{portA, portC}, [2]uint32{gotA, gotB})
	require.True(t, geo.Edges[edgeID].Empty())

	topPorts := geo.graph.PortsOnSide(left, ir.Top)
	require.Len(t, topPorts, 3)
	unavailable := topPorts[2]
	require.False(t, geo.PortUsable(unavailable))
	_, err = geo.ConnectPorts(unavailable, portB)
	require.ErrorIs(t, err, ErrPortUnavailable)
	require.ErrorIs(t, geo.ReconnectEdge(edgeID, portA, unavailable), ErrPortUnavailable)

	_, err = geo.ConnectPorts(portB, portC)
	require.NoError(t, err)
	require.ErrorIs(t, geo.ReconnectEdge(edgeID, portA, portB), ir.ErrDuplicateEdge)
	require.ErrorIs(t, geo.ReconnectEdge(edgeID, portB, portA), ir.ErrPortNotOnEdge)
	require.ErrorIs(t, geo.ReconnectEdge(edgeID, portC, portA), ir.ErrSamePort)
	_, err = geo.ConnectPorts(portA, portA)
	require.ErrorIs(t, err, ir.ErrSamePort)
	_, err = geo.ConnectPorts(portA, math.MaxUint32)
	require.ErrorIs(t, err, ir.ErrPortNotFound)
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
