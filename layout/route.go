package layout

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/coxley/dg/ir"
)

var ErrNoRoute = errors.New("no orthogonal route")

// Costs controls how the router compares candidate paths.
type Costs struct {
	// Step is charged for each route step that does not share a segment.
	//
	// The default value of 10 is the baseline distance cost. Increasing Step
	// favors shorter routes, even when they require more bends or crossings.
	// Decreasing it makes longer detours cheaper.
	Step uint32

	// SharedStep is charged when traversing a segment owned by an edge
	// with a common port.
	//
	// The default value of 2 makes a shared step cost one fifth of a new step.
	// Values below Step encourage shared trunks. Setting it equal to Step makes
	// sharing neutral. Values above Step discourage sharing without forbidding
	// it.
	SharedStep uint32

	// Bend is charged whenever a route changes direction.
	//
	// The default value of 5 makes two bends cost one new step. Increasing Bend
	// produces straighter routes. A value of zero gives no preference between
	// paths of equal step and crossing cost.
	Bend uint32

	// Crossing is charged when a route crosses an unrelated edge.
	//
	// The default value of 15 makes one crossing cost three bends or one and a
	// half new steps. Crossing / Bend expresses the number of additional bends
	// preferred over one crossing. Zero makes crossings cost-neutral.
	Crossing uint32
}

// Router configures orthogonal edge routing.
type Router struct {
	Costs Costs

	// ReroutePasses bounds additional passes that reconsider crossing edges.
	ReroutePasses uint8
}

// DefaultRouter returns the default orthogonal router.
func DefaultRouter() Router {
	return Router{
		Costs: Costs{
			Step:       10,
			SharedStep: 2,
			Bend:       5,
			Crossing:   15,
		},
		ReroutePasses: 1,
	}
}

type direction uint8

const (
	north direction = iota + 1
	east
	south
	west
)

type routeState struct {
	point Point
	dir   direction
}

type routeItem struct {
	state     routeState
	cost      uint64
	priority  uint64
	crossings uint32
	order     uint32
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
	if result := cmp.Compare(a.crossings, b.crossings); result != 0 {
		return result
	}
	return cmp.Compare(a.order, b.order)
}

type routeScore struct {
	cost      uint64
	crossings uint32
}

func compareRouteScore(a, b routeScore) int {
	if result := cmp.Compare(a.cost, b.cost); result != 0 {
		return result
	}
	return cmp.Compare(a.crossings, b.crossings)
}

type routeSegment struct {
	a, b Point
}

type routeUse struct {
	edge        uint32
	next        uint32
	connections Connections
}

type routeOwner struct {
	edge uint32
	next uint32
}

type routeOccupancy struct {
	segments    map[routeSegment]uint32
	cells       map[Point]uint32
	segmentUses []routeOwner
	cellUses    []routeUse
}

type routeSearch struct {
	scores   map[routeState]routeScore
	previous map[routeState]routeState
	queue    routeQueue
}

func (s *routeSearch) reset() {
	if s.scores == nil {
		s.scores = make(map[routeState]routeScore)
		s.previous = make(map[routeState]routeState)
	} else {
		clear(s.scores)
		clear(s.previous)
	}
	s.queue = s.queue[:0]
}

type routeScratch struct {
	occupancy routeOccupancy
	search    routeSearch
	paths     [][]Point
	candidate []Point
}

func (s *routeScratch) reset(edgeCount int) {
	s.occupancy.reset()
	if cap(s.paths) < edgeCount {
		s.paths = slices.Grow(s.paths, edgeCount-len(s.paths))
	}
	s.paths = s.paths[:edgeCount]
	for i := range s.paths {
		s.paths[i] = s.paths[i][:0]
	}
	s.candidate = s.candidate[:0]
}

func newRouteOccupancy() routeOccupancy {
	var occupancy routeOccupancy
	occupancy.reset()
	return occupancy
}

func (o *routeOccupancy) reset() {
	if o.segments == nil {
		o.segments = make(map[routeSegment]uint32)
		o.cells = make(map[Point]uint32)
	} else {
		clear(o.segments)
		clear(o.cells)
	}
	o.segmentUses = o.segmentUses[:0]
	o.cellUses = o.cellUses[:0]
}

func (o *routeOccupancy) add(edgeID uint32, path []Point) {
	for i := 1; i < len(path); i++ {
		segment := newRouteSegment(path[i-1], path[i])
		head := o.segments[segment]
		if !o.segmentContains(head, edgeID) {
			o.segmentUses = append(o.segmentUses, routeOwner{
				edge: edgeID,
				next: head,
			})
			o.segments[segment] = uint32(len(o.segmentUses))
		}
	}
	for i, point := range path {
		o.addCellUse(point, edgeID, connectionsAt(path, i))
	}
}

func (o *routeOccupancy) addCellUse(
	point Point,
	edgeID uint32,
	connections Connections,
) {
	head := o.cells[point]
	for index := head; index != 0; index = o.cellUses[index-1].next {
		use := &o.cellUses[index-1]
		if use.edge == edgeID {
			use.connections |= connections
			return
		}
	}
	o.cellUses = append(o.cellUses, routeUse{
		edge:        edgeID,
		next:        head,
		connections: connections,
	})
	o.cells[point] = uint32(len(o.cellUses))
}

func (o *routeOccupancy) remove(edgeID uint32, path []Point) {
	for i := 1; i < len(path); i++ {
		segment := newRouteSegment(path[i-1], path[i])
		head := o.removeSegmentOwner(o.segments[segment], edgeID)
		if head == 0 {
			delete(o.segments, segment)
		} else {
			o.segments[segment] = head
		}
	}
	for _, point := range path {
		head := o.removeCellUse(o.cells[point], edgeID)
		if head == 0 {
			delete(o.cells, point)
		} else {
			o.cells[point] = head
		}
	}
}

func (o *routeOccupancy) segmentContains(head, edgeID uint32) bool {
	for index := head; index != 0; index = o.segmentUses[index-1].next {
		if o.segmentUses[index-1].edge == edgeID {
			return true
		}
	}
	return false
}

func (o *routeOccupancy) removeSegmentOwner(head, edgeID uint32) uint32 {
	var previous uint32
	for index := head; index != 0; index = o.segmentUses[index-1].next {
		owner := &o.segmentUses[index-1]
		if owner.edge != edgeID {
			previous = index
			continue
		}
		if previous == 0 {
			return owner.next
		}
		o.segmentUses[previous-1].next = owner.next
		return head
	}
	return head
}

func (o *routeOccupancy) removeCellUse(head, edgeID uint32) uint32 {
	var previous uint32
	for index := head; index != 0; index = o.cellUses[index-1].next {
		use := &o.cellUses[index-1]
		if use.edge != edgeID {
			previous = index
			continue
		}
		if previous == 0 {
			return use.next
		}
		o.cellUses[previous-1].next = use.next
		return head
	}
	return head
}

func connectionsAt(path []Point, index int) Connections {
	var connections Connections
	if index > 0 {
		dir, ok := directionBetween(path[index], path[index-1])
		if ok {
			from, _ := directionConnections(dir)
			connections |= from
		}
	}
	if index+1 < len(path) {
		dir, ok := directionBetween(path[index], path[index+1])
		if ok {
			from, _ := directionConnections(dir)
			connections |= from
		}
	}
	return connections
}

func directionConnections(dir direction) (Connections, Connections) {
	switch dir {
	case north:
		return North, South
	case east:
		return East, West
	case south:
		return South, North
	case west:
		return West, East
	default:
		return 0, 0
	}
}

func newRouteSegment(a, b Point) routeSegment {
	if b.X < a.X || b.Y < a.Y {
		a, b = b, a
	}
	return routeSegment{a: a, b: b}
}

func (r Router) route(l *Layout) error {
	g := &l.graph
	scratch := &l.scratch
	scratch.reset(len(g.Edges))
	occupancy := &scratch.occupancy

	for i, edge := range g.Edges {
		if !g.EdgeExists(uint32(i)) {
			continue
		}
		path, err := r.findRoute(
			l,
			uint32(i),
			l.Ports[edge.PortA],
			l.Ports[edge.PortB],
			occupancy,
			&scratch.search,
			scratch.paths[i],
		)
		if err != nil {
			return fmt.Errorf("edge %d: %w", i, err)
		}
		scratch.paths[i] = path
		occupancy.add(uint32(i), path)
	}

	for range r.ReroutePasses {
		changed, err := r.rerouteCrossings(l)
		if err != nil {
			return err
		}
		if !changed {
			break
		}
	}

	for i := range scratch.paths {
		l.Edges[i].Points = compact(l.Edges[i].Points[:0], scratch.paths[i])
	}
	return nil
}

func (r Router) rerouteCrossings(
	l *Layout,
) (bool, error) {
	g := &l.graph
	scratch := &l.scratch
	paths := scratch.paths
	occupancy := &scratch.occupancy
	changed := false
	for i, edge := range g.Edges {
		edgeID := uint32(i)
		if !g.EdgeExists(edgeID) {
			continue
		}
		occupancy.remove(edgeID, paths[i])

		oldCost, oldCrossings, ok := r.scorePath(edgeID, paths[i], occupancy, g)
		if !ok {
			return false, fmt.Errorf("score edge %d", i)
		}
		if oldCrossings == 0 {
			occupancy.add(edgeID, paths[i])
			continue
		}

		candidate, err := r.findRoute(
			l,
			edgeID,
			l.Ports[edge.PortA],
			l.Ports[edge.PortB],
			occupancy,
			&scratch.search,
			scratch.candidate,
		)
		if err != nil {
			return false, fmt.Errorf("reroute edge %d: %w", i, err)
		}
		scratch.candidate = candidate
		candidateCost, candidateCrossings, ok := r.scorePath(edgeID, candidate, occupancy, g)
		if !ok {
			return false, fmt.Errorf("score rerouted edge %d", i)
		}
		if candidateCost < oldCost ||
			(candidateCost == oldCost && candidateCrossings < oldCrossings) {
			scratch.candidate = paths[i][:0]
			paths[i] = candidate
			changed = true
		} else {
			scratch.candidate = candidate[:0]
		}
		occupancy.add(edgeID, paths[i])
	}
	return changed, nil
}

func (r Router) findRoute(
	l *Layout,
	edgeID uint32,
	a, b Port,
	occupancy *routeOccupancy,
	search *routeSearch,
	path []Point,
) ([]Point, error) {
	startDir, ok := directionBetween(a.Anchor, a.Exit)
	if !ok {
		return nil, errors.New("invalid start port geometry")
	}
	endDir, ok := directionBetween(b.Exit, b.Anchor)
	if !ok {
		return nil, errors.New("invalid end port geometry")
	}

	bounds, err := l.routeBounds(a, b)
	if err != nil {
		return nil, fmt.Errorf("route bounds: %w", err)
	}
	if l.blocked(a.Exit) || l.blocked(b.Exit) {
		return nil, ErrNoRoute
	}

	start := routeState{point: a.Exit, dir: startDir}
	search.reset()
	search.scores[start] = routeScore{}
	search.queue.push(routeItem{
		state:    start,
		priority: r.heuristic(a.Exit, b.Exit, occupancy != nil),
	})

	var goal routeState
	bestGoal := routeScore{cost: math.MaxUint64, crossings: math.MaxUint32}
	var order uint32 = 1

	for len(search.queue) > 0 {
		item := search.queue.pop()
		itemScore := routeScore{cost: item.cost, crossings: item.crossings}
		if itemScore != search.scores[item.state] {
			continue
		}
		lowerBound := routeScore{cost: item.priority, crossings: item.crossings}
		if compareRouteScore(lowerBound, bestGoal) >= 0 {
			break
		}
		if item.state.point == b.Exit {
			candidate := itemScore
			if item.state.dir != endDir {
				candidate.cost = addCost(candidate.cost, uint64(r.Costs.Bend))
			}
			if compareRouteScore(candidate, bestGoal) < 0 {
				bestGoal = candidate
				goal = item.state
			}
			continue
		}

		for _, dir := range [...]direction{north, east, south, west} {
			nextPoint, ok := move(item.state.point, dir)
			if !ok || !bounds.Contains(nextPoint) || l.blocked(nextPoint) {
				continue
			}
			step, crossings, ok := r.stepCost(
				edgeID,
				item.state.point,
				nextPoint,
				occupancy,
				&l.graph,
			)
			if !ok {
				continue
			}

			next := routeState{point: nextPoint, dir: dir}
			nextScore := routeScore{
				cost:      addCost(item.cost, step),
				crossings: addCrossings(item.crossings, crossings),
			}
			if dir != item.state.dir {
				nextScore.cost = addCost(nextScore.cost, uint64(r.Costs.Bend))
			}
			if old, exists := search.scores[next]; exists &&
				compareRouteScore(old, nextScore) <= 0 {
				continue
			}

			search.scores[next] = nextScore
			search.previous[next] = item.state
			search.queue.push(routeItem{
				state:     next,
				cost:      nextScore.cost,
				priority:  addCost(nextScore.cost, r.heuristic(nextPoint, b.Exit, occupancy != nil)),
				crossings: nextScore.crossings,
				order:     order,
			})
			order++
		}
	}

	if bestGoal.cost == math.MaxUint64 {
		return nil, ErrNoRoute
	}

	path = append(path[:0], goal.point)
	for state := goal; state != start; {
		state = search.previous[state]
		path = append(path, state.point)
	}
	path = append(path, a.Anchor)
	reverse(path)
	path = append(path, b.Anchor)
	return path, nil
}

func (r Router) heuristic(a, b Point, sharing bool) uint64 {
	step := r.Costs.Step
	if sharing {
		step = min(step, r.Costs.SharedStep)
	}
	return multiplyCost(manhattan(a, b), uint64(step))
}

func (r Router) stepCost(
	edgeID uint32,
	a, b Point,
	occupancy *routeOccupancy,
	g *ir.Graph,
) (uint64, uint32, bool) {
	if occupancy == nil {
		return uint64(r.Costs.Step), 0, true
	}

	// Sharing applies to entire routes. More sophisticated routing may need to
	// restrict shared segments to the common port's vicinity.
	segmentOwners := occupancy.segments[newRouteSegment(a, b)]
	shared := false
	for index := segmentOwners; index != 0; index = occupancy.segmentUses[index-1].next {
		owner := occupancy.segmentUses[index-1].edge
		if owner == edgeID {
			continue
		}
		if !g.Edges[edgeID].SharesPort(g.Edges[owner]) {
			return 0, 0, false
		}
		shared = true
	}

	cost := uint64(r.Costs.Step)
	if shared {
		cost = uint64(r.Costs.SharedStep)
	}

	dir, ok := directionBetween(a, b)
	if !ok {
		return 0, 0, false
	}
	horizontal := dir == east || dir == west
	var crossings uint32
	for index := occupancy.cells[b]; index != 0; index = occupancy.cellUses[index-1].next {
		use := occupancy.cellUses[index-1]
		if use.edge == edgeID ||
			occupancy.segmentContains(segmentOwners, use.edge) ||
			g.Edges[edgeID].SharesPort(g.Edges[use.edge]) {
			continue
		}
		along, across := East|West, North|South
		if !horizontal {
			along, across = across, along
		}
		if use.connections.ContainsAny(along) {
			return 0, 0, false
		}
		if use.connections.ContainsAll(across) {
			crossings++
			continue
		}
		return 0, 0, false
	}
	cost = addCost(cost, multiplyCost(uint64(crossings), uint64(r.Costs.Crossing)))
	return cost, crossings, true
}

func (r Router) scorePath(
	edgeID uint32,
	path []Point,
	occupancy *routeOccupancy,
	g *ir.Graph,
) (uint64, uint32, bool) {
	if len(path) < 3 {
		return 0, 0, false
	}
	dir, ok := directionBetween(path[0], path[1])
	if !ok {
		return 0, 0, false
	}

	var cost uint64
	var crossings uint32
	for i := 2; i < len(path)-1; i++ {
		nextDir, ok := directionBetween(path[i-1], path[i])
		if !ok {
			return 0, 0, false
		}
		step, crossed, ok := r.stepCost(edgeID, path[i-1], path[i], occupancy, g)
		if !ok {
			return 0, 0, false
		}
		cost = addCost(cost, step)
		crossings = addCrossings(crossings, crossed)
		if nextDir != dir {
			cost = addCost(cost, uint64(r.Costs.Bend))
		}
		dir = nextDir
	}

	endDir, ok := directionBetween(path[len(path)-2], path[len(path)-1])
	if !ok {
		return 0, 0, false
	}
	if dir != endDir {
		cost = addCost(cost, uint64(r.Costs.Bend))
	}
	return cost, crossings, true
}

func (l *Layout) routeBounds(a, b Port) (Rect, error) {
	minX, maxX := min(a.Exit.X, b.Exit.X), max(a.Exit.X, b.Exit.X)
	minY, maxY := min(a.Exit.Y, b.Exit.Y), max(a.Exit.Y, b.Exit.Y)
	for obstacle := range l.Obstacles() {
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

func (l *Layout) blocked(p Point) bool {
	for obstacle := range l.Obstacles() {
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

func manhattan(a, b Point) uint64 {
	return uint64(max(a.X, b.X)-min(a.X, b.X)) +
		uint64(max(a.Y, b.Y)-min(a.Y, b.Y))
}

func addCost(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}

func multiplyCost(a, b uint64) uint64 {
	if a != 0 && b > math.MaxUint64/a {
		return math.MaxUint64
	}
	return a * b
}

func addCrossings(a, b uint32) uint32 {
	if math.MaxUint32-a < b {
		return math.MaxUint32
	}
	return a + b
}

func reverse(points []Point) {
	for left, right := 0, len(points)-1; left < right; left, right = left+1, right-1 {
		points[left], points[right] = points[right], points[left]
	}
}

func compact(dst, points []Point) []Point {
	if len(points) < 3 {
		return append(dst, points...)
	}

	dst = append(dst, points[0])
	for i := 1; i < len(points)-1; i++ {
		before, after := points[i-1], points[i+1]
		if before.X == after.X || before.Y == after.Y {
			continue
		}
		dst = append(dst, points[i])
	}
	return append(dst, points[len(points)-1])
}
