package layout

import (
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
	occupancy := newRouteOccupancy()
	occupancy.add(0, []Point{{X: 1, Y: 1}, {X: 2, Y: 1}})
	router := DefaultRouter()

	cost, crossings, ok := router.stepCost(
		1,
		Point{X: 1, Y: 1},
		Point{X: 2, Y: 1},
		&occupancy,
		&graph,
	)
	require.True(t, ok)
	require.Equal(t, uint64(router.Costs.SharedStep), cost)
	require.Zero(t, crossings)

	_, _, ok = router.stepCost(
		2,
		Point{X: 1, Y: 1},
		Point{X: 2, Y: 1},
		&occupancy,
		&graph,
	)
	require.False(t, ok)
}

func TestStepCostChargesUnrelatedCrossing(t *testing.T) {
	t.Parallel()

	graph := ir.Graph{Edges: []ir.Edge{
		{PortA: 0, PortB: 1},
		{PortA: 2, PortB: 3},
	}}
	occupancy := newRouteOccupancy()
	occupancy.add(0, []Point{
		{X: 2, Y: 1},
		{X: 2, Y: 2},
		{X: 2, Y: 3},
	})
	router := DefaultRouter()

	cost, crossings, ok := router.stepCost(
		1,
		Point{X: 1, Y: 2},
		Point{X: 2, Y: 2},
		&occupancy,
		&graph,
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
	occupancy := newRouteOccupancy()
	occupancy.add(0, []Point{
		{X: 2, Y: 2},
		{X: 2, Y: 3},
	})
	router := DefaultRouter()

	_, _, ok := router.stepCost(
		1,
		Point{X: 1, Y: 2},
		Point{X: 2, Y: 2},
		&occupancy,
		&graph,
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
		0,
		geo.scratch.paths[0],
		&geo.scratch.occupancy,
		&graph,
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
	got, ok := grid.At(Point{X: 4, Y: 1})
	require.True(t, ok)
	want := North | East | South
	require.Equal(t, want, got)
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
