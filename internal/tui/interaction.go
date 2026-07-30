package tui

import (
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
)

type connectionSession struct {
	source    uint32
	edge      uint32
	oldPort   uint32
	reconnect bool
}

type interactionSession struct {
	kind       sessionKind
	connection connectionSession
}

type gestureKind uint8

const (
	gestureNone gestureKind = iota
	gestureMove
	gestureResize
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

type interactionRenderCache struct {
	connectionPreview  []layout.Point
	connectionRaster   []layout.RasterCell
	duplicateLayout    *layout.Layout
	duplicateHighlight []bool
	moveHighlight      []bool
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
	transactionRectangle
	transactionDuplicate
	transactionLabelEdit
	transactionConnection
	transactionPreferences
)

type interactionTransaction struct {
	value layout.Transaction
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
	render      interactionRenderCache
	transaction interactionTransaction
}

func (s interactionState) mode() mode {
	switch s.session.kind {
	case sessionLabelEdit:
		return modeEditLabel
	case sessionConnection:
		return modeConnect
	case sessionNone:
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
}
