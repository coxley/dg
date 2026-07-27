package tui

import "github.com/coxley/dg/layout"

type selectionArea struct {
	min layout.Point
	max layout.Point
}

func newSelectionArea(a, b layout.Point) selectionArea {
	return selectionArea{
		min: layout.NewPoint(min(a.X, b.X), min(a.Y, b.Y)),
		max: layout.NewPoint(max(a.X, b.X), max(a.Y, b.Y)),
	}
}

func (a selectionArea) contains(point layout.Point) bool {
	return point.X >= a.min.X && point.X <= a.max.X &&
		point.Y >= a.min.Y && point.Y <= a.max.Y
}

func (m *Model) clearSelection() {
	m.geo.Selection().Clear()
}

func (m *Model) hasSelection() bool {
	return !m.geo.Selection().Empty()
}

func (m *Model) hasSelectedNodes() bool {
	return m.geo.Selection().HasNodes()
}

func (m *Model) selectedCounts() (nodes, edges int) {
	return m.geo.Selection().Counts()
}

func (m *Model) selectOnly(hit layout.Hit) {
	m.geo.Selection().SelectOnly(hit)
}

func (m *Model) hitSelected(hit layout.Hit) bool {
	return m.geo.Selection().Contains(hit)
}

func (m *Model) firstSelectedNode() (layout.Hit, bool) {
	nodeID, ok := m.geo.Selection().FirstNode()
	return layout.Hit{ID: nodeID, Kind: layout.HitNode}, ok
}

func (m *Model) expandSelection() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	if !m.hasSelection() {
		if hit, ok := m.activeHit(); ok {
			m.selectOnly(hit)
		}
	}
	m.geo.Selection().Expand()
	m.status = ""
}

func (m *Model) beginAreaSelection(point layout.Point) {
	m.clearSelection()
	m.selecting = true
	m.selectionStartPoint = point
	m.selectionEndPoint = point
	m.status = ""
}

func (m *Model) updateAreaSelection(point layout.Point) {
	m.selectionEndPoint = point
	m.cursor = point
}

func (m *Model) finishAreaSelection() {
	m.geo.Selection().SelectArea(
		m.selectionStartPoint,
		m.selectionEndPoint,
	)
	m.selecting = false
	m.refreshHits()
	m.status = ""
}

func (m *Model) marqueeArea() selectionArea {
	return newSelectionArea(m.selectionStartPoint, m.selectionEndPoint)
}
