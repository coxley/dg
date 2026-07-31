package layout

import (
	"errors"
	"fmt"
	"slices"

	"github.com/coxley/dg/ir"
)

// PinnedBend fixes one edge turn at Point. Incoming and Outgoing contain one
// cardinal connection each and must be perpendicular.
type PinnedBend struct {
	Point    Point
	Incoming Connections
	Outgoing Connections
}

// Valid reports whether the bend contains two perpendicular directions.
func (b PinnedBend) Valid() bool {
	incoming, incomingOK := connectionDirection(b.Incoming)
	outgoing, outgoingOK := connectionDirection(b.Outgoing)
	if !incomingOK || !outgoingOK {
		return false
	}
	return (incoming == north || incoming == south) !=
		(outgoing == north || outgoing == south)
}

// PinnedBends returns an independent copy of edgeID's ordered bend constraints.
func (l *Layout) PinnedBends(edgeID uint32) ([]PinnedBend, error) {
	if !l.graph.EdgeExists(edgeID) {
		return nil, fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	return slices.Clone(l.edgeBends[edgeID]), nil
}

// SetPinnedBends replaces edgeID's ordered bend constraints.
func (l *Layout) SetPinnedBends(edgeID uint32, bends []PinnedBend) error {
	if !l.graph.EdgeExists(edgeID) {
		return fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	if err := validatePinnedBends(bends); err != nil {
		return err
	}
	previous := l.edgeBends[edgeID]
	if slices.Equal(previous, bends) {
		return nil
	}
	var before []PinnedBend
	if l.recordingChanges() {
		before = slices.Clone(previous)
	}
	l.edgeBends[edgeID] = append(l.edgeBends[edgeID][:0], bends...)
	if l.recordingChanges() {
		l.recordChange(historyChange{
			Kind:   historySetPinnedBends,
			ID:     edgeID,
			Before: historyChangeState{Bends: before},
			After:  historyChangeState{Bends: slices.Clone(bends)},
		})
	}
	return nil
}

func validatePinnedBends(bends []PinnedBend) error {
	for i, bend := range bends {
		if !bend.Valid() {
			return fmt.Errorf("invalid pinned bend %d", i)
		}
		if i != 0 && bend.Point == bends[i-1].Point {
			return fmt.Errorf("pinned bends %d and %d overlap", i-1, i)
		}
	}
	return nil
}

func validPinnedBends(bends []PinnedBend) bool {
	return validatePinnedBends(bends) == nil
}

// PreviewPinnedBends returns edgeID's route with bends applied without
// changing the layout. The preview omits edgeID from route occupancy.
func (l *Layout) PreviewPinnedBends(
	dst []Point,
	edgeID uint32,
	bends []PinnedBend,
) ([]Point, error) {
	if !l.graph.EdgeExists(edgeID) {
		return nil, fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	if err := validatePinnedBends(bends); err != nil {
		return nil, err
	}

	scratch := &l.scratch
	occupancy := &scratch.occupancy
	occupancy.reset()
	scratch.obstacles.reset(l.Nodes)
	for candidateID, edge := range l.Edges {
		if uint32(candidateID) == edgeID || edge.Empty() {
			continue
		}
		var err error
		scratch.expanded, err = occupancy.addCompact(
			uint32(candidateID),
			scratch.expanded,
			edge.Points,
		)
		if err != nil {
			return nil, fmt.Errorf("expand edge %d: %w", candidateID, err)
		}
	}

	edge := l.graph.Edges[edgeID]
	style := l.edgeStyles[edgeID]
	route := routeEdge{
		id:            edgeID,
		ports:         edge,
		hasPorts:      true,
		straightStart: style.PortAArrow != ArrowNone,
		straightEnd:   style.PortBArrow != ArrowNone,
	}
	path, err := l.router.findRouteThroughBends(
		l,
		route,
		l.Ports[edge.PortA],
		l.Ports[edge.PortB],
		bends,
		occupancy,
		&scratch.search,
		scratch.candidate[:0],
	)
	if err != nil {
		return nil, fmt.Errorf("preview pinned Bends: %w", err)
	}
	scratch.candidate = path
	return compact(dst[:0], path), nil
}

func (r Router) findRouteThroughBends(
	l *Layout,
	route routeEdge,
	a, b Port,
	bends []PinnedBend,
	occupancy *routeOccupancy,
	search *routeSearch,
	path []Point,
) ([]Point, error) {
	current := a
	path = path[:0]
	for i, bend := range bends {
		incoming, _ := connectionDirection(bend.Incoming)
		previous, ok := move(bend.Point, oppositeDirection(incoming))
		if !ok {
			return nil, fmt.Errorf("pinned bend %d incoming direction: %w", i, ErrNoRoute)
		}
		part := route
		part.straightStart = i == 0 && route.straightStart
		part.straightEnd = false
		segmentBuffer := l.scratch.segment[:0]
		segment, err := r.findRouteFor(
			l,
			part,
			current,
			Port{Anchor: bend.Point, Exit: previous},
			occupancy,
			search,
			segmentBuffer,
		)
		if err != nil {
			return nil, fmt.Errorf("pinned bend %d: %w", i, err)
		}
		l.scratch.segment = append(segmentBuffer, segment...)
		path = appendRoutePart(path, l.scratch.segment)

		outgoing, _ := connectionDirection(bend.Outgoing)
		next, ok := move(bend.Point, outgoing)
		if !ok {
			return nil, fmt.Errorf("pinned bend %d outgoing direction: %w", i, ErrNoRoute)
		}
		current = Port{Anchor: bend.Point, Exit: next}
	}

	part := route
	part.straightStart = len(bends) == 0 && route.straightStart
	segmentBuffer := l.scratch.segment[:0]
	segment, err := r.findRouteFor(
		l,
		part,
		current,
		b,
		occupancy,
		search,
		segmentBuffer,
	)
	if err != nil {
		return nil, fmt.Errorf("after pinned Bends: %w", err)
	}
	l.scratch.segment = append(segmentBuffer, segment...)
	path = appendRoutePart(path, l.scratch.segment)
	if len(path) < 2 {
		return nil, errors.New("pinned route has no segments")
	}
	return path, nil
}

func appendRoutePart(path, part []Point) []Point {
	if len(path) != 0 && len(part) != 0 && path[len(path)-1] == part[0] {
		part = part[1:]
	}
	return append(path, part...)
}

func connectionDirection(connection Connections) (direction, bool) {
	switch connection {
	case North:
		return north, true
	case East:
		return east, true
	case South:
		return south, true
	case West:
		return west, true
	default:
		return 0, false
	}
}

func oppositeDirection(dir direction) direction {
	switch dir {
	case north:
		return south
	case east:
		return west
	case south:
		return north
	case west:
		return east
	default:
		return 0
	}
}

func clonePinnedBends(source [][]PinnedBend) [][]PinnedBend {
	cloned := make([][]PinnedBend, len(source))
	for i := range source {
		cloned[i] = slices.Clone(source[i])
	}
	return cloned
}
