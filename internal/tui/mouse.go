package tui

import (
	"errors"
	"math"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
)

const reconnectDragRadius = 3

func (m *Model) updateMouseClick(mouse tea.Mouse) {
	if m.toolbarContains(mouse.X, mouse.Y) {
		m.updateToolbarClick(mouse)
		return
	}
	point, ok := m.documentPoint(mouse.X, mouse.Y)
	if !ok {
		return
	}
	if m.mode == modeRectangle {
		if mouse.Button == tea.MouseLeft {
			m.startRectangle(point)
		}
		return
	}
	m.edgeDragPending = false
	if mouse.Button == tea.MouseRight {
		m.hasLastClick = false
		m.cursor = point
		m.refreshHits()
		m.beginResize(point)
		return
	}
	if mouse.Button != tea.MouseLeft {
		return
	}
	repeated := m.hasLastClick && point == m.lastClick
	m.lastClick, m.hasLastClick = point, true
	m.cursor = point
	m.refreshHits()
	m.prioritizeSelectedEdge()
	if repeated && m.mode == modeNavigate && m.autoSizeDoubleClickedNode() {
		return
	}
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
	if m.mode == modeNavigate && mouse.Mod.Contains(tea.ModAlt) {
		m.beginDuplicateDrag(point, hit)
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
	m.rigidMoving = m.geo.SelectionMovesRigidly()
	m.dragging = true
}

func (m *Model) updateToolbarClick(mouse tea.Mouse) bool {
	if mouse.Button != tea.MouseLeft ||
		mouse.Y != toolbarToolRow ||
		m.modal != modalNone {
		return false
	}
	x := mouse.X - max((m.width-toolbarBoxWidth)/2, 0) - 2
	switch {
	case x >= 0 && x < len(" Cursor "):
		m.cancelMode()
	case x < len(" Cursor ")+len(" Rectangle "):
		m.activateTool(modeRectangle)
	case x < toolbarToolsWidth:
		m.activateTool(modeConnect)
	default:
		return false
	}
	return true
}

func (m *Model) updateToolbarHover(mouse tea.Mouse) {
	m.hasToolbarHover = false
	if mouse.Y != toolbarToolRow || !m.toolbarContains(mouse.X, mouse.Y) {
		return
	}
	x := mouse.X - max((m.width-toolbarBoxWidth)/2, 0) - 2
	switch {
	case x >= 0 && x < len(" Cursor "):
		m.toolbarHover = modeNavigate
	case x < len(" Cursor ")+len(" Rectangle "):
		m.toolbarHover = modeRectangle
	case x < toolbarToolsWidth:
		m.toolbarHover = modeConnect
	default:
		return
	}
	m.hasToolbarHover = true
}

func (m *Model) toolbarContains(x, y int) bool {
	if m.width < toolbarBoxWidth ||
		y < toolbarTop ||
		y >= toolbarTop+toolbarBoxHeight {
		return false
	}
	left := (m.width - toolbarBoxWidth) / 2
	return x >= left && x < left+toolbarBoxWidth
}

func (m *Model) beginResize(point layout.Point) {
	if m.mode != modeNavigate {
		return
	}
	for _, hit := range m.hits {
		if hit.Kind != layout.HitNode {
			continue
		}
		rect := m.geo.Nodes[hit.ID].Rect
		limit := rect.Max()
		right, bottom := limit.X-1, limit.Y-1
		var corner resizeCorner
		if point.X-rect.Min.X > right-point.X {
			corner |= resizeEast
		}
		if point.Y-rect.Min.Y > bottom-point.Y {
			corner |= resizeSouth
		}
		fixed := layout.NewPoint(right, bottom)
		if corner&resizeEast != 0 {
			fixed.X = rect.Min.X
		}
		if corner&resizeSouth != 0 {
			fixed.Y = rect.Min.Y
		}

		m.selectOnly(hit)
		m.target = hit
		m.resizeCorner = corner
		m.resizeFixed = fixed
		m.beginTransaction()
		m.resizing = true
		m.dragging = false
		m.status = ""
		return
	}
}

func (m *Model) autoSizeDoubleClickedNode() bool {
	for _, hit := range m.hits {
		if hit.Kind != layout.HitNode {
			continue
		}
		if _, explicit := m.geo.ExplicitNodeSize(hit.ID); !explicit {
			return false
		}
		m.beginTransaction()
		if err := m.geo.AutoSizeNode(hit.ID); err != nil {
			m.setError(errors.Join(err, m.cancelTransaction()).Error())
			return true
		}
		if err := m.rebuild(); err != nil {
			m.setError(errors.Join(err, m.cancelTransaction()).Error())
			return true
		}
		if err := m.commitTransaction(); err != nil {
			m.setError(err.Error())
			return true
		}
		m.selectOnly(hit)
		m.refreshHits()
		m.status = ""
		return true
	}
	return false
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
			m.setError(err.Error())
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
	if m.updateRectangleMotion(mouse) {
		return
	}
	if m.updateDuplicateMotion(mouse) {
		return
	}
	if m.updateConnectionMotion(mouse) {
		return
	}
	if m.selecting && mouse.Button == tea.MouseLeft {
		if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
			m.updateAreaSelection(point)
		}
		return
	}
	if m.resizing && mouse.Button == tea.MouseRight {
		if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
			m.resizeNode(point)
		}
		return
	}
	if m.mode == modeEditLabel && m.editMouseDown && mouse.Button == tea.MouseLeft {
		point, ok := m.documentPoint(mouse.X, mouse.Y)
		if !ok {
			return
		}
		nodeID := m.target.ID
		m.commitLabelEdit()
		m.beginTransaction()
		m.editMouseDown = false
		m.rigidMoving = m.geo.SelectionMovesRigidly()
		m.dragging = true
		m.dragNode(nodeID, point)
		return
	}
	if !m.dragging || mouse.Button != tea.MouseLeft || !m.geo.NodeExists(m.target.ID) {
		return
	}
	point, ok := m.documentPoint(mouse.X, mouse.Y)
	if !ok {
		return
	}
	m.dragNode(m.target.ID, point)
}

func (m *Model) updateConnectionMotion(mouse tea.Mouse) bool {
	if m.edgeDragPending && mouse.Button == tea.MouseLeft {
		point, ok := m.documentPoint(mouse.X, mouse.Y)
		if !ok || point == m.edgeDragStart {
			return true
		}
		hit := m.edgeDragHit
		start := m.edgeDragStart
		m.clearConnection()
		m.cursor = start
		if err := m.startConnection(hit); err != nil {
			m.setError(err.Error())
			return true
		}
		if err := m.renderConnectionBase(); err != nil {
			m.clearConnection()
			m.setError(err.Error())
			return true
		}
		m.mode = modeConnect
		m.connectDragging = true
		m.cursor = point
		m.refreshHits()
		m.refreshConnectionPreview()
		m.ensureCursorVisible()
		m.status = ""
		return true
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
		return true
	}
	return false
}

func (m *Model) updateRectangleMotion(mouse tea.Mouse) bool {
	if !m.creatingRectangle || mouse.Button != tea.MouseLeft {
		return false
	}
	if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
		m.resizeNode(point)
	}
	return true
}

func (m *Model) updateMouseRelease(mouse tea.Mouse) {
	switch {
	case (m.duplicatePending || m.duplicateDragging) &&
		mouse.Button == tea.MouseLeft:
		point, ok := m.documentPoint(mouse.X, mouse.Y)
		if !ok {
			m.cancelDuplicateDrag()
			return
		}
		m.finishDuplicateDrag(point)
	case m.creatingRectangle && mouse.Button == tea.MouseLeft:
		if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
			m.resizeNode(point)
		}
		m.finishRectangle()
	case m.mode == modeConnect && m.connectDragging:
		m.updateConnectionRelease(mouse)
	case m.edgeDragPending:
		m.edgeDragPending = false
		m.status = ""
	case m.resizing && mouse.Button == tea.MouseRight:
		m.resizing = false
		m.finishMove()
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

func (m *Model) startRectangle(point layout.Point) {
	if m.creatingRectangle {
		return
	}
	m.beginTransaction()
	nodeID, err := m.geo.NewNodeAt("", point)
	if err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.geo.SetNodeStyle(nodeID, m.nodeStyle); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	hit := layout.Hit{ID: nodeID, Kind: layout.HitNode}
	m.target = hit
	m.resizeFixed = point
	m.resizeCorner = resizeEast | resizeSouth
	m.creatingRectangle = true
	m.selectOnly(hit)
	m.resizeNode(point)
}

func (m *Model) finishRectangle() {
	m.creatingRectangle = false
	m.mode = modeNavigate
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) resizeNode(point layout.Point) {
	nodeID := m.target.ID
	if !m.geo.NodeExists(nodeID) {
		m.resizing = false
		m.setError("selected node no longer exists")
		return
	}
	padding := m.geo.Padding()
	originX, width := resizeAxis(
		point.X,
		m.resizeFixed.X,
		m.resizeCorner&resizeEast != 0,
		uint32(padding.Left)+uint32(padding.Right)+2,
	)
	originY, height := resizeAxis(
		point.Y,
		m.resizeFixed.Y,
		m.resizeCorner&resizeSouth != 0,
		uint32(padding.Top)+uint32(padding.Bottom)+2,
	)
	origin := layout.NewPoint(originX, originY)
	size := layout.Size{
		Width:  width,
		Height: height,
	}
	if err := m.geo.PlaceNode(nodeID, origin); err != nil {
		m.abortResize(err)
		return
	}
	if err := m.geo.SetNodeSize(nodeID, size); err != nil {
		m.abortResize(err)
		return
	}
	if err := m.rebuild(); err != nil {
		m.abortResize(err)
		return
	}
	m.cursor = resizeCornerPoint(m.geo.Nodes[nodeID].Rect, m.resizeCorner)
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
}

func resizeAxis(
	point, fixed uint32,
	positive bool,
	minSize uint32,
) (uint32, uint32) {
	if positive {
		boundary := min(point, uint32(math.MaxUint32-1))
		boundary = max(boundary, fixed+minSize-1)
		return fixed, boundary - fixed + 1
	}
	origin := min(point, fixed-(minSize-1))
	return origin, fixed - origin + 1
}

func resizeCornerPoint(rect layout.Rect, corner resizeCorner) layout.Point {
	point := rect.Min
	limit := rect.Max()
	if corner&resizeEast != 0 {
		point.X = limit.X - 1
	}
	if corner&resizeSouth != 0 {
		point.Y = limit.Y - 1
	}
	return point
}

func (m *Model) abortResize(resizeErr error) {
	m.resizing = false
	if m.creatingRectangle {
		m.creatingRectangle = false
		m.mode = modeNavigate
	}
	m.setError(errors.Join(
		resizeErr,
		m.cancelTransaction(),
		m.render(),
	).Error())
	m.refreshHits()
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
