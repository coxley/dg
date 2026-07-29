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

func newHelpInspector() helpInspector {
	viewport := chrome.NewViewport("help-body")
	viewport.SetScrollbars(chrome.ScrollbarNever, chrome.ScrollbarAutomatic)
	pane := chrome.NewPane("help-pane", viewport)
	pane.SetFooter([]string{"? hide · wheel scroll"})
	return helpInspector{viewport: viewport, pane: pane}
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

func (h *helpInspector) setPlan(rect chrome.Rect, context string, bindings []chrome.EffectiveBinding) {
	h.pane.SetHeader([]string{"HELP · " + context})
	lines := make([]string, 0, len(bindings)+1)
	for _, binding := range bindings {
		lines = append(lines, fmt.Sprintf("%-14s %s", binding.Chord, binding.Label))
	}
	if len(lines) == 0 {
		lines = append(lines, "No active bindings")
	}
	h.viewport.SetContent(lines)
	h.pane.SetBounds(chrome.Rect{Width: rect.Width, Height: rect.Height})
}

func (h *helpInspector) lines() []string {
	if !h.visible {
		return nil
	}
	return h.pane.Lines()
}

func (h *helpInspector) update(message tea.Msg, rect chrome.Rect) bool {
	switch message := message.(type) {
	case tea.MouseClickMsg:
		if !rect.Contains(chrome.Point{X: message.X, Y: message.Y}) {
			return false
		}
		switch message.Button {
		case tea.MouseLeft:
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
		switch {
		case h.resizing && message.Button == tea.MouseRight:
			h.requested = resizeHelpRect(
				h.resizeRect,
				message.X-h.resizeStart.X,
				message.Y-h.resizeStart.Y,
			)
			h.positioned = true
			return true
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
	return h.dragPending || h.dragging || h.resizing
}

func (h *helpInspector) release() {
	h.dragPending = false
	h.dragging = false
	h.resizing = false
}

func resizeHelpRect(rect chrome.Rect, dx, dy int) chrome.Rect {
	rect.Width = max(rect.Width+dx, helpMinimumWidth)
	rect.Height = max(rect.Height+dy, helpMinimumHeight)
	return rect
}

func renderSurfaceLines(lines []string) string {
	return strings.TrimSuffix(strings.Join(lines, "\n"), "\n")
}
