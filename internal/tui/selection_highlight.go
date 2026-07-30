package tui

import (
	"slices"

	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
)

func (m *Model) refreshSelectionHighlight() {
	highlight := m.interaction.render.selectionHighlight[:0]
	if !m.geo.Selection().Empty() {
		highlight = appendSelectionHighlight(
			highlight,
			m.geo,
			&m.canvas,
			canvasview.BaseFrame,
		)
	}
	m.interaction.render.selectionHighlight = highlight
}

func appendSelectionHighlight(
	dst []bool,
	geo *layout.Layout,
	canvas *canvasview.Model,
	frameID canvasview.FrameID,
) []bool {
	frame := canvas.Frame(frameID)
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
		hit := layout.Hit{ID: edgeID, Kind: layout.HitEdge}
		for i := 1; i < len(points); i++ {
			markHighlightSegment(func(point layout.Point) {
				owner, ok := canvas.OwnerAt(frameID, point)
				if ok && owner == hit {
					mark(point)
				}
			}, points[i-1], points[i])
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
