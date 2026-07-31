package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	sidebarRootDropTarget = -2

	sidebarCanvasesTab chrome.FocusID = "sidebar-tab:canvases"
	sidebarDraftsTab   chrome.FocusID = "sidebar-tab:drafts"
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
	Items  []sidebarItem
	Footer string
}

type sidebarTab struct {
	ID     chrome.FocusID
	Label  string
	Drafts bool
}

type sidebarTabPlan struct {
	Tab  sidebarTab
	Rect chrome.Rect
}

var sidebarTabs = [...]sidebarTab{
	{ID: sidebarCanvasesTab, Label: "Canvases"},
	{ID: sidebarDraftsTab, Label: "Drafts", Drafts: true},
}

type sidebarDrag struct {
	source        sidebarItem
	start         chrome.Point
	targetSection string
	targetIndex   int
	active        bool
	moved         bool
	valid         bool
}

type sidebarRelease struct {
	source        sidebarItem
	targetSection string
	dragged       bool
	valid         bool
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
	drag        sidebarDrag
	tabs        [len(sidebarTabs)]sidebarTabPlan
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
	focus.Register(scopeCanvas, []chrome.FocusTarget{{
		ID: chrome.FocusID(surfaceCanvas), Enabled: true,
	}})
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
	sidebar.registerTargets()
	focus.Open(scopeCanvas)
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
	s.registerTargets()
}

func (s *sidebarState) setFooter(footer string) {
	s.declaration.Footer = footer
	s.render()
}

func (s *sidebarState) setBounds(bounds chrome.Rect) {
	s.pane.SetBounds(chrome.Rect{Width: bounds.Width, Height: bounds.Height})
	s.render()
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
	if _, ok := s.focusedItem(); ok {
		s.focus.Reveal(s.viewport)
	}
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
	if tab, ok := s.tabAt(local); ok {
		s.show()
		s.focusTarget(tab.ID)
		s.render()
		return true
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
	s.focusTarget(s.declaration.Items[index].ID)
	s.render()
	return s.declaration.Items[index].Kind != 0
}

func (s *sidebarState) beginCanvasDrag(point chrome.Point) bool {
	item, ok := s.focusedItem()
	if !ok || item.Kind != sidebarItemRecord || item.Entry.Draft {
		return false
	}
	s.drag = sidebarDrag{
		source: item, start: point, targetIndex: -1, active: true,
	}
	return true
}

func (s *sidebarState) motion(point chrome.Point, surface chrome.SurfacePlan) {
	local := chrome.Point{
		X: point.X - surface.Content.X,
		Y: point.Y - surface.Content.Y,
	}
	s.viewport.HoverScrollbar(local)
	s.viewport.DragScrollbar(local)
	if !s.drag.active || s.viewport.ScrollbarDragging() {
		return
	}
	moved := point != s.drag.start
	section, index, valid := s.dropTarget(local)
	if s.drag.moved == moved && s.drag.targetSection == section &&
		s.drag.targetIndex == index && s.drag.valid == valid {
		return
	}
	s.drag.moved = moved
	s.drag.targetSection = section
	s.drag.targetIndex = index
	s.drag.valid = valid
	s.render()
}

func (s *sidebarState) release() sidebarRelease {
	s.viewport.EndScrollbarDrag()
	if !s.drag.active {
		return sidebarRelease{}
	}
	release := sidebarRelease{
		source:        s.drag.source,
		targetSection: s.drag.targetSection,
		dragged:       s.drag.moved,
		valid:         s.drag.valid,
	}
	s.drag = sidebarDrag{}
	s.render()
	return release
}

func (s *sidebarState) clearHover() {
	s.viewport.ClearScrollbarHover()
}

func (s *sidebarState) capturesPointer() bool {
	return s.viewport.ScrollbarDragging() || s.drag.active
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
	s.renderHeader()
	s.pane.SetFooter([]string{s.styles.Footer.Render(s.declaration.Footer)})
	_, focused := s.focus.Current()
	lines := make([]string, len(s.declaration.Items))
	for i, item := range s.declaration.Items {
		style := s.styles.Item
		focusedStyle := s.styles.FocusedItem
		if item.Kind == sidebarItemSection {
			style = s.styles.Section
			focusedStyle = s.styles.FocusedSection
		}
		if s.focused && item.ID == focused {
			style = focusedStyle
		}
		if s.drag.moved && s.drag.valid && s.drag.targetIndex == i {
			style = focusedStyle
		}
		lines[i] = style.Render(item.Label)
	}
	s.viewport.SetContent(lines)
}

func (s *sidebarState) renderHeader() {
	header := s.styles.Header
	width := max(s.pane.Plan().Content.Width-header.GetHorizontalFrameSize(), 0)
	firstWidth := width / len(sidebarTabs)
	widths := [len(sidebarTabs)]int{firstWidth, width - firstWidth}
	_, focused := s.focus.Current()
	var content strings.Builder
	for i, tab := range sidebarTabs {
		style := s.styles.Tab
		switch {
		case tab.Drafts == s.drafts:
			style = s.styles.ActiveTab
		case s.focused && focused == tab.ID:
			style = s.styles.FocusedTab
		}
		content.WriteString(renderSidebarTab(style, tab.Label, widths[i]))
	}
	rendered := header.Width(width).Render(content.String())
	s.pane.SetHeader(strings.Split(rendered, "\n"))
	plan := s.pane.Plan().Header
	x := plan.X + header.GetMarginLeft() +
		header.GetBorderLeftSize() + header.GetPaddingLeft()
	y := plan.Y + header.GetMarginTop() +
		header.GetBorderTopSize() + header.GetPaddingTop()
	for i, tab := range sidebarTabs {
		s.tabs[i] = sidebarTabPlan{
			Tab:  tab,
			Rect: chrome.Rect{X: x, Y: y, Width: widths[i], Height: 1},
		}
		x += widths[i]
	}
}

func renderSidebarTab(style lipgloss.Style, label string, width int) string {
	contentWidth := max(width-style.GetHorizontalFrameSize(), 0)
	line := style.Width(contentWidth).Align(lipgloss.Center).Render(label)
	line = ansi.Truncate(line, width, "")
	return line + strings.Repeat(" ", max(width-ansi.StringWidth(line), 0))
}

func (s *sidebarState) dropTarget(local chrome.Point) (string, int, bool) {
	plan := s.pane.Plan()
	if plan.Header.Contains(local) {
		return "", sidebarRootDropTarget, true
	}
	if !plan.Body.Contains(local) {
		return "", -1, false
	}
	index := local.Y - plan.Body.Y + s.viewport.Plan().Offset.Y
	if index < 0 || index >= len(s.declaration.Items) {
		return "", sidebarRootDropTarget, true
	}
	item := s.declaration.Items[index]
	switch item.Kind {
	case sidebarItemSection:
		return item.Section, index, true
	case sidebarItemRecord:
		if !item.Entry.Draft {
			return item.Entry.Section, index, true
		}
	case sidebarItemClearDrafts:
	}
	return "", -1, false
}

func (s *sidebarState) focusTarget(id chrome.FocusID) bool {
	for range len(s.declaration.Items) + len(sidebarTabs) {
		_, focused := s.focus.Current()
		if focused == id {
			return true
		}
		s.focus.Move(1)
	}
	return false
}

func (s *sidebarState) focusTab(drafts bool) {
	for _, tab := range sidebarTabs {
		if tab.Drafts == drafts {
			s.focusTarget(tab.ID)
			s.render()
			return
		}
	}
}

func (s *sidebarState) focusedTab() (bool, bool) {
	_, focused := s.focus.Current()
	for _, tab := range sidebarTabs {
		if tab.ID == focused {
			return tab.Drafts, true
		}
	}
	return false, false
}

func (s *sidebarState) tabAt(point chrome.Point) (sidebarTab, bool) {
	for _, plan := range s.tabs {
		if plan.Rect.Contains(point) {
			return plan.Tab, true
		}
	}
	return sidebarTab{}, false
}

func (s *sidebarState) setContent(items []sidebarItem, allLabels []string) {
	s.declaration.Items = items
	s.measure(allLabels)
	s.render()
	s.registerTargets()
	s.render()
}

func (s *sidebarState) measure(allLabels []string) {
	tabFrame := max(
		s.styles.Tab.GetHorizontalFrameSize(),
		s.styles.FocusedTab.GetHorizontalFrameSize(),
		s.styles.ActiveTab.GetHorizontalFrameSize(),
	)
	tabWidth := 0
	for _, tab := range sidebarTabs {
		tabWidth = max(tabWidth, ansi.StringWidth(tab.Label)+tabFrame)
	}
	itemFrame := max(
		s.styles.Item.GetHorizontalFrameSize(),
		s.styles.FocusedItem.GetHorizontalFrameSize(),
		s.styles.Section.GetHorizontalFrameSize(),
		s.styles.FocusedSection.GetHorizontalFrameSize(),
	)
	width := max(
		tabWidth*len(sidebarTabs)+s.styles.Header.GetHorizontalFrameSize(),
		ansi.StringWidth(s.declaration.Footer)+s.styles.Footer.GetHorizontalFrameSize(),
	)
	for _, item := range s.declaration.Items {
		width = max(width, ansi.StringWidth(item.Label)+itemFrame)
	}
	for _, label := range allLabels {
		width = max(width, ansi.StringWidth(label)+itemFrame)
	}
	s.desired = max(sidebarMinimumWidth, width+s.styles.Container.GetHorizontalFrameSize())
}

func (s *sidebarState) registerTargets() {
	body := s.pane.Plan().Body
	targets := make([]chrome.FocusTarget, 0, len(sidebarTabs)+len(s.declaration.Items))
	for _, plan := range s.tabs {
		targets = append(targets, chrome.FocusTarget{
			ID: plan.Tab.ID, Rect: plan.Rect, Enabled: true,
		})
	}
	for i, item := range s.declaration.Items {
		targets = append(targets, chrome.FocusTarget{
			ID: item.ID,
			Rect: chrome.Rect{
				X:     0,
				Y:     i,
				Width: body.Width, Height: 1,
			},
			Enabled: true,
		})
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
