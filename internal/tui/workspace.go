package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	modalview "github.com/coxley/dg/internal/tui/modal"
)

const (
	surfaceCanvas      chrome.SurfaceID = "canvas"
	surfaceNavigation  chrome.SurfaceID = "navigation"
	surfaceHelp        chrome.SurfaceID = "help"
	surfaceSidebar     chrome.SurfaceID = "sidebar"
	surfaceNone        chrome.SurfaceID = ""
	surfacePreferences chrome.SurfaceID = "preferences-dialog"
	surfaceSave        chrome.SurfaceID = "save-dialog"
	surfaceExport      chrome.SurfaceID = "export-dialog"
	surfaceNotice      chrome.SurfaceID = "notice-dialog"
)

const (
	surfacePriorityCanvas     = 0
	surfacePrioritySidebar    = 5
	surfacePriorityNavigation = 10
	surfacePriorityDrawer     = 15
	surfacePriorityHelp       = 20
	surfacePriorityModal      = 30
)

func (m *Model) syncWorkspace() {
	plan := m.workspace.Plan()
	m.nav.SetWidth(plan.Main.Width)
	overlay := m.dialogs.Arrange(
		m.width,
		m.height,
		m.nav.Bounds().Bottom(),
	)
	surfaces := make([]chrome.Surface, 0, 4+len(dialogSpecs))
	surfaces = append(
		surfaces,
		chrome.Surface{
			ID:        surfaceCanvas,
			Role:      chrome.SurfacePassive,
			Anchor:    chrome.AnchorCanvas,
			Requested: chrome.Rect{Width: m.width, Height: m.height},
			Priority:  surfacePriorityCanvas,
			Visible:   m.width != 0 && m.height != 0,
		},
		chrome.Surface{
			ID:             surfaceSidebar,
			Role:           m.sidebar.surfaceRole(),
			Anchor:         chrome.AnchorWorkspace,
			Dock:           chrome.DockLeft,
			Requested:      chrome.Rect{Width: m.sidebarTargetWidth(), Height: plan.Main.Height},
			Priority:       m.sidebar.priority(),
			Visible:        m.sidebar.open || m.workspace.SurfacePosition(surfaceSidebar) != 0,
			Animated:       true,
			DismissOutside: m.sidebar.placement == sidebarDrawer,
			DismissBack:    m.sidebar.placement == sidebarDrawer,
			FocusOnOpen:    true,
		},
		chrome.Surface{
			ID:        surfaceNavigation,
			Role:      chrome.SurfaceFloating,
			Anchor:    chrome.AnchorWorkspace,
			Requested: m.nav.Bounds(),
			Priority:  surfacePriorityNavigation,
			Visible:   m.nav.Bounds().Width != 0,
		},
		m.helpInspector.declaration(plan.Main),
	)
	for _, spec := range dialogSpecs {
		surfaces = append(surfaces, chrome.Surface{
			ID:             spec.ID,
			Role:           chrome.SurfaceModal,
			Anchor:         chrome.AnchorTerminal,
			Requested:      overlayRect(overlay),
			Priority:       surfacePriorityModal,
			Visible:        m.dialogs.ActiveID() == spec.ID && overlay.Width != 0,
			DismissOutside: spec.DismissOutside,
			DismissBack:    true,
			FocusOnOpen:    true,
		})
	}
	if err := m.workspace.SetSurfaces(surfaces); err != nil {
		m.setError("arrange workspace: " + err.Error())
		return
	}
	if sidebar, ok := m.surfacePlan(surfaceSidebar); ok {
		m.sidebar.setBounds(sidebar.Content)
	}
	if help, ok := m.surfacePlan(surfaceHelp); ok {
		m.helpInspector.setPlan(
			help.Rect,
			m.helpContext(),
			m.contextualHelpBindings(),
			chrome.VocabularyForProfile(m.preferenceValue().KeyProfile),
		)
	}
}

func (m *Model) surfacePlan(id chrome.SurfaceID) (chrome.SurfacePlan, bool) {
	return m.workspace.Surface(id)
}

func (m *Model) helpContext() string {
	if m.dialogs.ActiveID() != surfaceNone {
		return m.dialogs.Context()
	}
	if m.sidebar.focused {
		return "sidebar"
	}
	if m.interaction.session.kind == sessionLabelEdit {
		return "label editor"
	}
	return string(surfaceCanvas)
}

func (m *Model) textEntryActive() bool {
	if m.dialogs.ActiveID() != surfaceNone {
		return m.dialogs.TextEntry()
	}
	return m.interaction.session.kind == sessionLabelEdit
}

func (m *Model) dismissSurface(id chrome.SurfaceID) tea.Cmd {
	switch id {
	case surfaceHelp:
		m.helpInspector.hide()
		return nil
	case surfaceSidebar:
		return m.dismissSidebar()
	default:
		if id == m.dialogs.ActiveID() {
			return m.dismissDialog()
		}
		return nil
	}
}

func (m *Model) updateSurfaceMouseClick(message tea.MouseClickMsg) tea.Cmd {
	point := chrome.Point{X: message.X, Y: message.Y}
	if id, ok := m.workspace.DismissAt(point); ok {
		return m.dismissSurface(id)
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
		if m.sidebar.placement == sidebarDocked {
			m.sidebar.blur()
		}
		m.updateMouseClick(message.Mouse())
		m.workspace.Capture(surfaceCanvas)
	case surfaceSidebar:
		surface, _ := m.surfacePlan(surfaceSidebar)
		m.sidebar.click(point, surface)
		if m.sidebar.capturesPointer() {
			m.workspace.Capture(surfaceSidebar)
		}
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
	default:
		if id != m.dialogs.ActiveID() {
			return nil
		}
		command := m.updateDialogMouseClick(message.Mouse())
		if m.dialogs.CapturesPointer() {
			m.workspace.Capture(id)
		}
		return command
	}
	return nil
}

func (m *Model) updateSurfaceMouseMotion(message tea.MouseMotionMsg) tea.Cmd {
	m.helpInspector.clearHover()
	m.sidebar.clearHover()
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
		case surfaceSidebar:
			surface, _ := m.surfacePlan(surfaceSidebar)
			m.sidebar.motion(chrome.Point{X: message.X, Y: message.Y}, surface)
		default:
			if id == m.dialogs.ActiveID() {
				return m.updateDialogMouseMotion(message.Mouse())
			}
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
	case surfaceSidebar:
		m.sidebar.release()
	default:
		if id != m.dialogs.ActiveID() {
			m.updateMouseRelease(message.Mouse())
			break
		}
		m.dialogs.Release(message)
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
		case surfaceSidebar:
			switch message.Button {
			case tea.MouseWheelUp:
				m.sidebar.scroll(-1)
			case tea.MouseWheelDown:
				m.sidebar.scroll(1)
			}
		default:
			if id == m.dialogs.ActiveID() {
				return m.updateDialogWheel(message)
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
