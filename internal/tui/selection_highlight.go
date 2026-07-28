package tui

import (
	"slices"

	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
)

func appendSelectionHighlight(
	dst []bool,
	geo *layout.Layout,
	frame render.Frame,
) []bool {
	bounds := frame.Bounds
	cellCount := int(bounds.Size.Width) * int(bounds.Size.Height)
	dst = slices.Grow(dst[:0], cellCount)[:cellCount]
	clear(dst)
	mark := func(point layout.Point) {
		if !bounds.Contains(point) {
			return
		}
		x := int(point.X - bounds.Min.X)
		y := int(point.Y - bounds.Min.Y)
		dst[y*int(bounds.Size.Width)+x] = true
	}
	for nodeID := range geo.Selection().Nodes() {
		rect := geo.Nodes[nodeID].Rect
		limit := rect.Max()
		for x := rect.Min.X; x < limit.X; x++ {
			mark(layout.NewPoint(x, rect.Min.Y))
			mark(layout.NewPoint(x, limit.Y-1))
		}
		for y := rect.Min.Y; y < limit.Y; y++ {
			mark(layout.NewPoint(rect.Min.X, y))
			mark(layout.NewPoint(limit.X-1, y))
		}
	}
	for edgeID := range geo.Selection().Edges() {
		points := geo.Edges[edgeID].Points
		for i := 1; i < len(points); i++ {
			markHighlightSegment(mark, points[i-1], points[i])
		}
	}
	return dst
}

func markHighlightSegment(mark func(layout.Point), a, b layout.Point) {
	switch {
	case a.X == b.X:
		end := max(a.Y, b.Y)
		for y := min(a.Y, b.Y); ; y++ {
			mark(layout.NewPoint(a.X, y))
			if y == end {
				break
			}
		}
	case a.Y == b.Y:
		end := max(a.X, b.X)
		for x := min(a.X, b.X); ; x++ {
			mark(layout.NewPoint(x, a.Y))
			if x == end {
				break
			}
		}
	}
}

func highlightContains(
	highlight []bool,
	frame render.Frame,
	point layout.Point,
) bool {
	bounds := frame.Bounds
	if !bounds.Contains(point) {
		return false
	}
	x := int(point.X - bounds.Min.X)
	y := int(point.Y - bounds.Min.Y)
	index := y*int(bounds.Size.Width) + x
	return index < len(highlight) && highlight[index]
}
