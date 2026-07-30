package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/layout"
)

func (m *Model) beginDuplicateDrag(point layout.Point, hit layout.Hit) {
	if hit.Kind != layout.HitNode {
		m.setError("Alt-drag duplication requires a node")
		return
	}
	if !m.geo.Selection().Contains(hit) {
		m.selectOnly(hit)
	}
	rank, ok := selectedNodeRank(m.geo.Selection(), hit.ID)
	if !ok {
		m.setError("duplicate target is not selected")
		return
	}
	m.target = hit
	m.interaction.gesture = pointerGesture{
		kind:          gestureDuplicatePending,
		target:        hit,
		start:         point,
		point:         point,
		duplicateRank: rank,
	}
	m.status = ""
}

func (m *Model) updateDuplicateMotion(mouse tea.Mouse) bool {
	kind := m.interaction.gesture.kind
	if kind != gestureDuplicatePending && kind != gestureDuplicate ||
		mouse.Button != tea.MouseLeft {
		return false
	}
	if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
		if mouse.Mod.Contains(tea.ModCtrl) {
			point = axisLockedPoint(m.interaction.gesture.start, point)
		}
		m.updateDuplicateDrag(point)
	}
	return true
}

func (m *Model) updateDuplicateDrag(point layout.Point) {
	if point == m.interaction.gesture.point {
		return
	}
	if m.interaction.gesture.kind == gestureDuplicatePending {
		if !m.startDuplicatePreview(point) {
			return
		}
	} else {
		if !m.moveDuplicatePreview(point) {
			return
		}
	}
	m.interaction.gesture.point = point
	if err := m.canvas.Render(
		canvasview.DuplicateFrame,
		m.interaction.render.duplicateLayout,
	); err != nil {
		m.setError(err.Error())
		return
	}
	m.refreshDuplicateHighlight()
	m.cursor = point
	if nodeID, ok := selectedNodeAt(
		m.interaction.render.duplicateLayout.Selection(),
		m.interaction.gesture.duplicateRank,
	); ok {
		m.refreshAttachmentCandidateFor(m.interaction.render.duplicateLayout, nodeID)
	}
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) startDuplicatePreview(point layout.Point) bool {
	cloned, err := m.geo.Clone()
	if err == nil {
		dx, dy := pointDelta(m.interaction.gesture.start, point)
		err = cloned.DuplicateSelection(dx, dy)
	}
	if err != nil {
		m.cancelDuplicateDrag()
		m.setError(err.Error())
		return false
	}
	m.interaction.render.duplicateLayout = cloned
	m.interaction.gesture.kind = gestureDuplicate
	return true
}

func (m *Model) moveDuplicatePreview(point layout.Point) bool {
	duplicate := m.interaction.render.duplicateLayout
	dx, dy := pointDelta(m.interaction.gesture.point, point)
	for nodeID := range duplicate.Selection().Nodes() {
		origin := duplicate.Nodes[nodeID].Rect.Min
		if _, ok := movePoint64(origin, dx, dy); !ok {
			return false
		}
	}
	for edgeID := range duplicate.Selection().Edges() {
		for _, point := range duplicate.Edges[edgeID].Points {
			if _, ok := movePoint64(point, dx, dy); !ok {
				return false
			}
		}
	}
	for nodeID := range duplicate.Selection().Nodes() {
		origin := duplicate.Nodes[nodeID].Rect.Min
		next, _ := movePoint64(origin, dx, dy)
		if err := duplicate.PlaceNode(nodeID, next); err != nil {
			m.setError(err.Error())
			return false
		}
	}
	for edgeID := range duplicate.Selection().Edges() {
		points := duplicate.Edges[edgeID].Points
		for i, point := range points {
			next, ok := movePoint64(point, dx, dy)
			if !ok {
				return false
			}
			points[i] = next
		}
	}
	return true
}

func (m *Model) finishDuplicateDrag(point layout.Point) {
	if m.interaction.gesture.kind != gestureDuplicate {
		m.cancelDuplicateDrag()
		return
	}
	gesture := m.interaction.gesture
	dx, dy := pointDelta(m.interaction.gesture.start, point)
	m.cancelDuplicateDrag()
	m.beginTransaction(transactionDuplicate)
	if err := m.geo.DuplicateSelection(dx, dy); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	duplicate, ok := selectedNodeAt(
		m.geo.Selection(),
		gesture.duplicateRank,
	)
	if !ok {
		m.setError(errors.Join(
			errors.New("duplicated target is unavailable"),
			m.cancelTransaction(),
		).Error())
		return
	}
	if gesture.hasAttachment {
		if err := m.geo.AttachNode(
			duplicate,
			gesture.attachmentEdge,
			gesture.attachmentPoint,
		); err != nil {
			m.setError(errors.Join(err, m.cancelTransaction()).Error())
			return
		}
	}
	if err := m.rebuild(); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction(), m.render()).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.target = layout.Hit{ID: duplicate, Kind: layout.HitNode}
	m.cursor = m.geo.Nodes[duplicate].LabelPoint
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
}

func selectedNodeRank(selection *layout.Selection, target uint32) (int, bool) {
	rank := 0
	for nodeID := range selection.Nodes() {
		if nodeID == target {
			return rank, true
		}
		rank++
	}
	return 0, false
}

func selectedNodeAt(selection *layout.Selection, rank int) (uint32, bool) {
	index := 0
	for nodeID := range selection.Nodes() {
		if index == rank {
			return nodeID, true
		}
		index++
	}
	return 0, false
}

func (m *Model) cancelDuplicateDrag() {
	if m.interaction.gesture.kind == gestureDuplicatePending ||
		m.interaction.gesture.kind == gestureDuplicate {
		m.interaction.resetGesture()
	}
	m.interaction.render.clearDuplicate(&m.canvas)
}

func (m *Model) refreshDuplicateHighlight() {
	m.interaction.render.duplicateHighlight = appendSelectionHighlight(
		m.interaction.render.duplicateHighlight,
		m.interaction.render.duplicateLayout,
		&m.canvas,
		canvasview.DuplicateFrame,
	)
}

func pointDelta(from, to layout.Point) (int64, int64) {
	return int64(to.X) - int64(from.X), int64(to.Y) - int64(from.Y)
}

func movePoint64(point layout.Point, dx, dy int64) (layout.Point, bool) {
	x, ok := moveCoordinate64(point.X, dx)
	if !ok {
		return layout.Point{}, false
	}
	y, ok := moveCoordinate64(point.Y, dy)
	if !ok {
		return layout.Point{}, false
	}
	return layout.NewPoint(x, y), true
}
