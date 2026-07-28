package chrome

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// HorizontalOverflow selects text behavior beyond its assigned width.
type HorizontalOverflow uint8

const (
	// WrapText wraps at display-cell boundaries.
	WrapText HorizontalOverflow = iota
	// ClipText truncates each logical line.
	ClipText
	// ScrollText preserves logical line width for a finite viewport.
	ScrollText
)

// VerticalOverflow selects content behavior beyond its assigned height.
type VerticalOverflow uint8

const (
	// ClipVertical clips rows outside the assigned height.
	ClipVertical VerticalOverflow = iota
	// ScrollVertical preserves rows for a finite viewport.
	ScrollVertical
)

// FitText applies horizontal overflow without splitting display cells.
func FitText(text string, width int, overflow HorizontalOverflow) []string {
	if width <= 0 {
		return nil
	}
	switch overflow {
	case WrapText:
		return strings.Split(ansi.Hardwrap(text, width, true), "\n")
	case ClipText:
		lines := strings.Split(text, "\n")
		for i := range lines {
			lines[i] = ansi.Truncate(lines[i], width, "")
		}
		return lines
	case ScrollText:
		return strings.Split(text, "\n")
	default:
		return nil
	}
}
