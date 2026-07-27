package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
	"github.com/rivo/uniseg"
)

func (m *Model) View() tea.View {
	m.viewBuffer = appendViewport(
		m.viewBuffer[:0],
		m.frame,
		m.frameRows,
		m.viewport,
		m.width,
		m.diagramHeight(),
	)
	if m.height >= 1 {
		m.viewBuffer = appendStatusLine(m.viewBuffer, m.statusLine(), m.width)
	}
	if m.height >= 2 {
		m.viewBuffer = appendStatusLine(m.viewBuffer, m.helpLine(), m.width)
	}

	view := tea.NewView(string(m.viewBuffer))
	view.AltScreen = true
	view.WindowTitle = "dg"
	if m.width > 0 && m.diagramHeight() > 0 {
		x := int(m.cursor.X - m.viewport.X)
		y := int(m.cursor.Y - m.viewport.Y)
		view.Cursor = tea.NewCursor(x, y)
	}
	return view
}

func (m *Model) statusLine() string {
	if m.status != "" {
		return m.status
	}
	if m.mode == modeEditLabel {
		return fmt.Sprintf("edit label: %s", m.editBuffer)
	}
	hit, ok := m.activeHit()
	if !ok {
		return fmt.Sprintf("%s  (%d,%d)  no hit", m.mode, m.cursor.X, m.cursor.Y)
	}
	return fmt.Sprintf(
		"%s  (%d,%d)  %s %d  hit %d/%d",
		m.mode,
		m.cursor.X,
		m.cursor.Y,
		hitKindName(hit.Kind),
		hit.ID,
		m.active+1,
		len(m.hits),
	)
}

func (m *Model) helpLine() string {
	switch m.mode {
	case modeMove:
		return "arrows/hjkl move node • enter/m/esc finish • ctrl+c quit"
	case modeEditLabel:
		return "type to edit • enter save • esc cancel • ctrl+c quit"
	default:
		return "arrows/hjkl move • tab cycle • enter/m move node • e edit • d delete • q quit"
	}
}

func hitKindName(kind layout.HitKind) string {
	switch kind {
	case layout.HitNode:
		return "node"
	case layout.HitPort:
		return "port"
	case layout.HitEdge:
		return "edge"
	default:
		return "unknown"
	}
}

func appendViewport(
	dst []byte,
	frame render.Frame,
	rows []rowSpan,
	origin layout.Point,
	width, height int,
) []byte {
	for screenY := range height {
		documentY := uint64(origin.Y) + uint64(screenY)
		if documentY < uint64(frame.Bounds.Min.Y) ||
			documentY >= uint64(frame.Bounds.Max().Y) {
			dst = appendSpaces(dst, width)
			dst = append(dst, '\n')
			continue
		}
		row := int(documentY - uint64(frame.Bounds.Min.Y))
		if row >= len(rows) {
			dst = appendSpaces(dst, width)
			dst = append(dst, '\n')
			continue
		}
		span := rows[row]
		dst = appendViewportRow(
			dst,
			frame.Text[span.start:span.end],
			frame.Bounds.Min.X,
			origin.X,
			width,
		)
		dst = append(dst, '\n')
	}
	return dst
}

func appendViewportRow(dst, row []byte, rowOrigin, viewportOrigin uint32, width int) []byte {
	viewportStart := uint64(viewportOrigin)
	viewportEnd := viewportStart + uint64(width)
	documentX := uint64(rowOrigin)
	screenX := 0
	state := -1

	for len(row) != 0 && screenX < width {
		cluster, rest, clusterWidth, nextState := uniseg.FirstGraphemeCluster(row, state)
		row, state = rest, nextState
		if clusterWidth == 0 {
			if documentX >= viewportStart && documentX < viewportEnd && screenX != 0 {
				dst = append(dst, cluster...)
			}
			continue
		}

		clusterStart := documentX
		clusterEnd := clusterStart + uint64(clusterWidth)
		documentX = clusterEnd
		if clusterEnd <= viewportStart {
			continue
		}
		if clusterStart >= viewportEnd {
			break
		}

		visibleStart := max(clusterStart, viewportStart)
		visibleEnd := min(clusterEnd, viewportEnd)
		targetX := int(visibleStart - viewportStart)
		dst = appendSpaces(dst, targetX-screenX)
		screenX = targetX
		if visibleStart == clusterStart && visibleEnd == clusterEnd {
			dst = append(dst, cluster...)
		} else {
			dst = appendSpaces(dst, int(visibleEnd-visibleStart))
		}
		screenX += int(visibleEnd - visibleStart)
	}
	return appendSpaces(dst, width-screenX)
}

func appendStatusLine(dst []byte, text string, width int) []byte {
	if width <= 0 {
		return dst
	}
	remaining := []byte(text)
	state := -1
	used := 0
	for len(remaining) != 0 {
		cluster, rest, clusterWidth, nextState := uniseg.FirstGraphemeCluster(remaining, state)
		if used+clusterWidth > width {
			break
		}
		dst = append(dst, cluster...)
		remaining, state = rest, nextState
		used += clusterWidth
	}
	dst = appendSpaces(dst, width-used)
	return append(dst, '\n')
}

func appendSpaces(dst []byte, count int) []byte {
	for range max(count, 0) {
		dst = append(dst, ' ')
	}
	return dst
}
