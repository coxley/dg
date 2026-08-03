package tui

import (
	"github.com/coxley/dg/history"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/layout"
)

type activeTool uint8

const (
	toolNavigate activeTool = iota
	toolRectangle
	toolConnect
)

type sessionKind uint8

const (
	sessionNone sessionKind = iota
	sessionLabelEdit
	sessionConnection
	sessionBend
)

type connectionSession struct {
	source    uint32
	edge      uint32
	oldPort   uint32
	reconnect bool
}

type bendTarget struct {
	edge       uint32
	routeIndex int
	bends      []layout.PinnedBend
	moves      []bendMove
}

type bendMove struct {
	index int
	start layout.Point
}

type bendSession struct {
	targets           []bendTarget
	valid             bool
	axis              bendDragAxis
	sharedPrepared    bool
	preserveSelection bool
}

func (s bendSession) primary() bendTarget {
	return s.targets[0]
}

func (s bendSession) multiple() bool {
	return len(s.targets) > 1
}

type bendDragAxis uint8

const (
	bendAxisNone bendDragAxis = iota
	bendAxisHorizontal
	bendAxisVertical
)

type interactionSession struct {
	kind       sessionKind
	connection connectionSession
	bend       bendSession
}

type gestureKind uint8

const (
	gestureNone gestureKind = iota
	gestureMove
	gestureResize
	gestureBend
	gestureRectangle
	gestureDuplicatePending
	gestureDuplicate
	gestureAreaSelection
	gestureConnectionPending
	gestureConnection
	gestureLabelPress
)

type pointerGesture struct {
	kind   gestureKind
	target layout.Hit
	start  layout.Point
	point  layout.Point
	offset layout.Point
	fixed  layout.Point
	corner resizeCorner
	rigid  bool

	duplicateRank   int
	attachmentEdge  uint32
	attachmentPoint layout.Point
	hasAttachment   bool
	moved           bool
}

func (g pointerGesture) duplicateActive() bool {
	return g.kind == gestureDuplicate
}

func (g pointerGesture) connectionActive() bool {
	return g.kind == gestureConnection
}

type clickTracker struct {
	point layout.Point
	valid bool
}

type controlDrag struct {
	target layout.Hit
	start  layout.Point
	offset layout.Point
	valid  bool
}

type interactionRenderCache struct {
	connectionPreview  []layout.Point
	connectionRaster   []layout.RasterCell
	bendPreview        []layout.Point
	bendRaster         []layout.RasterCell
	bendLayout         *layout.Layout
	duplicateLayout    *layout.Layout
	duplicateHighlight []bool
	selectionHighlight []bool
}

func (c *interactionRenderCache) clearDuplicate(canvas *canvasview.Model) {
	c.duplicateLayout = nil
	canvas.Clear(canvasview.DuplicateFrame)
	c.duplicateHighlight = c.duplicateHighlight[:0]
}

type transactionOwner uint8

const (
	transactionNone transactionOwner = iota
	transactionImmediate
	transactionKeyboardMove
	transactionPointerMove
	transactionResize
	transactionBend
	transactionRectangle
	transactionDuplicate
	transactionLabelEdit
	transactionConnection
	transactionPreferences
)

type interactionTransaction struct {
	value history.Transaction
	owner transactionOwner
}

func (t interactionTransaction) open() bool {
	return t.owner != transactionNone
}

type interactionState struct {
	tool        activeTool
	session     interactionSession
	gesture     pointerGesture
	click       clickTracker
	controlDrag controlDrag
	render      interactionRenderCache
	transaction interactionTransaction
}

func (s interactionState) mode() mode {
	switch s.session.kind {
	case sessionLabelEdit:
		return modeEditLabel
	case sessionConnection:
		return modeConnect
	case sessionNone, sessionBend:
	}
	switch s.tool {
	case toolRectangle:
		return modeRectangle
	case toolConnect:
		return modeConnect
	case toolNavigate:
		return modeNavigate
	default:
		return modeNavigate
	}
}

func (s interactionState) idle() bool {
	return s.tool == toolNavigate &&
		s.session.kind == sessionNone &&
		s.gesture.kind == gestureNone
}

func (s interactionState) movingRigidly() bool {
	return s.gesture.kind == gestureMove && s.gesture.rigid
}

func (s *interactionState) resetGesture() {
	s.gesture = pointerGesture{}
	s.controlDrag = controlDrag{}
}

func axisLockedPointer(
	start layout.Point,
	x, y int64,
) (int64, int64) {
	startX, startY := int64(start.X), int64(start.Y)
	dx := max(x, startX) - min(x, startX)
	dy := max(y, startY) - min(y, startY)
	if dx >= dy {
		y = startY
	} else {
		x = startX
	}
	return x, y
}

func axisLockedPoint(start, point layout.Point) layout.Point {
	x, y := axisLockedPointer(start, int64(point.X), int64(point.Y))
	return layout.NewPoint(uint32(x), uint32(y))
}
