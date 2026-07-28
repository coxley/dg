// Package chrome implements declarative terminal chrome mechanics.
package chrome

import (
	"errors"
	"fmt"
)

// ID identifies an arranged element across resize and reflow.
type ID string

// Point identifies a terminal cell.
type Point struct {
	X int
	Y int
}

// Size describes a width and height in terminal cells.
type Size struct {
	Width  int
	Height int
}

// Rect describes a terminal-cell rectangle.
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// Right returns the first column after r.
func (r Rect) Right() int {
	return r.X + r.Width
}

// Bottom returns the first row after r.
func (r Rect) Bottom() int {
	return r.Y + r.Height
}

// Contains reports whether r contains p.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.X < r.Right() &&
		p.Y >= r.Y && p.Y < r.Bottom()
}

// Inset returns r reduced by insets without producing negative dimensions.
func (r Rect) Inset(insets Insets) Rect {
	x := r.X + insets.Left
	y := r.Y + insets.Top
	return Rect{
		X:      x,
		Y:      y,
		Width:  max(r.Width-insets.Horizontal(), 0),
		Height: max(r.Height-insets.Vertical(), 0),
	}
}

// Insets describes space inside each edge of a rectangle.
type Insets struct {
	Top    int
	Right  int
	Bottom int
	Left   int
}

// Horizontal returns the total horizontal inset.
func (i Insets) Horizontal() int {
	return i.Left + i.Right
}

// Vertical returns the total vertical inset.
func (i Insets) Vertical() int {
	return i.Top + i.Bottom
}

// Constraints bounds measured dimensions.
type Constraints struct {
	Min Size
	Max Size
}

// Tight returns constraints fixed to size.
func Tight(size Size) Constraints {
	size = size.nonNegative()
	return Constraints{Min: size, Max: size}
}

func (c Constraints) normalize() Constraints {
	c.Min = c.Min.nonNegative()
	c.Max = c.Max.nonNegative()
	c.Min.Width = min(c.Min.Width, c.Max.Width)
	c.Min.Height = min(c.Min.Height, c.Max.Height)
	return c
}

func (c Constraints) constrain(size Size) Size {
	c = c.normalize()
	return Size{
		Width:  min(max(size.Width, c.Min.Width), c.Max.Width),
		Height: min(max(size.Height, c.Min.Height), c.Max.Height),
	}
}

func (s Size) nonNegative() Size {
	return Size{Width: max(s.Width, 0), Height: max(s.Height, 0)}
}

// Metrics contains the minimum and preferred size of an element.
type Metrics struct {
	Min       Size
	Preferred Size
}

// Density selects global spacing tokens.
type Density uint8

const (
	// Regular uses standard spacing.
	Regular Density = iota
	// Compact uses reduced spacing.
	Compact
)

// Environment contains layout inputs shared by a tree.
type Environment struct {
	Size    Size
	Density Density
}

// Arrangement records one element's final rectangle.
type Arrangement struct {
	ID   ID
	Rect Rect
}

// ErrDuplicateID reports an invalid declarative tree.
var ErrDuplicateID = errors.New("duplicate arranged ID")

// DuplicateIDError identifies the duplicate semantic ID.
type DuplicateIDError struct {
	ID ID
}

func (e DuplicateIDError) Error() string {
	return fmt.Sprintf("%v: %q", ErrDuplicateID, e.ID)
}

func (e DuplicateIDError) Unwrap() error {
	return ErrDuplicateID
}

// Plan is one immutable arrangement used by render and input.
type Plan struct {
	Version uint64
	Bounds  Rect
	entries []Arrangement
	index   map[ID]int
}

// Entries returns a copy of the arranged elements in stable tree order.
func (p Plan) Entries() []Arrangement {
	return append([]Arrangement(nil), p.entries...)
}

// Rect returns the arranged rectangle for id.
func (p Plan) Rect(id ID) (Rect, bool) {
	i, ok := p.index[id]
	if !ok {
		return Rect{}, false
	}
	return p.entries[i].Rect, true
}

func newPlan(version uint64, bounds Rect, entries []Arrangement) (Plan, error) {
	index := make(map[ID]int, len(entries))
	for i, entry := range entries {
		if _, exists := index[entry.ID]; exists {
			return Plan{}, DuplicateIDError{ID: entry.ID}
		}
		index[entry.ID] = i
	}
	return Plan{
		Version: version,
		Bounds:  bounds,
		entries: entries,
		index:   index,
	}, nil
}
