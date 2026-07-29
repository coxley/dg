package tui

import (
	"errors"
	"math"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	clipboardview "github.com/coxley/dg/internal/tui/clipboard"
	"github.com/coxley/dg/layout"
	"github.com/rivo/uniseg"
)

func (m *Model) copySelection() tea.Cmd {
	if !m.hasSelection() {
		m.setError("select something to copy")
		m.clipboard.CancelPending()
		return nil
	}
	text, err := m.selectionText()
	if err != nil {
		m.setError(err.Error())
		m.clipboard.CancelPending()
		return nil
	}
	m.status = ""
	return m.updateClipboard(clipboardview.RequestCopy(
		text,
		m.preferenceValue().CommentPrefix,
	))
}

func (m *Model) updateClipboard(message tea.Msg) tea.Cmd {
	model, command := m.clipboard.Update(message)
	m.clipboard = model.(*clipboardview.Model)
	return command
}

func (m *Model) selectionText() (string, error) {
	bounds, ok := m.selectionBounds()
	if !ok {
		return "", errors.New("empty selection")
	}
	lines := make([]string, 0, bounds.Size.Height)
	limit := bounds.Max()
	for y := bounds.Min.Y; y < limit.Y; y++ {
		lines = append(lines, m.selectionRow(bounds.Min.X, limit.X, y))
	}
	return clipboardview.TrimTrailingWhitespace(strings.Join(lines, "\n")), nil
}

func (m *Model) selectionBounds() (layout.Rect, bool) {
	var (
		minPoint layout.Point
		maxPoint layout.Point
		found    bool
	)
	include := func(point layout.Point) {
		limit := point.Add(1, 1)
		if !found {
			minPoint, maxPoint, found = point, limit, true
			return
		}
		minPoint.X = min(minPoint.X, point.X)
		minPoint.Y = min(minPoint.Y, point.Y)
		maxPoint.X = max(maxPoint.X, limit.X)
		maxPoint.Y = max(maxPoint.Y, limit.Y)
	}
	for nodeID := range m.geo.Selection().Nodes() {
		rect := m.geo.Nodes[nodeID].Rect
		include(rect.Min)
		limit := rect.Max()
		include(layout.NewPoint(limit.X-1, limit.Y-1))
	}
	for edgeID := range m.geo.Selection().Edges() {
		for _, point := range m.geo.Edges[edgeID].Points {
			if point.X == math.MaxUint32 || point.Y == math.MaxUint32 {
				continue
			}
			include(point)
		}
	}
	if !found {
		return layout.Rect{}, false
	}
	return layout.Rect{
		Min: minPoint,
		Size: layout.Size{
			Width:  maxPoint.X - minPoint.X,
			Height: maxPoint.Y - minPoint.Y,
		},
	}, true
}

func (m *Model) selectionRow(start, end, y uint32) string {
	frame := m.canvas.Frame(canvasview.BaseFrame)
	if y < frame.Bounds.Min.Y || y >= frame.Bounds.Max().Y {
		return ""
	}
	rowID := int(y - frame.Bounds.Min.Y)
	rows := m.canvas.Rows(canvasview.BaseFrame)
	if rowID >= len(rows) {
		return ""
	}
	span := rows[rowID]
	row := frame.Text[span.Start:span.End]
	documentX := frame.Bounds.Min.X
	outputX := start
	state := -1
	var line strings.Builder
	for len(row) != 0 && outputX < end {
		cluster, rest, width, nextState := uniseg.FirstGraphemeCluster(row, state)
		row, state = rest, nextState
		if width == 0 {
			continue
		}
		clusterStart := documentX
		clusterEnd := documentX + uint32(width)
		documentX = clusterEnd
		if clusterEnd <= start {
			continue
		}
		if clusterStart >= end {
			break
		}
		visibleStart := max(clusterStart, start)
		visibleEnd := min(clusterEnd, end)
		for outputX < visibleStart {
			line.WriteByte(' ')
			outputX++
		}
		if visibleStart == clusterStart &&
			visibleEnd == clusterEnd &&
			m.selectionContains(layout.NewPoint(clusterStart, y)) {
			line.Write(cluster)
		} else {
			for range visibleEnd - visibleStart {
				line.WriteByte(' ')
			}
		}
		outputX = visibleEnd
	}
	return strings.TrimRightFunc(line.String(), unicode.IsSpace)
}

func (m *Model) selectionContains(point layout.Point) bool {
	for nodeID := range m.geo.Selection().Nodes() {
		if m.geo.Nodes[nodeID].Rect.Contains(point) {
			return true
		}
	}
	for edgeID := range m.geo.Selection().Edges() {
		if m.geo.Edges[edgeID].Contains(point) {
			return true
		}
	}
	return false
}
