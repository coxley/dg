package tui

import (
	"errors"
	"fmt"
	"slices"

	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/layout"
)

const bendDragRadius = 3

func (m *Model) beginBendDrag(point layout.Point) bool {
	if !m.interaction.idle() {
		return false
	}
	edgeID, routeIndex, bend, ok := m.nearestEdgeBend(point)
	if !ok {
		return false
	}
	primary, err := m.prepareBendTarget(
		edgeID,
		routeIndex,
		[]routeBend{{index: routeIndex, bend: bend}},
	)
	if err != nil {
		m.setError(err.Error())
		return true
	}
	hit := layout.Hit{ID: edgeID, Kind: layout.HitEdge}
	preserveSelection := m.geo.Selection().Contains(hit)
	targets := append([]bendTarget(nil), primary)
	if !preserveSelection {
		m.selectOnly(hit)
	}
	m.target = hit
	m.beginTransaction(transactionBend)
	m.interaction.session = interactionSession{
		kind: sessionBend,
		bend: bendSession{
			targets:           targets,
			valid:             true,
			preserveSelection: preserveSelection,
		},
	}
	m.interaction.gesture = pointerGesture{
		kind:   gestureBend,
		target: hit,
		start:  bend.Point,
		point:  bend.Point,
	}
	if err := m.renderBendBase(); err != nil {
		m.abortBendDrag(err)
		return true
	}
	m.cursor = point
	m.refreshBendPreview()
	m.refreshHits()
	m.status = ""
	return true
}

func (m *Model) updateBendDrag(point layout.Point) {
	if m.interaction.session.kind != sessionBend {
		return
	}
	session := &m.interaction.session.bend
	start := m.interaction.gesture.start
	point = session.constrainPoint(start, point)
	axis := session.axis
	if axis == bendAxisNone {
		if point.X == start.X {
			axis = bendAxisVertical
		} else {
			axis = bendAxisHorizontal
		}
	}
	if session.axis != bendAxisNone && !session.sharedPrepared {
		if err := m.prepareSharedBendTargets(); err != nil {
			m.abortBendDrag(err)
			return
		}
	}
	for i := range session.targets {
		target := &session.targets[i]
		for _, move := range target.moves {
			targetPoint := move.start
			if axis == bendAxisHorizontal {
				targetPoint.X = point.X
			} else {
				targetPoint.Y = point.Y
			}
			target.bends[move.index].Point = targetPoint
		}
	}
	m.interaction.gesture.point = point
	m.cursor = point
	m.refreshBendPreview()
	m.ensureCursorVisible()
}

func (s *bendSession) constrainPoint(
	start, point layout.Point,
) layout.Point {
	dx := max(start.X, point.X) - min(start.X, point.X)
	dy := max(start.Y, point.Y) - min(start.Y, point.Y)
	axis := s.axis
	if axis == bendAxisNone {
		if dy > dx {
			axis = bendAxisVertical
		} else {
			axis = bendAxisHorizontal
		}
		if max(dx, dy) >= 2 {
			s.axis = axis
		}
	}
	if axis == bendAxisVertical {
		point.X = start.X
	} else {
		point.Y = start.Y
	}
	return point
}

func (m *Model) prepareSharedBendTargets() error {
	session := &m.interaction.session.bend
	session.sharedPrepared = true
	if !session.preserveSelection {
		return nil
	}
	primary := session.primary()
	primaryBend := bendAt(m.geo.Edges[primary.edge].Points, primary.routeIndex)
	arm := bendArm(primaryBend, session.axis)
	type targetSpec struct {
		edge       uint32
		routeIndex int
	}
	var specs []targetSpec
	for selectedID := range m.geo.Selection().Edges() {
		if selectedID == primary.edge {
			continue
		}
		index, _, ok := nearestAlignedBend(
			m.geo.Edges[selectedID].Points,
			primaryBend,
			session.axis,
			arm,
		)
		if !ok {
			continue
		}
		specs = append(specs, targetSpec{edge: selectedID, routeIndex: index})
	}
	if len(specs) == 0 {
		return nil
	}
	primary, err := m.prepareBendTarget(
		primary.edge,
		primary.routeIndex,
		bendSegment(m.geo.Edges[primary.edge].Points, primary.routeIndex, session.axis),
	)
	if err != nil {
		return err
	}
	session.targets[0] = primary
	for _, spec := range specs {
		target, err := m.prepareBendTarget(
			spec.edge,
			spec.routeIndex,
			bendSegment(m.geo.Edges[spec.edge].Points, spec.routeIndex, session.axis),
		)
		if err != nil {
			return err
		}
		session.targets = append(session.targets, target)
	}
	if !session.multiple() {
		return nil
	}
	preview, err := m.geo.Clone()
	if err != nil {
		return err
	}
	m.interaction.render.bendLayout = preview
	return nil
}

type routeBend struct {
	index int
	bend  layout.PinnedBend
}

func bendSegment(points []layout.Point, index int, axis bendDragAxis) []routeBend {
	bend := bendAt(points, index)
	neighbor := index + 1
	if !connectionFollowsAxis(bend.Incoming, axis) {
		neighbor = index - 1
	}
	first, last := min(index, neighbor), max(index, neighbor)
	result := make([]routeBend, 0, 2)
	for routeIndex := first; routeIndex <= last; routeIndex++ {
		candidate := bendAt(points, routeIndex)
		if candidate.Valid() && bendAligned(bend, candidate, axis) {
			result = append(result, routeBend{index: routeIndex, bend: candidate})
		}
	}
	return result
}

func nearestAlignedBend(
	points []layout.Point,
	primary layout.PinnedBend,
	axis bendDragAxis,
	arm layout.Connections,
) (int, layout.PinnedBend, bool) {
	bestIndex := 0
	bestDistance := uint64(0)
	var best layout.PinnedBend
	found := false
	for index := 1; index+1 < len(points); index++ {
		candidate := bendAt(points, index)
		if !candidate.Valid() ||
			!bendAligned(primary, candidate, axis) ||
			bendArm(candidate, axis) != arm {
			continue
		}
		distance := pointDistance(primary.Point, candidate.Point)
		if found && distance >= bestDistance {
			continue
		}
		bestIndex = index
		bestDistance = distance
		best = candidate
		found = true
	}
	return bestIndex, best, found
}

func bendAligned(a, b layout.PinnedBend, axis bendDragAxis) bool {
	if axis == bendAxisHorizontal {
		return a.Point.X == b.Point.X
	}
	return a.Point.Y == b.Point.Y
}

func bendArm(bend layout.PinnedBend, axis bendDragAxis) layout.Connections {
	if connectionFollowsAxis(bend.Incoming, axis) {
		return oppositeConnection(bend.Incoming)
	}
	if connectionFollowsAxis(bend.Outgoing, axis) {
		return bend.Outgoing
	}
	return 0
}

func connectionFollowsAxis(connection layout.Connections, axis bendDragAxis) bool {
	if axis == bendAxisHorizontal {
		return connection == layout.East || connection == layout.West
	}
	return connection == layout.North || connection == layout.South
}

func oppositeConnection(connection layout.Connections) layout.Connections {
	switch connection {
	case layout.North:
		return layout.South
	case layout.East:
		return layout.West
	case layout.South:
		return layout.North
	case layout.West:
		return layout.East
	default:
		return 0
	}
}

func (m *Model) finishBendDrag() {
	if m.interaction.session.kind != sessionBend {
		return
	}
	session := m.interaction.session.bend
	if !session.valid {
		m.abortBendDrag(errors.New("bend placement has no valid route"))
		return
	}
	for _, target := range session.targets {
		if err := m.geo.SetPinnedBends(target.edge, target.bends); err != nil {
			m.abortBendDrag(err)
			return
		}
	}
	if err := m.rebuildSelection(); err != nil {
		m.abortBendDrag(err)
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.abortBendDrag(err)
		return
	}
	hit := layout.Hit{ID: session.primary().edge, Kind: layout.HitEdge}
	m.target = hit
	m.clearBendDrag()
	if !session.preserveSelection {
		m.selectOnly(hit)
	}
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) prepareBendTarget(
	edgeID uint32,
	routeIndex int,
	constraints []routeBend,
) (bendTarget, error) {
	bends, err := m.geo.PinnedBends(edgeID)
	if err != nil {
		return bendTarget{}, err
	}
	moves := make([]bendMove, 0, len(constraints))
	for _, constraint := range constraints {
		index := 0
		existingBend := false
		for i, existing := range bends {
			if existing == constraint.bend {
				index = i
				existingBend = true
				break
			}
			if indexOfBend(m.geo.Edges[edgeID].Points, existing) < constraint.index {
				index = i + 1
			}
		}
		if !existingBend {
			bends = slices.Insert(bends, index, constraint.bend)
		}
		moves = append(moves, bendMove{index: index, start: constraint.bend.Point})
	}
	return bendTarget{
		edge:       edgeID,
		routeIndex: routeIndex,
		bends:      bends,
		moves:      moves,
	}, nil
}

func (m *Model) abortBendDrag(cause error) {
	rollbackErr := errors.Join(m.cancelTransaction(), m.render())
	m.clearBendDrag()
	m.refreshHits()
	m.setError(fmt.Errorf(
		"bend placement rejected: %w",
		errors.Join(cause, rollbackErr),
	).Error())
}

func (m *Model) nearestEdgeBend(
	point layout.Point,
) (uint32, int, layout.PinnedBend, bool) {
	selection := m.geo.Selection()
	bestDistance := uint64(bendDragRadius + 1)
	bestSelected := false
	bestEdge := uint32(0)
	bestIndex := 0
	var best layout.PinnedBend
	found := false
	candidates := m.nearbyVisibleEdges(point)
	for _, edgeID := range candidates {
		edge := m.geo.Edges[edgeID]
		if edge.Empty() {
			continue
		}
		hit := layout.Hit{ID: edgeID, Kind: layout.HitEdge}
		selected := selection.Contains(hit)
		for i := 1; i+1 < len(edge.Points); i++ {
			bend := bendAt(edge.Points, i)
			if !bend.Valid() {
				continue
			}
			distance := pointDistance(point, bend.Point)
			if distance > bendDragRadius ||
				found && selected == bestSelected && distance >= bestDistance ||
				found && !selected && bestSelected {
				continue
			}
			bestDistance = distance
			bestSelected = selected
			bestEdge = edgeID
			bestIndex = i
			best = bend
			found = true
		}
	}
	return bestEdge, bestIndex, best, found
}

func (m *Model) nearbyVisibleEdges(point layout.Point) []uint32 {
	var result []uint32
	for dy := -bendDragRadius; dy <= bendDragRadius; dy++ {
		for dx := -bendDragRadius; dx <= bendDragRadius; dx++ {
			if max(dx, -dx)+max(dy, -dy) > bendDragRadius {
				continue
			}
			candidate, ok := movePoint(point, dx, dy)
			if !ok {
				continue
			}
			hit, ok := m.canvas.OwnerAt(canvasview.BaseFrame, candidate)
			if !ok || hit.Kind != layout.HitEdge ||
				slices.Contains(result, hit.ID) {
				continue
			}
			result = append(result, hit.ID)
		}
	}
	return result
}

func indexOfBend(points []layout.Point, want layout.PinnedBend) int {
	for i := 1; i+1 < len(points); i++ {
		if bendAt(points, i) == want {
			return i
		}
	}
	return len(points)
}

func bendAt(points []layout.Point, index int) layout.PinnedBend {
	if index <= 0 || index+1 >= len(points) {
		return layout.PinnedBend{}
	}
	return layout.PinnedBend{
		Point:    points[index],
		Incoming: travelDirection(points[index-1], points[index]),
		Outgoing: travelDirection(points[index], points[index+1]),
	}
}

func travelDirection(from, to layout.Point) layout.Connections {
	switch {
	case from.X == to.X && from.Y > to.Y:
		return layout.North
	case from.X < to.X && from.Y == to.Y:
		return layout.East
	case from.X == to.X && from.Y < to.Y:
		return layout.South
	case from.X > to.X && from.Y == to.Y:
		return layout.West
	default:
		return 0
	}
}

func (m *Model) resetDoubleClickedEdge() bool {
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitEdge {
		return false
	}
	bends, err := m.geo.PinnedBends(hit.ID)
	if err != nil || len(bends) == 0 {
		return false
	}
	m.beginTransaction(transactionImmediate)
	if err := m.geo.SetPinnedBends(hit.ID, nil); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return true
	}
	m.selectOnly(hit)
	if err := m.rebuildSelection(); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction(), m.render()).Error())
		return true
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return true
	}
	m.target = hit
	m.refreshHits()
	m.selectTarget()
	m.status = ""
	return true
}
