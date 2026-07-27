package layout

import (
	"math"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestRouteQueueOrdersByPriorityThenInsertion(t *testing.T) {
	t.Parallel()

	var queue routeQueue
	for _, item := range []routeItem{
		{priority: 20, order: 0},
		{priority: 10, order: 2},
		{priority: 10, order: 1},
		{priority: 10, crossings: 1},
		{priority: 30, order: 0},
	} {
		queue.push(item)
	}

	want := []routeItem{
		{priority: 10, order: 1},
		{priority: 10, order: 2},
		{priority: 10, crossings: 1},
		{priority: 20, order: 0},
		{priority: 30, order: 0},
	}
	for i := range want {
		require.Equal(t, want[i], queue.pop(), "pop %d", i)
	}
}

func TestFindRouteAvoidsObstacle(t *testing.T) {
	t.Parallel()

	a := Port{
		Anchor: Point{X: 0, Y: 2},
		Exit:   Point{X: 1, Y: 2},
	}
	b := Port{
		Anchor: Point{X: 8, Y: 2},
		Exit:   Point{X: 7, Y: 2},
	}
	obstacle := Rect{
		Min:  Point{X: 3, Y: 1},
		Size: Size{Width: 3, Height: 3},
	}

	var search routeSearch
	points, err := DefaultRouter().findRoute(
		&Layout{Nodes: []Node{{Rect: obstacle}}},
		0,
		a,
		b,
		nil,
		&search,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, a.Anchor, points[0])
	require.Equal(t, b.Anchor, points[len(points)-1])

	for i := 1; i < len(points); i++ {
		for _, point := range segmentPoints(points[i-1], points[i]) {
			require.False(t, obstacle.Contains(point), "route crosses obstacle at %+v: %+v", point, points)
		}
	}
}

func TestStepCostSharesOnlyCommonPortSegments(t *testing.T) {
	t.Parallel()

	graph := ir.Graph{Edges: []ir.Edge{
		{PortA: 0, PortB: 1},
		{PortA: 0, PortB: 2},
		{PortA: 3, PortB: 4},
	}}
	geo := Layout{
		graph: graph,
		Ports: []Port{
			{Anchor: NewPoint(2, 1)},
			{Anchor: NewPoint(20, 1)},
			{Anchor: NewPoint(20, 3)},
			{Anchor: NewPoint(20, 5)},
			{Anchor: NewPoint(20, 7)},
		},
	}
	occupancy := newRouteOccupancy()
	occupancy.add(0, []Point{{X: 1, Y: 1}, {X: 2, Y: 1}})
	router := DefaultRouter()

	cost, crossings, ok := router.stepCost(
		&geo,
		1,
		Point{X: 1, Y: 1},
		Point{X: 2, Y: 1},
		&occupancy,
	)
	require.True(t, ok)
	require.Equal(t, uint64(router.Costs.SharedStep), cost)
	require.Zero(t, crossings)

	_, _, ok = router.stepCost(
		&geo,
		2,
		Point{X: 1, Y: 1},
		Point{X: 2, Y: 1},
		&occupancy,
	)
	require.False(t, ok)
}

func TestPreviewStepCostSharesDestinationPort(t *testing.T) {
	t.Parallel()

	geo := Layout{
		graph: ir.Graph{Edges: []ir.Edge{{PortA: 0, PortB: 1}}},
		Ports: []Port{
			{Anchor: NewPoint(1, 1)},
			{Anchor: NewPoint(20, 1)},
			{Anchor: NewPoint(1, 3)},
		},
	}
	occupancy := newRouteOccupancy()
	occupancy.add(0, []Point{NewPoint(18, 1), NewPoint(19, 1)})
	router := DefaultRouter()
	preview := routeEdge{
		id:       math.MaxUint32,
		ports:    ir.Edge{PortA: 2, PortB: 1},
		hasPorts: true,
	}

	cost, crossings, ok := router.stepCostFor(
		&geo,
		preview,
		NewPoint(18, 1),
		NewPoint(19, 1),
		&occupancy,
	)
	require.True(t, ok)
	require.Equal(t, uint64(router.Costs.SharedStep), cost)
	require.Zero(t, crossings)
}

func TestStepCostRejectsEarlyCommonEndpointMerge(t *testing.T) {
	t.Parallel()

	graph := ir.Graph{Edges: []ir.Edge{
		{PortA: 0, PortB: 1},
		{PortA: 0, PortB: 2},
	}}
	geo := Layout{
		graph: graph,
		Ports: []Port{
			{Anchor: NewPoint(20, 2)},
			{Anchor: NewPoint(1, 2)},
			{Anchor: NewPoint(1, 4)},
		},
	}
	occupancy := newRouteOccupancy()
	occupancy.add(0, []Point{NewPoint(1, 2), NewPoint(2, 2)})

	_, _, ok := DefaultRouter().stepCost(
		&geo,
		1,
		NewPoint(1, 2),
		NewPoint(2, 2),
		&occupancy,
	)
	require.False(t, ok)
}

func TestStepCostChargesUnrelatedCrossing(t *testing.T) {
	t.Parallel()

	graph := ir.Graph{Edges: []ir.Edge{
		{PortA: 0, PortB: 1},
		{PortA: 2, PortB: 3},
	}}
	geo := Layout{graph: graph}
	occupancy := newRouteOccupancy()
	occupancy.add(0, []Point{
		{X: 2, Y: 1},
		{X: 2, Y: 2},
		{X: 2, Y: 3},
	})
	router := DefaultRouter()

	cost, crossings, ok := router.stepCost(
		&geo,
		1,
		Point{X: 1, Y: 2},
		Point{X: 2, Y: 2},
		&occupancy,
	)
	want := uint64(router.Costs.Step + router.Costs.Crossing)
	require.True(t, ok)
	require.Equal(t, want, cost)
	require.Equal(t, uint32(1), crossings)
}

func TestStepCostRejectsUnrelatedTouch(t *testing.T) {
	t.Parallel()

	graph := ir.Graph{Edges: []ir.Edge{
		{PortA: 0, PortB: 1},
		{PortA: 2, PortB: 3},
	}}
	geo := Layout{graph: graph}
	occupancy := newRouteOccupancy()
	occupancy.add(0, []Point{
		{X: 2, Y: 2},
		{X: 2, Y: 3},
	})
	router := DefaultRouter()

	_, _, ok := router.stepCost(
		&geo,
		1,
		Point{X: 1, Y: 2},
		Point{X: 2, Y: 2},
		&occupancy,
	)
	require.False(t, ok)
}

func TestRerouteCrossingsReconsidersEarlierEdges(t *testing.T) {
	t.Parallel()

	graph := ir.Graph{Edges: []ir.Edge{
		{PortA: 0, PortB: 1},
		{PortA: 2, PortB: 3},
	}}
	ports := []Port{
		{Anchor: Point{X: 5, Y: 0}, Exit: Point{X: 5, Y: 1}},
		{Anchor: Point{X: 5, Y: 10}, Exit: Point{X: 5, Y: 9}},
		{Anchor: Point{X: 3, Y: 5}, Exit: Point{X: 4, Y: 5}},
		{Anchor: Point{X: 7, Y: 5}, Exit: Point{X: 6, Y: 5}},
	}
	paths := [][]Point{
		segmentPoints(ports[0].Anchor, ports[1].Anchor),
		segmentPoints(ports[2].Anchor, ports[3].Anchor),
	}
	geo := Layout{
		graph: graph,
		Ports: ports,
		Nodes: []Node{{Rect: Rect{
			Min:  Point{X: 0, Y: 15},
			Size: Size{Width: 2, Height: 2},
		}}},
		scratch: routeScratch{
			occupancy: newRouteOccupancy(),
			paths:     paths,
		},
	}
	for i := range paths {
		geo.scratch.occupancy.add(uint32(i), paths[i])
	}
	router := Router{
		Costs: Costs{
			Step:     1,
			Bend:     1,
			Crossing: 10,
		},
		ReroutePasses: 1,
	}
	changed, err := router.rerouteCrossings(&geo)
	require.NoError(t, err)
	require.True(t, changed)
	_, crossings, ok := router.scorePath(
		&geo,
		0,
		geo.scratch.paths[0],
		&geo.scratch.occupancy,
	)
	require.True(t, ok)
	require.Zero(t, crossings)
}

func TestRasterizeMergesRouteWithNodeBorder(t *testing.T) {
	t.Parallel()

	l := &Layout{
		Nodes: []Node{{
			Rect: Rect{
				Min:  Point{X: 0, Y: 0},
				Size: Size{Width: 5, Height: 3},
			},
		}},
		Edges: []Edge{{
			Points: []Point{{X: 4, Y: 1}, {X: 6, Y: 1}},
		}},
	}

	grid, err := Rasterize(l)
	require.NoError(t, err)
	cells := &grid.Cells[0]
	grid, err = RasterizeInto(grid.Cells, l)
	require.NoError(t, err)
	require.Same(t, cells, &grid.Cells[0])
	got, ok := grid.At(Point{X: 4, Y: 1})
	require.True(t, ok)
	want := North | East | South
	require.Equal(t, want, got)
}

func TestRouteAllowsOverlappingEndpointNodes(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(0, 1))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(5, 1))
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeSize(left, Size{Width: 8, Height: 3}))
	require.NoError(t, geo.SetNodeSize(right, Size{Width: 9, Height: 3}))
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)

	require.NoError(t, geo.Build())
	require.False(t, geo.Edges[edgeID].Empty())
}

func TestRoutePrefersDetourAroundEndpointNodes(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, endpointStep uint32) (*Layout, uint32) {
		t.Helper()

		router := DefaultRouter()
		router.Costs.EndpointStep = endpointStep
		geo, err := New(WithRouter(router))
		require.NoError(t, err)
		source, err := geo.NewNodeAt("source", NewPoint(1, 1))
		require.NoError(t, err)
		destination, err := geo.NewNodeAt("destination", NewPoint(1, 7))
		require.NoError(t, err)
		require.NoError(t, geo.SetNodeSize(
			source,
			Size{Width: 8, Height: 4},
		))
		require.NoError(t, geo.SetNodeSize(
			destination,
			Size{Width: 10, Height: 5},
		))
		edgeID := geo.ConnectNodes(
			source,
			ir.Top,
			ir.Top,
			destination,
		)
		require.NoError(t, geo.Build())
		return geo, edgeID
	}

	direct, directEdge := build(t, 0)
	require.True(t, routeTraversesEndpoint(
		direct,
		directEdge,
	))

	detour, detourEdge := build(t, DefaultRouter().Costs.EndpointStep)
	require.False(t, routeTraversesEndpoint(
		detour,
		detourEdge,
	))
}

func TestSmartArrowClearanceWorksFromRouteStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sinkY      uint32
		minSegment uint64
		maxSegment uint64
	}{
		{
			name:       "full clearance",
			sinkY:      7,
			minSegment: 3,
		},
		{
			name:       "close fallback",
			sinkY:      5,
			minSegment: 2,
			maxSegment: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			geo, err := New()
			require.NoError(t, err)
			sink, err := geo.NewNodeAt("sinks", NewPoint(7, test.sinkY))
			require.NoError(t, err)
			foo, err := geo.NewNodeAt("foo", NewPoint(4, 0))
			require.NoError(t, err)
			bar, err := geo.NewNodeAt("bar", NewPoint(12, 0))
			require.NoError(t, err)
			edges := [...]uint32{
				geo.ConnectNodes(foo, ir.Bottom, ir.Top, sink),
				geo.ConnectNodes(bar, ir.Bottom, ir.Top, sink),
			}
			for _, edgeID := range edges {
				require.NoError(t, geo.SetEdgeStyle(edgeID, EdgeStyle{
					PortAArrow: ArrowFilled,
				}))
			}
			require.NoError(t, geo.Build())

			for _, edgeID := range edges {
				points := geo.Edges[edgeID].Points
				require.GreaterOrEqual(t, len(points), 3)
				distance := manhattan(points[0], points[1])
				require.GreaterOrEqual(
					t,
					distance,
					test.minSegment,
				)
				if test.maxSegment != 0 {
					require.LessOrEqual(t, distance, test.maxSegment)
				}
			}
		})
	}
}

func TestSmartArrowClearanceChoosesShortTwoSidedRoute(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	sink, err := geo.NewNodeAt("sinks", NewPoint(7, 6))
	require.NoError(t, err)
	foo, err := geo.NewNodeAt("foo", NewPoint(4, 0))
	require.NoError(t, err)
	bar, err := geo.NewNodeAt("bar", NewPoint(12, 0))
	require.NoError(t, err)
	edges := [...]uint32{
		geo.ConnectNodes(foo, ir.Bottom, ir.Top, sink),
		geo.ConnectNodes(bar, ir.Bottom, ir.Top, sink),
	}
	for _, edgeID := range edges {
		require.NoError(t, geo.SetEdgeStyle(edgeID, EdgeStyle{
			PortAArrow: ArrowFilled,
			PortBArrow: ArrowFilled,
		}))
	}
	require.NoError(t, geo.Build())

	for _, edgeID := range edges {
		points := geo.Edges[edgeID].Points
		require.GreaterOrEqual(t, len(points), 3)
		require.Equal(t, uint64(2), manhattan(points[0], points[1]))
		require.Equal(
			t,
			uint64(2),
			manhattan(points[len(points)-2], points[len(points)-1]),
		)
	}
}

func TestPreviewRouteTreatsPointAsFloatingPort(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", NewPoint(1, 2))
	require.NoError(t, err)
	obstacle, err := geo.NewNodeAt("obstacle", NewPoint(13, 1))
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeSize(obstacle, Size{
		Width:  10,
		Height: 6,
	}))
	require.NoError(t, geo.Build())
	sourcePort, ok := geo.graph.PickCenterPort(source, ir.RightSide)
	require.True(t, ok)
	cursor := NewPoint(25, 3)

	preview, err := geo.PreviewRoute(nil, sourcePort, cursor)
	require.NoError(t, err)
	require.Equal(t, geo.Ports[sourcePort].Anchor, preview[0])
	require.Equal(t, cursor, preview[len(preview)-1])
	for i := 1; i < len(preview); i++ {
		for _, point := range segmentPoints(preview[i-1], preview[i]) {
			require.False(
				t,
				geo.Nodes[obstacle].Rect.Contains(point),
				"preview crosses obstacle at %+v: %+v",
				point,
				preview,
			)
		}
	}
}

func TestPreviewRouteWithoutEdgeValidatesEdge(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("node", NewPoint(2, 2))
	require.NoError(t, err)
	portID, ok := geo.graph.PickCenterPort(nodeID, ir.RightSide)
	require.True(t, ok)

	_, err = geo.PreviewRouteWithoutEdge(
		nil,
		portID,
		NewPoint(20, 2),
		0,
	)
	require.ErrorIs(t, err, ir.ErrEdgeNotFound)
}

func TestRasterizeOccludesUnrelatedCrossingByLayer(t *testing.T) {
	t.Parallel()

	vertical := Hit{ID: 0, Kind: HitEdge}
	horizontal := Hit{ID: 1, Kind: HitEdge}
	geo := Layout{
		graph: ir.Graph{Edges: []ir.Edge{
			{PortA: 0, PortB: 1},
			{PortA: 2, PortB: 3},
		}},
		Ports: []Port{
			{Anchor: NewPoint(2, 0)},
			{Anchor: NewPoint(2, 4)},
			{Anchor: NewPoint(0, 2)},
			{Anchor: NewPoint(4, 2)},
		},
		Edges: []Edge{
			{Points: []Point{NewPoint(2, 0), NewPoint(2, 4)}},
			{Points: []Point{NewPoint(0, 2), NewPoint(4, 2)}},
		},
		drawOrder: []Hit{vertical, horizontal},
	}

	grid, err := Rasterize(&geo)
	require.NoError(t, err)
	connections, ok := grid.At(NewPoint(2, 2))
	require.True(t, ok)
	require.Equal(t, East|West, connections)
	owner, ok := grid.OwnerAt(NewPoint(2, 2))
	require.True(t, ok)
	require.Equal(t, horizontal, owner)

	geo.drawOrder[0], geo.drawOrder[1] =
		geo.drawOrder[1], geo.drawOrder[0]
	grid, err = Rasterize(&geo)
	require.NoError(t, err)
	connections, ok = grid.At(NewPoint(2, 2))
	require.True(t, ok)
	require.Equal(t, North|South, connections)
	owner, ok = grid.OwnerAt(NewPoint(2, 2))
	require.True(t, ok)
	require.Equal(t, vertical, owner)
}

func TestRasterizeDoesNotJoinCrossingNodeBorders(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	back, err := geo.NewNodeAt("", Point{})
	require.NoError(t, err)
	front, err := geo.NewNodeAt("", NewPoint(3, 2))
	require.NoError(t, err)
	size := Size{Width: 7, Height: 4}
	require.NoError(t, geo.SetNodeSize(back, size))
	require.NoError(t, geo.SetNodeSize(front, size))
	require.NoError(t, geo.Build())

	grid, err := Rasterize(geo)
	require.NoError(t, err)
	index, ok := grid.Index(NewPoint(6, 2))
	require.True(t, ok)
	require.Equal(t, East|West, grid.Cells[index])
	require.Equal(t, Hit{ID: front, Kind: HitNode}, grid.Owners[index])
}

func routeTraversesEndpoint(geo *Layout, edgeID uint32) bool {
	edge := geo.graph.Edges[edgeID]
	source := geo.graph.Ports[edge.PortA].Node
	destination := geo.graph.Ports[edge.PortB].Node
	points := geo.Edges[edgeID].Points
	for i := 1; i < len(points); i++ {
		for _, point := range segmentPoints(points[i-1], points[i]) {
			if point == points[0] || point == points[len(points)-1] {
				continue
			}
			if geo.Nodes[source].Rect.Contains(point) ||
				geo.Nodes[destination].Rect.Contains(point) {
				return true
			}
		}
	}
	return false
}

func segmentPoints(a, b Point) []Point {
	points := []Point{a}
	for a != b {
		switch {
		case b.X < a.X:
			a.X--
		case b.X > a.X:
			a.X++
		case b.Y < a.Y:
			a.Y--
		case b.Y > a.Y:
			a.Y++
		}
		points = append(points, a)
	}
	return points
}
