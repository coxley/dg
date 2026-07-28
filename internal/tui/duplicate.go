package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
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
	m.target = hit
	m.duplicatePending = true
	m.duplicateDragging = false
	m.duplicateStart = point
	m.duplicatePoint = point
	m.status = ""
}

func (m *Model) updateDuplicateMotion(mouse tea.Mouse) bool {
	if !m.duplicatePending && !m.duplicateDragging ||
		mouse.Button != tea.MouseLeft {
		return false
	}
	if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
		m.updateDuplicateDrag(point)
	}
	return true
}

func (m *Model) updateDuplicateDrag(point layout.Point) {
	if point == m.duplicatePoint {
		return
	}
	if !m.duplicateDragging {
		if !m.startDuplicatePreview(point) {
			return
		}
	} else {
		if !m.moveDuplicatePreview(point) {
			return
		}
	}
	m.duplicatePoint = point
	frame, err := m.duplicateEncoder.EncodeFrame(
		m.duplicateFrame.Text[:0],
		m.duplicateGeo,
	)
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.duplicateFrame = frame
	m.duplicateRows = indexFrameRows(m.duplicateRows, frame.Text)
	m.refreshDuplicateHighlight()
	m.cursor = point
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) startDuplicatePreview(point layout.Point) bool {
	cloned, err := m.geo.Clone()
	if err == nil {
		dx, dy := pointDelta(m.duplicateStart, point)
		err = cloned.DuplicateSelection(dx, dy)
	}
	if err != nil {
		m.cancelDuplicateDrag()
		m.setError(err.Error())
		return false
	}
	m.duplicateGeo = cloned
	m.duplicateDragging = true
	return true
}

func (m *Model) moveDuplicatePreview(point layout.Point) bool {
	dx, dy := pointDelta(m.duplicatePoint, point)
	for nodeID := range m.duplicateGeo.Selection().Nodes() {
		origin := m.duplicateGeo.Nodes[nodeID].Rect.Min
		if _, ok := movePoint64(origin, dx, dy); !ok {
			return false
		}
	}
	for edgeID := range m.duplicateGeo.Selection().Edges() {
		for _, point := range m.duplicateGeo.Edges[edgeID].Points {
			if _, ok := movePoint64(point, dx, dy); !ok {
				return false
			}
		}
	}
	for nodeID := range m.duplicateGeo.Selection().Nodes() {
		origin := m.duplicateGeo.Nodes[nodeID].Rect.Min
		next, _ := movePoint64(origin, dx, dy)
		if err := m.duplicateGeo.PlaceNode(nodeID, next); err != nil {
			m.setError(err.Error())
			return false
		}
	}
	for edgeID := range m.duplicateGeo.Selection().Edges() {
		points := m.duplicateGeo.Edges[edgeID].Points
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
	if !m.duplicateDragging {
		m.cancelDuplicateDrag()
		return
	}
	dx, dy := pointDelta(m.duplicateStart, point)
	m.cancelDuplicateDrag()
	m.beginTransaction()
	if err := m.geo.DuplicateSelection(dx, dy); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.rebuild(); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction(), m.render()).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	if hit, ok := m.firstSelectedNode(); ok {
		m.target = hit
		m.cursor = m.geo.Nodes[hit.ID].LabelPoint
	}
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) cancelDuplicateDrag() {
	m.duplicatePending = false
	m.duplicateDragging = false
	m.duplicateGeo = nil
	m.duplicateFrame.Bounds = layout.Rect{}
	m.duplicateFrame.Text = m.duplicateFrame.Text[:0]
	m.duplicateRows = m.duplicateRows[:0]
	m.duplicateHighlight = m.duplicateHighlight[:0]
}

func (m *Model) refreshDuplicateHighlight() {
	m.duplicateHighlight = appendSelectionHighlight(
		m.duplicateHighlight,
		m.duplicateGeo,
		m.duplicateFrame,
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
