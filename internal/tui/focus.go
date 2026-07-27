package tui

import (
	"errors"

	"github.com/coxley/dg/layout"
)

func (m *Model) focusNode(delta int) {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	current, hasCurrent := m.focusedNode()
	var first, previous, last, chosen layout.Hit
	seenCurrent := false
	for hit := range m.geo.DrawOrder() {
		if hit.Kind != layout.HitNode {
			continue
		}
		if first == (layout.Hit{}) {
			first = hit
		}
		if delta > 0 && seenCurrent && chosen == (layout.Hit{}) {
			chosen = hit
		}
		if hasCurrent && hit == current {
			seenCurrent = true
			if delta < 0 {
				chosen = previous
			}
		}
		previous = hit
		last = hit
	}
	if first == (layout.Hit{}) {
		m.status = "no nodes"
		return
	}
	if !seenCurrent || chosen == (layout.Hit{}) {
		if delta < 0 {
			chosen = last
		} else {
			chosen = first
		}
	}
	m.target = chosen
	m.selectOnly(chosen)
	m.cursor = m.geo.Nodes[chosen.ID].LabelPoint
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) focusedNode() (layout.Hit, bool) {
	if m.target.Kind != layout.HitNode ||
		!m.geo.NodeExists(m.target.ID) ||
		!m.geo.Selection().Contains(m.target) {
		return layout.Hit{}, false
	}
	return m.target, true
}

func (m *Model) shiftFocusedNode(hit layout.Hit, dx, dy int) {
	origin, ok := movePoint(m.geo.Nodes[hit.ID].Rect.Min, dx, dy)
	if !ok {
		return
	}
	m.beginTransaction()
	if err := m.geo.PlaceNode(hit.ID, origin); err != nil {
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.rebuild(); err != nil {
		m.status = errors.Join(
			err,
			m.cancelTransaction(),
			m.render(),
		).Error()
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.status = err.Error()
		return
	}
	m.cursor = m.geo.Nodes[hit.ID].LabelPoint
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) shiftSelection(dx, dy int) {
	cursor := m.cursor
	if moved, ok := movePoint(cursor, dx, dy); ok {
		cursor = moved
	}
	m.beginTransaction()
	moved, err := m.moveSelectedNodes(int64(dx), int64(dy), cursor)
	if err != nil {
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if !moved {
		_ = m.cancelTransaction()
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.status = err.Error()
	}
}
