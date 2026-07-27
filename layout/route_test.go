package layout

import (
	"testing"

	"github.com/coxley/dg/ir"
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
		if got := queue.pop(); got != want[i] {
			t.Fatalf("pop %d = %+v, want %+v", i, got, want[i])
		}
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
	if err != nil {
		t.Fatal(err)
	}
	if points[0] != a.Anchor {
		t.Errorf("findRoute() starts at %+v, want %+v", points[0], a.Anchor)
	}
	if got := points[len(points)-1]; got != b.Anchor {
		t.Errorf("findRoute() ends at %+v, want %+v", got, b.Anchor)
	}

	for i := 1; i < len(points); i++ {
		for _, point := range segmentPoints(points[i-1], points[i]) {
			if obstacle.Contains(point) {
				t.Fatalf("findRoute() crosses obstacle at %+v: %+v", point, points)
			}
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
	if !ok || cost != uint64(router.Costs.SharedStep) || crossings != 0 {
		t.Errorf("shared step = (%d, %d, %t)", cost, crossings, ok)
	}

	if _, _, ok := router.stepCost(
		2,
		Point{X: 1, Y: 1},
		Point{X: 2, Y: 1},
		&occupancy,
		&graph,
	); ok {
		t.Error("unrelated edges shared a segment")
	}
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
	if !ok || cost != want || crossings != 1 {
		t.Errorf("crossing step = (%d, %d, %t), want (%d, 1, true)", cost, crossings, ok, want)
	}
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

	if _, _, ok := router.stepCost(
		1,
		Point{X: 1, Y: 2},
		Point{X: 2, Y: 2},
		&occupancy,
		&graph,
	); ok {
		t.Error("unrelated edge touched an endpoint")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("rerouteCrossings() did not replace the earlier crossing edge")
	}
	_, crossings, ok := router.scorePath(
		0,
		geo.scratch.paths[0],
		&geo.scratch.occupancy,
		&graph,
	)
	if !ok {
		t.Fatal("rerouted path is invalid")
	}
	if crossings != 0 {
		t.Errorf("rerouted path has %d crossing", crossings)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	got, ok := grid.At(Point{X: 4, Y: 1})
	if !ok {
		t.Fatal("Rasterize() omitted port cell")
	}
	want := North | East | South
	if got != want {
		t.Errorf("Rasterize() port connections = %04b, want %04b", got, want)
	}
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
