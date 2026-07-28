// Package flex lays out ANSI-styled terminal content.
package flex

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Item describes one single-line cell in a row.
type Item struct {
	Content string
	Grow    uint8
	Shrink  uint8
	Align   lipgloss.Position
}

// Row renders items within width display cells.
//
// Items begin at their content width. Grow weights divide unused cells and
// Shrink weights divide overflow. Content whose item shrinks gets truncated.
// If no eligible items can absorb overflow, Row returns the remaining content
// at its natural width.
func Row(width int, items ...Item) string {
	if len(items) == 0 {
		return ""
	}

	widths := make([]int, len(items))
	total := 0
	for i := range items {
		widths[i] = ansi.StringWidth(items[i].Content)
		total += widths[i]
	}

	if width > 0 {
		switch {
		case total < width:
			grow(widths, items, width-total)
		case total > width:
			shrink(widths, items, total-width)
		}
	}

	var row strings.Builder
	row.Grow(max(width, total))
	for i, item := range items {
		content := ansi.Truncate(item.Content, widths[i], "")
		row.WriteString(lipgloss.PlaceHorizontal(widths[i], item.Align, content))
	}
	return row.String()
}

func grow(widths []int, items []Item, cells int) {
	weight := 0
	for i := range items {
		weight += int(items[i].Grow)
	}
	if weight == 0 {
		return
	}

	base, remainder := cells/weight, cells%weight
	for i := range items {
		itemWeight := int(items[i].Grow)
		widths[i] += base*itemWeight + min(remainder, itemWeight)
		remainder = max(remainder-itemWeight, 0)
	}
}

func shrink(widths []int, items []Item, cells int) {
	for cells > 0 {
		weight := 0
		for i := range items {
			if widths[i] > 0 {
				weight += int(items[i].Shrink)
			}
		}
		if weight == 0 {
			return
		}

		base, remainder := cells/weight, cells%weight
		shrunk := 0
		for i := range items {
			itemWeight := int(items[i].Shrink)
			if widths[i] == 0 || itemWeight == 0 {
				continue
			}
			share := base*itemWeight + min(remainder, itemWeight)
			remainder = max(remainder-itemWeight, 0)
			share = min(share, widths[i])
			widths[i] -= share
			shrunk += share
		}
		if shrunk == 0 {
			return
		}
		cells -= shrunk
	}
}
