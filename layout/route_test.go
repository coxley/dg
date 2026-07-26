package layout

import "testing"

func TestRouteQueueOrdersByPriorityThenInsertion(t *testing.T) {
	t.Parallel()

	var queue routeQueue
	for _, item := range []routeItem{
		{priority: 20, order: 0},
		{priority: 10, order: 2},
		{priority: 10, order: 1},
		{priority: 30, order: 0},
	} {
		queue.push(item)
	}

	want := []routeItem{
		{priority: 10, order: 1},
		{priority: 10, order: 2},
		{priority: 20, order: 0},
		{priority: 30, order: 0},
	}
	for i := range want {
		if got := queue.pop(); got != want[i] {
			t.Fatalf("pop %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestRouteOrthogonalAvoidsObstacle(t *testing.T) {
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

	edge, err := RouteOrthogonal(a, b, []Rect{obstacle})
	if err != nil {
		t.Fatal(err)
	}
	if edge.Points[0] != a.Anchor {
		t.Errorf("RouteOrthogonal() starts at %+v, want %+v", edge.Points[0], a.Anchor)
	}
	if got := edge.Points[len(edge.Points)-1]; got != b.Anchor {
		t.Errorf("RouteOrthogonal() ends at %+v, want %+v", got, b.Anchor)
	}

	for i := 1; i < len(edge.Points); i++ {
		for _, point := range segmentPoints(edge.Points[i-1], edge.Points[i]) {
			if obstacle.Contains(point) {
				t.Fatalf("RouteOrthogonal() crosses obstacle at %+v: %+v", point, edge.Points)
			}
		}
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
