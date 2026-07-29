package tui

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/internal/tui/nav"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
	"github.com/rivo/uniseg"
)

func (m *Model) View() tea.View {
	tool := m.activeTool()
	frameID := m.activeFrame()
	frame, rows := m.canvas.Frame(frameID), m.canvas.Rows(frameID)
	toolbar := m.nav.LinesFor(tool)
	workspace := m.workspace.Geometry()
	canvasHost := workspace.Canvas
	toolbarBounds := chrome.Rect{}
	if surface, ok := m.surfacePlan(surfaceNavigation); ok {
		toolbarBounds = surface.Rect
		toolbarBounds.X -= canvasHost.X
		toolbarBounds.Y -= canvasHost.Y
	}
	m.viewBuffer = m.appendViewport(
		m.viewBuffer[:0],
		frame,
		rows,
		m.viewport,
		canvasHost.Width,
		canvasHost.Height,
		toolbar,
		toolbarBounds,
	)
	defaultBase := workspace.Canvas.X == 0 &&
		workspace.Canvas.Y == 0 &&
		workspace.Canvas.Width == workspace.Terminal.Width &&
		workspace.Footer.X == 0 &&
		workspace.Footer.Y == workspace.Canvas.Bottom() &&
		workspace.Footer.Width == workspace.Terminal.Width &&
		workspace.Footer.Height == 1
	if workspace.Footer.Height >= 1 {
		m.statusText = m.appendStatusText(m.statusText[:0])
		if defaultBase {
			m.viewBuffer = appendStatusLine(m.viewBuffer, m.statusText, workspace.Footer.Width)
		}
	}
	content := strings.TrimSuffix(string(m.viewBuffer), "\n")
	if workspace.Footer.Height >= 1 && !defaultBase {
		status := appendStatusLine(nil, m.statusText, workspace.Footer.Width)
		content = composeWorkspaceBase(
			content,
			strings.TrimSuffix(string(status), "\n"),
			workspace,
		)
	} else if workspace.Footer.Height == 0 &&
		(canvasHost.X != 0 || canvasHost.Y != 0 ||
			canvasHost.Width != workspace.Terminal.Width ||
			canvasHost.Height != workspace.Terminal.Height) {
		content = composeWorkspaceBase(content, "", workspace)
	}

	content = m.composeSurfaces(content)
	view := tea.NewView(content + "\n")
	view.AltScreen = true
	view.MouseMode = tea.MouseModeAllMotion
	view.WindowTitle = "dg"
	view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	view.KeyboardEnhancements.ReportAlternateKeys = true
	switch {
	case m.activeDialog != surfaceNone:
	case m.mode == modeEditLabel:
		if x, y, ok := m.cursorPosition(); ok && m.editCaretVisible {
			cursor := &m.viewCursor[m.nextCursor]
			m.nextCursor ^= 1
			cursor.X = x
			cursor.Y = y
			view.Cursor = cursor
		}
	}
	return view
}

func (m *Model) activeTool() nav.Tool {
	switch m.mode {
	case modeRectangle:
		return nav.Rectangle
	case modeConnect:
		return nav.Line
	case modeNavigate, modeMove, modeEditLabel:
		return nav.Cursor
	default:
		return nav.Cursor
	}
}

func (m *Model) activeFrame() canvasview.FrameID {
	switch {
	case m.reconnecting && m.connectDragging:
		return canvasview.ConnectionFrame
	case m.duplicateDragging:
		return canvasview.DuplicateFrame
	default:
		return canvasview.BaseFrame
	}
}

func (m *Model) composeSurfaces(content string) string {
	help, helpVisible := m.surfacePlan(surfaceHelp)
	overlay := m.dialog.Overlay()
	if !helpVisible && overlay.Content == "" {
		return content
	}
	layers := []*lipgloss.Layer{lipgloss.NewLayer(content)}
	if helpVisible {
		layers = append(layers, lipgloss.NewLayer(renderSurfaceLines(m.helpInspector.lines())).
			ID(string(surfaceHelp)).
			X(help.Rect.X).
			Y(help.Rect.Y).
			Z(surfacePriorityHelp))
	}
	if overlay.Content != "" {
		dialogRect := overlayRect(overlay)
		if surface, ok := m.surfacePlan(m.activeDialog); ok {
			dialogRect = surface.Rect
		}
		layers = append(layers, lipgloss.NewLayer(overlay.Content).
			ID(string(m.activeDialog)).
			X(dialogRect.X).
			Y(dialogRect.Y).
			Z(surfacePriorityModal))
	}
	return lipgloss.NewCompositor(layers...).Render()
}

func composeWorkspaceBase(canvas, status string, plan chrome.WorkspacePlan) string {
	if plan.Canvas.X == 0 &&
		plan.Canvas.Y == 0 &&
		plan.Canvas.Width == plan.Terminal.Width &&
		plan.Footer.X == 0 &&
		plan.Footer.Y == plan.Canvas.Bottom() &&
		plan.Footer.Width == plan.Terminal.Width &&
		plan.Footer.Height == 1 {
		return canvas + "\n" + status
	}
	line := strings.Repeat(" ", plan.Terminal.Width)
	rows := make([]string, plan.Terminal.Height)
	for i := range rows {
		rows[i] = line
	}
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(strings.Join(rows, "\n")),
		lipgloss.NewLayer(canvas).
			ID("canvas").
			X(plan.Canvas.X).
			Y(plan.Canvas.Y),
	}
	if status != "" && plan.Footer.Height != 0 {
		layers = append(layers, lipgloss.NewLayer(status).
			ID("status").
			X(plan.Footer.X).
			Y(plan.Footer.Y).
			Z(1))
	}
	return lipgloss.NewCompositor(layers...).Render()
}

type styledRunKey struct {
	text string
	port bool
}

func (m *Model) appendStatusText(dst []byte) []byte {
	if m.status != "" {
		if m.status == m.statusError {
			return append(dst, m.theme.Canvas.Error.Render(m.status)...)
		}
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
	host := m.workspace.Geometry().Canvas
	if host.Width <= 0 || host.Height <= 0 ||
		m.cursor.X < m.viewport.X || m.cursor.Y < m.viewport.Y {
		return 0, 0, false
	}
	x := uint64(m.cursor.X - m.viewport.X)
	y := uint64(m.cursor.Y - m.viewport.Y)
	if x >= uint64(host.Width) || y >= uint64(host.Height) {
		return 0, 0, false
	}
	return host.X + int(x), host.Y + int(y), true
}

func (m *Model) appendViewport(
	dst []byte,
	frame render.Frame,
	rows []canvasview.Span,
	origin layout.Point,
	width, height int,
	toolbar []string,
	toolbarBounds chrome.Rect,
) []byte {
	for screenY := range height {
		documentY := uint64(origin.Y) + uint64(screenY)
		var row []byte
		if documentY < uint64(frame.Bounds.Min.Y) ||
			documentY >= uint64(frame.Bounds.Max().Y) {
			dst = m.appendViewportLine(
				dst,
				nil,
				frame.Bounds.Min.X,
				origin.X,
				documentY,
				width,
				screenY,
				toolbar,
				toolbarBounds,
			)
			continue
		}
		rowID := int(documentY - uint64(frame.Bounds.Min.Y))
		if rowID < len(rows) {
			span := rows[rowID]
			row = frame.Text[span.Start:span.End]
		}
		dst = m.appendViewportLine(
			dst,
			row,
			frame.Bounds.Min.X,
			origin.X,
			documentY,
			width,
			screenY,
			toolbar,
			toolbarBounds,
		)
	}
	return dst
}

func (m *Model) appendViewportLine(
	dst, row []byte,
	rowOrigin, viewportOrigin uint32,
	documentY uint64,
	width int,
	screenY int,
	toolbar []string,
	toolbarBounds chrome.Rect,
) []byte {
	toolbarRow := screenY - toolbarBounds.Y
	if toolbarBounds.Width > 0 &&
		toolbarBounds.Right() <= width &&
		toolbarRow >= 0 &&
		toolbarRow < len(toolbar) {
		left := toolbarBounds.X
		dst = appendViewportSegment(
			dst,
			row,
			rowOrigin,
			uint64(viewportOrigin),
			documentY,
			left,
			m,
		)
		dst = append(dst, toolbar[toolbarRow]...)
		right := width - toolbarBounds.Right()
		dst = appendViewportSegment(
			dst,
			row,
			rowOrigin,
			uint64(viewportOrigin)+uint64(toolbarBounds.Right()),
			documentY,
			right,
			m,
		)
		return append(dst, '\n')
	}
	dst = appendViewportSegment(
		dst,
		row,
		rowOrigin,
		uint64(viewportOrigin),
		documentY,
		width,
		m,
	)
	return append(dst, '\n')
}

func (m *Model) appendStyledRun(dst, text []byte, selected bool) []byte {
	if !selected {
		return append(dst, text...)
	}
	port := m.mode == modeConnect
	key := styledRunKey{
		text: string(text),
		port: port,
	}
	rendered, ok := m.styledRuns[key]
	if !ok {
		rendered = m.highlightStyle().Render(key.text)
		m.styledRuns[key] = rendered
	}
	return append(dst, rendered...)
}

func (m *Model) styleTail(dst []byte, start int) []byte {
	tail := dst[start:]
	dst = dst[:start]
	return m.appendStyledRun(dst, tail, true)
}

func appendViewportSegment(
	dst, row []byte,
	rowOrigin uint32,
	viewportOrigin, documentY uint64,
	width int,
	model *Model,
) []byte {
	if len(row) == 0 ||
		viewportOrigin > math.MaxUint32 ||
		documentY > math.MaxUint32 {
		return appendViewportSpaces(
			dst,
			width,
			viewportOrigin,
			documentY,
			model,
		)
	}
	return appendViewportRow(
		dst,
		row,
		rowOrigin,
		uint32(viewportOrigin),
		uint32(documentY),
		width,
		model,
	)
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
	styledStart := -1

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
			if styledStart >= 0 {
				dst = model.styleTail(dst, styledStart)
				styledStart = -1
			}
			dst = appendHighlightedSpaces(
				dst,
				gap,
				viewportStart+uint64(screenX),
				uint64(documentY),
				model,
			)
		}
		screenX = targetX

		var visible []byte
		preview, hasPreview := previewGlyph(
			model,
			visibleStart,
			uint64(documentY),
		)
		if hasPreview && clusterWidth == 1 {
			visible = utf8.AppendRune(visible, preview)
		} else if visibleStart == clusterStart && visibleEnd == clusterEnd {
			visible = cluster
		} else {
			visible = appendSpaces(visible, int(visibleEnd-visibleStart))
		}
		selected := model != nil &&
			model.highlightedRange(
				documentY,
				uint32(visibleStart),
				uint32(visibleEnd),
			)
		if selected && styledStart < 0 {
			styledStart = len(dst)
		} else if !selected && styledStart >= 0 {
			dst = model.styleTail(dst, styledStart)
			styledStart = -1
		}
		dst = append(dst, visible...)
		screenX += int(visibleEnd - visibleStart)
	}
	if styledStart >= 0 {
		dst = model.styleTail(dst, styledStart)
	}
	dst = appendHighlightedSpaces(
		dst,
		width-screenX,
		viewportStart+uint64(screenX),
		uint64(documentY),
		model,
	)
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
	if m.duplicateDragging {
		return highlightContains(
			m.duplicateHighlight,
			m.canvas.Frame(canvasview.DuplicateFrame),
			point,
		)
	}
	if m.rigidMoving {
		return highlightContains(
			m.moveHighlight,
			m.canvas.Frame(canvasview.BaseFrame),
			point,
		)
	}
	if m.selecting && m.marqueeArea().contains(point) {
		return true
	}
	if m.mode == modeConnect {
		return m.portAt(point)
	}
	geo := m.geo
	if m.duplicateDragging {
		geo = m.duplicateGeo
	}
	if !geo.Selection().Empty() {
		for nodeID := range geo.Selection().Nodes() {
			if m.highlightForHit(
				layout.Hit{ID: nodeID, Kind: layout.HitNode},
				point,
			) {
				return true
			}
		}
		for edgeID := range geo.Selection().Edges() {
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
	geo := m.geo
	if m.duplicateDragging {
		geo = m.duplicateGeo
	}
	switch hit.Kind {
	case layout.HitNode:
		return geo.NodeExists(hit.ID) &&
			geo.Nodes[hit.ID].Rect.OnBoundary(point)
	case layout.HitPort:
		return false
	case layout.HitEdge:
		return geo.EdgeExists(hit.ID) &&
			geo.Edges[hit.ID].Contains(point)
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
	index, ok := slices.BinarySearchFunc(
		m.connectRaster,
		point,
		func(cell layout.RasterCell, point layout.Point) int {
			if order := cmp.Compare(cell.Point.Y, point.Y); order != 0 {
				return order
			}
			return cmp.Compare(cell.Point.X, point.X)
		},
	)
	if !ok {
		return 0, false
	}
	return m.connectRaster[index].Connections, true
}

func (m *Model) refreshConnectionPreview() {
	if !m.connectStarted || !m.geo.PortExists(m.connectSource) {
		m.connectPreview = m.connectPreview[:0]
		m.connectRaster = m.connectRaster[:0]
		return
	}
	var (
		preview []layout.Point
		err     error
	)
	style := m.connectionPreviewStyle()
	if m.reconnecting {
		preview, err = m.geo.PreviewRouteWithoutEdgeStyled(
			m.connectPreview[:0],
			m.connectSource,
			m.cursor,
			m.connectEdge,
			style,
		)
	} else {
		preview, err = m.geo.PreviewRouteStyled(
			m.connectPreview[:0],
			m.connectSource,
			m.cursor,
			style,
		)
	}
	if err != nil {
		m.connectPreview = m.connectPreview[:0]
		m.connectRaster = m.connectRaster[:0]
		return
	}
	m.connectPreview = preview
	destination := layout.NoPortID
	if portID, ok := m.usablePortAt(m.cursor); ok &&
		portID != m.connectSource {
		destination = portID
	}
	m.connectRaster, err = m.canvas.RasterizeEdge(
		m.connectRaster[:0],
		m.geo,
		layout.RasterEdge{
			Points: preview,
			PortA:  m.connectSource,
			PortB:  destination,
		},
	)
	if err != nil {
		m.connectPreview = m.connectPreview[:0]
		m.connectRaster = m.connectRaster[:0]
		return
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
	return appendHighlightedSpaces(
		dst,
		count,
		startX,
		y,
		model,
	)
}

func appendHighlightedSpaces(
	dst []byte,
	count int,
	startX, y uint64,
	model *Model,
) []byte {
	if model == nil ||
		!model.selecting &&
			(model.mode != modeConnect || !model.connectStarted) {
		return appendSpaces(dst, count)
	}
	styledStart := -1
	for offset := range max(count, 0) {
		x := startX + uint64(offset)
		var text []byte
		if preview, ok := previewGlyph(model, x, y); ok {
			text = utf8.AppendRune(text, preview)
		} else {
			text = append(text, ' ')
		}
		selected := x <= math.MaxUint32 &&
			y <= math.MaxUint32 &&
			model.highlightedPoint(layout.NewPoint(uint32(x), uint32(y)))
		if selected && styledStart < 0 {
			styledStart = len(dst)
		} else if !selected && styledStart >= 0 {
			dst = model.styleTail(dst, styledStart)
			styledStart = -1
		}
		dst = append(dst, text...)
	}
	if styledStart >= 0 {
		dst = model.styleTail(dst, styledStart)
	}
	return dst
}

func (m *Model) highlightStyle() lipgloss.Style {
	if m != nil && m.mode == modeConnect {
		return m.canvas.PortStyle()
	}
	return m.canvas.SelectionStyle()
}

func previewGlyph(model *Model, x, y uint64) (rune, bool) {
	if model == nil || x > math.MaxUint32 || y > math.MaxUint32 {
		return 0, false
	}
	point := layout.NewPoint(uint32(x), uint32(y))
	connections, ok := model.connectionPreviewConnections(point)
	if !ok {
		return 0, false
	}
	if glyph, ok := render.ArrowGlyphAt(
		model.connectPreview,
		model.connectionPreviewStyle(),
		point,
	); ok {
		return glyph, true
	}
	return render.StrokeGlyph(
		connections,
		model.connectionPreviewStyle().Stroke,
	), true
}

func (m *Model) connectionPreviewStyle() layout.EdgeStyle {
	if !m.reconnecting {
		return m.edgeStyle
	}
	style, _ := m.geo.EdgeStyle(m.connectEdge)
	portA, _, err := m.geo.EdgePorts(m.connectEdge)
	if err == nil && m.connectSource != portA {
		style.PortAArrow, style.PortBArrow = style.PortBArrow, style.PortAArrow
	}
	return style
}
