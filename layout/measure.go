package layout

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/coxley/dg/ir"
	"github.com/rivo/uniseg"
)

// LabelLine identifies one visual line as a byte range in its source label.
type LabelLine struct {
	Start uint32
	End   uint32
	Width uint32
}

// MeasureLabel returns the terminal-cell dimensions of a label.
func MeasureLabel(text string) (Size, error) {
	if text == "" {
		return Size{Width: 0, Height: 0}, nil
	}
	if uint64(len(text)) > math.MaxUint32 {
		return Size{}, errors.New("label text exceeds supported size")
	}

	for _, r := range text {
		if r != '\n' && unicode.IsControl(r) {
			return Size{}, fmt.Errorf("label contains control character %U", r)
		}
	}
	var size Size
	for line := range strings.SplitSeq(text, "\n") {
		width := uniseg.StringWidth(line)
		if uint64(width) > math.MaxUint32 {
			return Size{}, fmt.Errorf("label width %d exceeds supported size", width)
		}
		size.Width = max(size.Width, uint32(width))
		if size.Height == math.MaxUint32 {
			return Size{}, errors.New("label height exceeds supported size")
		}
		size.Height++
	}
	return size, nil
}

// AppendLabelLines appends visual label lines to dst. A positive wrapWidth
// enables Unicode line wrapping; zero preserves only explicit line breaks.
// Text must satisfy MeasureLabel.
func AppendLabelLines(dst []LabelLine, text string, wrapWidth uint32) []LabelLine {
	offset := uint32(0)
	for line := range strings.SplitSeq(text, "\n") {
		if wrapWidth == 0 {
			dst = append(dst, LabelLine{
				Start: offset,
				End:   offset + uint32(len(line)),
				Width: uint32(uniseg.StringWidth(line)),
			})
		} else {
			dst = appendWrappedLines(dst, line, offset, wrapWidth)
		}
		offset += uint32(len(line)) + 1
	}
	return dst
}

func appendWrappedLines(
	dst []LabelLine,
	text string,
	offset uint32,
	width uint32,
) []LabelLine {
	if text == "" {
		return append(dst, LabelLine{Start: offset, End: offset})
	}
	for start := 0; start < len(text); {
		graphemes := uniseg.NewGraphemes(text[start:])
		used := uint32(0)
		fitEnd := start
		breakEnd := start
		breakWidth := uint32(0)
		wrapped := false
		for graphemes.Next() {
			from, to := graphemes.Positions()
			clusterWidth := uint32(graphemes.Width())
			if used+clusterWidth > width && fitEnd != start {
				end, lineWidth := fitEnd, used
				if breakEnd != start {
					end, lineWidth = breakEnd, breakWidth
				}
				dst = append(dst, LabelLine{
					Start: offset + uint32(start),
					End:   offset + uint32(end),
					Width: lineWidth,
				})
				start = end
				wrapped = true
				break
			}
			used += clusterWidth
			fitEnd = start + to
			if graphemes.LineBreak() == uniseg.LineCanBreak {
				breakEnd = fitEnd
				breakWidth = used
			}
			if used > width && from == 0 {
				dst = append(dst, LabelLine{
					Start: offset + uint32(start),
					End:   offset + uint32(fitEnd),
					Width: used,
				})
				start = fitEnd
				wrapped = true
				break
			}
		}
		if !wrapped {
			dst = append(dst, LabelLine{
				Start: offset + uint32(start),
				End:   offset + uint32(len(text)),
				Width: used,
			})
			break
		}
	}
	return dst
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
