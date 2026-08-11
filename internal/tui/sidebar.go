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
	sidebarItemPrefix     = "  "
	sidebarFocusPrefix    = "▸ "
	sidebarNestedIndent   = "  "
	sidebarDividerLabel   = "────────"

	sidebarCanvasesTab   chrome.FocusID = "sidebar-tab:canvases"
	sidebarDraftsSection chrome.FocusID = "sidebar-section:drafts"
	sidebarDraftsDivider chrome.FocusID = "sidebar-divider:drafts"
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
	Drafts  bool
}

type sidebarItemKind uint8

const (
	sidebarItemRecord sidebarItemKind = iota + 1
	sidebarItemSection
	sidebarItemDivider
	sidebarItemClearDrafts
)

type sidebarDeclaration struct {
	Items  []sidebarItem
	Footer string
}

type sidebarTab struct {
	ID    chrome.FocusID
	Label string
}

type sidebarTabPlan struct {
	Tab  sidebarTab
	Rect chrome.Rect
}

type sidebarItemPlan struct {
	Index int
	Rect  chrome.Rect
}

var sidebarTabs = [...]sidebarTab{
	{ID: sidebarCanvasesTab, Label: "Canvases"},
}

type sidebarDrag struct {
	source        sidebarItem
	start         chrome.Point
	targetSection string
	targetDrafts  bool
	targetIndex   int
	active        bool
	moved         bool
	valid         bool
}

type sidebarRelease struct {
	source        sidebarItem
	targetSection string
	targetDrafts  bool
	dragged       bool
	valid         bool
}

type sidebarState struct {
	declaration     sidebarDeclaration
	styles          sidebarStyles
	viewport        *chrome.Viewport
	pane            *chrome.Pane
	focus           *chrome.FocusRegistry
	placement       sidebarPlacement
	open            bool
	focused         bool
	initialOpen     bool
	generation      uint64
	activeTab       chrome.FocusID
	active          canvasstore.Entry
	hasActive       bool
	collapsed       map[string]bool
	draftsCollapsed bool
	desired         int
	drag            sidebarDrag
	tabs            []sidebarTabPlan
	itemPlans       []sidebarItemPlan
	hoveredTab      chrome.FocusID
	tabHovered      bool
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
		activeTab:   sidebarTabs[0].ID,
		tabs:        make([]sidebarTabPlan, len(sidebarTabs)),
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

func (s *sidebarState) setActive(entry *canvasstore.Entry) {
	hasActive := entry != nil
	if s.hasActive == hasActive && (!hasActive || s.active.ID == entry.ID) {
		return
	}
	s.hasActive = hasActive
	if hasActive {
		s.active = *entry
	} else {
		s.active = canvasstore.Entry{}
	}
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
	if !s.focused || delta == 0 {
		return
	}
	_, focused := s.focus.Current()
	if _, tabFocused := s.focusedTab(); tabFocused {
		if delta > 0 {
			s.focusFirstItem()
		} else {
			s.focusLastItem()
		}
		return
	}
	first, hasFirst := s.firstFocusableItem()
	last, hasLast := s.lastFocusableItem()
	if hasFirst && hasLast {
		switch {
		case delta < 0 && focused == first.ID, delta > 0 && focused == last.ID:
			s.focusTab(s.activeTab)
			return
		}
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
	index, ok := s.itemAt(local.Y - body.Y + s.viewport.Plan().Offset.Y)
	if !ok {
		return false
	}
	s.show()
	s.focusTarget(s.declaration.Items[index].ID)
	s.render()
	return s.declaration.Items[index].Kind != 0
}

func (s *sidebarState) beginCanvasDrag(point chrome.Point) bool {
	item, ok := s.focusedItem()
	if !ok || item.Kind != sidebarItemRecord {
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
	tab, tabHovered := s.tabAt(local)
	if s.tabHovered != tabHovered || tabHovered && s.hoveredTab != tab.ID {
		s.tabHovered = tabHovered
		s.hoveredTab = tab.ID
		s.render()
	}
	s.viewport.HoverScrollbar(local)
	s.viewport.DragScrollbar(local)
	if !s.drag.active || s.viewport.ScrollbarDragging() {
		return
	}
	moved := point != s.drag.start
	section, drafts, index, valid := s.dropTarget(local)
	if s.drag.moved == moved && s.drag.targetSection == section &&
		s.drag.targetDrafts == drafts && s.drag.targetIndex == index && s.drag.valid == valid {
		return
	}
	s.drag.moved = moved
	s.drag.targetSection = section
	s.drag.targetDrafts = drafts
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
		targetDrafts:  s.drag.targetDrafts,
		dragged:       s.drag.moved,
		valid:         s.drag.valid,
	}
	s.drag = sidebarDrag{}
	s.render()
	return release
}

func (s *sidebarState) clearHover() {
	s.viewport.ClearScrollbarHover()
	if !s.tabHovered {
		return
	}
	s.tabHovered = false
	s.hoveredTab = ""
	s.render()
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
	lines := make([]string, 0, len(s.declaration.Items))
	s.itemPlans = s.itemPlans[:0]
	for i, item := range s.declaration.Items {
		style := s.styles.Item
		focusedStyle := s.styles.FocusedItem
		if item.Kind == sidebarItemSection {
			style = s.styles.Section
			focusedStyle = s.styles.FocusedSection
		}
		itemFocused := s.focused && item.ID == focused
		active := item.Kind == sidebarItemRecord && s.hasActive && item.Entry.ID == s.active.ID
		switch {
		case item.Kind == sidebarItemDivider:
			style = s.styles.Divider
		case item.Kind == sidebarItemClearDrafts:
			style = s.styles.ClearDrafts
		case active:
			style = s.styles.ActiveItem
		case itemFocused:
			style = focusedStyle
		}
		if s.drag.moved && s.drag.valid && s.drag.targetIndex == i {
			style = focusedStyle
		}
		content := sidebarItemIndent(item) + sidebarItemPrefix + item.Label
		if item.Kind == sidebarItemDivider {
			width := max(s.pane.Plan().Body.Width-style.GetHorizontalFrameSize(), 0)
			content = style.Width(width).Align(lipgloss.Center).Render(item.Label)
			style = lipgloss.NewStyle()
		} else if itemFocused && item.Kind != sidebarItemSection {
			content = sidebarItemIndent(item) + sidebarFocusPrefix + item.Label
		}
		rendered := strings.Split(
			style.Render(content),
			"\n",
		)
		start := len(lines) + style.GetMarginTop()
		height := max(len(rendered)-style.GetMarginTop()-style.GetMarginBottom(), 0)
		s.itemPlans = append(s.itemPlans, sidebarItemPlan{
			Index: i,
			Rect:  chrome.Rect{Y: start, Height: height},
		})
		lines = append(lines, rendered...)
	}
	s.viewport.SetContent(lines)
	s.registerTargets()
}

func sidebarItemIndent(item sidebarItem) string {
	if (item.Drafts && item.Kind != sidebarItemSection) ||
		(item.Kind == sidebarItemRecord && item.Entry.Section != "") {
		return sidebarNestedIndent
	}
	return ""
}

func (s *sidebarState) renderHeader() {
	header := s.styles.Header
	width := max(s.pane.Plan().Content.Width-header.GetHorizontalFrameSize(), 0)
	baseWidth := width / len(sidebarTabs)
	remainder := width % len(sidebarTabs)
	_, focused := s.focus.Current()
	var content strings.Builder
	x := 0
	for i, tab := range sidebarTabs {
		tabWidth := baseWidth
		if i < remainder {
			tabWidth++
		}
		style := s.styles.Tab
		switch {
		case tab.ID == s.activeTab:
			style = s.styles.ActiveTab
		case s.focused && focused == tab.ID:
			style = s.styles.FocusedTab
		case s.tabHovered && s.hoveredTab == tab.ID:
			style = s.styles.HoveredTab
		}
		content.WriteString(renderSidebarTab(style, tab.Label, tabWidth))
		s.tabs[i] = sidebarTabPlan{
			Tab:  tab,
			Rect: chrome.Rect{X: x, Width: tabWidth, Height: 1},
		}
		x += tabWidth
	}
	rendered := header.Width(width).Render(content.String())
	s.pane.SetHeader(strings.Split(rendered, "\n"))
	plan := s.pane.Plan().Header
	x = plan.X + header.GetMarginLeft() +
		header.GetBorderLeftSize() + header.GetPaddingLeft()
	y := plan.Y + header.GetMarginTop() +
		header.GetBorderTopSize() + header.GetPaddingTop()
	for i := range s.tabs {
		s.tabs[i].Rect.X += x
		s.tabs[i].Rect.Y = y
	}
}

func renderSidebarTab(style lipgloss.Style, label string, width int) string {
	contentWidth := max(width-style.GetHorizontalFrameSize(), 0)
	line := style.Width(contentWidth).Align(lipgloss.Center).Render(label)
	line = ansi.Truncate(line, width, "")
	return line + strings.Repeat(" ", max(width-ansi.StringWidth(line), 0))
}

func (s *sidebarState) dropTarget(local chrome.Point) (string, bool, int, bool) {
	plan := s.pane.Plan()
	if plan.Header.Contains(local) {
		return "", false, sidebarRootDropTarget, true
	}
	if !plan.Body.Contains(local) {
		return "", false, -1, false
	}
	row := local.Y - plan.Body.Y + s.viewport.Plan().Offset.Y
	index, ok := s.itemAt(row)
	if !ok {
		for _, itemPlan := range s.itemPlans {
			item := s.declaration.Items[itemPlan.Index]
			if item.Kind == sidebarItemDivider && row >= itemPlan.Rect.Y {
				return "", !s.drag.source.Entry.Draft, -1, !s.drag.source.Entry.Draft
			}
		}
		return "", false, sidebarRootDropTarget, true
	}
	item := s.declaration.Items[index]
	switch item.Kind {
	case sidebarItemSection:
		if item.Drafts {
			valid := !s.drag.source.Entry.Draft
			return "", valid, index, valid
		}
		return item.Section, false, index, true
	case sidebarItemRecord:
		if item.Entry.Draft {
			valid := !s.drag.source.Entry.Draft
			return "", valid, index, valid
		}
		return item.Entry.Section, false, index, true
	case sidebarItemClearDrafts:
		valid := !s.drag.source.Entry.Draft
		return "", valid, index, valid
	case sidebarItemDivider:
	}
	return "", false, -1, false
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

func (s *sidebarState) focusTab(id chrome.FocusID) {
	for _, tab := range sidebarTabs {
		if tab.ID == id {
			s.focusTarget(tab.ID)
			s.render()
			return
		}
	}
}

func (s *sidebarState) focusedTab() (sidebarTab, bool) {
	_, focused := s.focus.Current()
	for _, tab := range sidebarTabs {
		if tab.ID == focused {
			return tab, true
		}
	}
	return sidebarTab{}, false
}

func (s *sidebarState) focusActiveItem() bool {
	if !s.hasActive {
		return false
	}
	for _, item := range s.declaration.Items {
		if item.Kind != sidebarItemRecord || item.Entry.ID != s.active.ID {
			continue
		}
		return s.focusItem(item)
	}
	return false
}

func (s *sidebarState) focusFirstItem() bool {
	item, ok := s.firstFocusableItem()
	if !ok {
		return false
	}
	return s.focusItem(item)
}

func (s *sidebarState) focusLastItem() bool {
	item, ok := s.lastFocusableItem()
	if !ok {
		return false
	}
	return s.focusItem(item)
}

func (s *sidebarState) firstFocusableItem() (sidebarItem, bool) {
	for _, item := range s.declaration.Items {
		if item.Kind != sidebarItemDivider {
			return item, true
		}
	}
	return sidebarItem{}, false
}

func (s *sidebarState) lastFocusableItem() (sidebarItem, bool) {
	for i := len(s.declaration.Items) - 1; i >= 0; i-- {
		if s.declaration.Items[i].Kind != sidebarItemDivider {
			return s.declaration.Items[i], true
		}
	}
	return sidebarItem{}, false
}

func (s *sidebarState) focusItem(item sidebarItem) bool {
	if !s.focusTarget(item.ID) {
		return false
	}
	s.focus.Reveal(s.viewport)
	s.render()
	return true
}

func (s *sidebarState) tabAt(point chrome.Point) (sidebarTab, bool) {
	for _, plan := range s.tabs {
		if plan.Rect.Contains(point) {
			return plan.Tab, true
		}
	}
	return sidebarTab{}, false
}

func (s *sidebarState) itemAt(row int) (int, bool) {
	for _, plan := range s.itemPlans {
		if row >= plan.Rect.Y && row < plan.Rect.Bottom() {
			return plan.Index, true
		}
	}
	return 0, false
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
		s.styles.HoveredTab.GetHorizontalFrameSize(),
		s.styles.ActiveTab.GetHorizontalFrameSize(),
	)
	tabWidth := 0
	for _, tab := range sidebarTabs {
		tabWidth = max(tabWidth, ansi.StringWidth(tab.Label)+tabFrame)
	}
	itemFrame := max(
		s.styles.Item.GetHorizontalFrameSize(),
		s.styles.FocusedItem.GetHorizontalFrameSize(),
		s.styles.ActiveItem.GetHorizontalFrameSize(),
		s.styles.Section.GetHorizontalFrameSize(),
		s.styles.FocusedSection.GetHorizontalFrameSize(),
		s.styles.Divider.GetHorizontalFrameSize(),
		s.styles.ClearDrafts.GetHorizontalFrameSize(),
	)
	width := max(
		tabWidth*len(sidebarTabs)+s.styles.Header.GetHorizontalFrameSize(),
		ansi.StringWidth(s.declaration.Footer)+s.styles.Footer.GetHorizontalFrameSize(),
	)
	for _, item := range s.declaration.Items {
		width = max(
			width,
			ansi.StringWidth(sidebarItemIndent(item)+sidebarItemPrefix+item.Label)+itemFrame,
		)
	}
	for _, label := range allLabels {
		width = max(width, ansi.StringWidth(sidebarItemPrefix+label)+itemFrame)
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
	for _, plan := range s.itemPlans {
		item := s.declaration.Items[plan.Index]
		if item.Kind == sidebarItemDivider {
			continue
		}
		targets = append(targets, chrome.FocusTarget{
			ID: item.ID,
			Rect: chrome.Rect{
				X:     0,
				Y:     plan.Rect.Y,
				Width: body.Width, Height: plan.Rect.Height,
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
		m.focusActiveSidebarItem()
	case !m.sidebar.focused:
		m.sidebar.show()
		m.focusActiveSidebarItem()
	default:
		m.sidebar.hide()
	}
	return m.retargetSidebar()
}

func (m *Model) focusActiveSidebarItem() {
	if m.entry == nil {
		m.sidebar.focusFirstItem()
		return
	}
	if m.entry.Draft && m.sidebar.draftsCollapsed {
		m.sidebar.draftsCollapsed = false
		m.rebuildSidebarCatalog()
	}
	if section := m.entry.Section; !m.entry.Draft && section != "" && m.sidebar.collapsed[section] {
		m.sidebar.collapsed[section] = false
		m.rebuildSidebarCatalog()
	}
	if !m.sidebar.focusActiveItem() {
		m.sidebar.focusFirstItem()
	}
}

func (m *Model) syncSidebarShortcut() {
	chord, ok := m.bindings.ChordFor(scopeGlobal, commandSidebar)
	if !ok {
		m.sidebar.setFooter("Esc canvas")
		return
	}
	display := chrome.DisplayChord(
		chord,
		chrome.VocabularyForProfile(chrome.ProfileAuto),
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
