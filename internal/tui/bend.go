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
	primary, err := m.prepareBendTarget(edgeID, routeIndex, bend)
	if err != nil {
		m.setError(err.Error())
		return true
	}
	hit := layout.Hit{ID: edgeID, Kind: layout.HitEdge}
	preserveSelection := m.geo.Selection().Contains(hit)
	targets := append([]bendTarget(nil), primary)
	if preserveSelection {
		for selectedID := range m.geo.Selection().Edges() {
			if selectedID == edgeID {
				continue
			}
			for index := 1; index+1 < len(m.geo.Edges[selectedID].Points); index++ {
				shared := bendAt(m.geo.Edges[selectedID].Points, index)
				if !shared.Valid() || shared.Point != bend.Point {
					continue
				}
				target, targetErr := m.prepareBendTarget(selectedID, index, shared)
				if targetErr != nil {
					m.setError(targetErr.Error())
					return true
				}
				targets = append(targets, target)
				break
			}
		}
	} else {
		m.selectOnly(hit)
	}
	var preview *layout.Layout
	if len(targets) > 1 {
		preview, err = m.geo.Clone()
		if err != nil {
			m.setError(err.Error())
			return true
		}
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
	m.interaction.render.bendLayout = preview
	if len(targets) == 1 {
		if err := m.renderBendBase(); err != nil {
			m.abortBendDrag(err)
			return true
		}
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
	point = session.constrainPoint(m.interaction.gesture.start, point)
	for i := range session.targets {
		target := &session.targets[i]
		target.bends[target.index].Point = point
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
	bend layout.PinnedBend,
) (bendTarget, error) {
	bends, err := m.geo.PinnedBends(edgeID)
	if err != nil {
		return bendTarget{}, err
	}
	index := 0
	existingBend := false
	for i, existing := range bends {
		if existing == bend {
			index = i
			existingBend = true
			break
		}
		if indexOfBend(m.geo.Edges[edgeID].Points, existing) < routeIndex {
			index = i + 1
		}
	}
	if !existingBend {
		bends = slices.Insert(bends, index, bend)
	}
	return bendTarget{edge: edgeID, index: index, bends: bends}, nil
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
