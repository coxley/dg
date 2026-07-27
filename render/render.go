// Package render converts cell geometry into terminal output.
package render

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/coxley/dg/layout"
	"github.com/rivo/uniseg"
)

// Frame contains encoded terminal text and its document-space bounds.
type Frame struct {
	Bounds layout.Rect
	Text   []byte
}

// Encoder owns reusable rasterization and label-placement storage.
type Encoder struct {
	grid          layout.Grid
	labels        []string
	continuations []bool
	lines         []layout.LabelLine
}

// RasterizeEdge appends transient edge cells over the most recently encoded
// frame.
func (e *Encoder) RasterizeEdge(
	dst []layout.RasterCell,
	l *layout.Layout,
	edge layout.RasterEdge,
) ([]layout.RasterCell, error) {
	return layout.RasterizeEdgeInto(dst, &e.grid, l, edge)
}

// Unicode renders layout connectivity and labels with box-drawing characters.
func Unicode(l *layout.Layout) (string, error) {
	b, err := Encode(nil, l)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Encode(dst []byte, l *layout.Layout) ([]byte, error) {
	frame, err := EncodeFrame(dst, l)
	return frame.Text, err
}

// EncodeFrame appends a rendered layout to dst and includes its document bounds.
func EncodeFrame(dst []byte, l *layout.Layout) (Frame, error) {
	if l == nil {
		return Frame{}, errors.New("nil layout")
	}
	grid, err := layout.Rasterize(l)
	if err != nil {
		return Frame{}, fmt.Errorf("rasterize layout: %w", err)
	}
	labels := make([]string, len(grid.Cells))
	continuations := make([]bool, len(grid.Cells))
	frame, _, err := encodeFrame(dst, l, grid, labels, continuations, nil)
	return frame, err
}

// EncodeFrame appends a rendered layout to dst and includes its document
// bounds. It reuses the Encoder's rasterization and label-placement storage.
func (e *Encoder) EncodeFrame(dst []byte, l *layout.Layout) (Frame, error) {
	if l == nil {
		return Frame{}, errors.New("nil layout")
	}
	grid, err := layout.RasterizeOwnedInto(e.grid.Cells, e.grid.Owners, l)
	if err != nil {
		return Frame{}, fmt.Errorf("rasterize layout: %w", err)
	}
	return e.encodeFrame(dst, l, grid)
}

// EncodeFrameWithoutEdge renders a frame while omitting edgeID.
func (e *Encoder) EncodeFrameWithoutEdge(
	dst []byte,
	l *layout.Layout,
	edgeID uint32,
) (Frame, error) {
	if l == nil {
		return Frame{}, errors.New("nil layout")
	}
	grid, err := layout.RasterizeWithoutEdgeOwnedInto(
		e.grid.Cells,
		e.grid.Owners,
		l,
		edgeID,
	)
	if err != nil {
		return Frame{}, fmt.Errorf("rasterize layout: %w", err)
	}
	return e.encodeFrame(dst, l, grid)
}

func (e *Encoder) encodeFrame(
	dst []byte,
	l *layout.Layout,
	grid layout.Grid,
) (Frame, error) {
	e.grid = grid

	e.labels = slices.Grow(e.labels[:0], len(grid.Cells))[:len(grid.Cells)]
	e.continuations = slices.Grow(
		e.continuations[:0],
		len(grid.Cells),
	)[:len(grid.Cells)]
	clear(e.labels)
	clear(e.continuations)
	frame, lines, err := encodeFrame(
		dst,
		l,
		e.grid,
		e.labels,
		e.continuations,
		e.lines,
	)
	e.lines = lines
	return frame, err
}

func encodeFrame(
	dst []byte,
	l *layout.Layout,
	grid layout.Grid,
	labels []string,
	continuations []bool,
	lines []layout.LabelLine,
) (Frame, []layout.LabelLine, error) {
	for i := range l.Nodes {
		if l.Nodes[i].Empty() {
			continue
		}
		nodeID := uint32(i)
		bounds := l.LabelBounds(nodeID)
		wrapWidth := uint32(0)
		if _, explicit := l.ExplicitNodeSize(nodeID); explicit {
			wrapWidth = bounds.Size.Width
		}
		lines = layout.AppendLabelLines(
			lines[:0],
			l.Label(nodeID),
			wrapWidth,
		)
		for lineID, line := range lines[:min(len(lines), int(bounds.Size.Height))] {
			if err := placeLabelLine(
				&grid,
				labels,
				continuations,
				bounds.Min.Add(0, uint32(lineID)),
				l.Label(nodeID)[line.Start:line.End],
				bounds.Size.Width,
				layout.Hit{ID: nodeID, Kind: layout.HitNode},
			); err != nil {
				return Frame{}, lines, fmt.Errorf(
					"place node %d label line %d: %w",
					i,
					lineID,
					err,
				)
			}
		}
	}

	width := int(grid.Bounds.Size.Width)
	height := int(grid.Bounds.Size.Height)

	buf := bytes.NewBuffer(dst)
	buf.Grow((width + 1) * height)
	for y := range height {
		row := y * width
		for x := range width {
			index := row + x
			switch {
			case labels[index] != "":
				buf.WriteString(labels[index])
			case continuations[index]:
			default:
				buf.WriteRune(Glyph(grid.Cells[index]))
			}
		}
		buf.WriteRune('\n')
	}
	return Frame{Bounds: grid.Bounds, Text: buf.Bytes()}, lines, nil
}

func placeLabelLine(
	grid *layout.Grid,
	labels []string,
	continuations []bool,
	origin layout.Point,
	text string,
	maxWidth uint32,
	owner layout.Hit,
) error {
	x := origin.X
	used := uint32(0)
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		value := graphemes.Str()
		width := uniseg.StringWidth(value)
		if width == 0 {
			if x == origin.X {
				return errors.New("label starts with zero-width grapheme")
			}
			continue
		}
		if uint64(used)+uint64(width) > uint64(maxWidth) {
			break
		}
		point := layout.Point{X: x, Y: origin.Y}
		index, ok := grid.Index(point)
		if !ok {
			return fmt.Errorf("label cell %+v outside grid", point)
		}
		visible := grid.Owners[index] == owner
		for offset := 1; offset < width; offset++ {
			continuationX := x + uint32(offset)
			continuation, ok := grid.Index(layout.Point{X: continuationX, Y: origin.Y})
			if !ok {
				return fmt.Errorf("label cell at x=%d outside grid", continuationX)
			}
			visible = visible && grid.Owners[continuation] == owner
		}
		if !visible {
			x += uint32(width)
			used += uint32(width)
			continue
		}
		labels[index] = value
		for offset := 1; offset < width; offset++ {
			continuation, _ := grid.Index(layout.Point{
				X: x + uint32(offset),
				Y: origin.Y,
			})
			continuations[continuation] = true
		}
		x += uint32(width)
		used += uint32(width)
	}
	return nil
}

// Glyph returns the box-drawing rune for connections.
func Glyph(connections layout.Connections) rune {
	switch connections {
	case 0:
		return ' '
	case layout.North:
		return '╵'
	case layout.East:
		return '╶'
	case layout.South:
		return '╷'
	case layout.West:
		return '╴'
	case layout.North | layout.East:
		return '└'
	case layout.North | layout.South:
		return '│'
	case layout.North | layout.West:
		return '┘'
	case layout.East | layout.South:
		return '┌'
	case layout.East | layout.West:
		return '─'
	case layout.South | layout.West:
		return '┐'
	case layout.North | layout.East | layout.South:
		return '├'
	case layout.North | layout.East | layout.West:
		return '┴'
	case layout.North | layout.South | layout.West:
		return '┤'
	case layout.East | layout.South | layout.West:
		return '┬'
	case layout.North | layout.East | layout.South | layout.West:
		return '┼'
	default:
		panic(fmt.Sprintf("unknown connections %04b", connections))
	}
}
