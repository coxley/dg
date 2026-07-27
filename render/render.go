// Package render converts cell geometry into terminal output.
package render

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/coxley/dg/layout"
	"github.com/rivo/uniseg"
)

// Unicode renders layout connectivity and labels with box-drawing characters.
func Unicode(l *layout.Layout) (string, error) {
	b, err := Encode(nil, l)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Encode(dst []byte, l *layout.Layout) ([]byte, error) {
	if l == nil {
		return nil, errors.New("nil layout")
	}
	grid, err := layout.Rasterize(l)
	if err != nil {
		return nil, fmt.Errorf("rasterize layout: %w", err)
	}

	labels := make([]string, len(grid.Cells))
	continuations := make([]bool, len(grid.Cells))
	for i := range l.Nodes {
		if l.Nodes[i].Empty() {
			continue
		}
		if err := placeLabel(&grid, labels, continuations, l.Nodes[i].LabelPoint, l.Label(uint32(i))); err != nil {
			return nil, fmt.Errorf("place node %d label: %w", i, err)
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
				buf.WriteRune(glyph(grid.Cells[index]))
			}
		}
		buf.WriteRune('\n')
	}
	return buf.Bytes(), nil
}

func placeLabel(
	grid *layout.Grid,
	labels []string,
	continuations []bool,
	origin layout.Point,
	text string,
) error {
	x := origin.X
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
		point := layout.Point{X: x, Y: origin.Y}
		index, ok := cellIndex(grid.Bounds, point)
		if !ok {
			return fmt.Errorf("label cell %+v outside grid", point)
		}
		labels[index] = value
		for offset := 1; offset < width; offset++ {
			continuationX := x + uint32(offset)
			continuation, ok := cellIndex(grid.Bounds, layout.Point{X: continuationX, Y: origin.Y})
			if !ok {
				return fmt.Errorf("label cell at x=%d outside grid", continuationX)
			}
			continuations[continuation] = true
		}
		x += uint32(width)
	}
	return nil
}

func cellIndex(bounds layout.Rect, point layout.Point) (int, bool) {
	if !bounds.Contains(point) {
		return 0, false
	}
	x := point.X - bounds.Min.X
	y := point.Y - bounds.Min.Y
	return int(y)*int(bounds.Size.Width) + int(x), true
}

func glyph(connections layout.Connections) rune {
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
