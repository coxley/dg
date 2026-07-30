package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
)

const (
	helpMinimumWidth  = 24
	helpMinimumHeight = 5
	helpDefaultWidth  = 38
	helpDefaultHeight = 12
)

type helpInspector struct {
	viewport *chrome.Viewport
	pane     *chrome.Pane
	styles   helpStyles

	visible     bool
	requested   chrome.Rect
	positioned  bool
	dragPending bool
	dragging    bool
	dragOffset  chrome.Point
	resizing    bool
	resizeStart chrome.Point
	resizeRect  chrome.Rect
}

func newHelpInspector(styles helpStyles) helpInspector {
	viewport := chrome.NewViewport("help-body")
	viewport.SetScrollbars(chrome.ScrollbarNever, chrome.ScrollbarAutomatic)
	pane := chrome.NewPane("help-pane", viewport)
	inspector := helpInspector{
		viewport: viewport,
		pane:     pane,
	}
	inspector.setStyles(styles)
	return inspector
}

func (h *helpInspector) setStyles(styles helpStyles) {
	h.styles = styles
	h.viewport.SetScrollbarStyles(styles.Scrollbar)
	h.pane.SetFooter([]string{
		styles.Footer.Render("? hide · wheel scroll"),
	})
	h.applyContainerStyle()
}

func (h *helpInspector) toggle() {
	h.visible = !h.visible
}

func (h *helpInspector) hide() {
	h.visible = false
	h.release()
}

func (h *helpInspector) declaration(bounds chrome.Rect) chrome.Surface {
	if !h.positioned {
		width := min(helpDefaultWidth, bounds.Width)
		height := min(helpDefaultHeight, bounds.Height)
		h.requested = chrome.Rect{
			X:      bounds.Right() - width,
			Y:      bounds.Bottom() - height,
			Width:  width,
			Height: height,
		}
	}
	return chrome.Surface{
		ID:        surfaceHelp,
		Role:      chrome.SurfacePassive,
		Anchor:    chrome.AnchorTerminal,
		Requested: h.requested,
		Priority:  surfacePriorityHelp,
		Visible:   h.visible,
	}
}

func (h *helpInspector) setPlan(
	rect chrome.Rect,
	context string,
	bindings []chrome.EffectiveBinding,
	vocabulary chrome.ChordVocabulary,
) {
	h.pane.SetHeader([]string{
		h.styles.Header.Render("HELP · " + context),
	})
	lines := make([]string, 0, len(bindings)+1)
	for _, binding := range bindings {
		key := fmt.Sprintf(
			"%-14s",
			chrome.DisplayChord(binding.Chord, vocabulary),
		)
		lines = append(
			lines,
			h.styles.Key.Render(key)+
				h.styles.Description.Render(binding.Label),
		)
	}
	if len(lines) == 0 {
		lines = append(lines, h.styles.Description.Render("No active bindings"))
	}
	h.viewport.SetContent(lines)
	h.pane.SetBounds(chrome.Rect{Width: rect.Width, Height: rect.Height})
}

func (h *helpInspector) lines() []string {
	if !h.visible {
		return nil
	}
	h.applyContainerStyle()
	return h.pane.Lines()
}

func (h *helpInspector) update(message tea.Msg, rect chrome.Rect) bool {
	switch message := message.(type) {
	case tea.MouseClickMsg:
		if !rect.Contains(chrome.Point{X: message.X, Y: message.Y}) {
			return false
		}
		local := chrome.Point{X: message.X - rect.X, Y: message.Y - rect.Y}
		switch message.Button {
		case tea.MouseLeft:
			if h.viewport.BeginScrollbarDrag(local) {
				return true
			}
			if message.Y == rect.Y {
				h.dragPending = true
				h.dragOffset = chrome.Point{X: message.X - rect.X, Y: message.Y - rect.Y}
			}
		case tea.MouseRight:
			h.resizing = true
			h.resizeStart = chrome.Point{X: message.X, Y: message.Y}
			h.resizeRect = rect
		}
	case tea.MouseMotionMsg:
		h.viewport.HoverScrollbar(chrome.Point{
			X: message.X - rect.X,
			Y: message.Y - rect.Y,
		})
		switch {
		case h.resizing && message.Button == tea.MouseRight:
			h.requested = resizeHelpRect(
				h.resizeRect,
				message.X-h.resizeStart.X,
				message.Y-h.resizeStart.Y,
			)
			h.positioned = true
			return true
		case h.viewport.ScrollbarDragging() && message.Button == tea.MouseLeft:
			return h.viewport.DragScrollbar(chrome.Point{
				X: message.X - rect.X,
				Y: message.Y - rect.Y,
			})
		case h.dragPending && message.Button == tea.MouseLeft:
			h.dragPending = false
			h.dragging = true
		}
		if h.dragging && message.Button == tea.MouseLeft {
			h.requested.X = message.X - h.dragOffset.X
			h.requested.Y = message.Y - h.dragOffset.Y
			h.requested.Width = rect.Width
			h.requested.Height = rect.Height
			h.positioned = true
			return true
		}
	case tea.MouseReleaseMsg:
		h.viewport.EndScrollbarDrag()
		h.release()
	case tea.MouseWheelMsg:
		if !rect.Contains(chrome.Point{X: message.X, Y: message.Y}) {
			return false
		}
		switch message.Button {
		case tea.MouseWheelUp:
			h.viewport.Scroll(0, -1)
		case tea.MouseWheelDown:
			h.viewport.Scroll(0, 1)
		}
	}
	return false
}

func (h *helpInspector) capturesPointer() bool {
	return h.dragPending || h.dragging || h.resizing ||
		h.viewport.ScrollbarDragging()
}

func (h *helpInspector) release() {
	h.dragPending = false
	h.dragging = false
	h.resizing = false
}

func (h *helpInspector) clearHover() {
	h.viewport.ClearScrollbarHover()
}

func (h *helpInspector) applyContainerStyle() {
	style := h.styles.Container
	if h.dragging || h.resizing {
		style = h.styles.ActiveContainer
	}
	h.pane.SetStyle(style)
}

func resizeHelpRect(rect chrome.Rect, dx, dy int) chrome.Rect {
	rect.Width = max(rect.Width+dx, helpMinimumWidth)
	rect.Height = max(rect.Height+dy, helpMinimumHeight)
	return rect
}

func renderSurfaceLines(lines []string) string {
	return strings.TrimSuffix(strings.Join(lines, "\n"), "\n")
}
