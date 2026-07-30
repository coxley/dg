// Package canvas owns retained diagram frames and canvas-facing styles.
package canvas

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
)

// FrameID identifies a retained canvas frame.
type FrameID uint8

const (
	BaseFrame FrameID = iota
	ConnectionFrame
	DuplicateFrame
	frameCount
)

// Styles defines drawing-canvas appearance.
type Styles struct {
	Selection lipgloss.Style
	Port      lipgloss.Style
}

// Span bounds one encoded frame row.
type Span struct {
	Start int
	End   int
}

type retainedFrame struct {
	frame render.Frame
	rows  []Span
}

// Model owns render encoders, retained frames, row indexes, and styles.
type Model struct {
	styles           Styles
	encoder          render.Encoder
	duplicateEncoder render.Encoder
	frames           [frameCount]retainedFrame
}

// New returns a canvas model.
func New(styles Styles) Model {
	return Model{styles: styles}
}

// SetStyles replaces canvas styles.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
}

// SelectionStyle returns the selection highlight style.
func (m Model) SelectionStyle() lipgloss.Style {
	return m.styles.Selection
}

// PortStyle returns the connectable-port highlight style.
func (m Model) PortStyle() lipgloss.Style {
	return m.styles.Port
}

// Render stores a complete encoded frame.
func (m *Model) Render(id FrameID, geo *layout.Layout) error {
	retained := &m.frames[id]
	encoder := &m.encoder
	if id == DuplicateFrame {
		encoder = &m.duplicateEncoder
	}
	frame, err := encoder.EncodeFrame(retained.frame.Text[:0], geo)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}
	m.retain(id, frame)
	return nil
}

// RenderWithoutEdge stores a frame excluding edgeID.
func (m *Model) RenderWithoutEdge(
	id FrameID,
	geo *layout.Layout,
	edgeID uint32,
) error {
	retained := &m.frames[id]
	frame, err := m.encoder.EncodeFrameWithoutEdge(
		retained.frame.Text[:0],
		geo,
		edgeID,
	)
	if err != nil {
		return fmt.Errorf("encode frame without edge: %w", err)
	}
	m.retain(id, frame)
	return nil
}

// RasterizeEdge rasterizes an edge with the retained primary encoder.
func (m *Model) RasterizeEdge(
	dst []layout.RasterCell,
	geo *layout.Layout,
	edge layout.RasterEdge,
) ([]layout.RasterCell, error) {
	return m.encoder.RasterizeEdge(dst, geo, edge)
}

// Clear resets a retained frame while preserving its allocations.
func (m *Model) Clear(id FrameID) {
	retained := &m.frames[id]
	retained.frame.Bounds = layout.Rect{}
	retained.frame.Text = retained.frame.Text[:0]
	retained.rows = retained.rows[:0]
}

// Frame returns a retained frame.
func (m Model) Frame(id FrameID) render.Frame {
	return m.frames[id].frame
}

// Rows returns indexed rows for a retained frame.
func (m Model) Rows(id FrameID) []Span {
	return m.frames[id].rows
}

func (m *Model) retain(id FrameID, frame render.Frame) {
	retained := &m.frames[id]
	retained.frame = frame
	retained.rows = indexRows(retained.rows, frame.Text)
}

func indexRows(dst []Span, text []byte) []Span {
	dst = dst[:0]
	start := 0
	for i, value := range text {
		if value == '\n' {
			dst = append(dst, Span{Start: start, End: i})
			start = i + 1
		}
	}
	if start < len(text) {
		dst = append(dst, Span{Start: start, End: len(text)})
	}
	return dst
}
