package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	modalview "github.com/coxley/dg/internal/tui/modal"
)

const (
	surfaceCanvas     chrome.SurfaceID = "canvas"
	surfaceNavigation chrome.SurfaceID = "navigation"
	surfaceHelp       chrome.SurfaceID = "help"
	surfaceModal      chrome.SurfaceID = "modal"
)

const (
	surfacePriorityCanvas     = 0
	surfacePriorityNavigation = 10
	surfacePriorityHelp       = 20
	surfacePriorityModal      = 30
)

func (m *Model) syncWorkspace() {
	plan := m.workspace.Plan()
	m.nav.SetWidth(plan.Canvas.Width)
	overlay := m.currentModalOverlay()
	surfaces := []chrome.Surface{
		{
			ID:        surfaceCanvas,
			Role:      chrome.SurfacePassive,
			Anchor:    chrome.AnchorCanvas,
			Requested: chrome.Rect{Width: plan.Canvas.Width, Height: plan.Canvas.Height},
			Priority:  surfacePriorityCanvas,
			Visible:   plan.Canvas.Width != 0 && plan.Canvas.Height != 0,
		},
		{
			ID:        surfaceNavigation,
			Role:      chrome.SurfaceFloating,
			Anchor:    chrome.AnchorCanvas,
			Requested: m.nav.Bounds(),
			Priority:  surfacePriorityNavigation,
			Visible:   m.nav.Bounds().Width != 0,
		},
		m.helpInspector.declaration(plan.Main),
		{
			ID:             surfaceModal,
			Role:           chrome.SurfaceModal,
			Anchor:         chrome.AnchorTerminal,
			Requested:      overlayRect(overlay),
			Priority:       surfacePriorityModal,
			Visible:        m.modal != modalNone && overlay.Width != 0,
			DismissOutside: m.modal != modalNotice,
			DismissBack:    true,
			FocusOnOpen:    true,
		},
	}
	if err := m.workspace.SetSurfaces(surfaces); err != nil {
		m.setError("arrange workspace: " + err.Error())
		return
	}
	if help, ok := m.surfacePlan(surfaceHelp); ok {
		m.helpInspector.setPlan(
			help.Rect,
			m.helpContext(),
			m.bindings.Effective(m.activeBindingScopes()),
		)
	}
}

func (m *Model) surfacePlan(id chrome.SurfaceID) (chrome.SurfacePlan, bool) {
	return m.workspace.Surface(id)
}

func (m *Model) helpContext() string {
	switch {
	case m.modal == modalPreferences &&
		m.preferenceForm != nil &&
		m.preferenceForm.DirectoryOpen():
		return "directory picker"
	case m.modal == modalPreferences:
		return "preferences"
	case m.modal == modalSave:
		return "save"
	case m.modal == modalExport:
		return "export"
	case m.modal == modalNotice:
		return "notice"
	case m.mode == modeEditLabel:
		return "label editor"
	default:
		return "canvas"
	}
}

func (m *Model) textEntryActive() bool {
	return m.mode == modeEditLabel || m.modal == modalSave
}

func (m *Model) dismissSurface(id chrome.SurfaceID) {
	switch id {
	case surfaceHelp:
		m.helpInspector.hide()
	case surfaceModal:
		m.closeModal()
	}
}

func (m *Model) updateSurfaceMouseClick(message tea.MouseClickMsg) tea.Cmd {
	point := chrome.Point{X: message.X, Y: message.Y}
	if id, ok := m.workspace.DismissAt(point); ok {
		m.dismissSurface(id)
		return nil
	}
	id, ok := m.workspace.SurfaceAt(point)
	if !ok {
		if !m.workspace.PointerBlocked(point) {
			m.updateMouseClick(message.Mouse())
		}
		return nil
	}
	switch id {
	case surfaceCanvas:
		m.updateMouseClick(message.Mouse())
		m.workspace.Capture(surfaceCanvas)
	case surfaceNavigation:
		var command tea.Cmd
		m.nav, command = m.nav.Update(m.navigationMessage(message))
		return command
	case surfaceHelp:
		plan, _ := m.surfacePlan(surfaceHelp)
		m.helpInspector.update(message, plan.Rect)
		if m.helpInspector.capturesPointer() {
			m.workspace.Capture(surfaceHelp)
		}
	case surfaceModal:
		command := m.updateModalMouseClick(message.Mouse())
		if m.dialog.CapturesPointer() {
			m.workspace.Capture(surfaceModal)
		}
		return command
	}
	return nil
}

func (m *Model) updateSurfaceMouseMotion(message tea.MouseMotionMsg) tea.Cmd {
	id, ok := m.workspace.SurfaceAt(chrome.Point{X: message.X, Y: message.Y})
	if ok {
		switch id {
		case surfaceCanvas:
			m.updateMouseMotion(message.Mouse())
		case surfaceNavigation:
			m.nav, _ = m.nav.Update(m.navigationMessage(message))
		case surfaceHelp:
			plan, _ := m.surfacePlan(surfaceHelp)
			m.helpInspector.update(message, plan.Rect)
		case surfaceModal:
			return m.updateModalMouseMotion(message.Mouse())
		}
		return nil
	}
	m.nav, _ = m.nav.Update(message)
	if !m.workspace.PointerBlocked(chrome.Point{X: message.X, Y: message.Y}) {
		m.updateMouseMotion(message.Mouse())
	}
	return nil
}

func (m *Model) updateSurfaceMouseRelease(message tea.MouseReleaseMsg) {
	id := m.workspace.CaptureID()
	switch id {
	case surfaceCanvas:
		m.updateMouseRelease(message.Mouse())
	case surfaceHelp:
		plan, _ := m.surfacePlan(surfaceHelp)
		m.helpInspector.update(message, plan.Rect)
	case surfaceModal:
		m.dialog, _ = m.dialog.Update(message)
	default:
		m.updateMouseRelease(message.Mouse())
	}
	m.workspace.Release()
}

func (m *Model) updateSurfaceMouseWheel(message tea.MouseWheelMsg) tea.Cmd {
	point := chrome.Point{X: message.X, Y: message.Y}
	id, ok := m.workspace.SurfaceAt(point)
	if ok {
		switch id {
		case surfaceCanvas:
			m.updateMouseWheel(message.Mouse())
		case surfaceHelp:
			plan, _ := m.surfacePlan(surfaceHelp)
			m.helpInspector.update(message, plan.Rect)
		case surfaceModal:
			if m.modal == modalPreferences {
				return m.updateSettingsWheel(message)
			}
			if m.modal == modalExport {
				return m.updateClipboard(message)
			}
		}
		return nil
	}
	if !m.workspace.PointerBlocked(point) {
		m.updateMouseWheel(message.Mouse())
	}
	return nil
}

func overlayRect(overlay modalview.Overlay) chrome.Rect {
	return chrome.Rect{
		X: overlay.Left, Y: overlay.Top,
		Width: overlay.Width, Height: overlay.Height,
	}
}

func (m *Model) navigationMessage(message tea.Msg) tea.Msg {
	surface, ok := m.surfacePlan(surfaceNavigation)
	if !ok {
		return message
	}
	switch message := message.(type) {
	case tea.MouseClickMsg:
		mouse := message.Mouse()
		mouse.X -= surface.Anchor.X
		mouse.Y -= surface.Anchor.Y
		return tea.MouseClickMsg(mouse)
	case tea.MouseMotionMsg:
		mouse := message.Mouse()
		mouse.X -= surface.Anchor.X
		mouse.Y -= surface.Anchor.Y
		return tea.MouseMotionMsg(mouse)
	default:
		return message
	}
}
