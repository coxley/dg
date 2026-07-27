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

const (
	modeNavigate mode = iota
	modeMove
	modeEditLabel
	modeConnect
	modeSavePath
)

const finishOperation = "finish the current operation first"

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
	geo *layout.Layout

	cursor         layout.Point
	viewport       layout.Point
	hits           []layout.Hit
	active         int
	target         layout.Hit
	connectSource  uint32
	connectEdge    uint32
	connectOldPort uint32
	reconnecting   bool

	mode         mode
	editBuffer   []byte
	editDraft    []byte
	editOriginal string
	editCaret    int
	editCreated  bool

	dragging      bool
	dragOffset    layout.Point
	editMouseDown bool
	lastClick     layout.Point
	hasLastClick  bool

	frame      render.Frame
	encoder    render.Encoder
	frameRows  []rowSpan
	viewBuffer []byte
	statusText []byte

	// Bubble Tea compares consecutive cursor pointers, so each View writes the
	// cursor value that the previous View does not reference.
	viewCursor [2]tea.Cursor
	nextCursor uint8

	width  int
	height int
	status string
	path   string

	saveHint string
}

// New returns a TUI model for geo.
func New(geo *layout.Layout) (*Model, error) {
	return newModel(geo, "")
}

func newModel(geo *layout.Layout, path string) (*Model, error) {
	if geo == nil {
		return nil, errors.New("nil layout")
	}
	m := &Model{geo: geo, path: path}
	m.viewCursor[0] = *tea.NewCursor(0, 0)
	m.viewCursor[1] = m.viewCursor[0]
	for i := range geo.Nodes {
		if !geo.Nodes[i].Empty() {
			m.cursor = geo.Nodes[i].LabelPoint
			break
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
			return m, tea.Quit
		}
		m.hasLastClick = false
		if m.mode == modeSavePath {
			m.updateSavePath(message)
		} else if key.Code == 's' && key.Mod == tea.ModCtrl {
			m.requestSave()
		} else if m.mode == modeEditLabel {
			m.updateLabel(message)
		} else {
			if key.Code == 'q' && key.Mod == 0 {
				return m, tea.Quit
			}
			m.updateCommand(key)
		}
	case tea.PasteMsg:
		switch m.mode {
		case modeEditLabel:
			m.insertLabelText(message.Content)
		case modeSavePath:
			m.insertSavePathText(message.Content)
		default:
		}
	case tea.MouseClickMsg:
		if m.mode != modeSavePath {
			m.updateMouseClick(message.Mouse())
		}
	case tea.MouseReleaseMsg:
		if m.mode != modeSavePath {
			m.dragging = false
			m.editMouseDown = false
		}
	case tea.MouseMotionMsg:
		if m.mode != modeSavePath {
			m.updateMouseMotion(message.Mouse())
		}
	case tea.MouseWheelMsg:
		if m.mode != modeSavePath {
			m.updateMouseWheel(message.Mouse())
		}
	}
	return m, nil
}

func (m *Model) updateCommand(key tea.Key) {
	if key.Code == tea.KeyTab && key.Mod == tea.ModShift {
		m.cycleHit(-1)
		return
	}
	if key.Mod != 0 {
		return
	}
	switch key.Code {
	case tea.KeyUp, 'k':
		m.move(0, -1)
	case tea.KeyRight, 'l':
		m.move(1, 0)
	case tea.KeyDown, 'j':
		m.move(0, 1)
	case tea.KeyLeft, 'h':
		m.move(-1, 0)
	case tea.KeyTab:
		m.cycleHit(1)
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
	case 'c':
		if m.mode == modeConnect {
			m.completeConnection()
		} else {
			m.beginConnection()
		}
	case 'd', tea.KeyDelete:
		m.deleteActive()
	case tea.KeyEscape:
		m.cancelMode()
	}
}

func (m *Model) move(dx, dy int) {
	if m.mode == modeMove {
		m.moveNode(dx, dy)
		return
	}
	point, ok := movePoint(m.cursor, dx, dy)
	if !ok {
		return
	}
	m.cursor = point
	m.refreshHits()
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) moveNode(dx, dy int) {
	if m.target.Kind != layout.HitNode || !m.geo.NodeExists(m.target.ID) {
		m.mode = modeNavigate
		m.status = "selected node no longer exists"
		return
	}
	origin, ok := movePoint(m.geo.Nodes[m.target.ID].Rect.Min, dx, dy)
	if !ok {
		return
	}
	cursor, ok := movePoint(m.cursor, dx, dy)
	if !ok {
		return
	}
	m.placeNode(m.target.ID, origin, cursor)
}

func (m *Model) placeNode(nodeID uint32, origin, cursor layout.Point) {
	if err := m.geo.PlaceNode(nodeID, origin); err != nil {
		m.status = err.Error()
		return
	}
	if err := m.rebuild(); err != nil {
		m.status = err.Error()
		return
	}
	m.cursor = cursor
	m.target = layout.Hit{ID: nodeID, Kind: layout.HitNode}
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) beginMove() {
	if m.mode == modeMove {
		m.mode = modeNavigate
		m.dragging = false
		m.status = ""
		return
	}
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitNode {
		m.status = "select a node to move"
		return
	}
	m.target = hit
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
	m.startLabelEdit(hit, false)
}

func (m *Model) newNode() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	nodeID, err := m.geo.NewNodeAt("", m.cursor)
	if err != nil {
		m.status = err.Error()
		return
	}
	if err := m.rebuild(); err != nil {
		_ = m.geo.DeleteNode(nodeID)
		_ = m.rebuild()
		m.status = err.Error()
		return
	}
	m.startLabelEdit(layout.Hit{ID: nodeID, Kind: layout.HitNode}, true)
}

func (m *Model) beginConnection() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	hit, ok := m.activeHit()
	if !ok {
		m.status = "select a port or edge"
		return
	}
	switch hit.Kind {
	case layout.HitPort:
		m.connectSource = hit.ID
		m.reconnecting = false
	case layout.HitEdge:
		portA, portB, err := m.geo.EdgePorts(hit.ID)
		if err != nil {
			m.status = err.Error()
			return
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
		m.status = "select a port or edge"
		return
	}
	m.mode = modeConnect
	m.status = ""
}

func (m *Model) completeConnection() {
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitPort {
		m.status = "select a destination port"
		return
	}
	edgeID := m.connectEdge
	var err error
	if m.reconnecting {
		err = m.geo.ReconnectEdge(edgeID, m.connectOldPort, hit.ID)
	} else {
		edgeID, err = m.geo.ConnectPorts(m.connectSource, hit.ID)
	}
	if err != nil {
		m.status = err.Error()
		return
	}
	if err := m.rebuild(); err != nil {
		m.status = err.Error()
		return
	}
	m.mode = modeNavigate
	m.target = layout.Hit{ID: edgeID, Kind: layout.HitEdge}
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
	hit, ok := m.activeHit()
	if !ok {
		m.status = "nothing selected"
		return
	}
	var err error
	switch hit.Kind {
	case layout.HitNode:
		err = m.geo.DeleteNode(hit.ID)
	case layout.HitEdge:
		err = m.geo.DeleteEdge(hit.ID)
	case layout.HitPort:
		m.status = "ports cannot be deleted independently"
		return
	}
	if err != nil {
		m.status = err.Error()
		return
	}
	if err := m.rebuild(); err != nil {
		m.status = err.Error()
		return
	}
	m.mode = modeNavigate
	m.target = layout.Hit{}
	m.dragging = false
	m.refreshHits()
	m.status = ""
}

func (m *Model) cancelMode() {
	m.mode = modeNavigate
	m.clearConnection()
	m.dragging = false
	m.status = ""
}

func (m *Model) clearConnection() {
	m.connectSource = 0
	m.connectEdge = 0
	m.connectOldPort = 0
	m.reconnecting = false
}

func (m *Model) rebuild() error {
	if err := m.geo.Build(); err != nil {
		return fmt.Errorf("build layout: %w", err)
	}
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
	m.indexFrameRows()
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
	return max(m.height-2, 0)
}

func (m *Model) indexFrameRows() {
	m.frameRows = m.frameRows[:0]
	start := 0
	for i, value := range m.frame.Text {
		if value == '\n' {
			m.frameRows = append(m.frameRows, rowSpan{start: start, end: i})
			start = i + 1
		}
	}
	if start < len(m.frame.Text) {
		m.frameRows = append(m.frameRows, rowSpan{start: start, end: len(m.frame.Text)})
	}
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
	switch {
	case delta < 0 && value < uint32(-delta):
		return value, false
	case delta > 0 && value > math.MaxUint32-uint32(delta):
		return value, false
	default:
		return uint32(int64(value) + int64(delta)), true
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
