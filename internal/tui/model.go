// Package tui provides an interactive terminal editor for diagram layouts.
package tui

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
	"github.com/rivo/uniseg"
)

type mode uint8

const (
	modeNavigate mode = iota
	modeMove
	modeEditLabel
)

func (m mode) String() string {
	switch m {
	case modeNavigate:
		return "navigate"
	case modeMove:
		return "move"
	case modeEditLabel:
		return "edit label"
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

	cursor   layout.Point
	viewport layout.Point
	hits     []layout.Hit
	active   int
	target   layout.Hit

	mode       mode
	editBuffer []byte
	frame      render.Frame
	frameRows  []rowSpan
	viewBuffer []byte

	width  int
	height int
	status string
}

// New returns a TUI model for geo.
func New(geo *layout.Layout) (*Model, error) {
	if geo == nil {
		return nil, errors.New("nil layout")
	}
	m := &Model{geo: geo}
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

// Run starts an interactive terminal editor for geo.
func Run(geo *layout.Layout) error {
	model, err := New(geo)
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
		if message.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.mode == modeEditLabel {
			m.updateLabel(message)
		} else {
			if message.String() == "q" {
				return m, tea.Quit
			}
			m.updateCommand(message.String())
		}
	case tea.PasteMsg:
		if m.mode == modeEditLabel {
			m.appendLabelText(message.Content)
		}
	}
	return m, nil
}

func (m *Model) updateCommand(key string) {
	switch key {
	case "up", "k":
		m.move(0, -1)
	case "right", "l":
		m.move(1, 0)
	case "down", "j":
		m.move(0, 1)
	case "left", "h":
		m.move(-1, 0)
	case "tab":
		m.cycleHit(1)
	case "shift+tab":
		m.cycleHit(-1)
	case "enter", "m":
		m.beginMove()
	case "e":
		m.beginLabelEdit()
	case "d", "delete":
		m.deleteActive()
	case "esc":
		m.mode = modeNavigate
		m.status = ""
	}
}

func (m *Model) updateLabel(key tea.KeyPressMsg) {
	switch key.String() {
	case "esc":
		m.editBuffer = m.editBuffer[:0]
		m.mode = modeNavigate
		m.status = ""
	case "enter":
		label := string(m.editBuffer)
		if err := m.geo.SetNodeLabel(m.target.ID, label); err != nil {
			m.status = err.Error()
			return
		}
		if err := m.rebuild(); err != nil {
			m.status = err.Error()
			return
		}
		m.editBuffer = m.editBuffer[:0]
		m.mode = modeNavigate
		m.refreshHits()
		m.status = ""
	case "backspace":
		m.deleteLastGrapheme()
	default:
		m.appendLabelText(key.Key().Text)
	}
}

func (m *Model) appendLabelText(text string) {
	if text == "" {
		return
	}
	if strings.ContainsAny(text, "\r\n") {
		m.status = "labels currently support one line"
		return
	}
	m.editBuffer = append(m.editBuffer, text...)
	m.status = ""
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
	if err := m.geo.PlaceNode(m.target.ID, origin); err != nil {
		m.status = err.Error()
		return
	}
	m.cursor = cursor
	if err := m.rebuild(); err != nil {
		m.status = err.Error()
		return
	}
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) beginMove() {
	if m.mode == modeMove {
		m.mode = modeNavigate
		m.status = ""
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
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitNode {
		m.status = "select a node to edit"
		return
	}
	m.target = hit
	m.editBuffer = append(m.editBuffer[:0], m.geo.Label(hit.ID)...)
	m.mode = modeEditLabel
	m.status = ""
}

func (m *Model) deleteActive() {
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
	m.refreshHits()
	m.status = ""
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
	frame, err := render.EncodeFrame(m.frame.Text[:0], m.geo)
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

func (m *Model) deleteLastGrapheme() {
	remaining := m.editBuffer
	start := 0
	offset := 0
	state := -1
	for len(remaining) != 0 {
		start = offset
		cluster, rest, _, nextState := uniseg.FirstGraphemeCluster(remaining, state)
		offset += len(cluster)
		remaining, state = rest, nextState
	}
	m.editBuffer = m.editBuffer[:start]
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

var _ tea.Model = (*Model)(nil)
