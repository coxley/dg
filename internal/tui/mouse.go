package tui

import (
	"math"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
)

func (m *Model) updateMouseClick(mouse tea.Mouse) {
	if mouse.Button != tea.MouseLeft {
		return
	}
	point, ok := m.documentPoint(mouse.X, mouse.Y)
	if !ok {
		return
	}
	repeated := m.hasLastClick && point == m.lastClick
	m.lastClick, m.hasLastClick = point, true
	m.cursor = point
	m.refreshHits()
	if repeated {
		m.cycleHit(1)
	}

	if m.mode == modeEditLabel {
		m.moveCaretToPoint(point)
		if m.geo.Nodes[m.target.ID].Rect.Contains(point) {
			rect := m.geo.Nodes[m.target.ID].Rect
			m.dragOffset = layout.NewPoint(point.X-rect.Min.X, point.Y-rect.Min.Y)
			m.editMouseDown = true
		}
		return
	}
	if m.mode == modeConnect {
		if hit, ok := m.activeHit(); ok && hit.Kind == layout.HitPort {
			m.completeConnection()
		}
		return
	}
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitNode {
		m.dragging = false
		return
	}
	rect := m.geo.Nodes[hit.ID].Rect
	m.target = hit
	m.dragOffset = layout.NewPoint(point.X-rect.Min.X, point.Y-rect.Min.Y)
	m.beginTransaction()
	m.dragging = true
}

func (m *Model) updateMouseMotion(mouse tea.Mouse) {
	if m.mode == modeEditLabel && m.editMouseDown && mouse.Button == tea.MouseLeft {
		point, ok := m.documentPoint(mouse.X, mouse.Y)
		if !ok || point.X < m.dragOffset.X || point.Y < m.dragOffset.Y {
			return
		}
		nodeID := m.target.ID
		m.commitLabelEdit()
		m.beginTransaction()
		m.editMouseDown = false
		m.dragging = true
		origin := layout.NewPoint(point.X-m.dragOffset.X, point.Y-m.dragOffset.Y)
		m.placeNode(nodeID, origin, point)
		return
	}
	if !m.dragging || mouse.Button != tea.MouseLeft || !m.geo.NodeExists(m.target.ID) {
		return
	}
	point, ok := m.documentPoint(mouse.X, mouse.Y)
	if !ok || point.X < m.dragOffset.X || point.Y < m.dragOffset.Y {
		return
	}
	origin := layout.NewPoint(point.X-m.dragOffset.X, point.Y-m.dragOffset.Y)
	m.placeNode(m.target.ID, origin, point)
}

func (m *Model) updateMouseWheel(mouse tea.Mouse) {
	const step = 3
	switch mouse.Button {
	case tea.MouseWheelUp:
		m.viewport.Y = scrollCoordinate(m.viewport.Y, -step)
	case tea.MouseWheelDown:
		m.viewport.Y = scrollCoordinate(m.viewport.Y, step)
	case tea.MouseWheelLeft:
		m.viewport.X = scrollCoordinate(m.viewport.X, -step)
	case tea.MouseWheelRight:
		m.viewport.X = scrollCoordinate(m.viewport.X, step)
	}
}

func (m *Model) documentPoint(x, y int) (layout.Point, bool) {
	if x < 0 || y < 0 || x >= m.width || y >= m.diagramHeight() {
		return layout.Point{}, false
	}
	documentX := uint64(m.viewport.X) + uint64(x)
	documentY := uint64(m.viewport.Y) + uint64(y)
	if documentX > math.MaxUint32 || documentY > math.MaxUint32 {
		return layout.Point{}, false
	}
	return layout.NewPoint(uint32(documentX), uint32(documentY)), true
}

func scrollCoordinate(value uint32, delta int) uint32 {
	switch {
	case delta < 0:
		return value - min(value, uint32(-delta))
	case delta > 0:
		return value + min(math.MaxUint32-value, uint32(delta))
	default:
		return value
	}
}
