// Package tui provides an interactive terminal editor for diagram layouts.
package tui

import (
	"errors"
	"fmt"
	"math"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
)

type mode uint8

type (
	resizeCorner uint8
	modal        uint8
)

const (
	modeNavigate mode = iota
	modeMove
	modeEditLabel
	modeConnect
	modeRectangle
	modeSavePath
)

const (
	modalNone modal = iota
	modalHelp
	modalPreferences
	modalSave
)

const (
	resizeEast resizeCorner = 1 << iota
	resizeSouth
)

const (
	finishOperation = "finish the current operation first"
	dragFromSource  = "drag from a source port"
)

func (m mode) String() string {
	switch m {
	case modeNavigate:
		return "navigate"
	case modeMove:
		return "move"
	case modeEditLabel:
		return "edit label"
	case modeConnect:
		return "connect"
	case modeRectangle:
		return "rectangle"
	case modeSavePath:
		return "save path"
	default:
		return "unknown"
	}
}

type rowSpan struct {
	start int
	end   int
}

// Model coordinates terminal interaction with a layout.
type Model struct {
	geo     *layout.Layout
	history *layout.History

	cursor            layout.Point
	viewport          layout.Point
	hits              []layout.Hit
	active            int
	target            layout.Hit
	connectSource     uint32
	connectEdge       uint32
	connectOldPort    uint32
	reconnecting      bool
	connectStarted    bool
	connectDragging   bool
	connectPreview    []layout.Point
	connectRaster     []layout.RasterCell
	edgeDragPending   bool
	edgeDragHit       layout.Hit
	edgeDragStart     layout.Point
	creatingRectangle bool
	duplicatePending  bool
	duplicateDragging bool
	duplicateStart    layout.Point
	duplicatePoint    layout.Point
	duplicateGeo      *layout.Layout

	mode             mode
	modal            modal
	editBuffer       []byte
	editDraft        []byte
	editLines        []layout.LabelLine
	editCaret        int
	editCaretVisible bool

	dragging      bool
	rigidMoving   bool
	resizing      bool
	resizeCorner  resizeCorner
	resizeFixed   layout.Point
	dragOffset    layout.Point
	editMouseDown bool
	lastClick     layout.Point
	hasLastClick  bool

	frame              render.Frame
	connectFrame       render.Frame
	duplicateFrame     render.Frame
	encoder            render.Encoder
	duplicateEncoder   render.Encoder
	duplicateHighlight []bool
	moveHighlight      []bool
	frameRows          []rowSpan
	connectFrameRows   []rowSpan
	duplicateRows      []rowSpan
	viewBuffer         []byte
	statusText         []byte

	// Bubble Tea compares consecutive cursor pointers, so each View writes the
	// cursor value that the previous View does not reference.
	viewCursor [2]tea.Cursor
	nextCursor uint8

	width  int
	height int
	status string
	path   string

	saveHint string

	preferences   preferenceState
	preferenceRow int

	nodeStyle layout.NodeStyle
	edgeStyle layout.EdgeStyle

	transaction     layout.Transaction
	transactionOpen bool

	moveOrigins         []layout.Point
	focusNodes          []layout.Hit
	selecting           bool
	selectionStartPoint layout.Point
	selectionEndPoint   layout.Point
}

// New returns a TUI model for geo.
func New(geo *layout.Layout) (*Model, error) {
	return newModel(geo, "")
}

func newModel(geo *layout.Layout, path string) (*Model, error) {
	if geo == nil {
		return nil, errors.New("nil layout")
	}
	m := &Model{geo: geo, history: geo.History(), path: path}
	m.viewCursor[0] = *tea.NewCursor(0, 0)
	m.viewCursor[1] = m.viewCursor[0]
	for i := range geo.Nodes {
		if !geo.Nodes[i].Empty() {
			m.cursor = geo.Nodes[i].LabelPoint
			break
		}
	}
	for hit := range geo.DrawOrder() {
		switch hit.Kind {
		case layout.HitNode:
			m.nodeStyle, _ = geo.NodeStyle(hit.ID)
		case layout.HitEdge:
			m.edgeStyle, _ = geo.EdgeStyle(hit.ID)
		case layout.HitPort:
		}
	}
	if err := m.rebuild(); err != nil {
		return nil, err
	}
	m.refreshHits()
	return m, nil
}

// Run starts an interactive terminal editor for geo and saves it to path.
func Run(geo *layout.Layout, path string) error {
	model, err := newModel(geo, path)
	if err != nil {
		return err
	}
	model.loadPreferences()
	defer model.interruptInteraction()
	_, err = tea.NewProgram(model).Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = max(message.Width, 0)
		m.height = max(message.Height, 0)
		m.ensureCursorVisible()
	case tea.KeyPressMsg:
		key := message.Key()
		if key.Code == 'c' && key.Mod == tea.ModCtrl {
			m.interruptInteraction()
			return m, tea.Quit
		}
		m.hasLastClick = false
		if m.modal != modalNone {
			m.updateModal(message)
		} else if key.Code == 's' && key.Mod == tea.ModCtrl {
			m.requestSave()
		} else if m.mode == modeEditLabel {
			m.updateLabel(message)
		} else {
			if key.Code == 'q' && key.Mod == 0 {
				m.interruptInteraction()
				return m, tea.Quit
			}
			m.updateCommand(key)
		}
	case tea.PasteMsg:
		switch {
		case m.modal == modalSave:
			m.insertSavePathText(message.Content)
		case m.mode == modeEditLabel:
			m.insertLabelText(message.Content)
		default:
		}
	case tea.MouseClickMsg:
		if m.modal == modalNone {
			m.updateMouseClick(message.Mouse())
		}
	case tea.MouseReleaseMsg:
		if m.modal == modalNone {
			m.updateMouseRelease(message.Mouse())
		}
	case tea.MouseMotionMsg:
		if m.modal == modalNone {
			m.updateMouseMotion(message.Mouse())
		}
	case tea.MouseWheelMsg:
		if m.modal == modalNone {
			m.updateMouseWheel(message.Mouse())
		}
	case tea.BlurMsg:
		m.interruptInteraction()
	}
	return m, nil
}

func (m *Model) updateCommand(key tea.Key) {
	if key.Code == 'u' && key.Mod == 0 ||
		key.Code == 'z' && key.Mod == tea.ModCtrl {
		m.undo()
		return
	}
	if key.Code == 'r' && key.Mod == tea.ModCtrl ||
		key.Code == 'y' && key.Mod == tea.ModCtrl ||
		key.Code == 'z' && key.Mod == tea.ModCtrl|tea.ModShift {
		m.redo()
		return
	}
	if key.Code == 'a' && key.Mod == tea.ModCtrl {
		m.expandSelection()
		return
	}
	if key.Code == tea.KeyTab && key.Mod == tea.ModCtrl {
		m.cycleHit(1)
		return
	}
	if key.Code == tea.KeyTab && key.Mod == tea.ModCtrl|tea.ModShift {
		m.cycleHit(-1)
		return
	}
	if key.Code == tea.KeyTab && key.Mod == tea.ModShift {
		m.focusNode(-1)
		return
	}
	if key.Mod == tea.ModShift {
		switch key.Code {
		case '[', '{':
			m.reorderLayer(true, true)
		case ']', '}':
			m.reorderLayer(true, false)
		case 'a', 'A':
			m.cycleEdgeArrow(true)
		case 't', 'T':
			m.cycleTextAlignment(true)
		}
		return
	}
	if key.Mod != 0 {
		return
	}
	m.updateNavigationCommand(key.Code)
}

func (m *Model) updateNavigationCommand(code rune) {
	switch code {
	case '?':
		m.openHelp()
	case tea.KeyUp:
		m.move(0, -1)
	case tea.KeyRight:
		m.move(1, 0)
	case tea.KeyDown:
		m.move(0, 1)
	case tea.KeyLeft:
		m.move(-1, 0)
	case tea.KeyTab:
		m.focusNode(1)
	case '[':
		m.reorderLayer(false, true)
	case ']':
		m.reorderLayer(false, false)
	case '{':
		m.reorderLayer(true, true)
	case '}':
		m.reorderLayer(true, false)
	case tea.KeyEnter:
		if m.mode == modeConnect {
			m.completeConnection()
		} else {
			m.beginMove()
		}
	case 'm':
		m.beginMove()
	case 'e':
		m.beginLabelEdit()
	case 'n':
		m.newNode()
	case 'r':
		m.beginRectangle()
	case 'b':
		m.cycleBorder()
	case 'a':
		m.cycleEdgeArrow(false)
	case 't':
		m.cycleTextAlignment(false)
	case 'l':
		if m.mode != modeConnect {
			m.beginConnection()
		}
	case 'd':
		m.duplicateSelectionDefault()
	case tea.KeyBackspace, tea.KeyDelete:
		m.deleteActive()
	case tea.KeyEscape:
		m.cancelMode()
	}
}

func (m *Model) reorderLayer(all, backward bool) {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	hit, ok := m.selectedLayer()
	if !ok {
		hit, ok = m.activeHit()
	}
	if !ok || hit.Kind == layout.HitPort {
		m.status = "select a node or edge to reorder"
		return
	}
	var err error
	switch {
	case all && backward:
		err = m.geo.SendToBack(hit)
	case all:
		err = m.geo.BringToFront(hit)
	case backward:
		err = m.geo.SendBackward(hit)
	default:
		err = m.geo.BringForward(hit)
	}
	if err != nil {
		m.status = err.Error()
		return
	}
	if err := m.render(); err != nil {
		m.status = err.Error()
		return
	}
	m.selectOnly(hit)
	m.target = hit
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) selectedLayer() (layout.Hit, bool) {
	var selected layout.Hit
	ok := false
	for hit := range m.geo.DrawOrder() {
		if m.geo.Selection().Contains(hit) {
			selected, ok = hit, true
		}
	}
	return selected, ok
}

func (m *Model) move(dx, dy int) {
	if m.mode == modeMove {
		m.moveNode(dx, dy)
		return
	}
	if m.mode == modeNavigate {
		nodes, edges := m.selectedCounts()
		if nodes > 0 && nodes+edges > 1 {
			m.shiftSelection(dx, dy)
			return
		}
		if hit, ok := m.focusedNode(); ok {
			m.shiftFocusedNode(hit, dx, dy)
			return
		}
	}
	point, ok := movePoint(m.cursor, dx, dy)
	if !ok {
		return
	}
	m.cursor = point
	m.refreshHits()
	if m.mode == modeConnect {
		m.refreshConnectionPreview()
	}
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) moveNode(dx, dy int) {
	if m.target.Kind != layout.HitNode || !m.geo.NodeExists(m.target.ID) {
		m.mode = modeNavigate
		m.status = "selected node no longer exists"
		return
	}
	cursor, ok := movePoint(m.cursor, dx, dy)
	if !ok {
		return
	}
	if _, err := m.moveSelectedNodes(int64(dx), int64(dy), cursor); err != nil {
		m.status = err.Error()
	}
}

func (m *Model) placeNode(nodeID uint32, origin, cursor layout.Point) {
	if !m.geo.Selection().Contains(
		layout.Hit{ID: nodeID, Kind: layout.HitNode},
	) {
		m.selectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	}
	previous := m.geo.Nodes[nodeID].Rect.Min
	dx := int64(origin.X) - int64(previous.X)
	dy := int64(origin.Y) - int64(previous.Y)
	if _, err := m.moveSelectedNodes(dx, dy, cursor); err != nil {
		m.status = err.Error()
	}
}

func (m *Model) moveSelectedNodes(
	dx, dy int64,
	cursor layout.Point,
) (bool, error) {
	if missing := len(m.geo.Nodes) - len(m.moveOrigins); missing > 0 {
		m.moveOrigins = slices.Grow(m.moveOrigins, missing)[:len(m.geo.Nodes)]
	}
	for nodeID := range m.geo.Selection().Nodes() {
		origin := m.geo.Nodes[nodeID].Rect.Min
		_, ok := moveCoordinate64(origin.X, dx)
		if !ok {
			return false, nil
		}
		_, ok = moveCoordinate64(origin.Y, dy)
		if !ok {
			return false, nil
		}
		m.moveOrigins[nodeID] = origin
	}
	if err := m.geo.MoveSelection(dx, dy); err != nil {
		return false, errors.Join(err, m.restoreMovedNodes())
	}
	if m.rigidMoving {
		if err := m.render(); err != nil {
			return false, errors.Join(err, m.restoreMovedNodes())
		}
		m.moveHighlight = appendSelectionHighlight(
			m.moveHighlight,
			m.geo,
			m.frame,
		)
	} else if err := m.rebuildSelection(); err != nil {
		return false, errors.Join(err, m.restoreMovedNodes())
	}
	m.cursor = cursor
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
	return true, nil
}

func (m *Model) restoreMovedNodes() error {
	var restoreErr error
	for nodeID := range m.geo.Selection().Nodes() {
		restoreErr = errors.Join(
			restoreErr,
			m.geo.PlaceNode(nodeID, m.moveOrigins[nodeID]),
		)
	}
	return errors.Join(restoreErr, m.rebuild())
}

func (m *Model) beginMove() {
	if m.mode == modeMove {
		m.finishMove()
		return
	}
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	if !m.hasSelectedNodes() {
		hit, ok := m.activeHit()
		if !ok || hit.Kind != layout.HitNode {
			m.status = "select a node to move"
			return
		}
		m.selectOnly(hit)
	}
	target, ok := m.firstSelectedNode()
	if !ok {
		m.status = "select a node to move"
		return
	}
	m.target = target
	m.beginTransaction()
	m.rigidMoving = m.geo.SelectionMovesRigidly()
	m.mode = modeMove
	m.status = ""
}

func (m *Model) beginLabelEdit() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitNode {
		m.status = "select a node to edit"
		return
	}
	m.beginTransaction()
	m.startLabelEdit(hit)
}

func (m *Model) newNode() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	m.beginTransaction()
	nodeID, err := m.geo.NewNodeAt("", m.cursor)
	if err != nil {
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.geo.SetNodeStyle(nodeID, m.nodeStyle); err != nil {
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.rebuild(); err != nil {
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	m.startLabelEdit(layout.Hit{ID: nodeID, Kind: layout.HitNode})
}

func (m *Model) beginRectangle() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	m.clearConnection()
	m.mode = modeRectangle
	m.status = "drag to create a rectangle"
}

func (m *Model) beginConnection() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	m.clearConnection()
	m.mode = modeConnect
	hit, ok := m.activeHit()
	if !ok {
		m.status = dragFromSource
		return
	}
	if err := m.startConnection(hit); err != nil {
		m.status = err.Error()
		return
	}
	m.status = ""
}

func (m *Model) startConnection(hit layout.Hit) error {
	switch hit.Kind {
	case layout.HitPort:
		m.connectSource = hit.ID
		m.reconnecting = false
	case layout.HitEdge:
		portA, portB, err := m.geo.EdgePorts(hit.ID)
		if err != nil {
			return err
		}
		moving, stationary := portA, portB
		if pointDistance(m.cursor, m.geo.Ports[portB].Anchor) <
			pointDistance(m.cursor, m.geo.Ports[portA].Anchor) {
			moving, stationary = portB, portA
		}
		m.connectEdge = hit.ID
		m.connectOldPort = moving
		m.connectSource = stationary
		m.reconnecting = true
	default:
		return errors.New(dragFromSource)
	}
	m.connectStarted = true
	m.refreshConnectionPreview()
	return nil
}

func (m *Model) completeConnection() {
	if !m.connectStarted {
		m.status = dragFromSource
		return
	}
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitPort {
		m.status = "select a destination port"
		return
	}
	m.completeConnectionTo(hit.ID)
}

func (m *Model) completeConnectionTo(destination uint32) {
	m.beginTransaction()
	edgeID := m.connectEdge
	var err error
	if m.reconnecting {
		err = m.geo.ReconnectEdge(edgeID, m.connectOldPort, destination)
	} else {
		edgeID, err = m.geo.ConnectPorts(m.connectSource, destination)
	}
	if err != nil {
		_ = m.cancelTransaction()
		m.status = err.Error()
		return
	}
	if !m.reconnecting {
		if err := m.geo.SetEdgeStyle(edgeID, m.edgeStyle); err != nil {
			_ = m.cancelTransaction()
			m.status = err.Error()
			return
		}
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
	m.mode = modeNavigate
	m.target = layout.Hit{ID: edgeID, Kind: layout.HitEdge}
	m.selectOnly(m.target)
	m.clearConnection()
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) deleteActive() {
	if m.mode != modeNavigate && m.mode != modeMove {
		m.status = finishOperation
		return
	}
	if m.mode == modeMove {
		if err := m.commitTransaction(); err != nil {
			m.status = err.Error()
			return
		}
		m.mode = modeNavigate
		m.dragging = false
	}
	if !m.hasSelection() {
		hit, ok := m.activeHit()
		if !ok {
			m.status = "nothing selected"
			return
		}
		if hit.Kind == layout.HitPort {
			m.status = "ports cannot be deleted independently"
			return
		}
		m.selectOnly(hit)
	}
	m.beginTransaction()
	var err error
	for nodeID := range m.geo.Selection().Nodes() {
		err = m.geo.DeleteNode(nodeID)
		if err != nil {
			break
		}
	}
	if err == nil {
		for edgeID := range m.geo.Selection().Edges() {
			err = m.geo.DeleteEdge(edgeID)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		_ = m.cancelTransaction()
		m.status = err.Error()
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
	m.mode = modeNavigate
	m.target = layout.Hit{}
	m.dragging = false
	m.clearSelection()
	m.refreshHits()
	m.status = ""
}

func (m *Model) cancelMode() {
	if m.mode == modeMove {
		m.finishMove()
		return
	}
	if m.mode == modeRectangle && m.creatingRectangle {
		m.creatingRectangle = false
		m.mode = modeNavigate
		err := errors.Join(
			m.cancelTransaction(),
			m.render(),
		)
		if err != nil {
			m.status = err.Error()
		} else {
			m.status = ""
		}
		m.refreshHits()
		return
	}
	m.mode = modeNavigate
	m.clearConnection()
	m.cancelDuplicateDrag()
	m.dragging = false
	m.selecting = false
	m.creatingRectangle = false
	m.status = ""
}

func (m *Model) beginTransaction() {
	if m.history == nil {
		return
	}
	m.transaction = m.history.Begin()
	m.transactionOpen = true
}

func (m *Model) commitTransaction() error {
	if !m.transactionOpen {
		return nil
	}
	m.transactionOpen = false
	return m.transaction.Commit()
}

func (m *Model) cancelTransaction() error {
	if !m.transactionOpen {
		return nil
	}
	m.transactionOpen = false
	return m.transaction.Cancel()
}

func (m *Model) finishMove() {
	var routeErr error
	if m.rigidMoving {
		routeErr = m.rebuildSelection()
	}
	err := errors.Join(routeErr, m.commitTransaction())
	m.mode = modeNavigate
	m.dragging = false
	m.rigidMoving = false
	m.moveHighlight = m.moveHighlight[:0]
	if err != nil {
		m.status = err.Error()
	} else {
		m.status = ""
	}
}

func (m *Model) interruptInteraction() {
	if m.history != nil {
		m.history.Interrupt()
	}
	m.transactionOpen = false
	if m.mode == modeEditLabel {
		m.finishLabelEdit()
	} else {
		m.mode = modeNavigate
	}
	m.clearConnection()
	m.cancelDuplicateDrag()
	m.dragging = false
	m.rigidMoving = false
	m.moveHighlight = m.moveHighlight[:0]
	m.resizing = false
	m.creatingRectangle = false
	m.editMouseDown = false
	m.selecting = false
}

func (m *Model) undo() {
	if m.history == nil {
		m.status = "undo history is unavailable"
		return
	}
	changed, err := m.history.Undo()
	m.afterHistoryChange(changed, err, "nothing to undo")
}

func (m *Model) redo() {
	if m.history == nil {
		m.status = "undo history is unavailable"
		return
	}
	changed, err := m.history.Redo()
	m.afterHistoryChange(changed, err, "nothing to redo")
}

func (m *Model) afterHistoryChange(changed bool, err error, unchanged string) {
	m.mode = modeNavigate
	m.dragging = false
	m.transactionOpen = false
	m.clearConnection()
	m.clearSelection()
	if err != nil {
		m.status = err.Error()
		return
	}
	if !changed {
		m.status = unchanged
		return
	}
	if err := m.render(); err != nil {
		m.status = err.Error()
		return
	}
	m.refreshHits()
	m.status = ""
}

func (m *Model) clearConnection() {
	m.connectSource = 0
	m.connectEdge = 0
	m.connectOldPort = 0
	m.reconnecting = false
	m.connectStarted = false
	m.connectDragging = false
	m.connectPreview = m.connectPreview[:0]
	m.connectRaster = m.connectRaster[:0]
	m.edgeDragPending = false
	m.edgeDragHit = layout.Hit{}
	m.edgeDragStart = layout.Point{}
}

func (m *Model) rebuild() error {
	if err := m.geo.Build(); err != nil {
		return fmt.Errorf("build layout: %w", err)
	}
	return m.render()
}

func (m *Model) rebuildSelection() error {
	if err := m.geo.BuildSelection(); err != nil {
		return fmt.Errorf("build selection: %w", err)
	}
	return m.render()
}

func (m *Model) render() error {
	if !hasGeometry(m.geo) {
		m.frame = render.Frame{}
		m.frameRows = m.frameRows[:0]
		return nil
	}
	frame, err := m.encoder.EncodeFrame(m.frame.Text[:0], m.geo)
	if err != nil {
		return fmt.Errorf("render layout: %w", err)
	}
	m.frame = frame
	m.frameRows = indexFrameRows(m.frameRows, m.frame.Text)
	return nil
}

func (m *Model) renderConnectionBase() error {
	frame, err := m.encoder.EncodeFrameWithoutEdge(
		m.connectFrame.Text[:0],
		m.geo,
		m.connectEdge,
	)
	if err != nil {
		return fmt.Errorf("render connection base: %w", err)
	}
	m.connectFrame = frame
	m.connectFrameRows = indexFrameRows(
		m.connectFrameRows,
		m.connectFrame.Text,
	)
	return nil
}

func (m *Model) refreshHits() {
	m.hits = slices.AppendSeq(m.hits[:0], m.geo.Hits(m.cursor))
	m.active = 0
}

func (m *Model) cycleHit(delta int) {
	if len(m.hits) == 0 {
		return
	}
	m.active = (m.active + delta + len(m.hits)) % len(m.hits)
	m.clearSelection()
	m.status = ""
}

func (m *Model) activeHit() (layout.Hit, bool) {
	if m.active < 0 || m.active >= len(m.hits) {
		return layout.Hit{}, false
	}
	return m.hits[m.active], true
}

func (m *Model) selectTarget() {
	for i, hit := range m.hits {
		if hit == m.target {
			m.active = i
			return
		}
	}
}

func (m *Model) ensureCursorVisible() {
	height := m.diagramHeight()
	m.viewport.X = visibleOrigin(m.viewport.X, m.cursor.X, m.width)
	m.viewport.Y = visibleOrigin(m.viewport.Y, m.cursor.Y, height)
}

func (m *Model) diagramHeight() int {
	return max(m.height-toolbarHeight-1, 0)
}

func indexFrameRows(rows []rowSpan, text []byte) []rowSpan {
	rows = rows[:0]
	start := 0
	for i, value := range text {
		if value == '\n' {
			rows = append(rows, rowSpan{start: start, end: i})
			start = i + 1
		}
	}
	if start < len(text) {
		rows = append(rows, rowSpan{start: start, end: len(text)})
	}
	return rows
}

func hasGeometry(geo *layout.Layout) bool {
	for i := range geo.Nodes {
		if !geo.Nodes[i].Empty() {
			return true
		}
	}
	for i := range geo.Edges {
		if !geo.Edges[i].Empty() {
			return true
		}
	}
	return false
}

func movePoint(point layout.Point, dx, dy int) (layout.Point, bool) {
	x, ok := moveCoordinate(point.X, dx)
	if !ok {
		return point, false
	}
	y, ok := moveCoordinate(point.Y, dy)
	if !ok {
		return point, false
	}
	return layout.NewPoint(x, y), true
}

func moveCoordinate(value uint32, delta int) (uint32, bool) {
	return moveCoordinate64(value, int64(delta))
}

func moveCoordinate64(value uint32, delta int64) (uint32, bool) {
	switch {
	case delta < 0 && int64(value)+delta < 0:
		return value, false
	case delta > 0 && uint64(delta) > uint64(math.MaxUint32-value):
		return value, false
	default:
		return uint32(int64(value) + delta), true
	}
}

func visibleOrigin(origin, cursor uint32, size int) uint32 {
	if size <= 0 || cursor < origin {
		return cursor
	}
	size64 := uint64(size)
	if size64 > uint64(math.MaxUint32)+1 {
		return 0
	}
	if uint64(cursor) < uint64(origin)+size64 {
		return origin
	}
	return uint32(uint64(cursor) - size64 + 1)
}

func pointDistance(a, b layout.Point) uint64 {
	return uint64(max(a.X, b.X)-min(a.X, b.X)) +
		uint64(max(a.Y, b.Y)-min(a.Y, b.Y))
}

var _ tea.Model = (*Model)(nil)
