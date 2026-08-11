package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	modalview "github.com/coxley/dg/internal/tui/modal"
)

const (
	surfaceCanvas       chrome.SurfaceID = "canvas"
	surfaceNavigation   chrome.SurfaceID = "navigation"
	surfaceArrange      chrome.SurfaceID = "arrange"
	surfaceHelp         chrome.SurfaceID = "help"
	surfaceUpdate       chrome.SurfaceID = "update"
	surfaceSidebar      chrome.SurfaceID = "sidebar"
	surfaceNone         chrome.SurfaceID = ""
	surfacePreferences  chrome.SurfaceID = "preferences-dialog"
	surfaceSave         chrome.SurfaceID = "save-dialog"
	surfaceExport       chrome.SurfaceID = "export-dialog"
	surfaceNotice       chrome.SurfaceID = "notice-dialog"
	surfaceConfirmation chrome.SurfaceID = "confirmation-dialog"
)

const (
	surfacePriorityCanvas     = 0
	surfacePrioritySidebar    = 5
	surfacePriorityNavigation = 10
	surfacePriorityArrange    = 11
	surfacePriorityDrawer     = 15
	surfacePriorityHelp       = 20
	surfacePriorityUpdate     = 25
	surfacePriorityModal      = 30
)

func (m *Model) syncWorkspace() {
	if m.arrangeOpen && !m.arrangeSelectionAvailable() {
		m.cancelArrange()
	}
	plan := m.workspace.Plan()
	m.nav.SetWidth(plan.Main.Width)
	overlay := m.dialogs.Arrange(
		m.width,
		m.height,
		m.nav.Bounds().Bottom(),
	)
	updateView := m.updateNotice.view()
	updateWidth := lipgloss.Width(updateView)
	updateHeight := lipgloss.Height(updateView)
	arrangeBounds := m.arrange.Bounds()
	arrangeBounds = placeArrangeForm(arrangeBounds, m.nav.Bounds(), plan.Main)
	surfaces := make([]chrome.Surface, 0, 6+len(dialogSpecs))
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
			FocusOnOpen:    m.sidebar.focused,
		},
		chrome.Surface{
			ID:        surfaceNavigation,
			Role:      chrome.SurfaceFloating,
			Anchor:    chrome.AnchorTerminal,
			Requested: m.nav.Bounds(),
			Priority:  surfacePriorityNavigation,
			Visible:   m.nav.Bounds().Width != 0,
		},
		chrome.Surface{
			ID:             surfaceArrange,
			Role:           chrome.SurfaceFloating,
			Anchor:         chrome.AnchorTerminal,
			Requested:      arrangeBounds,
			Priority:       surfacePriorityArrange,
			Visible:        m.arrangeOpen && arrangeBounds.Width != 0,
			DismissOutside: true,
			DismissBack:    true,
		},
		m.helpInspector.declaration(plan.Main),
		chrome.Surface{
			ID:       surfaceUpdate,
			Role:     chrome.SurfaceFloating,
			Anchor:   chrome.AnchorTerminal,
			Priority: surfacePriorityUpdate,
			Requested: chrome.Rect{
				X:      max(m.width-updateWidth, 0),
				Width:  updateWidth,
				Height: updateHeight,
			},
			Visible: m.updateNotice.visible(),
		},
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
			chrome.VocabularyForProfile(chrome.ProfileAuto),
		)
	}
}

func placeArrangeForm(form, navigation, workspace chrome.Rect) chrome.Rect {
	form.X = navigation.Right() + 1
	form.Y = navigation.Y
	if form.Right() <= workspace.Right() {
		return form
	}
	form.X = navigation.X - form.Width - 1
	if form.X >= workspace.X {
		return form
	}
	form.X = max(workspace.Right()-form.Width, workspace.X)
	form.Y = navigation.Bottom() + 1
	if form.Bottom() > workspace.Bottom() {
		form.Y = max(navigation.Y-form.Height-1, workspace.Y)
	}
	return form
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
	case surfaceArrange:
		m.commitArrange()
		return nil
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
	if m.updateNotice.focused {
		m.updateNotice.blur()
	}
	point := chrome.Point{X: message.X, Y: message.Y}
	if id, ok := m.workspace.DismissAt(point); ok {
		command := m.dismissSurface(id)
		if id != surfaceArrange || m.arrangeOpen {
			return command
		}
	}
	id, ok := m.workspace.SurfaceAt(point)
	if !ok {
		if !m.workspace.PointerBlocked(point) {
			m.updateMouseClick(message.Mouse())
		}
		return nil
	}
	switch id {
	case surfaceUpdate:
		if message.Button == tea.MouseLeft {
			m.updateNotice.focus()
		}
	case surfaceCanvas:
		if m.sidebar.placement == sidebarDocked {
			m.sidebar.blur()
		}
		m.updateMouseClick(message.Mouse())
		m.workspace.Capture(surfaceCanvas)
	case surfaceSidebar:
		surface, _ := m.surfacePlan(surfaceSidebar)
		m.sidebar.show()
		if m.sidebar.click(point, surface) {
			if message.Button == tea.MouseLeft && m.sidebar.beginCanvasDrag(point) {
				m.workspace.Capture(surfaceSidebar)
				return nil
			}
			return m.activateSidebar()
		}
		if m.sidebar.capturesPointer() {
			m.workspace.Capture(surfaceSidebar)
		}
	case surfaceNavigation:
		var command tea.Cmd
		m.nav, command = m.nav.Update(m.navigationMessage(message))
		return command
	case surfaceArrange:
		local := m.arrangeMessage(message).(tea.MouseClickMsg)
		return m.arrange.Click(chrome.Point{X: local.X, Y: local.Y})
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
	id, ok := m.workspace.SurfaceAt(chrome.Point{X: message.X, Y: message.Y})
	if ok {
		if id != surfaceSidebar {
			m.sidebar.clearHover()
		}
		switch id {
		case surfaceCanvas:
			m.updateMouseMotion(message.Mouse())
		case surfaceNavigation:
			m.nav, _ = m.nav.Update(m.navigationMessage(message))
		case surfaceArrange:
			m.arrange.Update(m.arrangeMessage(message))
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
	m.sidebar.clearHover()
	m.nav, _ = m.nav.Update(message)
	m.arrange.Update(message)
	if !m.workspace.PointerBlocked(chrome.Point{X: message.X, Y: message.Y}) {
		m.updateMouseMotion(message.Mouse())
	}
	return nil
}

func (m *Model) updateSurfaceMouseRelease(message tea.MouseReleaseMsg) tea.Cmd {
	id := m.workspace.CaptureID()
	var command tea.Cmd
	switch id {
	case surfaceCanvas:
		m.updateMouseRelease(message.Mouse())
	case surfaceHelp:
		plan, _ := m.surfacePlan(surfaceHelp)
		m.helpInspector.update(message, plan.Rect)
	case surfaceSidebar:
		release := m.sidebar.release()
		switch {
		case release.dragged && release.valid && release.targetDrafts:
			entry := release.source.Entry
			m.demoteCanvas(entry, m.entry != nil && sameCanvas(*m.entry, entry))
			command = m.retargetSidebar()
		case release.dragged && release.valid:
			command = m.moveCanvasToSection(release.source.Entry, release.targetSection)
		case release.source.Kind != 0 && !release.dragged:
			command = m.activateSidebar()
		}
	default:
		if id != m.dialogs.ActiveID() {
			m.updateMouseRelease(message.Mouse())
			break
		}
		m.dialogs.Release(message)
	}
	m.workspace.Release()
	return command
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

func (m *Model) arrangeMessage(message tea.Msg) tea.Msg {
	surface, ok := m.surfacePlan(surfaceArrange)
	if !ok {
		return message
	}
	switch message := message.(type) {
	case tea.MouseClickMsg:
		mouse := message.Mouse()
		mouse.X -= surface.Rect.X
		mouse.Y -= surface.Rect.Y
		return tea.MouseClickMsg(mouse)
	case tea.MouseMotionMsg:
		mouse := message.Mouse()
		mouse.X -= surface.Rect.X
		mouse.Y -= surface.Rect.Y
		return tea.MouseMotionMsg(mouse)
	default:
		return message
	}
}
