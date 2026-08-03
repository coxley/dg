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
	defer m.retainPanic()
	tool := m.activeTool()
	frameID := m.activeFrame()
	frame, rows := m.canvas.Frame(frameID), m.canvas.Rows(frameID)
	toolbar := m.nav.LinesFor(tool)
	workspace := m.workspace.Geometry()
	canvasHost := workspace.Canvas
	toolbarBounds := chrome.Rect{}
	if surface, ok := m.surfacePlan(surfaceNavigation); ok {
		if rectWithin(surface.Rect, canvasHost) {
			toolbarBounds = surface.Rect
			toolbarBounds.X -= canvasHost.X
			toolbarBounds.Y -= canvasHost.Y
		} else {
			toolbar = nil
		}
	}
	m.viewBuffer = m.appendViewport(
		m.viewBuffer[:0],
		frameID,
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
		statusStyle := m.theme.Status.Normal
		if m.status != "" && m.status == m.statusError {
			statusStyle = m.theme.Status.Error
		}
		styled := statusStyle.Render(string(m.statusText))
		m.statusText = append(m.statusText[:0], styled...)
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
	view.ReportFocus = true
	view.MouseMode = tea.MouseModeAllMotion
	view.WindowTitle = m.windowTitle
	view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	view.KeyboardEnhancements.ReportAlternateKeys = true
	switch {
	case m.dialogs.ActiveID() != surfaceNone:
	case m.interaction.session.kind == sessionLabelEdit:
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
	switch m.interaction.tool {
	case toolRectangle:
		return nav.Rectangle
	case toolConnect:
		return nav.Line
	case toolNavigate:
		return nav.Cursor
	default:
		return nav.Cursor
	}
}

func (m *Model) activeFrame() canvasview.FrameID {
	switch {
	case m.interaction.session.kind == sessionBend &&
		m.interaction.gesture.kind == gestureBend:
		return canvasview.ConnectionFrame
	case m.interaction.session.kind == sessionConnection &&
		m.interaction.session.connection.reconnect &&
		m.interaction.gesture.connectionActive():
		return canvasview.ConnectionFrame
	case m.interaction.gesture.duplicateActive():
		return canvasview.DuplicateFrame
	default:
		return canvasview.BaseFrame
	}
}

func (m *Model) composeSurfaces(content string) string {
	help, helpVisible := m.surfacePlan(surfaceHelp)
	sidebar, sidebarVisible := m.surfacePlan(surfaceSidebar)
	navigation, navigationVisible := m.surfacePlan(surfaceNavigation)
	navigationVisible = navigationVisible &&
		!rectWithin(navigation.Rect, m.workspace.Geometry().Canvas)
	overlay := m.dialogs.Overlay()
	if !helpVisible && !sidebarVisible && !navigationVisible && overlay.Content == "" {
		return content
	}
	layers := []*lipgloss.Layer{lipgloss.NewLayer(content)}
	if sidebarVisible {
		layers = append(layers, lipgloss.NewLayer(renderSurfaceLines(m.sidebar.lines(sidebar))).
			ID(string(surfaceSidebar)).
			X(sidebar.Rect.X).
			Y(sidebar.Rect.Y).
			Z(sidebar.Surface.Priority))
	}
	if navigationVisible {
		layers = append(layers, lipgloss.NewLayer(renderSurfaceLines(
			m.nav.LinesFor(m.activeTool()),
		)).
			ID(string(surfaceNavigation)).
			X(navigation.Rect.X).
			Y(navigation.Rect.Y).
			Z(surfacePriorityNavigation))
	}
	if helpVisible {
		layers = append(layers, lipgloss.NewLayer(renderSurfaceLines(m.helpInspector.lines())).
			ID(string(surfaceHelp)).
			X(help.Rect.X).
			Y(help.Rect.Y).
			Z(surfacePriorityHelp))
	}
	if overlay.Content != "" {
		dialogRect := overlayRect(overlay)
		if surface, ok := m.surfacePlan(m.dialogs.ActiveID()); ok {
			dialogRect = surface.Rect
		}
		layers = append(layers, lipgloss.NewLayer(overlay.Content).
			ID(string(m.dialogs.ActiveID())).
			X(dialogRect.X).
			Y(dialogRect.Y).
			Z(surfacePriorityModal))
	}
	return lipgloss.NewCompositor(layers...).Render()
}

func rectWithin(rect, bounds chrome.Rect) bool {
	return rect.X >= bounds.X && rect.Y >= bounds.Y &&
		rect.Right() <= bounds.Right() && rect.Bottom() <= bounds.Bottom()
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
	kind highlightKind
}

type highlightKind uint8

const (
	highlightNone highlightKind = iota
	highlightSelection
	highlightPort
	highlightCandidateEdge
)

func (m *Model) appendStatusText(dst []byte) []byte {
	if m.status != "" {
		return append(dst, m.status...)
	}
	if m.interaction.session.kind == sessionLabelEdit {
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

	dst = append(dst, m.interaction.mode().String()...)
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
	frameID canvasview.FrameID,
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
		rowOrigin := frame.Bounds.Min.X
		if rowID < len(rows) {
			row, rowOrigin = m.canvas.Row(frameID, rowID, origin.X)
		}
		dst = m.appendViewportLine(
			dst,
			row,
			rowOrigin,
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

func (m *Model) appendStyledRun(dst, text []byte, kind highlightKind) []byte {
	if kind == highlightNone {
		return append(dst, text...)
	}
	key := styledRunKey{
		text: string(text),
		kind: kind,
	}
	rendered, ok := m.styledRuns[key]
	if !ok {
		rendered = m.highlightStyle(kind).Render(key.text)
		m.styledRuns[key] = rendered
	}
	return append(dst, rendered...)
}

func (m *Model) styleTail(dst []byte, start int, kind highlightKind) []byte {
	tail := dst[start:]
	dst = dst[:start]
	return m.appendStyledRun(dst, tail, kind)
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
	styledKind := highlightNone

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
				dst = model.styleTail(dst, styledStart, styledKind)
				styledStart = -1
				styledKind = highlightNone
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
		kind := highlightNone
		if model != nil {
			kind = model.highlightedRangeKind(
				documentY,
				uint32(visibleStart),
				uint32(visibleEnd),
			)
		}
		if kind != styledKind && styledStart >= 0 {
			dst = model.styleTail(dst, styledStart, styledKind)
			styledStart = -1
		}
		if kind != highlightNone && styledStart < 0 {
			styledStart = len(dst)
			styledKind = kind
		}
		dst = append(dst, visible...)
		screenX += int(visibleEnd - visibleStart)
	}
	if styledStart >= 0 {
		dst = model.styleTail(dst, styledStart, styledKind)
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

func (m *Model) highlightedRangeKind(y, start, end uint32) highlightKind {
	kind := highlightNone
	for x := start; x < end; x++ {
		candidate := m.highlightKindAt(layout.NewPoint(x, y))
		if candidate > kind {
			kind = candidate
		}
	}
	return kind
}

func (m *Model) highlightedPoint(point layout.Point) bool {
	return m.highlightKindAt(point) != highlightNone
}

func (m *Model) highlightKindAt(point layout.Point) highlightKind {
	gesture := m.interaction.gesture
	if gesture.hasAttachment {
		geo := m.geo
		frame := canvasview.BaseFrame
		if gesture.duplicateActive() {
			geo = m.interaction.render.duplicateLayout
			frame = canvasview.DuplicateFrame
		}
		owner, ok := m.canvas.OwnerAt(frame, point)
		if ok && owner == (layout.Hit{
			ID:   gesture.attachmentEdge,
			Kind: layout.HitEdge,
		}) && geo.EdgeExists(gesture.attachmentEdge) {
			return highlightCandidateEdge
		}
	}
	if m.interaction.gesture.duplicateActive() {
		if highlightContains(
			m.interaction.render.duplicateHighlight,
			m.canvas.Frame(canvasview.DuplicateFrame),
			point,
		) {
			return highlightSelection
		}
		return highlightNone
	}
	if m.interaction.gesture.kind == gestureAreaSelection &&
		m.marqueeArea().contains(point) {
		return highlightSelection
	}
	if m.interaction.tool == toolConnect {
		if m.portAt(point) {
			return highlightPort
		}
		return highlightNone
	}
	geo := m.geo
	if m.interaction.gesture.duplicateActive() {
		geo = m.interaction.render.duplicateLayout
	}
	if !geo.Selection().Empty() {
		if highlightContains(
			m.interaction.render.selectionHighlight,
			m.canvas.Frame(canvasview.BaseFrame),
			point,
		) {
			return highlightSelection
		}
		return highlightNone
	}
	hit, ok := m.primaryHighlight()
	if !ok {
		return highlightNone
	}
	if m.highlightForHit(hit, point) {
		return highlightSelection
	}
	return highlightNone
}

func (m *Model) highlightForHit(hit layout.Hit, point layout.Point) bool {
	geo := m.geo
	frame := canvasview.BaseFrame
	if m.interaction.gesture.duplicateActive() {
		geo = m.interaction.render.duplicateLayout
		frame = canvasview.DuplicateFrame
	}
	switch hit.Kind {
	case layout.HitNode:
		return geo.NodeExists(hit.ID) &&
			geo.Nodes[hit.ID].Rect.OnBoundary(point)
	case layout.HitPort:
		return false
	case layout.HitEdge:
		if !geo.EdgeExists(hit.ID) {
			return false
		}
		owner, ok := m.canvas.OwnerAt(frame, point)
		return ok && owner == hit
	}
	return false
}

func (m *Model) portAt(point layout.Point) bool {
	_, ok := m.usablePortAt(point)
	return ok
}

func (m *Model) usablePortAt(point layout.Point) (uint32, bool) {
	return m.geo.UsablePortAt(point)
}

func (m *Model) connectionPreviewConnections(
	point layout.Point,
) (layout.Connections, bool) {
	return rasterConnections(m.interaction.render.connectionRaster, point)
}

func rasterConnections(
	raster []layout.RasterCell,
	point layout.Point,
) (layout.Connections, bool) {
	index, ok := slices.BinarySearchFunc(
		raster,
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
	return raster[index].Connections, true
}

func (m *Model) refreshConnectionPreview() {
	if m.interaction.session.kind != sessionConnection {
		m.interaction.render.connectionPreview = m.interaction.render.connectionPreview[:0]
		m.interaction.render.connectionRaster = m.interaction.render.connectionRaster[:0]
		return
	}
	connection := m.interaction.session.connection
	if !m.geo.PortExists(connection.source) {
		m.interaction.render.connectionPreview = m.interaction.render.connectionPreview[:0]
		m.interaction.render.connectionRaster = m.interaction.render.connectionRaster[:0]
		return
	}
	var (
		preview     []layout.Point
		err         error
		destination = layout.NoPortID
		target      = m.cursor
	)
	if portID, ok := m.nearestConnectionPort(m.cursor, connection.source); ok {
		destination = portID
		target = m.geo.Ports[portID].Anchor
	}
	style := m.connectionPreviewStyle()
	if connection.reconnect {
		preview, err = m.geo.PreviewRouteWithoutEdgeStyled(
			m.interaction.render.connectionPreview[:0],
			connection.source,
			target,
			connection.edge,
			style,
		)
	} else {
		preview, err = m.geo.PreviewRouteStyled(
			m.interaction.render.connectionPreview[:0],
			connection.source,
			target,
			style,
		)
	}
	if err != nil {
		m.interaction.render.connectionPreview = m.interaction.render.connectionPreview[:0]
		m.interaction.render.connectionRaster = m.interaction.render.connectionRaster[:0]
		return
	}
	m.interaction.render.connectionPreview = preview
	m.interaction.render.connectionRaster, err = m.canvas.RasterizeEdge(
		m.interaction.render.connectionRaster[:0],
		m.geo,
		layout.RasterEdge{
			Points: preview,
			PortA:  connection.source,
			PortB:  destination,
		},
	)
	if err != nil {
		m.interaction.render.connectionPreview = m.interaction.render.connectionPreview[:0]
		m.interaction.render.connectionRaster = m.interaction.render.connectionRaster[:0]
		return
	}
}

func (m *Model) refreshBendPreview() {
	if m.interaction.session.kind != sessionBend {
		m.interaction.render.bendPreview = m.interaction.render.bendPreview[:0]
		m.interaction.render.bendRaster = m.interaction.render.bendRaster[:0]
		return
	}
	session := &m.interaction.session.bend
	if session.multiple() {
		preview := m.interaction.render.bendLayout
		for _, target := range session.targets {
			if err := preview.SetPinnedBends(target.edge, target.bends); err != nil {
				session.valid = false
				m.status = err.Error()
				_ = m.render()
				return
			}
		}
		if err := preview.BuildSelection(); err != nil {
			session.valid = false
			m.status = err.Error()
			_ = m.render()
			return
		}
		if err := m.canvas.Render(canvasview.ConnectionFrame, preview); err != nil {
			session.valid = false
			m.status = err.Error()
			_ = m.render()
			return
		}
		m.interaction.render.bendPreview = m.interaction.render.bendPreview[:0]
		m.interaction.render.bendRaster = m.interaction.render.bendRaster[:0]
		m.interaction.render.selectionHighlight = appendSelectionHighlight(
			m.interaction.render.selectionHighlight[:0],
			preview,
			&m.canvas,
			canvasview.ConnectionFrame,
		)
		session.valid = true
		m.status = ""
		return
	}
	primary := session.primary()
	preview, err := m.geo.PreviewPinnedBends(
		m.interaction.render.bendPreview[:0],
		primary.edge,
		primary.bends,
	)
	if err != nil {
		session.valid = false
		m.interaction.render.bendPreview = m.interaction.render.bendPreview[:0]
		m.interaction.render.bendRaster = m.interaction.render.bendRaster[:0]
		m.status = err.Error()
		return
	}
	portA, portB, err := m.geo.EdgePorts(primary.edge)
	if err != nil {
		session.valid = false
		m.interaction.render.bendPreview = m.interaction.render.bendPreview[:0]
		m.interaction.render.bendRaster = m.interaction.render.bendRaster[:0]
		m.status = err.Error()
		return
	}
	m.interaction.render.bendPreview = preview
	m.interaction.render.bendRaster, err = m.canvas.RasterizeEdge(
		m.interaction.render.bendRaster[:0],
		m.geo,
		layout.RasterEdge{
			Points: preview,
			PortA:  portA,
			PortB:  portB,
		},
	)
	if err != nil {
		session.valid = false
		m.interaction.render.bendPreview = m.interaction.render.bendPreview[:0]
		m.interaction.render.bendRaster = m.interaction.render.bendRaster[:0]
		m.status = err.Error()
		return
	}
	session.valid = true
	m.status = ""
}

func (m *Model) primaryHighlight() (layout.Hit, bool) {
	switch m.interaction.session.kind {
	case sessionLabelEdit:
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
		model.interaction.gesture.kind != gestureAreaSelection &&
			model.interaction.session.kind != sessionConnection &&
			model.interaction.session.kind != sessionBend {
		return appendSpaces(dst, count)
	}
	styledStart := -1
	styledKind := highlightNone
	for offset := range max(count, 0) {
		x := startX + uint64(offset)
		var text []byte
		if preview, ok := previewGlyph(model, x, y); ok {
			text = utf8.AppendRune(text, preview)
		} else {
			text = append(text, ' ')
		}
		kind := highlightNone
		if x <= math.MaxUint32 && y <= math.MaxUint32 {
			kind = model.highlightKindAt(
				layout.NewPoint(uint32(x), uint32(y)),
			)
		}
		if kind != styledKind && styledStart >= 0 {
			dst = model.styleTail(dst, styledStart, styledKind)
			styledStart = -1
		}
		if kind != highlightNone && styledStart < 0 {
			styledStart = len(dst)
			styledKind = kind
		}
		dst = append(dst, text...)
	}
	if styledStart >= 0 {
		dst = model.styleTail(dst, styledStart, styledKind)
	}
	return dst
}

func (m *Model) highlightStyle(kind highlightKind) lipgloss.Style {
	switch kind {
	case highlightPort:
		return m.canvas.PortStyle()
	case highlightCandidateEdge:
		return m.theme.CandidateEdge
	default:
		return m.canvas.SelectionStyle()
	}
}

func previewGlyph(model *Model, x, y uint64) (rune, bool) {
	if model == nil || x > math.MaxUint32 || y > math.MaxUint32 {
		return 0, false
	}
	point := layout.NewPoint(uint32(x), uint32(y))
	points, raster, style := model.activeEdgePreview()
	connections, ok := rasterConnections(raster, point)
	if !ok {
		return 0, false
	}
	if glyph, ok := render.ArrowGlyphAt(
		points,
		style,
		point,
	); ok {
		return glyph, true
	}
	return render.StrokeGlyph(
		connections,
		style.Stroke,
	), true
}

func (m *Model) activeEdgePreview() (
	[]layout.Point,
	[]layout.RasterCell,
	layout.EdgeStyle,
) {
	if m.interaction.session.kind == sessionBend {
		if m.interaction.session.bend.multiple() {
			return nil, nil, layout.EdgeStyle{}
		}
		style, _ := m.geo.EdgeStyle(m.interaction.session.bend.primary().edge)
		return m.interaction.render.bendPreview,
			m.interaction.render.bendRaster,
			style
	}
	return m.interaction.render.connectionPreview,
		m.interaction.render.connectionRaster,
		m.connectionPreviewStyle()
}

func (m *Model) connectionPreviewStyle() layout.EdgeStyle {
	connection := m.interaction.session.connection
	if !connection.reconnect {
		return m.edgeStyle
	}
	style, _ := m.geo.EdgeStyle(connection.edge)
	portA, _, err := m.geo.EdgePorts(connection.edge)
	if err == nil && connection.source != portA {
		style.PortAArrow, style.PortBArrow = style.PortBArrow, style.PortAArrow
	}
	return style
}
