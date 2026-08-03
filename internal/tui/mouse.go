package tui

import (
	"errors"
	"math"

	tea "charm.land/bubbletea/v2"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
)

const (
	reconnectDragRadius  = 3
	connectionSnapRadius = 2
)

func (m *Model) updateMouseClick(mouse tea.Mouse) {
	point, ok := m.documentPoint(mouse.X, mouse.Y)
	if !ok {
		return
	}
	m.interaction.controlDrag = controlDrag{}
	if m.interaction.tool == toolRectangle {
		if mouse.Button == tea.MouseLeft {
			m.startRectangle(point)
		}
		return
	}
	if m.interaction.gesture.kind == gestureConnectionPending {
		m.interaction.resetGesture()
	}
	if mouse.Button == tea.MouseRight {
		m.interaction.click.valid = false
		m.cursor = point
		m.refreshHits()
		if m.beginResize(point) || m.beginBendDrag(point) {
			return
		}
		return
	}
	if mouse.Button != tea.MouseLeft {
		return
	}
	repeated := m.interaction.click.valid && point == m.interaction.click.point
	m.interaction.click = clickTracker{point: point, valid: true}
	m.cursor = point
	m.refreshHits()
	m.prioritizeSelectedEdge()
	if repeated && m.handleRepeatedClick() {
		return
	}

	if m.interaction.session.kind == sessionLabelEdit {
		m.moveCaretToPoint(point)
		if m.geo.Nodes[m.target.ID].Rect.Contains(point) {
			rect := m.geo.Nodes[m.target.ID].Rect
			m.interaction.gesture = pointerGesture{
				kind:   gestureLabelPress,
				target: m.target,
				start:  point,
				point:  point,
				offset: layout.NewPoint(point.X-rect.Min.X, point.Y-rect.Min.Y),
			}
		}
		return
	}
	if m.interaction.tool == toolConnect {
		m.updateConnectionClick(point)
		return
	}
	hit, ok := m.activeHit()
	if !ok {
		m.interaction.resetGesture()
		if m.interaction.idle() && !mouse.Mod.Contains(tea.ModCtrl) {
			m.beginAreaSelection(point)
		}
		return
	}
	if m.interaction.idle() && mouse.Mod.Contains(tea.ModAlt) {
		m.beginDuplicateDrag(point, hit)
		return
	}
	if m.interaction.idle() && mouse.Mod.Contains(tea.ModCtrl) {
		m.geo.Selection().Toggle(hit)
		m.refreshSelectionHighlight()
		m.interaction.resetGesture()
		if hit.Kind == layout.HitNode {
			rect := m.geo.Nodes[hit.ID].Rect
			m.interaction.controlDrag = controlDrag{
				target: hit,
				start:  point,
				offset: layout.NewPoint(
					point.X-rect.Min.X,
					point.Y-rect.Min.Y,
				),
				valid: true,
			}
		}
		m.status = ""
		return
	}
	if m.interaction.idle() &&
		hit.Kind == layout.HitEdge &&
		m.nearEdgeEndpoint(hit.ID, point) {
		m.selectOnly(hit)
		m.interaction.gesture = pointerGesture{
			kind:   gestureConnectionPending,
			target: hit,
			start:  point,
		}
		m.status = ""
		return
	}
	if hit.Kind != layout.HitNode {
		m.selectOnly(hit)
		m.interaction.resetGesture()
		return
	}
	if !m.hitSelected(hit) {
		m.selectOnly(hit)
	}
	rect := m.geo.Nodes[hit.ID].Rect
	m.target = hit
	m.beginTransaction(transactionPointerMove)
	m.interaction.gesture = pointerGesture{
		kind:   gestureMove,
		target: hit,
		start:  point,
		point:  point,
		offset: layout.NewPoint(point.X-rect.Min.X, point.Y-rect.Min.Y),
		rigid:  m.geo.SelectionMovesRigidly(),
	}
}

func (m *Model) handleRepeatedClick() bool {
	if m.interaction.gesture.kind == gestureConnectionPending {
		m.interaction.resetGesture()
	}
	if m.interaction.idle() && m.resetDoubleClickedObject() {
		return true
	}
	m.cycleHit(1)
	return false
}

func (m *Model) resetDoubleClickedObject() bool {
	return m.autoSizeDoubleClickedNode() || m.resetDoubleClickedEdge()
}

func (m *Model) beginResize(point layout.Point) bool {
	if !m.interaction.idle() {
		return false
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
		m.beginTransaction(transactionResize)
		m.interaction.gesture = pointerGesture{
			kind:   gestureResize,
			target: hit,
			fixed:  fixed,
			corner: corner,
		}
		m.status = ""
		return true
	}
	return false
}

func (m *Model) autoSizeDoubleClickedNode() bool {
	for _, hit := range m.hits {
		if hit.Kind != layout.HitNode {
			continue
		}
		if _, explicit := m.geo.ExplicitNodeSize(hit.ID); !explicit {
			return false
		}
		m.beginTransaction(transactionImmediate)
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
	owner, ok := m.canvas.OwnerAt(canvasview.BaseFrame, m.cursor)
	for i, hit := range m.hits {
		if hit.Kind != layout.HitEdge || !selection.Contains(hit) {
			continue
		}
		if (ok && hit == owner) ||
			m.nearEdgeEndpoint(hit.ID, m.cursor) {
			m.active = i
			return
		}
	}
}

func (m *Model) updateConnectionClick(point layout.Point) {
	if m.interaction.session.kind != sessionConnection {
		portID, ok := m.nearestConnectionPort(point, layout.NoPortID)
		if !ok {
			m.status = dragFromSource
			return
		}
		m.interaction.session = interactionSession{
			kind: sessionConnection,
			connection: connectionSession{
				source: portID,
			},
		}
	}
	if m.interaction.session.connection.reconnect {
		if err := m.renderConnectionBase(); err != nil {
			m.setError(err.Error())
			return
		}
	}
	m.refreshConnectionPreview()
	m.interaction.gesture = pointerGesture{kind: gestureConnection}
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
	if m.startControlMove(mouse) {
		return
	}
	switch m.interaction.gesture.kind {
	case gestureNone:
	case gestureMove:
		m.updateMoveMotion(mouse)
	case gestureResize:
		if mouse.Button == tea.MouseRight {
			if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
				m.resizeNode(point)
			}
		}
	case gestureBend:
		if mouse.Button == tea.MouseRight {
			x, y := m.unboundedDocumentPoint(mouse.X, mouse.Y)
			m.updateBendDrag(clampDocumentPoint(x, y))
		}
	case gestureRectangle:
		m.updateRectangleMotion(mouse)
	case gestureDuplicatePending, gestureDuplicate:
		m.updateDuplicateMotion(mouse)
	case gestureAreaSelection:
		m.updateAreaSelectionMotion(mouse)
	case gestureConnectionPending, gestureConnection:
		m.updateConnectionMotion(mouse)
	case gestureLabelPress:
		m.updateLabelPressMotion(mouse)
	}
}

func (m *Model) startControlMove(mouse tea.Mouse) bool {
	pending := m.interaction.controlDrag
	if !pending.valid ||
		m.interaction.gesture.kind != gestureNone ||
		mouse.Button != tea.MouseLeft ||
		!mouse.Mod.Contains(tea.ModCtrl) {
		return false
	}
	x, y := m.unboundedDocumentPoint(mouse.X, mouse.Y)
	if x == int64(pending.start.X) && y == int64(pending.start.Y) {
		return true
	}
	if !m.geo.Selection().Contains(pending.target) {
		m.geo.Selection().Toggle(pending.target)
		m.refreshSelectionHighlight()
	}
	m.target = pending.target
	m.beginTransaction(transactionPointerMove)
	m.interaction.controlDrag = controlDrag{}
	m.interaction.gesture = pointerGesture{
		kind:   gestureMove,
		target: pending.target,
		start:  pending.start,
		point:  pending.start,
		offset: pending.offset,
		rigid:  m.geo.SelectionMovesRigidly(),
	}
	m.updateMoveMotion(mouse)
	return true
}

func (m *Model) updateAreaSelectionMotion(mouse tea.Mouse) {
	if mouse.Button == tea.MouseLeft {
		x, y := m.unboundedDocumentPoint(mouse.X, mouse.Y)
		m.updateAreaSelection(clampDocumentPoint(x, y))
	}
}

func (m *Model) updateLabelPressMotion(mouse tea.Mouse) {
	if mouse.Button != tea.MouseLeft {
		return
	}
	gesture := m.interaction.gesture
	nodeID := gesture.target.ID
	m.commitLabelEdit()
	m.beginTransaction(transactionPointerMove)
	m.interaction.gesture = pointerGesture{
		kind:   gestureMove,
		target: gesture.target,
		start:  gesture.start,
		point:  gesture.point,
		offset: gesture.offset,
		rigid:  m.geo.SelectionMovesRigidly(),
	}
	x, y := m.unboundedDocumentPoint(mouse.X, mouse.Y)
	if mouse.Mod.Contains(tea.ModCtrl) {
		x, y = axisLockedPointer(gesture.start, x, y)
	}
	m.dragNode(nodeID, x, y)
}

func (m *Model) updateMoveMotion(mouse tea.Mouse) {
	target := m.interaction.gesture.target
	if mouse.Button != tea.MouseLeft || !m.geo.NodeExists(target.ID) {
		return
	}
	x, y := m.unboundedDocumentPoint(mouse.X, mouse.Y)
	if mouse.Mod.Contains(tea.ModCtrl) {
		x, y = axisLockedPointer(m.interaction.gesture.start, x, y)
	}
	m.dragNode(target.ID, x, y)
}

func (m *Model) updateConnectionMotion(mouse tea.Mouse) {
	if m.interaction.gesture.kind == gestureConnectionPending &&
		mouse.Button == tea.MouseLeft {
		point, ok := m.documentPoint(mouse.X, mouse.Y)
		gesture := m.interaction.gesture
		if !ok || point == gesture.start {
			return
		}
		hit := gesture.target
		start := gesture.start
		m.clearConnection()
		m.cursor = start
		if err := m.startConnection(hit); err != nil {
			m.setError(err.Error())
			return
		}
		if err := m.renderConnectionBase(); err != nil {
			m.clearConnection()
			m.setError(err.Error())
			return
		}
		m.interaction.gesture = pointerGesture{kind: gestureConnection}
		m.cursor = point
		m.refreshHits()
		m.refreshConnectionPreview()
		m.ensureCursorVisible()
		m.status = ""
		return
	}
	if m.interaction.session.kind == sessionConnection &&
		m.interaction.gesture.kind == gestureConnection &&
		mouse.Button == tea.MouseLeft {
		if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
			m.cursor = point
			m.refreshHits()
			m.refreshConnectionPreview()
			m.ensureCursorVisible()
			m.status = ""
		}
	}
}

func (m *Model) updateRectangleMotion(mouse tea.Mouse) {
	if mouse.Button != tea.MouseLeft {
		return
	}
	if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
		m.resizeNode(point)
	}
}

func (m *Model) updateMouseRelease(mouse tea.Mouse) {
	if m.interaction.gesture.kind == gestureNone &&
		m.interaction.controlDrag.valid &&
		mouse.Button == tea.MouseLeft {
		m.interaction.controlDrag = controlDrag{}
		return
	}
	switch m.interaction.gesture.kind {
	case gestureNone:
	case gestureDuplicatePending, gestureDuplicate:
		if mouse.Button != tea.MouseLeft {
			return
		}
		point, ok := m.documentPoint(mouse.X, mouse.Y)
		if !ok {
			m.cancelDuplicateDrag()
			return
		}
		if mouse.Mod.Contains(tea.ModCtrl) {
			point = axisLockedPoint(m.interaction.gesture.start, point)
		}
		m.finishDuplicateDrag(point)
	case gestureRectangle:
		if mouse.Button != tea.MouseLeft {
			return
		}
		if point, ok := m.documentPoint(mouse.X, mouse.Y); ok {
			m.resizeNode(point)
		}
		m.finishRectangle()
	case gestureConnection:
		m.updateConnectionRelease(mouse)
	case gestureConnectionPending:
		m.interaction.resetGesture()
		m.status = ""
	case gestureResize:
		if mouse.Button == tea.MouseRight {
			m.finishMove()
		}
	case gestureBend:
		if mouse.Button == tea.MouseRight {
			x, y := m.unboundedDocumentPoint(mouse.X, mouse.Y)
			m.updateBendDrag(clampDocumentPoint(x, y))
			m.finishBendDrag()
		}
	case gestureAreaSelection:
		x, y := m.unboundedDocumentPoint(mouse.X, mouse.Y)
		m.updateAreaSelection(clampDocumentPoint(x, y))
		m.finishAreaSelection()
	case gestureMove:
		if mouse.Button == tea.MouseLeft {
			m.finishMove()
		}
	case gestureLabelPress:
		m.interaction.resetGesture()
	}
}

func (m *Model) startRectangle(point layout.Point) {
	if m.interaction.gesture.kind == gestureRectangle {
		return
	}
	m.beginTransaction(transactionRectangle)
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
	m.interaction.gesture = pointerGesture{
		kind:   gestureRectangle,
		target: hit,
		fixed:  point,
		corner: resizeEast | resizeSouth,
	}
	m.selectOnly(hit)
	m.resizeNode(point)
}

func (m *Model) finishRectangle() {
	m.interaction.resetGesture()
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.refreshHits()
	m.selectTarget()
	m.status = "drag to create a rectangle"
}

func (m *Model) resizeNode(point layout.Point) {
	nodeID := m.target.ID
	if !m.geo.NodeExists(nodeID) {
		m.interaction.resetGesture()
		m.setError("selected node no longer exists")
		return
	}
	gesture := m.interaction.gesture
	padding, _ := m.geo.NodePadding(nodeID)
	originX, width := resizeAxis(
		point.X,
		gesture.fixed.X,
		gesture.corner&resizeEast != 0,
		uint32(padding.Left)+uint32(padding.Right)+2,
	)
	originY, height := resizeAxis(
		point.Y,
		gesture.fixed.Y,
		gesture.corner&resizeSouth != 0,
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
	m.cursor = resizeCornerPoint(m.geo.Nodes[nodeID].Rect, gesture.corner)
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
	m.interaction.resetGesture()
	m.setError(errors.Join(
		resizeErr,
		m.cancelTransaction(),
		m.render(),
	).Error())
	m.refreshHits()
}

func (m *Model) updateConnectionRelease(mouse tea.Mouse) {
	connection := m.interaction.session.connection
	m.interaction.resetGesture()
	point, ok := m.documentPoint(mouse.X, mouse.Y)
	if !ok {
		if connection.reconnect {
			m.cancelMode()
			return
		}
		m.clearConnection()
		m.status = dragFromSource
		return
	}
	m.cursor = point
	m.refreshHits()
	m.refreshConnectionPreview()
	destination, ok := m.nearestConnectionPort(point, connection.source)
	if !ok {
		if connection.reconnect {
			m.cancelMode()
			return
		}
		m.clearConnection()
		m.status = dragFromSource
		return
	}
	m.completeConnectionTo(destination)
}

func (m *Model) nearestConnectionPort(
	point layout.Point,
	excluded uint32,
) (uint32, bool) {
	best := layout.NoPortID
	bestDistance := uint64(connectionSnapRadius + 1)
	for dy := -connectionSnapRadius; dy <= connectionSnapRadius; dy++ {
		for dx := -connectionSnapRadius; dx <= connectionSnapRadius; dx++ {
			distance := uint64(max(dx, -dx) + max(dy, -dy))
			if distance > connectionSnapRadius {
				continue
			}
			candidate, ok := movePoint(point, dx, dy)
			if !ok {
				continue
			}
			portID, ok := m.usablePortAt(candidate)
			if !ok || portID == excluded ||
				distance > bestDistance ||
				distance == bestDistance && portID >= best {
				continue
			}
			best = portID
			bestDistance = distance
		}
	}
	return best, best != layout.NoPortID
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
	canvas, ok := m.workspace.ScreenToCanvas(chrome.Point{X: x, Y: y})
	if !ok {
		return layout.Point{}, false
	}
	documentX := uint64(m.viewport.X) + uint64(canvas.X)
	documentY := uint64(m.viewport.Y) + uint64(canvas.Y)
	if documentX > math.MaxUint32 || documentY > math.MaxUint32 {
		return layout.Point{}, false
	}
	return layout.NewPoint(uint32(documentX), uint32(documentY)), true
}

func (m *Model) unboundedDocumentPoint(x, y int) (int64, int64) {
	canvas := m.workspace.Geometry().Canvas
	return int64(m.viewport.X) + int64(x) - int64(canvas.X),
		int64(m.viewport.Y) + int64(y) - int64(canvas.Y)
}

func clampDocumentPoint(x, y int64) layout.Point {
	return layout.NewPoint(
		uint32(min(max(x, 0), int64(math.MaxUint32))),
		uint32(min(max(y, 0), int64(math.MaxUint32))),
	)
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
