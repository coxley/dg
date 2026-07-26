package layout

import (
	"cmp"
	"errors"
	"fmt"
	"math"
)

var ErrNoRoute = errors.New("no orthogonal route")

type direction uint8

const (
	north direction = iota + 1
	east
	south
	west
)

var allDirections = [...]direction{north, east, south, west}

const (
	stepCost = 10
	bendCost = 5
)

type routeState struct {
	point Point
	dir   direction
}

type routeItem struct {
	state    routeState
	cost     int
	priority int
	order    int
}

type routeQueue []routeItem

func (q *routeQueue) push(item routeItem) {
	*q = append(*q, item)
	index := len(*q) - 1
	for index > 0 {
		parent := (index - 1) / 2
		if compareRouteItem((*q)[parent], (*q)[index]) <= 0 {
			break
		}
		(*q)[parent], (*q)[index] = (*q)[index], (*q)[parent]
		index = parent
	}
}

func (q *routeQueue) pop() routeItem {
	root := (*q)[0]
	last := len(*q) - 1
	(*q)[0] = (*q)[last]
	(*q)[last] = routeItem{}
	*q = (*q)[:last]

	for parent := 0; ; {
		left := 2*parent + 1
		if left >= len(*q) {
			break
		}
		child := left
		right := left + 1
		if right < len(*q) && compareRouteItem((*q)[right], (*q)[left]) < 0 {
			child = right
		}
		if compareRouteItem((*q)[parent], (*q)[child]) <= 0 {
			break
		}
		(*q)[parent], (*q)[child] = (*q)[child], (*q)[parent]
		parent = child
	}
	return root
}

func compareRouteItem(a, b routeItem) int {
	if result := cmp.Compare(a.priority, b.priority); result != 0 {
		return result
	}
	return cmp.Compare(a.order, b.order)
}

// RouteOrthogonal finds a short, low-bend route that avoids obstacle cells.
func RouteOrthogonal(a, b Port, obstacles []Rect) (Edge, error) {
	startDir, ok := directionBetween(a.Anchor, a.Exit)
	if !ok {
		return Edge{}, errors.New("invalid start port geometry")
	}
	endDir, ok := directionBetween(b.Exit, b.Anchor)
	if !ok {
		return Edge{}, errors.New("invalid end port geometry")
	}

	bounds, err := routeBounds(a, b, obstacles)
	if err != nil {
		return Edge{}, fmt.Errorf("route bounds: %w", err)
	}
	if blocked(a.Exit, obstacles) || blocked(b.Exit, obstacles) {
		return Edge{}, ErrNoRoute
	}

	start := routeState{point: a.Exit, dir: startDir}
	costs := map[routeState]int{start: 0}
	previous := make(map[routeState]routeState)
	queue := routeQueue{{
		state:    start,
		priority: manhattan(a.Exit, b.Exit) * stepCost,
	}}

	var goal routeState
	bestGoal := math.MaxInt
	order := 1

	for len(queue) > 0 {
		item := queue.pop()
		if item.cost != costs[item.state] {
			continue
		}
		if item.priority >= bestGoal {
			break
		}
		if item.state.point == b.Exit {
			total := item.cost
			if item.state.dir != endDir {
				total += bendCost
			}
			if total < bestGoal {
				bestGoal = total
				goal = item.state
			}
			continue
		}

		for _, dir := range allDirections {
			nextPoint, ok := move(item.state.point, dir)
			if !ok || !bounds.Contains(nextPoint) || blocked(nextPoint, obstacles) {
				continue
			}
			next := routeState{point: nextPoint, dir: dir}
			nextCost := item.cost + stepCost
			if dir != item.state.dir {
				nextCost += bendCost
			}
			if old, exists := costs[next]; exists && old <= nextCost {
				continue
			}

			costs[next] = nextCost
			previous[next] = item.state
			queue.push(routeItem{
				state:    next,
				cost:     nextCost,
				priority: nextCost + manhattan(nextPoint, b.Exit)*stepCost,
				order:    order,
			})
			order++
		}
	}

	if bestGoal == math.MaxInt {
		return Edge{}, ErrNoRoute
	}

	points := []Point{goal.point}
	for state := goal; state != start; {
		state = previous[state]
		points = append(points, state.point)
	}
	reverse(points)
	points = append([]Point{a.Anchor}, points...)
	points = append(points, b.Anchor)

	return Edge{Points: compact(points)}, nil
}

func routeBounds(a, b Port, obstacles []Rect) (Rect, error) {
	minX, maxX := min(a.Exit.X, b.Exit.X), max(a.Exit.X, b.Exit.X)
	minY, maxY := min(a.Exit.Y, b.Exit.Y), max(a.Exit.Y, b.Exit.Y)
	for _, obstacle := range obstacles {
		obstacleMax := obstacle.Max()
		minX = min(minX, obstacle.Min.X)
		minY = min(minY, obstacle.Min.Y)
		maxX = max(maxX, obstacleMax.X-1)
		maxY = max(maxY, obstacleMax.Y-1)
	}
	const margin uint32 = 2
	if maxX >= math.MaxUint32-margin || maxY >= math.MaxUint32-margin {
		return Rect{}, errors.New("route bounds exceed coordinate space")
	}
	origin := Point{
		X: minX - min(minX, margin),
		Y: minY - min(minY, margin),
	}
	limit := Point{X: maxX, Y: maxY}.Add(margin+1, margin+1)
	return NewRect(origin, limit)
}

func blocked(p Point, obstacles []Rect) bool {
	for _, obstacle := range obstacles {
		if obstacle.Contains(p) {
			return true
		}
	}
	return false
}

func move(p Point, dir direction) (Point, bool) {
	switch dir {
	case north:
		if p.Y == 0 {
			return Point{}, false
		}
		p.Y--
	case east:
		p.X++
	case south:
		p.Y++
	case west:
		if p.X == 0 {
			return Point{}, false
		}
		p.X--
	}
	return p, true
}

func directionBetween(a, b Point) (direction, bool) {
	switch {
	case a.X == b.X && b.Y < a.Y && a.Y-b.Y == 1:
		return north, true
	case a.Y == b.Y && b.X > a.X && b.X-a.X == 1:
		return east, true
	case a.X == b.X && b.Y > a.Y && b.Y-a.Y == 1:
		return south, true
	case a.Y == b.Y && b.X < a.X && a.X-b.X == 1:
		return west, true
	default:
		return 0, false
	}
}

func manhattan(a, b Point) int {
	return int(max(a.X, b.X)-min(a.X, b.X)) + int(max(a.Y, b.Y)-min(a.Y, b.Y))
}

func reverse(points []Point) {
	for left, right := 0, len(points)-1; left < right; left, right = left+1, right-1 {
		points[left], points[right] = points[right], points[left]
	}
}

func compact(points []Point) []Point {
	if len(points) < 3 {
		return points
	}

	compacted := make([]Point, 0, len(points))
	compacted = append(compacted, points[0])
	for i := 1; i < len(points)-1; i++ {
		before, after := points[i-1], points[i+1]
		if before.X == after.X || before.Y == after.Y {
			continue
		}
		compacted = append(compacted, points[i])
	}
	return append(compacted, points[len(points)-1])
}
