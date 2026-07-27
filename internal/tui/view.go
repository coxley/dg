package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
	"github.com/rivo/uniseg"
)

const (
	selectionStart = "\x1b[48;5;24;38;5;231m"
	selectionEnd   = "\x1b[0m"
)

func (m *Model) View() tea.View {
	m.viewBuffer = m.appendViewport(
		m.viewBuffer[:0],
		m.frame,
		m.frameRows,
		m.viewport,
		m.width,
		m.diagramHeight(),
	)
	if m.height >= 1 {
		m.statusText = m.appendStatusText(m.statusText[:0])
		m.viewBuffer = appendStatusLine(m.viewBuffer, m.statusText, m.width)
	}
	if m.height >= 2 {
		m.statusText = append(m.statusText[:0], m.helpLine()...)
		m.viewBuffer = appendStatusLine(m.viewBuffer, m.statusText, m.width)
	}

	view := tea.NewView(string(m.viewBuffer))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = "dg"
	switch m.mode {
	case modeEditLabel:
		if x, y, ok := m.cursorPosition(); ok {
			cursor := &m.viewCursor[m.nextCursor]
			m.nextCursor ^= 1
			cursor.X = x
			cursor.Y = y
			view.Cursor = cursor
		}
	case modeSavePath:
		x := len("save path: ") + displayWidth(m.editBuffer[:m.editCaret])
		if m.height >= 2 && x < m.width {
			cursor := &m.viewCursor[m.nextCursor]
			m.nextCursor ^= 1
			cursor.X = x
			cursor.Y = m.diagramHeight()
			view.Cursor = cursor
		}
	default:
	}
	return view
}

func (m *Model) appendStatusText(dst []byte) []byte {
	if m.mode == modeSavePath {
		dst = append(dst, "save path: "...)
		return append(dst, m.editBuffer...)
	}
	if m.status != "" {
		return append(dst, m.status...)
	}
	if m.mode == modeEditLabel {
		dst = append(dst, "edit label  node "...)
		dst = strconv.AppendUint(dst, uint64(m.target.ID), 10)
		dst = append(dst, "  cell "...)
		dst = strconv.AppendInt(dst, int64(displayWidth(m.editBuffer[:m.editCaret])), 10)
		dst = append(dst, '/')
		return strconv.AppendInt(dst, int64(displayWidth(m.editBuffer)), 10)
	}

	dst = append(dst, m.mode.String()...)
	dst = append(dst, "  ("...)
	dst = strconv.AppendUint(dst, uint64(m.cursor.X), 10)
	dst = append(dst, ',')
	dst = strconv.AppendUint(dst, uint64(m.cursor.Y), 10)
	dst = append(dst, ")  "...)
	hit, ok := m.activeHit()
	if !ok {
		return append(dst, "no hit"...)
	}
	dst = append(dst, hitKindName(hit.Kind)...)
	dst = append(dst, ' ')
	dst = strconv.AppendUint(dst, uint64(hit.ID), 10)
	dst = append(dst, "  hit "...)
	dst = strconv.AppendInt(dst, int64(m.active+1), 10)
	dst = append(dst, '/')
	return strconv.AppendInt(dst, int64(len(m.hits)), 10)
}

func (m *Model) helpLine() string {
	switch m.mode {
	case modeMove:
		return "arrows/hjkl move node • enter/m/esc finish • mouse drag • ctrl+c quit"
	case modeEditLabel:
		return "type • ctrl-a/e ends • alt-b back word • ctrl-w/u delete • enter save • esc cancel"
	case modeConnect:
		if m.reconnecting {
			return "select replacement port • enter/c/click move endpoint • esc cancel"
		}
		return "select destination port • enter/c/click connect • esc cancel"
	case modeSavePath:
		if m.status != "" {
			return m.status
		}
		if m.saveHint != "" {
			return m.saveHint
		}
		return "type path • ctrl-a/e/u/w • alt-b • tab complete • enter/ctrl+s save • esc cancel"
	default:
		return "arrows/hjkl move • tab cycle • n new • m move • e edit • c connect • d delete • ctrl+s save"
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

func (m *Model) cursorPosition() (int, int, bool) {
	height := m.diagramHeight()
	if m.width <= 0 || height <= 0 ||
		m.cursor.X < m.viewport.X || m.cursor.Y < m.viewport.Y {
		return 0, 0, false
	}
	x := uint64(m.cursor.X - m.viewport.X)
	y := uint64(m.cursor.Y - m.viewport.Y)
	if x >= uint64(m.width) || y >= uint64(height) {
		return 0, 0, false
	}
	return int(x), int(y), true
}

func (m *Model) appendViewport(
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
			uint32(documentY),
			width,
			m,
		)
		dst = append(dst, '\n')
	}
	return dst
}

func appendViewportRow(
	dst, row []byte,
	rowOrigin, viewportOrigin, documentY uint32,
	width int,
	model *Model,
) []byte {
	viewportStart := uint64(viewportOrigin)
	viewportEnd := viewportStart + uint64(width)
	documentX := uint64(rowOrigin)
	screenX := 0
	state := -1
	styled := false

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
		if gap := targetX - screenX; gap != 0 {
			if styled {
				dst = append(dst, selectionEnd...)
				styled = false
			}
			dst = appendSpaces(dst, gap)
		}
		screenX = targetX

		selected := model != nil &&
			model.highlightedRange(
				documentY,
				uint32(visibleStart),
				uint32(visibleEnd),
			)
		if selected && !styled {
			dst = append(dst, selectionStart...)
			styled = true
		} else if !selected && styled {
			dst = append(dst, selectionEnd...)
			styled = false
		}
		if visibleStart == clusterStart && visibleEnd == clusterEnd {
			dst = append(dst, cluster...)
		} else {
			dst = appendSpaces(dst, int(visibleEnd-visibleStart))
		}
		screenX += int(visibleEnd - visibleStart)
	}
	if styled {
		dst = append(dst, selectionEnd...)
	}
	return appendSpaces(dst, width-screenX)
}

func (m *Model) highlightedRange(y, start, end uint32) bool {
	for x := start; x < end; x++ {
		if m.highlightedPoint(layout.NewPoint(x, y)) {
			return true
		}
	}
	return false
}

func (m *Model) highlightedPoint(point layout.Point) bool {
	if m.mode == modeConnect {
		return m.portAt(point)
	}
	hit, ok := m.primaryHighlight()
	if !ok {
		return false
	}
	return m.highlightForHit(hit, point)
}

func (m *Model) highlightForHit(hit layout.Hit, point layout.Point) bool {
	switch hit.Kind {
	case layout.HitNode:
		return m.geo.NodeExists(hit.ID) &&
			m.geo.Nodes[hit.ID].Rect.OnBoundary(point)
	case layout.HitPort:
		return false
	case layout.HitEdge:
		return m.geo.EdgeExists(hit.ID) &&
			m.geo.Edges[hit.ID].Contains(point)
	}
	return false
}

func (m *Model) portAt(point layout.Point) bool {
	for i := range m.geo.Ports {
		portID := uint32(i)
		if m.geo.PortUsable(portID) && m.geo.Ports[i].Anchor == point {
			return true
		}
	}
	return false
}

func (m *Model) primaryHighlight() (layout.Hit, bool) {
	switch m.mode {
	case modeMove, modeEditLabel:
		return m.target, true
	default:
		return m.activeHit()
	}
}

func appendStatusLine(dst, text []byte, width int) []byte {
	if width <= 0 {
		return dst
	}
	remaining := text
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
