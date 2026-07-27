package tui

import (
	"math"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
)

const reconnectDragRadius = 3

func (m *Model) updateMouseClick(mouse tea.Mouse) {
	if mouse.Button != tea.MouseLeft {
		return
	}
	point, ok := m.documentPoint(mouse.X, mouse.Y)
	if !ok {
		return
	}
	m.edgeDragPending = false
	repeated := m.hasLastClick && point == m.lastClick
	m.lastClick, m.hasLastClick = point, true
	m.cursor = point
	m.refreshHits()
	m.prioritizeSelectedEdge()
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
		m.updateConnectionClick(point)
		return
	}
	hit, ok := m.activeHit()
	if !ok {
		m.dragging = false
		if m.mode == modeNavigate && !mouse.Mod.Contains(tea.ModCtrl) {
			m.beginAreaSelection(point)
		}
		return
	}
	if m.mode == modeNavigate && mouse.Mod.Contains(tea.ModCtrl) {
		m.geo.Selection().Toggle(hit)
		m.dragging = false
		m.status = ""
		return
	}
	if m.mode == modeNavigate &&
		hit.Kind == layout.HitEdge &&
		m.nearEdgeEndpoint(hit.ID, point) {
		m.selectOnly(hit)
		m.edgeDragPending = true
		m.edgeDragHit = hit
		m.edgeDragStart = point
		m.dragging = false
		m.status = ""
		return
	}
	if hit.Kind != layout.HitNode {
		m.selectOnly(hit)
		m.dragging = false
		return
	}
	if !m.hitSelected(hit) {
		m.selectOnly(hit)
	}
	rect := m.geo.Nodes[hit.ID].Rect
	m.target = hit
	m.dragOffset = layout.NewPoint(point.X-rect.Min.X, point.Y-rect.Min.Y)
	m.beginTransaction()
	m.dragging = true
}

func (m *Model) prioritizeSelectedEdge() {
	selection := m.geo.Selection()
	for i, hit := range m.hits {
		if hit.Kind == layout.HitEdge && selection.Contains(hit) {
			m.active = i
			return
		}
	}
}

func (m *Model) updateConnectionClick(point layout.Point) {
	if !m.connectStarted {
		portID, ok := m.usablePortAt(point)
		if !ok {
			m.status = dragFromSource
			return
		}
		m.connectSource = portID
		m.connectStarted = true
		m.reconnecting = false
	}
	if m.reconnecting {
		if err := m.renderConnectionBase(); err != nil {
			m.status = err.Error()
			return
		}
	}
	m.refreshConnectionPreview()
	m.connectDragging = true
	m.status = ""
}

func (m *Model) nearEdgeEndpoint(edgeID uint32, point layout.Point) bool {
	portA, portB, err := m.geo.EdgePorts(edgeID)
	if err != nil {
		return false
	}
	return min(
		pointDistance(point, m.geo.Ports[portA].Anchor),
		pointDistance(point, m.geo.Ports[portB].Anchor),
	) <= reconnectDragRadius
}

func (m *Model) updateMouseMotion(mouse tea.Mouse) {
	if m.edgeDragPending && mouse.Button == tea.MouseLeft {
		point, ok := m.documentPoint(mouse.X, mouse.Y)
		if !ok || point == m.edgeDragStart {
			return
		}
		hit := m.edgeDragHit
		start := m.edgeDragStart
		m.clearConnection()
		m.cursor = start
		if err := m.startConnection(hit); err != nil {
			m.status = err.Error()
			return
		}
		if err := m.renderConnectionBase(); err != nil {
			m.clearConnection()
			m.status = err.Error()
			return
		}
		m.mode = modeConnect
		m.connectDragging = true
		m.cursor = point
		m.refreshHits()
		m.refreshConnectionPreview()
		m.ensureCursorVisible()
		m.status = ""
		return
	}
	if m.mode == modeConnect &&
		m.connectDragging &&
		mouse.Button == tea.MouseLeft {
		if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
			m.cursor = point
			m.refreshHits()
			m.refreshConnectionPreview()
			m.ensureCursorVisible()
			m.status = ""
		}
		return
	}
	if m.selecting && mouse.Button == tea.MouseLeft {
		if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
			m.updateAreaSelection(point)
		}
		return
	}
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

func (m *Model) updateMouseRelease(mouse tea.Mouse) {
	switch {
	case m.mode == modeConnect && m.connectDragging:
		m.updateConnectionRelease(mouse)
	case m.edgeDragPending:
		m.edgeDragPending = false
		m.status = ""
	case m.selecting:
		if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
			m.updateAreaSelection(point)
		}
		m.finishAreaSelection()
	case m.dragging:
		m.finishMove()
	}
	m.editMouseDown = false
}

func (m *Model) updateConnectionRelease(mouse tea.Mouse) {
	m.connectDragging = false
	point, ok := m.documentPoint(mouse.X, mouse.Y)
	if !ok {
		if m.reconnecting {
			m.cancelMode()
			return
		}
		m.status = "select a destination port"
		return
	}
	m.cursor = point
	m.refreshHits()
	m.refreshConnectionPreview()
	destination, ok := m.usablePortAt(point)
	if !ok || destination == m.connectSource {
		if m.reconnecting {
			m.cancelMode()
			return
		}
		m.status = "drag to a destination port"
		return
	}
	m.completeConnectionTo(destination)
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
