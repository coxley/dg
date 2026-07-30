package tui

import (
	"math"

	"github.com/coxley/dg/layout"
)

func (m *Model) refreshAttachmentCandidate(nodeID uint32) {
	m.refreshAttachmentCandidateFor(m.geo, nodeID)
}

func (m *Model) refreshAttachmentCandidateFor(geo *layout.Layout, nodeID uint32) {
	gesture := &m.interaction.gesture
	gesture.hasAttachment = false
	if !geo.NodeExists(nodeID) {
		return
	}
	rect := geo.Nodes[nodeID].Rect
	bestDistance := uint64(math.MaxUint64)
	order := geo.DrawOrder()
	m.candidateHits = m.candidateHits[:0]
	for hit := range order {
		m.candidateHits = append(m.candidateHits, hit)
	}
	for i := len(m.candidateHits) - 1; i >= 0; i-- {
		hit := m.candidateHits[i]
		if hit.Kind != layout.HitEdge ||
			!geo.CanAttachNode(nodeID, hit.ID) {
			continue
		}
		point, ok := closestEdgePoint(
			geo.Edges[hit.ID].Points,
			rect,
			m.cursor,
		)
		if !ok {
			continue
		}
		if !geo.CanAttachNodeAt(nodeID, hit.ID, point) {
			continue
		}
		distance := pointDistance(point, m.cursor)
		if distance >= bestDistance {
			continue
		}
		bestDistance = distance
		gesture.attachmentEdge = hit.ID
		gesture.attachmentPoint = point
		gesture.hasAttachment = true
	}
}

func closestEdgePoint(
	points []layout.Point,
	rect layout.Rect,
	target layout.Point,
) (layout.Point, bool) {
	if rect.Empty() {
		return layout.Point{}, false
	}
	limit := rect.Max()
	maxX, maxY := limit.X-1, limit.Y-1
	bestDistance := uint64(math.MaxUint64)
	var best layout.Point
	found := false
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		var candidate layout.Point
		switch {
		case a.X == b.X:
			if a.X < rect.Min.X || a.X > maxX {
				continue
			}
			low := max(min(a.Y, b.Y), rect.Min.Y)
			high := min(max(a.Y, b.Y), maxY)
			if low > high {
				continue
			}
			candidate = layout.NewPoint(a.X, min(max(target.Y, low), high))
		case a.Y == b.Y:
			if a.Y < rect.Min.Y || a.Y > maxY {
				continue
			}
			low := max(min(a.X, b.X), rect.Min.X)
			high := min(max(a.X, b.X), maxX)
			if low > high {
				continue
			}
			candidate = layout.NewPoint(min(max(target.X, low), high), a.Y)
		default:
			continue
		}
		distance := pointDistance(candidate, target)
		if distance < bestDistance {
			best, bestDistance, found = candidate, distance, true
		}
	}
	return best, found
}

func (m *Model) applyNodeAttachment() error {
	gesture := m.interaction.gesture
	if gesture.kind != gestureMove ||
		!gesture.moved ||
		!m.geo.NodeExists(gesture.target.ID) {
		return nil
	}
	if gesture.hasAttachment {
		return m.geo.AttachNode(
			gesture.target.ID,
			gesture.attachmentEdge,
			gesture.attachmentPoint,
		)
	}
	_, attached := m.geo.NodeAttachment(gesture.target.ID)
	if attached {
		return m.geo.DetachNode(gesture.target.ID)
	}
	return nil
}
