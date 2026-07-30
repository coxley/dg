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
	symbols       []rune
	endpoints     []layout.Connections
	continuations []bool
	lines         []layout.LabelLine
}

// OwnerAt returns the topmost object in the most recently encoded frame.
func (e *Encoder) OwnerAt(point layout.Point) (layout.Hit, bool) {
	return e.grid.OwnerAt(point)
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
	symbols := make([]rune, len(grid.Cells))
	endpoints := make([]layout.Connections, len(grid.Cells))
	continuations := make([]bool, len(grid.Cells))
	frame, _, err := encodeFrame(
		dst,
		l,
		grid,
		labels,
		symbols,
		endpoints,
		continuations,
		nil,
	)
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
	e.symbols = slices.Grow(e.symbols[:0], len(grid.Cells))[:len(grid.Cells)]
	e.endpoints = slices.Grow(
		e.endpoints[:0],
		len(grid.Cells),
	)[:len(grid.Cells)]
	e.continuations = slices.Grow(
		e.continuations[:0],
		len(grid.Cells),
	)[:len(grid.Cells)]
	clear(e.labels)
	clear(e.symbols)
	clear(e.endpoints)
	clear(e.continuations)
	frame, lines, err := encodeFrame(
		dst,
		l,
		e.grid,
		e.labels,
		e.symbols,
		e.endpoints,
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
	symbols []rune,
	endpoints []layout.Connections,
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
		visible := lines[:min(len(lines), int(bounds.Size.Height))]
		for lineID, line := range visible {
			label := l.Label(nodeID)[line.Start:line.End]
			lineWidth := uint32(uniseg.StringWidth(label))
			point, _ := l.LabelLinePoint(
				nodeID,
				uint32(lineID),
				uint32(len(visible)),
				lineWidth,
			)
			if err := placeLabelLine(
				&grid,
				labels,
				continuations,
				point,
				label,
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
	for edgeID := range l.Edges {
		if l.Edges[edgeID].Empty() {
			continue
		}
		if err := placeEdgeArrows(
			&grid,
			symbols,
			endpoints,
			l,
			uint32(edgeID),
		); err != nil {
			return Frame{}, lines, fmt.Errorf(
				"place edge %d arrows: %w",
				edgeID,
				err,
			)
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
			case symbols[index] != 0:
				buf.WriteRune(symbols[index])
			case continuations[index]:
			default:
				buf.WriteRune(cellGlyph(l, grid, endpoints, index))
			}
		}
		buf.WriteRune('\n')
	}
	return Frame{Bounds: grid.Bounds, Text: buf.Bytes()}, lines, nil
}

func placeEdgeArrows(
	grid *layout.Grid,
	glyphs []rune,
	endpoints []layout.Connections,
	l *layout.Layout,
	edgeID uint32,
) error {
	style, ok := l.EdgeStyle(edgeID)
	if !ok {
		return nil
	}
	points := l.Edges[edgeID].Points
	portA, portB, err := l.EdgePorts(edgeID)
	if err != nil {
		return err
	}
	for _, endpoint := range [...]struct {
		style  layout.ArrowStyle
		anchor layout.Point
	}{
		{style: style.PortAArrow, anchor: l.Ports[portA].Anchor},
		{style: style.PortBArrow, anchor: l.Ports[portB].Anchor},
	} {
		point, ok := edgeArrowPointAt(points, endpoint.anchor)
		if !ok {
			continue
		}
		anchorIndex, ok := grid.Index(endpoint.anchor)
		if !ok {
			return fmt.Errorf("arrow anchor %+v outside grid", endpoint.anchor)
		}
		direction := connectionToward(endpoint.anchor, point)
		if endpoint.style == layout.ArrowNone {
			continue
		}
		index, ok := grid.Index(point)
		if !ok {
			return fmt.Errorf("arrow cell %+v outside grid", point)
		}
		if grid.Owners[index] != (layout.Hit{ID: edgeID, Kind: layout.HitEdge}) {
			continue
		}
		endpoints[anchorIndex] |= direction
		glyphs[index] = arrowGlyph(endpoint.style, point, endpoint.anchor)
	}
	return nil
}

func edgeArrowPointAt(
	points []layout.Point,
	anchor layout.Point,
) (layout.Point, bool) {
	switch {
	case len(points) < 2:
		return layout.Point{}, false
	case points[0] == anchor:
		point, _, ok := edgeArrowPoint(points, true)
		return point, ok
	case points[len(points)-1] == anchor:
		point, _, ok := edgeArrowPoint(points, false)
		return point, ok
	default:
		return layout.Point{}, false
	}
}

func edgeArrowPoint(
	points []layout.Point,
	start bool,
) (layout.Point, layout.Point, bool) {
	if len(points) < 2 {
		return layout.Point{}, layout.Point{}, false
	}
	anchor := points[0]
	if !start {
		anchor = points[len(points)-1]
	}
	point, ok := pathPoint(points, start, 1)
	return point, anchor, ok
}

func pathPoint(points []layout.Point, start bool, distance uint32) (layout.Point, bool) {
	remaining := distance
	for step := 1; step < len(points); step++ {
		fromIndex, toIndex := step-1, step
		if !start {
			fromIndex = len(points) - step
			toIndex = fromIndex - 1
		}
		from, to := points[fromIndex], points[toIndex]
		length := absDiff(from.X, to.X) + absDiff(from.Y, to.Y)
		if remaining > length {
			remaining -= length
			continue
		}
		switch {
		case to.X < from.X:
			from.X -= remaining
		case to.X > from.X:
			from.X += remaining
		case to.Y < from.Y:
			from.Y -= remaining
		default:
			from.Y += remaining
		}
		return from, true
	}
	return layout.Point{}, false
}

func absDiff(a, b uint32) uint32 {
	if a < b {
		return b - a
	}
	return a - b
}

func arrowGlyph(
	style layout.ArrowStyle,
	point, anchor layout.Point,
) rune {
	switch {
	case anchor.Y < point.Y:
		if style == layout.ArrowFilled {
			return '▲'
		}
		return '△'
	case anchor.X > point.X:
		if style == layout.ArrowFilled {
			return '▶'
		}
		return '▷'
	case anchor.Y > point.Y:
		if style == layout.ArrowFilled {
			return '▼'
		}
		return '▽'
	default:
		if style == layout.ArrowFilled {
			return '◀'
		}
		return '◁'
	}
}

// ArrowGlyphAt returns the endpoint marker drawn at point.
func ArrowGlyphAt(
	points []layout.Point,
	style layout.EdgeStyle,
	point layout.Point,
) (rune, bool) {
	for _, endpoint := range [...]struct {
		style layout.ArrowStyle
		start bool
	}{
		{style: style.PortAArrow, start: true},
		{style: style.PortBArrow},
	} {
		if endpoint.style == layout.ArrowNone {
			continue
		}
		arrow, anchor, ok := edgeArrowPoint(points, endpoint.start)
		if ok && arrow == point {
			return arrowGlyph(endpoint.style, arrow, anchor), true
		}
	}
	return 0, false
}

func cellGlyph(
	l *layout.Layout,
	grid layout.Grid,
	endpoints []layout.Connections,
	index int,
) rune {
	connections := grid.Cells[index] &^ endpoints[index]
	owner := grid.Owners[index]
	switch owner.Kind {
	case layout.HitEdge:
		style, ok := l.EdgeStyle(owner.ID)
		if ok {
			return StrokeGlyph(connections, style.Stroke)
		}
	case layout.HitNode:
		style, ok := l.NodeStyle(owner.ID)
		if !ok {
			break
		}
		if style.Stroke == layout.StrokeDashed {
			return StrokeGlyph(connections, style.Stroke)
		}
		switch style.Border {
		case layout.BorderRounded:
			return roundedGlyph(connections)
		case layout.BorderDouble:
			return doubleGlyph(connections)
		case layout.BorderSolid, layout.BorderNone:
		}
	case layout.HitPort:
	}
	return Glyph(connections)
}

func roundedGlyph(connections layout.Connections) rune {
	switch connections {
	case layout.North | layout.East:
		return '╰'
	case layout.East | layout.South:
		return '╭'
	case layout.South | layout.West:
		return '╮'
	case layout.North | layout.West:
		return '╯'
	default:
		return Glyph(connections)
	}
}

func doubleGlyph(connections layout.Connections) rune {
	switch connections {
	case layout.North | layout.East:
		return '╚'
	case layout.North | layout.South:
		return '║'
	case layout.North | layout.West:
		return '╝'
	case layout.East | layout.South:
		return '╔'
	case layout.East | layout.West:
		return '═'
	case layout.South | layout.West:
		return '╗'
	case layout.North | layout.East | layout.South:
		return '╠'
	case layout.North | layout.East | layout.West:
		return '╩'
	case layout.North | layout.South | layout.West:
		return '╣'
	case layout.East | layout.South | layout.West:
		return '╦'
	case layout.North | layout.East | layout.South | layout.West:
		return '╬'
	default:
		return Glyph(connections)
	}
}

func connectionToward(from, to layout.Point) layout.Connections {
	switch {
	case to.Y < from.Y:
		return layout.North
	case to.X > from.X:
		return layout.East
	case to.Y > from.Y:
		return layout.South
	default:
		return layout.West
	}
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

// StrokeGlyph returns the glyph for connections using stroke.
func StrokeGlyph(
	connections layout.Connections,
	stroke layout.StrokeStyle,
) rune {
	if stroke == layout.StrokeDashed {
		switch connections {
		case layout.North | layout.South:
			return '╎'
		case layout.East | layout.West:
			return '╌'
		case layout.North, layout.East, layout.South, layout.West:
		}
	}
	return Glyph(connections)
}
