package tui

import (
	"cmp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/sahilm/fuzzy"
)

const (
	palettePreferredWidth = 64
	paletteDefaultRows    = 10
	paletteFrameRows      = 4
	palettePrompt         = "Search: "
	paletteScrollbarGap   = 1
)

type paletteStyles struct {
	Input      chrome.TextInputStyles
	Prompt     lipgloss.Style
	Divider    lipgloss.Style
	Relevant   lipgloss.Style
	Irrelevant lipgloss.Style
	Selected   lipgloss.Style
	Hovered    lipgloss.Style
	Footer     lipgloss.Style
	Scrollbar  chrome.ScrollbarStyles
}

type paletteItem struct {
	Command  chrome.CommandID
	Label    string
	Binding  string
	Relevant bool
}

type paletteSelectedMsg struct {
	Command chrome.CommandID
}

type paletteDismissedMsg struct{}

type paletteDialogBody struct {
	query    *chrome.TextInput
	viewport *chrome.Viewport
	styles   paletteStyles
	items    []paletteItem
	filtered []paletteItem
	bounds   chrome.Rect
	selected int
	hovered  int
}

func newPaletteDialogBody(styles paletteStyles) *paletteDialogBody {
	query := chrome.NewTextInput("", "", styles.Input)
	query.Focus()
	viewport := chrome.NewViewport("palette-results")
	viewport.SetOverflow(chrome.ClipText)
	viewport.SetScrollbars(chrome.ScrollbarNever, chrome.ScrollbarAutomatic)
	viewport.SetScrollbarStyles(styles.Scrollbar)
	return &paletteDialogBody{
		query: query, viewport: viewport, styles: styles,
		selected: -1, hovered: -1,
	}
}

func (m *Model) openPalette() tea.Cmd {
	scopes := append([]chrome.ScopeID(nil), m.activeBindingScopes()...)
	m.dialogs.OpenPalette(m.paletteItems(scopes))
	m.status = ""
	m.syncWorkspace()
	return nil
}

func (m *Model) paletteItems(scopes []chrome.ScopeID) []paletteItem {
	items := make([]paletteItem, 0, len(applicationActions))
	for _, action := range applicationActions {
		if action.HidePalette {
			continue
		}
		items = append(items, paletteItem{
			Command:  action.Command,
			Label:    action.Label,
			Binding:  m.paletteBinding(action, scopes),
			Relevant: m.paletteActionRelevant(action, scopes),
		})
	}
	return items
}

func (m *Model) paletteBinding(action action, scopes []chrome.ScopeID) string {
	for _, binding := range action.Bindings {
		if !slices.Contains(scopes, binding.Scope) {
			continue
		}
		return m.displayPaletteBinding(binding.Scope, action.Command)
	}
	for _, binding := range action.Bindings {
		if chord := m.displayPaletteBinding(binding.Scope, action.Command); chord != "" {
			return chord
		}
	}
	return ""
}

func (m *Model) displayPaletteBinding(
	scope chrome.ScopeID,
	command chrome.CommandID,
) string {
	chord, ok := m.bindings.ChordFor(scope, command)
	if !ok {
		return ""
	}
	return chrome.DisplayChord(
		chord,
		chrome.VocabularyForProfile(chrome.ProfileAuto),
	)
}

func (m *Model) paletteActionRelevant(action action, scopes []chrome.ScopeID) bool {
	for _, binding := range action.Bindings {
		if slices.Contains(scopes, binding.Scope) &&
			m.paletteCommandAvailable(binding.Scope, action.Command) {
			return true
		}
	}
	return false
}

func (m *Model) paletteCommandAvailable(
	scope chrome.ScopeID,
	command chrome.CommandID,
) bool {
	switch scope {
	case scopeCanvas, scopeLabel:
		return m.canvasCommandAvailable(command)
	case scopeGlobal:
		switch command {
		case commandCopyCanvasPath:
			return m.canvasStore != nil && m.entry != nil &&
				m.dialogs.ActiveID() == surfaceNone
		case commandHelp:
			return true
		case commandPreferences:
			return m.dialogs.ActiveID() == surfaceNone && m.interaction.idle() ||
				m.dialogs.ActiveID() == surfacePreferences
		case commandSave:
			switch m.dialogs.ActiveID() {
			case surfaceSave, surfacePreferences:
				return true
			case surfaceNone:
				return m.canvasStore != nil && m.interaction.idle()
			default:
				return false
			}
		case commandSidebar:
			return m.dialogs.ActiveID() == surfaceNone && m.interaction.idle()
		default:
			return false
		}
	case scopeSidebar:
		switch command {
		case commandNewCanvas:
			return m.canvasStore != nil
		case commandSidebarActivate:
			_, tab := m.sidebar.focusedTab()
			_, item := m.sidebar.focusedItem()
			return tab || item
		case commandSidebarDelete:
			item, ok := m.sidebar.focusedItem()
			return ok && (item.Kind == sidebarItemRecord || item.Kind == sidebarItemClearDrafts)
		default:
			return true
		}
	case scopeDirectory, scopeModal, scopePreferences:
		return true
	default:
		return false
	}
}

func (b *paletteDialogBody) Reset(items []paletteItem) {
	b.items = append(b.items[:0], items...)
	b.query.SetValue("")
	b.query.Focus()
	b.selected = -1
	b.hovered = -1
	b.filter()
}

func (*paletteDialogBody) Context() string { return "command palette" }

func (*paletteDialogBody) PreferredWidth() int { return palettePreferredWidth }

func (*paletteDialogBody) Scopes() []chrome.ScopeID {
	return []chrome.ScopeID{scopePalette}
}

func (*paletteDialogBody) TextEntry() bool { return true }

func (b *paletteDialogBody) PointerOccupied(point chrome.Point) bool {
	return point.Y == 0 || b.viewport.Plan().Bounds.Contains(point)
}

func (b *paletteDialogBody) SetBounds(bounds chrome.Rect) {
	bounds.Width = max(bounds.Width, 0)
	bounds.Height = max(bounds.Height, 0)
	b.bounds = bounds
	promptWidth := ansi.StringWidth(palettePrompt)
	b.query.SetWidth(max(bounds.Width-promptWidth, 0))
	b.viewport.SetBounds(chrome.Rect{
		Y: 2, Width: bounds.Width, Height: b.resultHeight(),
	})
	b.renderResults()
}

func (b *paletteDialogBody) Update(message tea.Msg) dialogBodyResult {
	switch message := message.(type) {
	case dialogClickMsg:
		return b.click(message.Point)
	case dialogWheelMsg:
		switch message.Mouse.Button {
		case tea.MouseWheelUp:
			b.viewport.Scroll(0, -3)
		case tea.MouseWheelDown:
			b.viewport.Scroll(0, 3)
		}
		return dialogBodyResult{handled: true}
	case tea.MouseMotionMsg:
		b.hover(chrome.Point{X: message.X, Y: message.Y})
		return dialogBodyResult{handled: true}
	case dialogBackMsg, dialogCloseMsg:
		return dialogBodyResult{message: paletteDismissedMsg{}, handled: true}
	case tea.KeyPressMsg:
		switch message.Code {
		case tea.KeyEscape:
			return dialogBodyResult{message: paletteDismissedMsg{}, handled: true}
		case tea.KeyUp:
			b.moveSelection(-1)
			return dialogBodyResult{handled: true}
		case tea.KeyDown:
			b.moveSelection(1)
			return dialogBodyResult{handled: true}
		case tea.KeyEnter:
			return b.selectCurrent()
		}
	}

	before := b.query.Value()
	b.query.Update(message)
	if b.query.Value() != before {
		b.filter()
	}
	return dialogBodyResult{handled: true}
}

func (b *paletteDialogBody) View() string {
	width := b.bounds.Width
	if width == 0 {
		width = palettePreferredWidth
	}
	query := b.styles.Prompt.Render(palettePrompt) + b.query.View()
	divider := b.styles.Divider.Render(strings.Repeat("─", width))
	footer := b.styles.Footer.Render(ansi.Truncate(
		"↑↓ navigate · enter execute · esc dismiss",
		width,
		"",
	))
	results := b.viewport.Lines()
	lines := make([]string, 0, len(results)+4)
	lines = append(lines, padPaletteLine(query, width), divider)
	lines = append(lines, results...)
	lines = append(lines, divider, padPaletteLine(footer, width))
	return strings.Join(lines, "\n")
}

func (b *paletteDialogBody) SetStyles(styles paletteStyles) {
	b.styles = styles
	b.query.SetStyles(styles.Input)
	b.viewport.SetScrollbarStyles(styles.Scrollbar)
	b.renderResults()
}

func (b *paletteDialogBody) filter() {
	query := strings.TrimSpace(b.query.Value())
	b.filtered = b.filtered[:0]
	if query == "" {
		b.filtered = append(b.filtered, b.items...)
		slices.SortStableFunc(b.filtered, comparePaletteItems)
	} else {
		labels := make([]string, len(b.items))
		for i := range b.items {
			labels[i] = b.items[i].Label
		}
		for _, match := range fuzzy.Find(query, labels) {
			b.filtered = append(b.filtered, b.items[match.Index])
		}
		slices.SortStableFunc(b.filtered, func(a, c paletteItem) int {
			return comparePaletteRelevance(a, c)
		})
	}
	b.selected = firstRelevantPaletteItem(b.filtered)
	b.hovered = -1
	b.renderResults()
	b.revealSelection()
}

func comparePaletteItems(a, b paletteItem) int {
	if relevance := comparePaletteRelevance(a, b); relevance != 0 {
		return relevance
	}
	return cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
}

func comparePaletteRelevance(a, b paletteItem) int {
	if a.Relevant == b.Relevant {
		return 0
	}
	if a.Relevant {
		return -1
	}
	return 1
}

func firstRelevantPaletteItem(items []paletteItem) int {
	for i := range items {
		if items[i].Relevant {
			return i
		}
	}
	return -1
}

func (b *paletteDialogBody) moveSelection(delta int) {
	if len(b.filtered) == 0 || delta == 0 {
		return
	}
	start := b.selected
	if start < 0 {
		if delta > 0 {
			start = -1
		} else {
			start = len(b.filtered)
		}
	}
	for step := 1; step <= len(b.filtered); step++ {
		index := (start + step*delta) % len(b.filtered)
		if index < 0 {
			index += len(b.filtered)
		}
		if b.filtered[index].Relevant {
			b.selected = index
			b.hovered = -1
			b.renderResults()
			b.revealSelection()
			return
		}
	}
}

func (b *paletteDialogBody) selectCurrent() dialogBodyResult {
	if b.selected < 0 || b.selected >= len(b.filtered) ||
		!b.filtered[b.selected].Relevant {
		return dialogBodyResult{handled: true}
	}
	return dialogBodyResult{
		message: paletteSelectedMsg{Command: b.filtered[b.selected].Command},
		handled: true,
	}
}

func (b *paletteDialogBody) click(point chrome.Point) dialogBodyResult {
	if point.Y == 0 {
		b.query.Click(point.X - ansi.StringWidth(palettePrompt))
		return dialogBodyResult{handled: true}
	}
	index, ok := b.itemAt(point)
	if !ok || !b.filtered[index].Relevant {
		return dialogBodyResult{handled: true}
	}
	b.selected = index
	b.hovered = -1
	b.renderResults()
	return b.selectCurrent()
}

func (b *paletteDialogBody) hover(point chrome.Point) {
	index, ok := b.itemAt(point)
	if !ok {
		index = -1
	}
	if b.hovered == index {
		return
	}
	b.hovered = index
	b.renderResults()
}

func (b *paletteDialogBody) itemAt(point chrome.Point) (int, bool) {
	content, ok := b.viewport.ContentPoint(point)
	if !ok || content.Y < 0 || content.Y >= len(b.filtered) {
		return 0, false
	}
	return content.Y, true
}

func (b *paletteDialogBody) renderResults() {
	width := b.resultWidth()
	if len(b.filtered) == 0 {
		b.viewport.SetContent([]string{
			padPaletteLine(b.styles.Irrelevant.Render("No matching actions"), width),
		})
		return
	}
	lines := make([]string, len(b.filtered))
	for i, item := range b.filtered {
		line := paletteItemLine(item, width)
		style := b.styles.Relevant
		switch {
		case i == b.selected:
			style = b.styles.Selected
		case i == b.hovered && item.Relevant:
			style = b.styles.Hovered
		case !item.Relevant:
			style = b.styles.Irrelevant
		}
		lines[i] = style.Render(line)
	}
	b.viewport.SetContent(lines)
}

func (b *paletteDialogBody) resultWidth() int {
	width := b.bounds.Width
	if width == 0 {
		width = palettePreferredWidth
	}
	if len(b.filtered) > b.resultHeight() {
		width -= 1 + paletteScrollbarGap
	}
	return max(width, 0)
}

func (b *paletteDialogBody) resultHeight() int {
	if b.bounds.Height > paletteFrameRows {
		return b.bounds.Height - paletteFrameRows
	}
	return min(max(len(b.filtered), 1), paletteDefaultRows)
}

func (b *paletteDialogBody) revealSelection() {
	if b.selected < 0 {
		return
	}
	b.viewport.Reveal(chrome.Rect{Y: b.selected, Width: 1, Height: 1})
}

func paletteItemLine(item paletteItem, width int) string {
	keyWidth := ansi.StringWidth(item.Binding)
	labelWidth := width
	if keyWidth != 0 {
		labelWidth = max(width-keyWidth-1, 0)
	}
	label := ansi.Truncate(item.Label, labelWidth, "…")
	spaces := max(width-ansi.StringWidth(label)-keyWidth, 0)
	return label + strings.Repeat(" ", spaces) + item.Binding
}

func padPaletteLine(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	return line + strings.Repeat(" ", max(width-ansi.StringWidth(line), 0))
}
