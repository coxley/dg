package layout

import (
	"errors"
	"fmt"
	"math"
	"unicode"

	"github.com/coxley/dg/ir"
	"github.com/rivo/uniseg"
)

var ErrMultilineLabel = errors.New("multiline label")

// MeasureLabel returns the terminal-cell dimensions of a single-line label.
func MeasureLabel(text string) (Size, error) {
	if text == "" {
		return Size{Width: 0, Height: 0}, nil
	}

	for _, r := range text {
		if r == '\n' || r == '\r' {
			return Size{}, ErrMultilineLabel
		}
		if unicode.IsControl(r) {
			return Size{}, fmt.Errorf("label contains control character %U", r)
		}
	}
	width := uniseg.StringWidth(text)
	if uint64(width) > math.MaxUint32 {
		return Size{}, fmt.Errorf("label width %d exceeds supported size", width)
	}
	return Size{Width: uint32(width), Height: 1}, nil
}

// ResolvePort maps a normalized side offset to a node boundary cell.
func ResolvePort(rect Rect, side ir.Side, offset float32) (Port, error) {
	if rect.Size.Width < 2 || rect.Size.Height < 2 {
		return Port{}, fmt.Errorf("port rectangle too small: %+v", rect.Size)
	}
	if offset < 0 || offset > 1 {
		return Port{}, fmt.Errorf("port offset %v outside [0, 1]", offset)
	}

	maxp := rect.Max()
	switch side {
	case ir.Top:
		p := Point{X: rect.Min.X + scaleOffset(offset, rect.Size.Width), Y: rect.Min.Y}
		exit := p
		if exit.Y > 0 {
			exit.Y--
		}
		return Port{Anchor: p, Exit: exit}, nil
	case ir.RightSide:
		p := Point{X: maxp.X - 1, Y: rect.Min.Y + scaleOffset(offset, rect.Size.Height)}
		return Port{Anchor: p, Exit: Point{X: p.X + 1, Y: p.Y}}, nil
	case ir.Bottom:
		p := Point{X: rect.Min.X + scaleOffset(offset, rect.Size.Width), Y: maxp.Y - 1}
		return Port{Anchor: p, Exit: Point{X: p.X, Y: p.Y + 1}}, nil
	case ir.LeftSide:
		p := Point{X: rect.Min.X, Y: rect.Min.Y + scaleOffset(offset, rect.Size.Height)}
		exit := p
		if exit.X > 0 {
			exit.X--
		}
		return Port{Anchor: p, Exit: exit}, nil
	default:
		return Port{}, fmt.Errorf("unknown port side %v", side)
	}
}

func scaleOffset(offset float32, length uint32) uint32 {
	return uint32(float64(offset)*float64(length-1) + 0.5)
}
