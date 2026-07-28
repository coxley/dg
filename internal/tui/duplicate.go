package tui

import (
	"errors"

	"github.com/coxley/dg/layout"
)

func (m *Model) beginDuplicateDrag(point layout.Point, hit layout.Hit) {
	if hit.Kind != layout.HitNode {
		m.status = "Alt-drag duplication requires a node"
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

func (m *Model) updateDuplicateDrag(point layout.Point) {
	if point == m.duplicatePoint {
		return
	}
	if !m.duplicateDragging {
		cloned, err := m.geo.Clone()
		if err != nil {
			m.cancelDuplicateDrag()
			m.status = err.Error()
			return
		}
		dx, dy := pointDelta(m.duplicateStart, point)
		if err := cloned.DuplicateSelection(dx, dy); err != nil {
			m.cancelDuplicateDrag()
			m.status = err.Error()
			return
		}
		m.duplicateGeo = cloned
		m.duplicateDragging = true
	} else {
		dx, dy := pointDelta(m.duplicatePoint, point)
		for nodeID := range m.duplicateGeo.Selection().Nodes() {
			origin := m.duplicateGeo.Nodes[nodeID].Rect.Min
			next, ok := movePoint64(origin, dx, dy)
			if !ok {
				return
			}
			if err := m.duplicateGeo.PlaceNode(nodeID, next); err != nil {
				m.status = err.Error()
				return
			}
		}
	}
	m.duplicatePoint = point
	buildErr := m.duplicateGeo.Build()
	frame, err := m.duplicateEncoder.EncodeFrame(
		m.duplicateFrame.Text[:0],
		m.duplicateGeo,
	)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.duplicateFrame = frame
	m.duplicateRows = indexFrameRows(m.duplicateRows, frame.Text)
	m.cursor = point
	m.ensureCursorVisible()
	if buildErr != nil {
		m.status = buildErr.Error()
	} else {
		m.status = ""
	}
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
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.rebuild(); err != nil {
		m.status = errors.Join(err, m.cancelTransaction(), m.render()).Error()
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.status = err.Error()
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
