package tui

import (
	"cmp"
	"errors"
	"slices"

	"github.com/coxley/dg/layout"
)

func (m *Model) focusNode(delta int) {
	if m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	current, hasCurrent := m.focusedNode()
	m.focusNodes = m.focusNodes[:0]
	for hit := range m.geo.DrawOrder() {
		if hit.Kind == layout.HitNode {
			m.focusNodes = append(m.focusNodes, hit)
		}
	}
	if len(m.focusNodes) == 0 {
		m.setError("no nodes")
		return
	}
	slices.SortFunc(m.focusNodes, func(a, b layout.Hit) int {
		pa := m.geo.Nodes[a.ID].Rect.Min
		pb := m.geo.Nodes[b.ID].Rect.Min
		if order := cmp.Compare(pa.Y, pb.Y); order != 0 {
			return order
		}
		if order := cmp.Compare(pa.X, pb.X); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
	index := -1
	if hasCurrent {
		index, _ = slices.BinarySearchFunc(
			m.focusNodes,
			current,
			func(hit, current layout.Hit) int {
				if hit == current {
					return 0
				}
				pa := m.geo.Nodes[hit.ID].Rect.Min
				pb := m.geo.Nodes[current.ID].Rect.Min
				if order := cmp.Compare(pa.Y, pb.Y); order != 0 {
					return order
				}
				if order := cmp.Compare(pa.X, pb.X); order != 0 {
					return order
				}
				return cmp.Compare(hit.ID, current.ID)
			},
		)
	} else if delta < 0 {
		index = 0
	}
	index = (index + delta + len(m.focusNodes)) % len(m.focusNodes)
	chosen := m.focusNodes[index]
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
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.rebuild(); err != nil {
		m.setError(errors.Join(
			err,
			m.cancelTransaction(),
			m.render(),
		).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
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
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if !moved {
		_ = m.cancelTransaction()
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
	}
}
