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

	// ReroutePasses bounds additional route-improvement work. A nonzero value
	// first aligns earlier common-port routes with their later siblings, then
	// reconsiders crossing edges for at most this many passes.
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
	return l.previewRoute(dst, sourcePort, point, math.MaxUint32, EdgeStyle{})
}

// PreviewRouteStyled returns a preview that reserves straight endpoint cells
// for smart arrows.
func (l *Layout) PreviewRouteStyled(
	dst []Point,
	sourcePort uint32,
	point Point,
	style EdgeStyle,
) ([]Point, error) {
	if !style.Valid() {
		return nil, errors.New("invalid edge arrow style")
	}
	return l.previewRoute(dst, sourcePort, point, math.MaxUint32, style)
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
	return l.previewRoute(dst, sourcePort, point, edgeID, EdgeStyle{})
}

// PreviewRouteWithoutEdgeStyled returns a styled preview that omits edgeID
// from occupancy.
func (l *Layout) PreviewRouteWithoutEdgeStyled(
	dst []Point,
	sourcePort uint32,
	point Point,
	edgeID uint32,
	style EdgeStyle,
) ([]Point, error) {
	if !l.graph.EdgeExists(edgeID) {
		return nil, fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	if !style.Valid() {
		return nil, errors.New("invalid edge arrow style")
	}
	return l.previewRoute(dst, sourcePort, point, edgeID, style)
}

type direction uint8

const (
	north direction = iota + 1
	east
	south
	west
)

// routeState includes arrival direction because bends affect future cost.
type routeState struct {
	point Point
	dir   direction
}

// routeEdge caches route-specific obstacle and arrow constraints.
type routeEdge struct {
	id            uint32
	ports         ir.Edge
	endpointNodes [2]uint32
	endpointCount uint8
	hasPorts      bool
	straightStart bool
	straightEnd   bool
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

type sharedBranchCell struct {
	commonPort uint32
	point      Point
}

// routeOccupancy indexes every cell and unit segment of expanded routes.
type routeOccupancy struct {
	segments    map[routeSegment]uint32
	cells       map[Point]uint32
	segmentUses []routeOwner
	cellUses    []routeUse
	edgeUses    []uint32
}

type obstacleUse struct {
	node uint32
	next uint32
}

// obstacleIndex uses 16 by 16 cell buckets without rasterizing covered cells.
// It scans nodes larger than 64 buckets directly to bound index storage.
type obstacleIndex struct {
	buckets map[Point]uint32
	uses    []obstacleUse
	large   []uint32
	ready   bool
}

// routeSearch retains A* scores, predecessors, and its priority queue.
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

// routeScratch retains all routing work buffers across builds.
type routeScratch struct {
	occupancy       routeOccupancy
	obstacles       obstacleIndex
	search          routeSearch
	paths           [][]Point
	candidate       []Point
	expanded        []Point
	relaxed         []Point
	segment         []Point
	arrowPort       []bool
	lastEdge        []uint32
	affected        []bool
	affectedPort    []bool
	shortBranchArms map[sharedBranchCell]struct{}
}

func (s *routeScratch) reset(
	edgeCount, portCount int,
	nodes []Node,
) {
	s.occupancy.reset()
	s.obstacles.reset(nodes)
	if cap(s.paths) < edgeCount {
		s.paths = slices.Grow(s.paths, edgeCount-len(s.paths))
	}
	s.paths = s.paths[:edgeCount]
	for i := range s.paths {
		s.paths[i] = s.paths[i][:0]
	}
	s.candidate = s.candidate[:0]
	s.relaxed = s.relaxed[:0]
	s.segment = s.segment[:0]
	s.arrowPort = slices.Grow(
		s.arrowPort[:0],
		portCount,
	)[:portCount]
	clear(s.arrowPort)
	s.lastEdge = slices.Grow(s.lastEdge[:0], portCount)[:portCount]
	clear(s.lastEdge)
}

func (s *routeScratch) resetAffected(edgeCount, portCount int) {
	s.affected = slices.Grow(s.affected[:0], edgeCount)[:edgeCount]
	clear(s.affected)
	s.affectedPort = slices.Grow(s.affectedPort[:0], portCount)[:portCount]
	clear(s.affectedPort)
}

func (s *routeScratch) resetShortBranchArms(g *ir.Graph, ports []Port) {
	if s.shortBranchArms == nil {
		s.shortBranchArms = make(map[sharedBranchCell]struct{})
	} else {
		clear(s.shortBranchArms)
	}
	for edgeID, edge := range g.Edges {
		if !g.EdgeExists(uint32(edgeID)) {
			continue
		}
		s.markShortBranchArm(edge.PortA, edge.PortB, ports)
		s.markShortBranchArm(edge.PortB, edge.PortA, ports)
	}
}

func (s *routeScratch) markShortBranchArm(
	commonPort, distinctPort uint32,
	ports []Port,
) {
	if uint64(distinctPort) >= uint64(len(ports)) {
		return
	}
	port := ports[distinctPort]
	dir, ok := directionBetween(port.Anchor, port.Exit)
	if !ok || dir == north || dir == south {
		return
	}
	s.shortBranchArms[sharedBranchCell{
		commonPort: commonPort,
		point:      port.Exit,
	}] = struct{}{}
}

const (
	obstacleBucketShift       = 4
	maxObstacleBucketsPerNode = 64
)

func (i *obstacleIndex) reset(nodes []Node) {
	if i.buckets == nil {
		i.buckets = make(map[Point]uint32)
	} else {
		clear(i.buckets)
	}
	i.uses = i.uses[:0]
	i.large = i.large[:0]
	for nodeID, node := range nodes {
		if node.Empty() {
			continue
		}
		limit := node.Rect.Max()
		first := obstacleBucket(node.Rect.Min)
		last := obstacleBucket(Point{X: limit.X - 1, Y: limit.Y - 1})
		width := uint64(last.X-first.X) + 1
		height := uint64(last.Y-first.Y) + 1
		if width*height > maxObstacleBucketsPerNode {
			i.large = append(i.large, uint32(nodeID))
			continue
		}
		for y := first.Y; y <= last.Y; y++ {
			for x := first.X; x <= last.X; x++ {
				bucket := Point{X: x, Y: y}
				i.uses = append(i.uses, obstacleUse{
					node: uint32(nodeID),
					next: i.buckets[bucket],
				})
				i.buckets[bucket] = uint32(len(i.uses))
			}
		}
	}
	i.ready = true
}

func obstacleBucket(point Point) Point {
	return Point{
		X: point.X >> obstacleBucketShift,
		Y: point.Y >> obstacleBucketShift,
	}
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
	clear(o.edgeUses)
}

// addExpanded adds a path that lists every traversed cell.
func (o *routeOccupancy) addExpanded(edgeID uint32, path []Point) {
	if len(path) == 0 {
		return
	}
	if uint64(edgeID) >= uint64(len(o.edgeUses)) {
		oldLen := len(o.edgeUses)
		o.edgeUses = slices.Grow(
			o.edgeUses,
			int(edgeID)+1-oldLen,
		)[:int(edgeID)+1]
		clear(o.edgeUses[oldLen:])
	}
	o.edgeUses[edgeID]++

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

// addCompact expands bend vertices into occupancy and reuses dst for the path.
func (o *routeOccupancy) addCompact(
	edgeID uint32,
	dst []Point,
	path []Point,
) ([]Point, error) {
	expanded, err := appendExpandedPath(dst[:0], path)
	if err != nil {
		return dst, err
	}
	o.addExpanded(edgeID, expanded)
	return expanded, nil
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

// removeExpanded removes a path that lists every traversed cell.
func (o *routeOccupancy) removeExpanded(edgeID uint32, path []Point) {
	if len(path) == 0 {
		return
	}
	if uint64(edgeID) < uint64(len(o.edgeUses)) &&
		o.edgeUses[edgeID] > 0 {
		o.edgeUses[edgeID]--
	}

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

// canShare reports whether an active edge shares an exact endpoint port.
func (o *routeOccupancy) canShare(route routeEdge, edges []ir.Edge) bool {
	if !route.hasPorts {
		return false
	}
	for edgeID, uses := range o.edgeUses {
		if uses == 0 ||
			uint32(edgeID) == route.id ||
			edgeID >= len(edges) {
			continue
		}
		if route.ports.SharesPort(edges[edgeID]) {
			return true
		}
	}
	return false
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
	style EdgeStyle,
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
	scratch.obstacles.reset(l.Nodes)
	scratch.resetShortBranchArms(&l.graph, l.Ports)
	for edgeID, edge := range l.Edges {
		if uint32(edgeID) == excludedEdge || edge.Empty() {
			continue
		}
		var err error
		scratch.expanded, err = occupancy.addCompact(
			uint32(edgeID),
			scratch.expanded,
			edge.Points,
		)
		if err != nil {
			return nil, fmt.Errorf("expand edge %d: %w", edgeID, err)
		}
	}

	var destinations [4]Port
	var destinationPorts [4]uint32
	destinationCount := 0
	if portID, ok := l.usablePortAt(point, sourcePort); ok {
		destinations[0] = l.Ports[portID]
		destinationPorts[0] = portID
		destinationCount = 1
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
			hasPorts:      true,
			straightStart: style.PortAArrow != ArrowNone,
			straightEnd:   style.PortBArrow != ArrowNone,
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

// appendExpandedPath expands endpoint and bend vertices into traversed cells.
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

// route computes and commits all live edge routes:
//  1. Reset scratch and derive arrow-clearance requirements.
//  2. Route edges in ID order and add expanded paths to occupancy.
//  3. Reconsider crossing routes for the configured number of passes.
//  4. Compact expanded paths to endpoint and bend vertices.
func (r Router) route(l *Layout) error {
	g := &l.graph
	scratch := &l.scratch
	scratch.reset(len(g.Edges), len(g.Ports), l.Nodes)
	scratch.resetShortBranchArms(g, l.Ports)
	for edgeID, edge := range g.Edges {
		if !g.EdgeExists(uint32(edgeID)) ||
			uint64(edgeID) >= uint64(len(l.edgeStyles)) {
			continue
		}
		style := l.edgeStyles[edgeID]
		if style.PortAArrow != ArrowNone {
			scratch.arrowPort[edge.PortA] = true
		}
		if style.PortBArrow != ArrowNone {
			scratch.arrowPort[edge.PortB] = true
		}
		scratch.lastEdge[edge.PortA] = uint32(edgeID) + 1
		scratch.lastEdge[edge.PortB] = uint32(edgeID) + 1
	}
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
		occupancy.addExpanded(uint32(i), path)
	}

	if r.ReroutePasses != 0 {
		if _, err := r.rerouteSharedPredecessors(l, nil); err != nil {
			return err
		}
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

// rerouteSharedPredecessors aligns an earlier route's first branch with a later
// sibling's branch when the resulting path remains legal and no more costly.
func (r Router) rerouteSharedPredecessors(
	l *Layout,
	affected []bool,
) (bool, error) {
	g := &l.graph
	scratch := &l.scratch
	paths := scratch.paths
	occupancy := &scratch.occupancy
	changed := false
	for i, edge := range g.Edges {
		edgeID := uint32(i)
		if !g.EdgeExists(edgeID) || len(l.edgeBends[edgeID]) != 0 ||
			affected != nil && !affected[i] {
			continue
		}
		siblingID, commonPort, ok := l.laterSibling(edgeID, edge, affected)
		if !ok {
			continue
		}
		occupancy.removeExpanded(edgeID, paths[i])

		oldCost, oldCrossings, ok := r.scorePath(l, edgeID, paths[i], occupancy)
		if !ok {
			return false, fmt.Errorf("score shared edge %d", i)
		}
		currentReversed := edge.PortB == commonPort
		sibling := g.Edges[siblingID]
		candidate, ok := alignSharedBranch(
			scratch.candidate[:0],
			paths[i],
			paths[siblingID],
			currentReversed,
			sibling.PortB == commonPort,
		)
		if !ok || !l.pathClearForRoute(edgeID, candidate) {
			scratch.candidate = candidate[:0]
			occupancy.addExpanded(edgeID, paths[i])
			continue
		}
		scratch.candidate = candidate
		candidateCost, candidateCrossings, ok := r.scorePath(
			l,
			edgeID,
			candidate,
			occupancy,
		)
		if !ok {
			return false, fmt.Errorf("score rerouted shared edge %d", i)
		}
		oldScore := routeScore{cost: oldCost, crossings: oldCrossings}
		candidateScore := routeScore{
			cost:      candidateCost,
			crossings: candidateCrossings,
		}
		if !slices.Equal(candidate, paths[i]) &&
			compareRouteScore(candidateScore, oldScore) <= 0 {
			scratch.candidate = paths[i][:0]
			paths[i] = candidate
			changed = true
		} else {
			scratch.candidate = candidate[:0]
		}
		occupancy.addExpanded(edgeID, paths[i])
	}
	return changed, nil
}

func (l *Layout) laterSibling(
	edgeID uint32,
	edge ir.Edge,
	affected []bool,
) (uint32, uint32, bool) {
	for _, portID := range [...]uint32{edge.PortA, edge.PortB} {
		later := l.scratch.lastEdge[portID]
		if later > edgeID+1 &&
			(affected == nil || affected[later-1]) {
			return later - 1, portID, true
		}
	}
	return 0, 0, false
}

func alignSharedBranch(
	dst, current, sibling []Point,
	currentReversed, siblingReversed bool,
) ([]Point, bool) {
	pointAt := func(path []Point, reversed bool, index int) Point {
		if reversed {
			return path[len(path)-1-index]
		}
		return path[index]
	}
	limit := min(len(current), len(sibling))
	common := 0
	for common < limit &&
		pointAt(current, currentReversed, common) ==
			pointAt(sibling, siblingReversed, common) {
		common++
	}
	if common < 2 || common == limit {
		return dst, false
	}
	branch := pointAt(current, currentReversed, common-1)
	incoming, ok := directionBetween(
		pointAt(current, currentReversed, common-2),
		branch,
	)
	if !ok {
		return dst, false
	}
	currentNext, ok := directionBetween(
		branch,
		pointAt(current, currentReversed, common),
	)
	if !ok || currentNext != incoming {
		return dst, false
	}
	siblingNext, ok := directionBetween(
		branch,
		pointAt(sibling, siblingReversed, common),
	)
	if !ok || siblingNext == incoming || siblingNext == oppositeDirection(incoming) {
		return dst, false
	}

	firstTurn := common
	for firstTurn < len(current) {
		dir, valid := directionBetween(
			pointAt(current, currentReversed, firstTurn-1),
			pointAt(current, currentReversed, firstTurn),
		)
		if !valid {
			return dst, false
		}
		if dir != incoming {
			break
		}
		firstTurn++
	}
	if firstTurn == len(current) {
		return dst, false
	}
	turnDirection, _ := directionBetween(
		pointAt(current, currentReversed, firstTurn-1),
		pointAt(current, currentReversed, firstTurn),
	)
	secondTurn := firstTurn + 1
	for secondTurn < len(current) {
		dir, valid := directionBetween(
			pointAt(current, currentReversed, secondTurn-1),
			pointAt(current, currentReversed, secondTurn),
		)
		if !valid {
			return dst, false
		}
		if dir != turnDirection {
			if dir != incoming {
				return dst, false
			}
			break
		}
		secondTurn++
	}
	if secondTurn == len(current) {
		return dst, false
	}
	oldSecondBend := pointAt(current, currentReversed, secondTurn-1)
	alignedBend := Point{X: branch.X, Y: oldSecondBend.Y}
	if incoming == north || incoming == south {
		alignedBend = Point{X: oldSecondBend.X, Y: branch.Y}
	}
	if alignedBend == branch {
		return dst, false
	}

	dst = slices.Grow(dst[:0], len(current))
	for index := range common {
		dst = append(dst, pointAt(current, currentReversed, index))
	}
	if err := walkSegment(branch, alignedBend, func(point Point) {
		dst = append(dst, point)
	}); err != nil {
		return dst[:0], false
	}
	afterBend := pointAt(current, currentReversed, secondTurn)
	if err := walkSegment(alignedBend, afterBend, func(point Point) {
		dst = append(dst, point)
	}); err != nil {
		return dst[:0], false
	}
	for index := secondTurn + 1; index < len(current); index++ {
		dst = append(dst, pointAt(current, currentReversed, index))
	}
	if currentReversed {
		reverse(dst)
	}
	return dst, true
}

// routeSelection computes and commits only affected routes:
//  1. Seed occupancy with expanded routes outside the selection.
//  2. Preserve legal internal routes whose endpoints moved together.
//  3. Route the remaining affected edges against current occupancy.
//  4. Compact and commit only affected paths.
func (r Router) routeSelection(l *Layout) error {
	g := &l.graph
	scratch := &l.scratch
	scratch.reset(len(g.Edges), len(g.Ports), l.Nodes)
	scratch.resetAffected(len(g.Edges), len(g.Ports))
	scratch.resetShortBranchArms(g, l.Ports)
	for edgeID, edge := range g.Edges {
		id := uint32(edgeID)
		if !g.EdgeExists(id) {
			continue
		}
		style := l.edgeStyles[edgeID]
		if style.PortAArrow != ArrowNone {
			scratch.arrowPort[edge.PortA] = true
		}
		if style.PortBArrow != ArrowNone {
			scratch.arrowPort[edge.PortB] = true
		}
		scratch.lastEdge[edge.PortA] = id + 1
		scratch.lastEdge[edge.PortB] = id + 1
		if l.edgeDirectlySelectedForRouting(id) {
			scratch.affected[edgeID] = true
			scratch.affectedPort[edge.PortA] = true
			scratch.affectedPort[edge.PortB] = true
		}
	}
	for edgeID, edge := range g.Edges {
		if g.EdgeExists(uint32(edgeID)) &&
			(scratch.affectedPort[edge.PortA] || scratch.affectedPort[edge.PortB]) {
			scratch.affected[edgeID] = true
		}
	}
	for edgeID := range g.Edges {
		id := uint32(edgeID)
		if !g.EdgeExists(id) {
			continue
		}
		scratch.paths[edgeID] = append(
			scratch.paths[edgeID][:0],
			l.Edges[edgeID].Points...,
		)
		if !scratch.affected[edgeID] {
			var err error
			scratch.expanded, err = scratch.occupancy.addCompact(
				id,
				scratch.expanded,
				scratch.paths[edgeID],
			)
			if err != nil {
				return fmt.Errorf("expand edge %d: %w", edgeID, err)
			}
		}
	}

	for edgeID, edge := range g.Edges {
		id := uint32(edgeID)
		if !g.EdgeExists(id) || !scratch.affected[edgeID] {
			continue
		}
		if l.edgeEndpointsSelected(edge) && len(scratch.paths[edgeID]) >= 2 {
			expanded, err := appendExpandedPath(
				scratch.expanded[:0],
				scratch.paths[edgeID],
			)
			if err != nil {
				return fmt.Errorf("expand edge %d: %w", edgeID, err)
			}
			scratch.expanded = expanded
			valid := l.pathClearForRoute(id, expanded)
			if valid {
				_, _, valid = r.scorePath(
					l,
					id,
					expanded,
					&scratch.occupancy,
				)
			}
			if valid {
				scratch.paths[edgeID] = append(
					scratch.paths[edgeID][:0],
					expanded...,
				)
				scratch.occupancy.addExpanded(id, expanded)
				continue
			}
		}
		path, err := r.findRoute(
			l,
			id,
			l.Ports[edge.PortA],
			l.Ports[edge.PortB],
			&scratch.occupancy,
			&scratch.search,
			scratch.paths[edgeID],
		)
		if err != nil {
			return fmt.Errorf("edge %d: %w", edgeID, err)
		}
		scratch.paths[edgeID] = path
		scratch.occupancy.addExpanded(id, path)
	}
	if r.ReroutePasses != 0 {
		if _, err := r.rerouteSharedPredecessors(l, scratch.affected); err != nil {
			return err
		}
	}
	for edgeID := range g.Edges {
		id := uint32(edgeID)
		if !g.EdgeExists(id) || !scratch.affected[edgeID] {
			continue
		}
		l.Edges[edgeID].Points = compact(
			l.Edges[edgeID].Points[:0],
			scratch.paths[edgeID],
		)
	}
	return nil
}

func (l *Layout) pathClearForRoute(edgeID uint32, path []Point) bool {
	if len(path) < 2 {
		return false
	}
	route := l.routeEdge(edgeID)
	for _, point := range path[1 : len(path)-1] {
		if l.blockedForRoute(route, point) {
			return false
		}
	}
	return true
}

func (l *Layout) edgeDirectlySelectedForRouting(edgeID uint32) bool {
	if l.selection.Contains(Hit{ID: edgeID, Kind: HitEdge}) {
		return true
	}
	edge := l.graph.Edges[edgeID]
	nodeA := l.graph.Ports[edge.PortA].Node
	nodeB := l.graph.Ports[edge.PortB].Node
	return l.selection.Contains(Hit{ID: nodeA, Kind: HitNode}) ||
		l.selection.Contains(Hit{ID: nodeB, Kind: HitNode})
}

// rerouteCrossings replaces crossing routes only when score improves.
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
		occupancy.removeExpanded(edgeID, paths[i])

		oldCost, oldCrossings, ok := r.scorePath(l, edgeID, paths[i], occupancy)
		if !ok {
			return false, fmt.Errorf("score edge %d", i)
		}
		if oldCrossings == 0 {
			occupancy.addExpanded(edgeID, paths[i])
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
		occupancy.addExpanded(edgeID, paths[i])
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
	route := l.routeEdge(edgeID)
	if uint64(edgeID) < uint64(len(l.edgeBends)) &&
		len(l.edgeBends[edgeID]) != 0 {
		return r.findRouteThroughBends(
			l,
			route,
			a,
			b,
			l.edgeBends[edgeID],
			occupancy,
			search,
			path,
		)
	}
	return r.findRouteFor(
		l,
		route,
		a,
		b,
		occupancy,
		search,
		path,
	)
}

// findRouteFor prefers full smart-arrow clearance, then compares a relaxed
// search when full clearance fails or lengthens the route.
func (r Router) findRouteFor(
	l *Layout,
	route routeEdge,
	a, b Port,
	occupancy *routeOccupancy,
	search *routeSearch,
	path []Point,
) ([]Point, error) {
	route.endpointCount = 0
	if route.hasPorts {
		for _, portID := range [...]uint32{
			route.ports.PortA,
			route.ports.PortB,
		} {
			if uint64(portID) >= uint64(len(l.graph.Ports)) {
				continue
			}
			route.endpointNodes[route.endpointCount] = l.graph.Ports[portID].Node
			route.endpointCount++
		}
	}
	result, err := r.findRouteForClearance(
		l,
		route,
		a,
		b,
		occupancy,
		search,
		path,
		true,
	)
	if !route.straightStart && !route.straightEnd {
		return result, err
	}
	if err != nil && !errors.Is(err, ErrNoRoute) {
		return nil, err
	}
	if err == nil &&
		uint64(len(result)-1) == manhattan(a.Anchor, b.Anchor) {
		return result, nil
	}
	relaxed, relaxedErr := r.findRouteForClearance(
		l,
		route,
		a,
		b,
		occupancy,
		search,
		l.scratch.relaxed[:0],
		false,
	)
	l.scratch.relaxed = relaxed
	if relaxedErr != nil {
		if err == nil {
			return result, nil
		}
		return nil, relaxedErr
	}
	if err == nil {
		fullScore, fullCrossings, fullOK := r.scorePathFor(
			l,
			route,
			result,
			occupancy,
		)
		relaxedScore, relaxedCrossings, relaxedOK := r.scorePathFor(
			l,
			route,
			relaxed,
			occupancy,
		)
		if fullOK && (!relaxedOK || compareRouteScore(
			routeScore{cost: fullScore, crossings: fullCrossings},
			routeScore{cost: relaxedScore, crossings: relaxedCrossings},
		) <= 0) {
			return result, nil
		}
	}
	return append(path[:0], relaxed...), nil
}

// findRouteForClearance runs bounded A* over point and arrival direction.
func (r Router) findRouteForClearance(
	l *Layout,
	route routeEdge,
	a, b Port,
	occupancy *routeOccupancy,
	search *routeSearch,
	path []Point,
	extraClearance bool,
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
	startClearance, hasStartClearance := move(a.Exit, startDir)
	// SharedStep is legal only when an occupied route shares an exact port.
	minimumStep := r.Costs.Step
	if occupancy != nil &&
		r.Costs.SharedStep < minimumStep &&
		occupancy.canShare(route, l.graph.Edges) {
		minimumStep = r.Costs.SharedStep
	}
	search.reset()
	search.scores[start] = routeScore{}
	search.queue.push(routeItem{
		state:    start,
		priority: routeHeuristic(start.point, b.Exit, minimumStep),
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
			if route.straightEnd && item.state.dir != endDir {
				continue
			}
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
			if !routeStartDirectionAllowed(
				route,
				item.state,
				start,
				startClearance,
				hasStartClearance,
				dir,
				startDir,
				extraClearance,
			) {
				continue
			}
			nextPoint, ok := move(item.state.point, dir)
			if !ok ||
				!bounds.Contains(nextPoint) ||
				l.blockedForRoute(route, nextPoint) {
				continue
			}
			if !routeEndDirectionAllowed(
				route,
				nextPoint,
				b.Exit,
				item.state.dir,
				dir,
				endDir,
				extraClearance,
			) {
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
				state: next,
				cost:  nextScore.cost,
				priority: addCost(
					nextScore.cost,
					routeHeuristic(next.point, b.Exit, minimumStep),
				),
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

func routeStartDirectionAllowed(
	route routeEdge,
	state, start routeState,
	clearance Point,
	hasClearance bool,
	dir, startDir direction,
	extraClearance bool,
) bool {
	if !route.straightStart || dir == startDir {
		return true
	}
	return state != start &&
		(!extraClearance || !hasClearance || state.point != clearance)
}

func routeEndDirectionAllowed(
	route routeEdge,
	next, exit Point,
	previousDir, dir, endDir direction,
	extraClearance bool,
) bool {
	if !route.straightEnd || next != exit {
		return true
	}
	return dir == endDir && (!extraClearance || previousDir == endDir)
}

// routeHeuristic returns an admissible Manhattan lower bound.
func routeHeuristic(
	point Point,
	goal Point,
	step uint32,
) uint64 {
	return multiplyCost(manhattan(point, goal), uint64(step))
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

	// Common-endpoint routes may share across the distance between their common
	// port and branch point. Each distinct port retains a visible arm from the
	// shared route into its node.
	segmentOwners := occupancy.segments[newRouteSegment(a, b)]
	shared := false
	clearBranchArms := true
	nearCommon := true
	for index := segmentOwners; index != 0; index = occupancy.segmentUses[index-1].next {
		owner := occupancy.segmentUses[index-1].edge
		if owner == route.id {
			continue
		}
		if !route.hasPorts ||
			!route.ports.SharesPort(g.Edges[owner]) {
			return 0, 0, false
		}
		shared = true
		armsClear, stepNearCommon := l.sharedStepPreferences(route, owner, b)
		clearBranchArms = clearBranchArms && armsClear
		nearCommon = nearCommon && stepNearCommon
	}

	cost := uint64(r.Costs.Step)
	if shared && clearBranchArms && nearCommon {
		cost = uint64(r.Costs.SharedStep)
	} else if shared && !clearBranchArms {
		// Prefer one more arm cell when both paths have the same length and bends.
		cost = addCost(cost, uint64(r.Costs.Bend))
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
	cost = addCost(cost, uint64(r.endpointStepCostFor(l, route, b)))
	return cost, crossings, true
}

func (l *Layout) sharedStepPreferences(
	route routeEdge,
	edgeBID uint32,
	point Point,
) (bool, bool) {
	if uint64(edgeBID) >= uint64(len(l.graph.Edges)) {
		return false, false
	}
	edgeA, edgeB := route.ports, l.graph.Edges[edgeBID]
	if edgeB.HasPort(edgeA.PortA) && edgeB.HasPort(edgeA.PortB) {
		return true, true
	}
	common, otherA, ok := sharedPorts(edgeA, edgeB)
	if !ok {
		return false, false
	}
	otherB := edgeB.PortA
	if otherB == common {
		otherB = edgeB.PortB
	}
	if uint64(common) >= uint64(len(l.Ports)) ||
		uint64(otherB) >= uint64(len(l.Ports)) {
		return false, false
	}
	commonDistance := manhattan(point, l.Ports[common].Anchor)
	armsClear := true
	stepNearCommon := true
	if uint64(otherA) < uint64(len(l.Ports)) {
		armsClear = branchArmClear(point, l.Ports[otherA])
		stepNearCommon = commonDistance <= manhattan(point, l.Ports[otherA].Anchor)
	}
	armsClear = armsClear && branchArmClear(point, l.Ports[otherB])
	stepNearCommon = stepNearCommon &&
		commonDistance <= manhattan(point, l.Ports[otherB].Anchor)
	_, short := l.scratch.shortBranchArms[sharedBranchCell{
		commonPort: common,
		point:      point,
	}]
	return armsClear && !short, stepNearCommon
}

func branchArmClear(point Point, port Port) bool {
	// Two columns match one row in the terminal cell aspect ratio.
	minimum := uint64(1)
	dir, ok := directionBetween(port.Anchor, port.Exit)
	if ok && (dir == east || dir == west) {
		minimum = 2
	}
	return manhattan(point, port.Anchor) >= minimum
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
	if route.straightStart {
		nextDir, ok := directionBetween(path[1], path[2])
		if !ok || nextDir != dir {
			return 0, 0, false
		}
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
	if route.straightEnd && dir != endDir {
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

// routeBounds encloses both ports and every obstacle with a two-cell margin.
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
	edge := l.graph.Edges[edgeID]
	straightStart := uint64(edge.PortA) < uint64(len(l.scratch.arrowPort)) &&
		l.scratch.arrowPort[edge.PortA]
	straightEnd := uint64(edge.PortB) < uint64(len(l.scratch.arrowPort)) &&
		l.scratch.arrowPort[edge.PortB]
	return routeEdge{
		id:            edgeID,
		ports:         edge,
		hasPorts:      true,
		straightStart: straightStart,
		straightEnd:   straightEnd,
	}
}

// blockedForRoute checks indexed node obstacles with endpoint, container, and
// host-edge attachment exemptions.
func (l *Layout) blockedForRoute(route routeEdge, p Point) bool {
	if !route.hasPorts {
		return l.blocked(p)
	}
	obstacles := &l.scratch.obstacles
	if obstacles.ready {
		for index := obstacles.buckets[obstacleBucket(p)]; index != 0; {
			use := obstacles.uses[index-1]
			if l.nodeBlocksRoute(route, use.node, p) {
				return true
			}
			index = use.next
		}
		for _, nodeID := range obstacles.large {
			if l.nodeBlocksRoute(route, nodeID, p) {
				return true
			}
		}
		return false
	}
	// Direct searches may run before routing prepares the obstacle index.
	for nodeID, node := range l.Nodes {
		if !node.Empty() && l.nodeBlocksRoute(route, uint32(nodeID), p) {
			return true
		}
	}
	return false
}

func (l *Layout) nodeBlocksRoute(
	route routeEdge,
	nodeID uint32,
	point Point,
) bool {
	if !l.Nodes[nodeID].Rect.Contains(point) {
		return false
	}
	// Endpoint nodes may overlap. Their edge can traverse the overlap and
	// the raster layer later hides the covered part of the route.
	for i := range route.endpointCount {
		endpointNode := route.endpointNodes[i]
		if endpointNode == nodeID || l.nodeContainsNode(nodeID, endpointNode) {
			return false
		}
	}
	if uint64(nodeID) < uint64(len(l.attachments)) {
		attachment := l.attachments[nodeID]
		if attachment != (Attachment{}) && attachment.EdgeID == route.id {
			return false
		}
	}
	return true
}

func (l *Layout) nodeContainsNode(containerID, nodeID uint32) bool {
	if uint64(containerID) >= uint64(len(l.Nodes)) ||
		uint64(nodeID) >= uint64(len(l.Nodes)) {
		return false
	}
	container := l.Nodes[containerID].Rect
	node := l.Nodes[nodeID].Rect
	if container.Empty() || node.Empty() {
		return false
	}
	containerMax := container.Max()
	nodeMax := node.Max()
	return node.Min.X >= container.Min.X && node.Min.Y >= container.Min.Y &&
		nodeMax.X <= containerMax.X && nodeMax.Y <= containerMax.Y
}

func (l *Layout) blocked(p Point) bool {
	obstacles := &l.scratch.obstacles
	if obstacles.ready {
		for index := obstacles.buckets[obstacleBucket(p)]; index != 0; {
			use := obstacles.uses[index-1]
			if l.Nodes[use.node].Rect.Contains(p) {
				return true
			}
			index = use.next
		}
		for _, nodeID := range obstacles.large {
			if l.Nodes[nodeID].Rect.Contains(p) {
				return true
			}
		}
		return false
	}
	// Direct searches may run before routing prepares the obstacle index.
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

// compact removes collinear interior cells and keeps endpoints and bends.
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
