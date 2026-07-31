package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
	canvasstore "github.com/coxley/dg/store"
)

const (
	sidebarPreferredWidth = 30
	sidebarMinimumWidth   = 30
	sidebarCanvasMinimum  = 48
	compactWidthThreshold = sidebarMinimumWidth + sidebarCanvasMinimum
	sidebarMotionFPS      = 60
	sidebarMotionInterval = time.Second / sidebarMotionFPS
)

type sidebarPlacement uint8

const (
	sidebarDocked sidebarPlacement = iota
	sidebarDrawer
)

type sidebarItem struct {
	ID      chrome.FocusID
	Label   string
	Kind    sidebarItemKind
	Section string
	Entry   canvasstore.Entry
}

type sidebarItemKind uint8

const (
	sidebarItemRecord sidebarItemKind = iota + 1
	sidebarItemSection
	sidebarItemClearDrafts
)

type sidebarDeclaration struct {
	Header string
	Items  []sidebarItem
	Footer string
}

type sidebarState struct {
	declaration sidebarDeclaration
	styles      sidebarStyles
	viewport    *chrome.Viewport
	pane        *chrome.Pane
	focus       *chrome.FocusRegistry
	placement   sidebarPlacement
	open        bool
	focused     bool
	initialOpen bool
	generation  uint64
	drafts      bool
	collapsed   map[string]bool
	desired     int
}

type sidebarMotionMsg struct {
	generation uint64
	delta      time.Duration
}

func newSidebar(declaration sidebarDeclaration, styles sidebarStyles) sidebarState {
	viewport := chrome.NewViewport("sidebar-body")
	viewport.SetScrollbars(chrome.ScrollbarNever, chrome.ScrollbarAutomatic)
	viewport.SetScrollbarStyles(styles.Scrollbar)
	pane := chrome.NewPane("sidebar-pane", viewport)
	pane.SetStyle(styles.Container)
	focus := chrome.NewFocusRegistry()
	targets := make([]chrome.FocusTarget, len(declaration.Items))
	for i, item := range declaration.Items {
		targets[i] = chrome.FocusTarget{ID: item.ID, Enabled: true}
	}
	focus.Register(scopeCanvas, []chrome.FocusTarget{{
		ID: chrome.FocusID(surfaceCanvas), Enabled: true,
	}})
	focus.Register(scopeSidebar, targets)
	focus.Open(scopeCanvas)
	sidebar := sidebarState{
		declaration: declaration,
		styles:      styles,
		viewport:    viewport,
		pane:        pane,
		focus:       focus,
		collapsed:   make(map[string]bool),
	}
	sidebar.measure(nil)
	sidebar.render()
	return sidebar
}

func (s *sidebarState) surfaceRole() chrome.SurfaceRole {
	if s.placement == sidebarDocked {
		return chrome.SurfaceDock
	}
	return chrome.SurfaceDrawer
}

func (s *sidebarState) priority() int {
	if s.placement == sidebarDocked {
		return surfacePrioritySidebar
	}
	return surfacePriorityDrawer
}

func (s *sidebarState) setStyles(styles sidebarStyles) {
	s.styles = styles
	s.pane.SetStyle(styles.Container)
	s.viewport.SetScrollbarStyles(styles.Scrollbar)
	s.render()
}

func (s *sidebarState) setFooter(footer string) {
	s.declaration.Footer = footer
	s.render()
}

func (s *sidebarState) setBounds(bounds chrome.Rect) {
	s.pane.SetBounds(chrome.Rect{Width: bounds.Width, Height: bounds.Height})
	s.registerTargets()
}

func (s *sidebarState) lines(surface chrome.SurfacePlan) []string {
	lines := s.pane.Lines()
	offset := max(surface.Rect.X-surface.Content.X, 0)
	for i, line := range lines {
		line = ansi.Cut(line, offset, offset+surface.Rect.Width)
		lines[i] = line + strings.Repeat(
			" ",
			max(surface.Rect.Width-ansi.StringWidth(line), 0),
		)
	}
	return lines
}

func (s *sidebarState) show() {
	s.open = true
	if !s.focused {
		s.focus.Open(scopeSidebar)
		s.focused = true
	}
	s.render()
}

func (s *sidebarState) openInitially() {
	s.open = true
	s.initialOpen = true
	s.render()
}

func (s *sidebarState) hide() {
	s.open = false
	s.blur()
}

func (s *sidebarState) blur() {
	if !s.focused {
		return
	}
	s.focus.Close(scopeCanvas)
	s.focused = false
	s.render()
}

func (s *sidebarState) moveFocus(delta int) {
	if !s.focused {
		return
	}
	s.focus.Move(delta)
	s.focus.Reveal(s.viewport)
	s.render()
}

func (s *sidebarState) click(point chrome.Point, surface chrome.SurfacePlan) bool {
	local := chrome.Point{
		X: point.X - surface.Content.X,
		Y: point.Y - surface.Content.Y,
	}
	if s.viewport.BeginScrollbarDrag(local) {
		return false
	}
	body := s.pane.Plan().Body
	if !body.Contains(local) {
		return false
	}
	index := local.Y - body.Y + s.viewport.Plan().Offset.Y
	if index < 0 || index >= len(s.declaration.Items) {
		return false
	}
	s.show()
	for range len(s.declaration.Items) {
		_, focused := s.focus.Current()
		if focused == s.declaration.Items[index].ID {
			break
		}
		s.focus.Move(1)
	}
	s.render()
	return s.declaration.Items[index].Kind != 0
}

func (s *sidebarState) motion(point chrome.Point, surface chrome.SurfacePlan) {
	local := chrome.Point{
		X: point.X - surface.Content.X,
		Y: point.Y - surface.Content.Y,
	}
	s.viewport.HoverScrollbar(local)
	s.viewport.DragScrollbar(local)
}

func (s *sidebarState) release() {
	s.viewport.EndScrollbarDrag()
}

func (s *sidebarState) clearHover() {
	s.viewport.ClearScrollbarHover()
}

func (s *sidebarState) capturesPointer() bool {
	return s.viewport.ScrollbarDragging()
}

func (s *sidebarState) scroll(delta int) {
	s.viewport.Scroll(0, delta)
}

func (s *sidebarState) render() {
	container := s.styles.Container
	if s.focused {
		container = s.styles.FocusedContainer
	}
	s.pane.SetStyle(container)
	s.viewport.SetFocused(s.focused)
	s.pane.SetHeader([]string{s.styles.Header.Render(s.declaration.Header)})
	s.pane.SetFooter([]string{s.styles.Footer.Render(s.declaration.Footer)})
	_, focused := s.focus.Current()
	lines := make([]string, len(s.declaration.Items))
	for i, item := range s.declaration.Items {
		style := s.styles.Item
		if s.focused && item.ID == focused {
			style = s.styles.FocusedItem
		}
		lines[i] = style.Render(item.Label)
	}
	s.viewport.SetContent(lines)
}

func (s *sidebarState) setContent(header string, items []sidebarItem, allLabels []string) {
	s.declaration.Header = header
	s.declaration.Items = items
	s.measure(allLabels)
	s.registerTargets()
	s.render()
}

func (s *sidebarState) measure(allLabels []string) {
	width := max(
		ansi.StringWidth(s.declaration.Header)+s.styles.Header.GetHorizontalFrameSize(),
		ansi.StringWidth(s.declaration.Footer)+s.styles.Footer.GetHorizontalFrameSize(),
	)
	for _, item := range s.declaration.Items {
		width = max(width, ansi.StringWidth(item.Label)+s.styles.Item.GetHorizontalFrameSize())
	}
	for _, label := range allLabels {
		width = max(width, ansi.StringWidth(label)+s.styles.Item.GetHorizontalFrameSize())
	}
	s.desired = max(sidebarMinimumWidth, width+s.styles.Container.GetHorizontalFrameSize())
}

func (s *sidebarState) registerTargets() {
	body := s.pane.Plan().Body
	targets := make([]chrome.FocusTarget, len(s.declaration.Items))
	for i, item := range s.declaration.Items {
		targets[i] = chrome.FocusTarget{
			ID: item.ID,
			Rect: chrome.Rect{
				X:     0,
				Y:     i,
				Width: body.Width, Height: 1,
			},
			Enabled: true,
		}
	}
	s.focus.Register(scopeSidebar, targets)
}

func sidebarMotionTick(generation uint64) tea.Cmd {
	return tea.Tick(sidebarMotionInterval, func(time.Time) tea.Msg {
		return sidebarMotionMsg{
			generation: generation,
			delta:      sidebarMotionInterval,
		}
	})
}

func (m *Model) toggleSidebar() tea.Cmd {
	if m.dialogs.ActiveID() != surfaceNone || !m.interaction.idle() {
		return nil
	}
	switch {
	case !m.sidebar.open:
		m.sidebar.show()
	case !m.sidebar.focused:
		m.sidebar.show()
	default:
		m.sidebar.hide()
	}
	return m.retargetSidebar()
}

func (m *Model) syncSidebarShortcut() {
	chord, ok := m.bindings.ChordFor(scopeGlobal, commandSidebar)
	if !ok {
		m.sidebar.setFooter("Esc canvas")
		return
	}
	display := chrome.DisplayChord(
		chord,
		chrome.VocabularyForProfile(m.preferenceValue().KeyProfile),
	)
	m.sidebar.setFooter("Esc canvas  " + display + " close")
}

func (m *Model) dismissSidebar() tea.Cmd {
	m.sidebar.hide()
	return m.retargetSidebar()
}

func (m *Model) retargetSidebar() tea.Cmd {
	m.sidebar.placement = m.sidebarPlacement()
	target := 0
	if m.sidebar.open {
		target = m.sidebarTargetWidth()
	}
	m.sidebar.generation++
	if m.sidebar.initialOpen {
		m.sidebar.initialOpen = false
		m.workspace.SetMotionEnabled(false)
		m.workspace.RetargetSurface(surfaceSidebar, target)
		m.workspace.SetMotionEnabled(true)
		return nil
	}
	if !m.workspace.RetargetSurface(surfaceSidebar, target) {
		return nil
	}
	return sidebarMotionTick(m.sidebar.generation)
}

func (m *Model) sidebarPlacement() sidebarPlacement {
	mainWidth := m.workspace.Geometry().Main.Width
	if mainWidth < m.sidebar.desired+sidebarCanvasMinimum {
		return sidebarDrawer
	}
	return sidebarDocked
}

func (m *Model) sidebarTargetWidth() int {
	mainWidth := m.workspace.Geometry().Main.Width
	if m.sidebar.placement == sidebarDrawer {
		return min(m.sidebar.desired, mainWidth)
	}
	return min(m.sidebar.desired, max(mainWidth-sidebarCanvasMinimum, 0))
}

func (m *Model) updateSidebarMotion(message sidebarMotionMsg) tea.Cmd {
	if message.generation != m.sidebar.generation {
		return nil
	}
	m.workspace.AdvanceSurface(surfaceSidebar, message.delta)
	if !m.workspace.SurfaceMoving(surfaceSidebar) {
		return nil
	}
	return sidebarMotionTick(message.generation)
}

func (m *Model) setMotionEnabled(enabled bool) {
	m.workspace.SetMotionEnabled(enabled)
	m.sidebar.generation++
}
