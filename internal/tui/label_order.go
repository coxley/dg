package tui

import (
	"cmp"
	"math"
	"slices"

	"github.com/coxley/dg/layout"
)

type labelOrderNode struct {
	id     uint32
	rect   layout.Rect
	labelY uint32
	row    int
}

type labelOrderRow struct {
	id     int
	top    uint32
	bottom uint32
}

func orderLabelTargets(geo *layout.Layout, targets []uint32) {
	nodes := make([]labelOrderNode, len(targets))
	for i, nodeID := range targets {
		nodes[i] = labelOrderNode{
			id:     nodeID,
			rect:   geo.Nodes[nodeID].Rect,
			labelY: renderedLabelY(geo, nodeID),
		}
	}

	// Short nodes establish rows before tall nodes choose among every row they span.
	slices.SortFunc(nodes, func(a, b labelOrderNode) int {
		if order := cmp.Compare(a.rect.Size.Height, b.rect.Size.Height); order != 0 {
			return order
		}
		if order := cmp.Compare(a.rect.Min.Y, b.rect.Min.Y); order != 0 {
			return order
		}
		if order := cmp.Compare(a.rect.Min.X, b.rect.Min.X); order != 0 {
			return order
		}
		return cmp.Compare(a.id, b.id)
	})

	rows := make([]labelOrderRow, 0, len(nodes))
	for i := range nodes {
		node := &nodes[i]
		limit := node.rect.Max()
		best := -1
		bestDistance := uint64(math.MaxUint64)
		for j := range rows {
			row := rows[j]
			if node.rect.Min.Y >= row.bottom || limit.Y <= row.top {
				continue
			}
			distance := labelRowDistance(node.labelY, row.top, row.bottom)
			if distance < bestDistance ||
				distance == bestDistance && (best < 0 || row.top < rows[best].top) {
				best = j
				bestDistance = distance
			}
		}
		if best < 0 {
			node.row = len(rows)
			rows = append(rows, labelOrderRow{
				id: node.row, top: node.rect.Min.Y, bottom: limit.Y,
			})
			continue
		}
		node.row = rows[best].id
		rows[best].top = max(rows[best].top, node.rect.Min.Y)
		rows[best].bottom = min(rows[best].bottom, limit.Y)
	}

	slices.SortFunc(rows, func(a, b labelOrderRow) int {
		if order := cmp.Compare(a.top, b.top); order != 0 {
			return order
		}
		if order := cmp.Compare(a.bottom, b.bottom); order != 0 {
			return order
		}
		return cmp.Compare(a.id, b.id)
	})
	rowRanks := make([]int, len(rows))
	for rank, row := range rows {
		rowRanks[row.id] = rank
	}
	slices.SortFunc(nodes, func(a, b labelOrderNode) int {
		if order := cmp.Compare(rowRanks[a.row], rowRanks[b.row]); order != 0 {
			return order
		}
		if order := cmp.Compare(a.rect.Min.X, b.rect.Min.X); order != 0 {
			return order
		}
		if order := cmp.Compare(a.labelY, b.labelY); order != 0 {
			return order
		}
		return cmp.Compare(a.id, b.id)
	})
	for i := range nodes {
		targets[i] = nodes[i].id
	}
}

func renderedLabelY(geo *layout.Layout, nodeID uint32) uint32 {
	wrapWidth := uint32(0)
	if _, explicit := geo.ExplicitNodeSize(nodeID); explicit {
		wrapWidth = geo.LabelBounds(nodeID).Size.Width
	}
	lines := layout.AppendLabelLines(nil, geo.Label(nodeID), wrapWidth)
	point, visible := geo.LabelLinePoint(nodeID, 0, uint32(len(lines)), 0)
	if visible {
		visibleLines := min(uint32(len(lines)), geo.LabelBounds(nodeID).Size.Height)
		return point.Y + (visibleLines-1)/2
	}
	return geo.Nodes[nodeID].LabelPoint.Y
}

func labelRowDistance(y, top, bottom uint32) uint64 {
	if y < top {
		return uint64(top - y)
	}
	if y >= bottom {
		return uint64(y) - uint64(bottom) + 1
	}
	return 0
}
