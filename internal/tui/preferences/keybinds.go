package preferences

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
)

const mappingLimit = 3

// Keybind contains up to three mappings for one scoped action.
type Keybind struct {
	Scope    chrome.ScopeID
	Command  chrome.CommandID
	Mappings [mappingLimit]chrome.Chord
}

// KeybindAction describes one configurable action row.
type KeybindAction struct {
	Scope      chrome.ScopeID
	ScopeLabel string
	Command    chrome.CommandID
	Label      string
}

type mappingPlan struct {
	row  int
	cell int
	rect chrome.Rect
}

type keybindModel struct {
	actions   []KeybindAction
	values    []Keybind
	styles    Styles
	width     int
	height    int
	row       int
	cell      int
	active    bool
	offset    int
	hovered   bool
	hoverRow  int
	hoverCell int
	plans     []mappingPlan
	cached    string
	dirty     bool
}

func newKeybindModel(actions []KeybindAction, values []Keybind, styles Styles) *keybindModel {
	m := &keybindModel{styles: styles}
	m.reset(actions, values)
	return m
}

func (m *keybindModel) reset(actions []KeybindAction, values []Keybind) {
	m.actions = append(m.actions[:0], actions...)
	m.values = make([]Keybind, len(m.actions))
	for i, action := range m.actions {
		m.values[i] = Keybind{Scope: action.Scope, Command: action.Command}
		if configured, ok := findKeybind(values, action.Scope, action.Command); ok {
			m.values[i].Mappings = configured.Mappings
		}
	}
	m.row = min(m.row, max(len(m.actions)-1, 0))
	m.cell = min(m.cell, mappingLimit-1)
	m.active = false
	m.offset = 0
	m.hovered = false
	m.dirty = true
}

func (m *keybindModel) setBounds(width, height int) {
	width = max(width, 0)
	height = max(height, 0)
	if m.width == width && m.height == height {
		return
	}
	m.width = width
	m.height = height
	m.reveal()
	m.dirty = true
}

func (m *keybindModel) setStyles(styles Styles) {
	m.styles = styles
	m.dirty = true
}

func (m *keybindModel) value() []Keybind {
	return append([]Keybind(nil), m.values...)
}

func (m *keybindModel) capturesKey() bool {
	return m.active
}

func (m *keybindModel) updateKey(message tea.KeyPressMsg) bool {
	if len(m.values) == 0 {
		return false
	}
	if m.active {
		if message.Code == tea.KeyEscape {
			m.active = false
			m.dirty = true
			return true
		}
		if modifierOnly(message.Code) {
			return true
		}
		m.values[m.row].Mappings[m.cell] = chrome.ChordForKey(message)
		m.active = false
		m.dirty = true
		return true
	}
	switch {
	case message.Mod == 0 && message.Code == tea.KeyUp:
		m.moveRow(-1)
	case message.Mod == 0 && message.Code == tea.KeyDown:
		m.moveRow(1)
	case message.Code == tea.KeyTab && message.Mod == tea.ModShift:
		m.cell = (m.cell + mappingLimit - 1) % mappingLimit
	case message.Code == tea.KeyTab && message.Mod == 0:
		m.cell = (m.cell + 1) % mappingLimit
	case message.Mod == 0 && (message.Code == tea.KeyBackspace || message.Code == tea.KeyDelete):
		m.values[m.row].Mappings[m.cell] = ""
	case message.Mod == 0 && message.Code == tea.KeyEnter:
		m.active = true
	default:
		return false
	}
	m.dirty = true
	return true
}

func (m *keybindModel) click(point chrome.Point) bool {
	plan, ok := m.mappingAt(point)
	if !ok {
		return false
	}
	m.row = plan.row
	m.cell = plan.cell
	m.active = true
	m.reveal()
	m.dirty = true
	return true
}

func (m *keybindModel) hover(point chrome.Point) {
	hovered := false
	row, cell := 0, 0
	if plan, ok := m.mappingAt(point); ok {
		hovered = true
		row, cell = plan.row, plan.cell
	}
	if m.hovered == hovered && (!hovered || m.hoverRow == row && m.hoverCell == cell) {
		return
	}
	m.hovered = hovered
	m.hoverRow = row
	m.hoverCell = cell
	m.dirty = true
}

func (m *keybindModel) mappingAt(point chrome.Point) (mappingPlan, bool) {
	for _, plan := range m.plans {
		if plan.rect.Contains(point) {
			return plan, true
		}
	}
	return mappingPlan{}, false
}

func (m *keybindModel) moveRow(delta int) {
	m.row = min(max(m.row+delta, 0), max(len(m.actions)-1, 0))
	m.reveal()
	m.dirty = true
}

func (m *keybindModel) render() string {
	if !m.dirty {
		return m.cached
	}
	if m.width == 0 || m.height == 0 {
		m.plans = m.plans[:0]
		m.cached = ""
		m.dirty = false
		return ""
	}
	starts, headers := m.layoutRows()
	m.revealStarts(starts)
	lines := make([]string, m.height)
	for line, label := range headers {
		if line >= m.offset && line < m.offset+m.height {
			lines[line-m.offset] = m.styles.Scope.Render(label)
		}
	}
	conflicts := m.conflicts()
	mappingHeight := m.mappingHeight()
	for row, start := range starts {
		if start+mappingHeight <= m.offset || start >= m.offset+m.height {
			continue
		}
		block := strings.Split(m.actionRow(row, conflicts), "\n")
		for i, line := range block {
			y := start + i - m.offset
			if y >= 0 && y < len(lines) {
				lines[y] = line
			}
		}
	}
	m.buildPlans(starts)
	m.cached = strings.Join(lines, "\n")
	m.dirty = false
	return m.cached
}

func (m *keybindModel) actionRow(
	row int,
	conflicts map[[2]string]bool,
) string {
	labelWidth, gap, pillWidth := m.columnWidths()
	action := m.actions[row]
	parts := []string{m.styles.Action.Width(labelWidth).MaxWidth(labelWidth).Render(
		ansi.Truncate(action.Label, labelWidth, "…"),
	)}
	for cell := range mappingLimit {
		value := string(m.values[row].Mappings[cell])
		pill := NewMappingPill(value, m.styles.Mapping)
		pill.SetState(
			value,
			row == m.row && cell == m.cell,
			m.active && row == m.row && cell == m.cell,
			conflicts[[2]string{string(action.Scope), value}],
			m.hovered && row == m.hoverRow && cell == m.hoverCell,
		)
		if cell != 0 {
			parts = append(parts, strings.Repeat(" ", gap))
		}
		parts = append(parts, pill.View(pillWidth))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

func (m *keybindModel) conflicts() map[[2]string]bool {
	counts := make(map[[2]string]int)
	for _, keybind := range m.values {
		seen := make(map[chrome.Chord]bool, mappingLimit)
		for _, chord := range keybind.Mappings {
			if chord == "" || seen[chord] {
				continue
			}
			seen[chord] = true
			counts[[2]string{string(keybind.Scope), string(chord)}]++
		}
	}
	conflicts := make(map[[2]string]bool)
	for key, count := range counts {
		if count > 1 {
			conflicts[key] = true
		}
	}
	return conflicts
}

func (m *keybindModel) reveal() {
	starts, _ := m.layoutRows()
	m.revealStarts(starts)
}

func (m *keybindModel) layoutRows() ([]int, map[int]string) {
	starts := make([]int, len(m.actions))
	headers := make(map[int]string)
	line := 0
	mappingHeight := m.mappingHeight()
	previousScope := chrome.ScopeID("")
	for i, action := range m.actions {
		if action.Scope != previousScope {
			if i != 0 {
				line++
			}
			headers[line] = action.ScopeLabel
			line++
			previousScope = action.Scope
		}
		starts[i] = line
		line += mappingHeight
	}
	return starts, headers
}

func (m *keybindModel) mappingHeight() int {
	pill := NewMappingPill("", m.styles.Mapping)
	return max(lipgloss.Height(pill.View(5)), 1)
}

func (m *keybindModel) revealStarts(starts []int) {
	if len(starts) == 0 || m.height == 0 {
		return
	}
	start := starts[m.row]
	end := start + m.mappingHeight()
	if start < m.offset {
		m.offset = start
	}
	if end > m.offset+m.height {
		m.offset = end - m.height
	}
	m.offset = max(m.offset, 0)
}

func (m *keybindModel) buildPlans(starts []int) {
	m.plans = m.plans[:0]
	labelWidth, gap, pillWidth := m.columnWidths()
	mappingHeight := m.mappingHeight()
	x := labelWidth
	for row, start := range starts {
		y := start - m.offset
		if y+mappingHeight <= 0 || y >= m.height {
			continue
		}
		for cell := range mappingLimit {
			m.plans = append(m.plans, mappingPlan{
				row:  row,
				cell: cell,
				rect: chrome.Rect{X: x, Y: y, Width: pillWidth, Height: mappingHeight},
			})
			x += pillWidth + gap
		}
		x = labelWidth
	}
}

func (m *keybindModel) columnWidths() (label, gap, pill int) {
	gap = 1
	minimumPills := mappingLimit * 2
	if m.width < minimumPills+mappingLimit-1 {
		gap = 0
	}
	label = min(24, max(m.width/3, 0), max(m.width-minimumPills-gap*(mappingLimit-1), 0))
	pill = max((m.width-label-gap*(mappingLimit-1))/mappingLimit, 2)
	return label, gap, pill
}

func findKeybind(values []Keybind, scope chrome.ScopeID, command chrome.CommandID) (Keybind, bool) {
	for _, value := range values {
		if value.Scope == scope && value.Command == command {
			return value, true
		}
	}
	return Keybind{}, false
}

func modifierOnly(code rune) bool {
	switch code {
	case tea.KeyLeftShift, tea.KeyRightShift,
		tea.KeyLeftAlt, tea.KeyRightAlt,
		tea.KeyLeftCtrl, tea.KeyRightCtrl,
		tea.KeyLeftMeta, tea.KeyRightMeta,
		tea.KeyLeftHyper, tea.KeyRightHyper,
		tea.KeyLeftSuper, tea.KeyRightSuper:
		return true
	default:
		return false
	}
}
