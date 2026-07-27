package tui

import (
	"math"
	"strconv"
	"unicode/utf8"

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
	frame, rows := m.frame, m.frameRows
	if m.reconnecting && m.connectDragging {
		frame, rows = m.connectFrame, m.connectFrameRows
	}
	m.viewBuffer = m.appendViewport(
		m.viewBuffer[:0],
		frame,
		rows,
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
		if x, y, ok := m.cursorPosition(); ok && m.editCaretVisible {
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
	if nodes, edges := m.selectedCounts(); nodes != 0 || edges != 0 {
		dst = append(dst, "selected  nodes "...)
		dst = strconv.AppendInt(dst, int64(nodes), 10)
		dst = append(dst, "  edges "...)
		return strconv.AppendInt(dst, int64(edges), 10)
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
		return "arrows move selection • enter/m/esc finish • left-drag move • right-drag resize • ctrl+c quit"
	case modeEditLabel:
		return "type • enter newline • ctrl-enter/esc save • arrows move • ctrl-a/e ends • alt-b back word • ctrl-w/u delete"
	case modeConnect:
		if m.reconnecting {
			return "drag to replacement port • enter move endpoint • esc cancel"
		}
		return "drag between ports • enter connects selected port • esc cancel"
	case modeSavePath:
		if m.status != "" {
			return m.status
		}
		if m.saveHint != "" {
			return m.saveHint
		}
		return "type path • ctrl-a/e/u/w • alt-b • tab complete • enter/ctrl+s save • esc cancel"
	default:
		return "arrows move • left-drag move • right-drag resize • drag empty select • ctrl-click toggle • ctrl-a expand/all • n new • e edit • l line/drag edge ends • d delete • u/ctrl-z undo • ctrl-r/y redo • ctrl+s save"
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
			dst = appendViewportSpaces(
				dst,
				width,
				uint64(origin.X),
				documentY,
				m,
			)
			dst = append(dst, '\n')
			continue
		}
		row := int(documentY - uint64(frame.Bounds.Min.Y))
		if row >= len(rows) {
			dst = appendViewportSpaces(
				dst,
				width,
				uint64(origin.X),
				documentY,
				m,
			)
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
			dst, styled = appendHighlightedSpaces(
				dst,
				gap,
				viewportStart+uint64(screenX),
				uint64(documentY),
				model,
				styled,
			)
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
		preview, hasPreview := previewGlyph(
			model,
			visibleStart,
			uint64(documentY),
		)
		if hasPreview && clusterWidth == 1 {
			dst = utf8.AppendRune(dst, preview)
		} else if visibleStart == clusterStart && visibleEnd == clusterEnd {
			dst = append(dst, cluster...)
		} else {
			dst = appendSpaces(dst, int(visibleEnd-visibleStart))
		}
		screenX += int(visibleEnd - visibleStart)
	}
	if styled {
		dst = append(dst, selectionEnd...)
		styled = false
	}
	dst, styled = appendHighlightedSpaces(
		dst,
		width-screenX,
		viewportStart+uint64(screenX),
		uint64(documentY),
		model,
		styled,
	)
	if styled {
		dst = append(dst, selectionEnd...)
	}
	return dst
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
	if m.selecting && m.marqueeArea().contains(point) {
		return true
	}
	if m.mode == modeConnect {
		return m.portAt(point)
	}
	if m.hasSelection() {
		for nodeID := range m.geo.Selection().Nodes() {
			if m.highlightForHit(
				layout.Hit{ID: nodeID, Kind: layout.HitNode},
				point,
			) {
				return true
			}
		}
		for edgeID := range m.geo.Selection().Edges() {
			if m.highlightForHit(
				layout.Hit{ID: edgeID, Kind: layout.HitEdge},
				point,
			) {
				return true
			}
		}
		return false
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
	_, ok := m.usablePortAt(point)
	return ok
}

func (m *Model) usablePortAt(point layout.Point) (uint32, bool) {
	for i := range m.geo.Ports {
		portID := uint32(i)
		if m.geo.PortUsable(portID) && m.geo.Ports[i].Anchor == point {
			return portID, true
		}
	}
	return 0, false
}

func (m *Model) connectionPreviewConnections(
	point layout.Point,
) (layout.Connections, bool) {
	if m.connectPreviewLen < 2 {
		return 0, false
	}
	var connections layout.Connections
	for i := 1; i < int(m.connectPreviewLen); i++ {
		start, finish := m.connectPreview[i-1], m.connectPreview[i]
		if !pointOnOrthogonalSegment(point, start, finish) {
			continue
		}
		connections |= connectionToward(point, start)
		connections |= connectionToward(point, finish)
	}
	return connections, connections != 0
}

func (m *Model) refreshConnectionPreview() {
	if !m.connectStarted || !m.geo.PortExists(m.connectSource) {
		m.connectPreviewLen = 0
		return
	}
	source := m.geo.Ports[m.connectSource]
	end := m.cursor
	var destination layout.Port
	hasDestination := false
	if portID, ok := m.usablePortAt(m.cursor); ok &&
		portID != m.connectSource {
		destination = m.geo.Ports[portID]
		end = destination.Exit
		hasDestination = true
	}
	bend := layout.NewPoint(end.X, source.Exit.Y)
	if source.Exit.X == source.Anchor.X {
		bend = layout.NewPoint(source.Exit.X, end.Y)
	}
	m.connectPreview = [...]layout.Point{
		source.Anchor,
		source.Exit,
		bend,
		end,
		destination.Anchor,
	}
	m.connectPreviewLen = 4
	if hasDestination {
		m.connectPreviewLen++
	}
}

func pointOnOrthogonalSegment(point, start, end layout.Point) bool {
	return start.X == end.X &&
		point.X == start.X &&
		point.Y >= min(start.Y, end.Y) &&
		point.Y <= max(start.Y, end.Y) ||
		start.Y == end.Y &&
			point.Y == start.Y &&
			point.X >= min(start.X, end.X) &&
			point.X <= max(start.X, end.X)
}

func connectionToward(point, other layout.Point) layout.Connections {
	switch {
	case other.Y < point.Y:
		return layout.North
	case other.X > point.X:
		return layout.East
	case other.Y > point.Y:
		return layout.South
	case other.X < point.X:
		return layout.West
	default:
		return 0
	}
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

func appendViewportSpaces(
	dst []byte,
	count int,
	startX, y uint64,
	model *Model,
) []byte {
	dst, styled := appendHighlightedSpaces(
		dst,
		count,
		startX,
		y,
		model,
		false,
	)
	if styled {
		dst = append(dst, selectionEnd...)
	}
	return dst
}

func appendHighlightedSpaces(
	dst []byte,
	count int,
	startX, y uint64,
	model *Model,
	styled bool,
) ([]byte, bool) {
	if model == nil ||
		!model.selecting &&
			(model.mode != modeConnect || !model.connectStarted) {
		if styled {
			dst = append(dst, selectionEnd...)
		}
		return appendSpaces(dst, count), false
	}
	for offset := range max(count, 0) {
		x := startX + uint64(offset)
		selected := model != nil &&
			x <= math.MaxUint32 &&
			y <= math.MaxUint32 &&
			model.highlightedPoint(
				layout.NewPoint(uint32(x), uint32(y)),
			)
		if selected && !styled {
			dst = append(dst, selectionStart...)
			styled = true
		} else if !selected && styled {
			dst = append(dst, selectionEnd...)
			styled = false
		}
		if preview, ok := previewGlyph(model, x, y); ok {
			dst = utf8.AppendRune(dst, preview)
		} else {
			dst = append(dst, ' ')
		}
	}
	return dst, styled
}

func previewGlyph(model *Model, x, y uint64) (rune, bool) {
	if model == nil || x > math.MaxUint32 || y > math.MaxUint32 {
		return 0, false
	}
	connections, ok := model.connectionPreviewConnections(
		layout.NewPoint(uint32(x), uint32(y)),
	)
	if !ok {
		return 0, false
	}
	return render.Glyph(connections), true
}
