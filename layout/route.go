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

	// EndpointStep is charged in addition to Step or SharedStep for each cell
	// traversed inside either endpoint node.
	//
	// The default value of 40 makes one endpoint cell cost four open steps.
	// Increasing EndpointStep favors longer routes around source and destination
	// nodes. Zero lets endpoint-node traversal compete on ordinary path cost.
	// The cost does not forbid traversal, so overlapping endpoints remain
	// routable.
	EndpointStep uint32
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
			Step:         10,
			SharedStep:   2,
			Bend:         5,
			Crossing:     15,
			EndpointStep: 40,
		},
		ReroutePasses: 1,
	}
}

// PreviewRoute returns an obstacle-aware route from sourcePort to point.
// The route targets a usable port at point when one exists; otherwise point
// acts as a temporary port whose approach side is selected by route cost.
// PreviewRoute reuses dst when it has sufficient capacity.
func (l *Layout) PreviewRoute(
	dst []Point,
	sourcePort uint32,
	point Point,
) ([]Point, error) {
	return l.previewRoute(dst, sourcePort, point, math.MaxUint32)
}

// PreviewRouteWithoutEdge returns a preview that omits edgeID from occupancy
// and reuses dst when it has sufficient capacity.
func (l *Layout) PreviewRouteWithoutEdge(
	dst []Point,
	sourcePort uint32,
	point Point,
	edgeID uint32,
) ([]Point, error) {
	if !l.graph.EdgeExists(edgeID) {
		return nil, fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	return l.previewRoute(dst, sourcePort, point, edgeID)
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

type routeEdge struct {
	id       uint32
	ports    ir.Edge
	hasPorts bool
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
	expanded  []Point
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

func (l *Layout) previewRoute(
	dst []Point,
	sourcePort uint32,
	point Point,
	excludedEdge uint32,
) ([]Point, error) {
	if !l.graph.PortExists(sourcePort) {
		return nil, fmt.Errorf("%w: %d", ir.ErrPortNotFound, sourcePort)
	}
	if !l.PortUsable(sourcePort) {
		return nil, fmt.Errorf("%w: %d", ErrPortUnavailable, sourcePort)
	}

	scratch := &l.scratch
	occupancy := &scratch.occupancy
	occupancy.reset()
	for edgeID, edge := range l.Edges {
		if uint32(edgeID) == excludedEdge || edge.Empty() {
			continue
		}
		var err error
		scratch.expanded, err = appendExpandedPath(
			scratch.expanded[:0],
			edge.Points,
		)
		if err != nil {
			return nil, fmt.Errorf("expand edge %d: %w", edgeID, err)
		}
		occupancy.add(uint32(edgeID), scratch.expanded)
	}

	var destinations [4]Port
	var destinationPorts [4]uint32
	destinationCount := 0
	for portID, port := range l.Ports {
		if uint32(portID) != sourcePort &&
			l.graph.PortExists(uint32(portID)) &&
			l.PortUsable(uint32(portID)) &&
			port.Anchor == point {
			destinations[0] = port
			destinationPorts[0] = uint32(portID)
			destinationCount = 1
			break
		}
	}
	if destinationCount == 0 {
		for _, dir := range [...]direction{north, east, south, west} {
			exit, ok := move(point, dir)
			if !ok {
				continue
			}
			destinations[destinationCount] = Port{
				Anchor: point,
				Exit:   exit,
			}
			destinationPorts[destinationCount] = NoPortID
			destinationCount++
		}
	}

	best := routeScore{cost: math.MaxUint64, crossings: math.MaxUint32}
	found := false
	for i, destination := range destinations[:destinationCount] {
		route := routeEdge{
			id: math.MaxUint32,
			ports: ir.Edge{
				PortA: sourcePort,
				PortB: destinationPorts[i],
			},
			hasPorts: true,
		}
		candidate, err := l.router.findRouteFor(
			l,
			route,
			l.Ports[sourcePort],
			destination,
			occupancy,
			&scratch.search,
			scratch.candidate[:0],
		)
		if errors.Is(err, ErrNoRoute) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("route preview: %w", err)
		}
		scratch.candidate = candidate
		cost, crossings, ok := l.router.scorePathFor(
			l,
			route,
			candidate,
			occupancy,
		)
		if !ok {
			return nil, errors.New("score preview route")
		}
		score := routeScore{cost: cost, crossings: crossings}
		if found && compareRouteScore(best, score) <= 0 {
			continue
		}
		dst = compact(dst[:0], candidate)
		best = score
		found = true
	}
	if !found {
		return nil, ErrNoRoute
	}
	return dst, nil
}

func appendExpandedPath(dst, path []Point) ([]Point, error) {
	if len(path) == 0 {
		return dst, nil
	}
	dst = append(dst, path[0])
	for i := 1; i < len(path); i++ {
		err := walkSegment(path[i-1], path[i], func(point Point) {
			dst = append(dst, point)
		})
		if err != nil {
			return nil, fmt.Errorf("segment %d: %w", i-1, err)
		}
	}
	return dst, nil
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

		oldCost, oldCrossings, ok := r.scorePath(l, edgeID, paths[i], occupancy)
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
		candidateCost, candidateCrossings, ok := r.scorePath(
			l,
			edgeID,
			candidate,
			occupancy,
		)
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
	return r.findRouteFor(
		l,
		l.routeEdge(edgeID),
		a,
		b,
		occupancy,
		search,
		path,
	)
}

func (r Router) findRouteFor(
	l *Layout,
	route routeEdge,
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
	if l.blockedForRoute(route, a.Exit) ||
		l.blockedForRoute(route, b.Exit) {
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
			if !ok ||
				!bounds.Contains(nextPoint) ||
				l.blockedForRoute(route, nextPoint) {
				continue
			}
			step, crossings, ok := r.stepCostFor(
				l,
				route,
				item.state.point,
				nextPoint,
				occupancy,
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
	l *Layout,
	edgeID uint32,
	a, b Point,
	occupancy *routeOccupancy,
) (uint64, uint32, bool) {
	return r.stepCostFor(l, l.routeEdge(edgeID), a, b, occupancy)
}

func (r Router) stepCostFor(
	l *Layout,
	route routeEdge,
	a, b Point,
	occupancy *routeOccupancy,
) (uint64, uint32, bool) {
	g := &l.graph
	if occupancy == nil {
		return addCost(
			uint64(r.Costs.Step),
			uint64(r.endpointStepCostFor(l, route, b)),
		), 0, true
	}

	// Common-endpoint routes share only after the common port becomes nearer
	// than either distinct port. Earlier joins resemble a connection between
	// the distinct endpoints.
	segmentOwners := occupancy.segments[newRouteSegment(a, b)]
	shared := false
	for index := segmentOwners; index != 0; index = occupancy.segmentUses[index-1].next {
		owner := occupancy.segmentUses[index-1].edge
		if owner == route.id {
			continue
		}
		if !route.hasPorts ||
			!route.ports.SharesPort(g.Edges[owner]) {
			return 0, 0, false
		}
		if !l.edgesShareAt(route.ports, g.Edges[owner], b) {
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
		if use.edge == route.id ||
			occupancy.segmentContains(segmentOwners, use.edge) {
			continue
		}
		if route.hasPorts &&
			route.ports.SharesPort(g.Edges[use.edge]) {
			if l.edgesShareAt(route.ports, g.Edges[use.edge], b) {
				continue
			}
			return 0, 0, false
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
	cost = addCost(cost, uint64(r.endpointStepCostFor(l, route, b)))
	return cost, crossings, true
}

func (l *Layout) edgesShareAt(edgeA, edgeB ir.Edge, point Point) bool {
	common, otherA, ok := sharedPorts(edgeA, edgeB)
	if !ok {
		return false
	}
	otherB := edgeB.PortA
	if otherB == common {
		otherB = edgeB.PortB
	}
	if uint64(common) >= uint64(len(l.Ports)) ||
		uint64(otherA) >= uint64(len(l.Ports)) ||
		uint64(otherB) >= uint64(len(l.Ports)) {
		return false
	}
	commonDistance := manhattan(point, l.Ports[common].Anchor)
	return commonDistance <= manhattan(point, l.Ports[otherA].Anchor) &&
		commonDistance <= manhattan(point, l.Ports[otherB].Anchor)
}

func sharedPorts(a, b ir.Edge) (common, otherA uint32, ok bool) {
	switch {
	case b.HasPort(a.PortA):
		return a.PortA, a.PortB, true
	case b.HasPort(a.PortB):
		return a.PortB, a.PortA, true
	default:
		return 0, 0, false
	}
}

func (r Router) scorePath(
	l *Layout,
	edgeID uint32,
	path []Point,
	occupancy *routeOccupancy,
) (uint64, uint32, bool) {
	return r.scorePathFor(l, l.routeEdge(edgeID), path, occupancy)
}

func (r Router) scorePathFor(
	l *Layout,
	route routeEdge,
	path []Point,
	occupancy *routeOccupancy,
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
		step, crossed, ok := r.stepCostFor(
			l,
			route,
			path[i-1],
			path[i],
			occupancy,
		)
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

func (r Router) endpointStepCostFor(
	l *Layout,
	route routeEdge,
	point Point,
) uint32 {
	if r.Costs.EndpointStep == 0 || !route.hasPorts {
		return 0
	}
	for _, portID := range [...]uint32{route.ports.PortA, route.ports.PortB} {
		if uint64(portID) >= uint64(len(l.graph.Ports)) {
			continue
		}
		nodeID := l.graph.Ports[portID].Node
		if uint64(nodeID) < uint64(len(l.Nodes)) &&
			l.Nodes[nodeID].Rect.Contains(point) {
			return r.Costs.EndpointStep
		}
	}
	return 0
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

func (l *Layout) routeEdge(edgeID uint32) routeEdge {
	if !l.graph.EdgeExists(edgeID) {
		return routeEdge{id: edgeID}
	}
	return routeEdge{
		id:       edgeID,
		ports:    l.graph.Edges[edgeID],
		hasPorts: true,
	}
}

func (l *Layout) blockedForRoute(route routeEdge, p Point) bool {
	if !route.hasPorts {
		return l.blocked(p)
	}
	var endpointNodes [2]uint32
	endpointCount := 0
	for _, portID := range [...]uint32{route.ports.PortA, route.ports.PortB} {
		if uint64(portID) >= uint64(len(l.graph.Ports)) {
			continue
		}
		endpointNodes[endpointCount] = l.graph.Ports[portID].Node
		endpointCount++
	}
	for nodeID, node := range l.Nodes {
		// Endpoint nodes may overlap. Their edge can traverse the overlap and
		// the raster layer later hides the covered part of the route.
		if !slices.Contains(endpointNodes[:endpointCount], uint32(nodeID)) &&
			node.Rect.Contains(p) {
			return true
		}
	}
	return false
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
		if p.X == math.MaxUint32 {
			return Point{}, false
		}
		p.X++
	case south:
		if p.Y == math.MaxUint32 {
			return Point{}, false
		}
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
