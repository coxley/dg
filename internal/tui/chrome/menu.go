package chrome

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// MenuItem declares one semantic menu item.
type MenuItem struct {
	ID    ID
	Label string
}

// Menu retains one arranged menu plan.
type Menu struct {
	id      ID
	style   lipgloss.Style
	items   []MenuItem
	width   int
	top     int
	version uint64
	plan    Plan
	err     error
}

// NewMenu returns a retained horizontal menu.
func NewMenu(id ID, style lipgloss.Style, items []MenuItem) *Menu {
	m := &Menu{id: id, style: style, top: 1}
	m.SetItems(items)
	return m
}

// SetItems replaces menu declarations and arranges immediately.
func (m *Menu) SetItems(items []MenuItem) {
	m.items = append(m.items[:0], items...)
	m.invalidate()
}

// SetStyle replaces menu frame geometry and arranges immediately.
func (m *Menu) SetStyle(style lipgloss.Style) {
	m.style = style
	m.invalidate()
}

// SetViewport sets the terminal width and menu top edge.
func (m *Menu) SetViewport(width, top int) {
	width = max(width, 0)
	top = max(top, 0)
	if m.width == width && m.top == top {
		return
	}
	m.width, m.top = width, top
	m.invalidate()
}

// Plan returns the current arrangement.
func (m *Menu) Plan() (Plan, error) {
	return m.plan, m.err
}

// Bounds returns the current arranged menu rectangle.
func (m *Menu) Bounds() Rect {
	return m.plan.Bounds
}

// Contains reports whether the current plan contains p.
func (m *Menu) Contains(p Point) bool {
	return m.err == nil && m.plan.Bounds.Contains(p)
}

// ItemAt returns the semantic item under p.
func (m *Menu) ItemAt(p Point) (MenuItem, bool) {
	if m.err != nil {
		return MenuItem{}, false
	}
	for _, item := range m.items {
		rect, ok := m.plan.Rect(item.ID)
		if ok && rect.Contains(p) {
			return item, true
		}
	}
	return MenuItem{}, false
}

// ItemRect returns the current arranged rectangle for id.
func (m *Menu) ItemRect(id ID) (Rect, bool) {
	if m.err != nil {
		return Rect{}, false
	}
	return m.plan.Rect(id)
}

// Lines renders the current plan using renderItem for visual state.
func (m *Menu) Lines(renderItem func(MenuItem) string) []string {
	if m.err != nil || m.plan.Bounds.Width == 0 {
		return nil
	}
	var content strings.Builder
	x := m.plan.Bounds.X +
		m.style.GetBorderLeftSize() +
		m.style.GetPaddingLeft()
	for _, item := range m.items {
		rect, ok := m.plan.Rect(item.ID)
		if !ok {
			continue
		}
		content.WriteString(strings.Repeat(" ", max(rect.X-x, 0)))
		content.WriteString(ansi.Truncate(renderItem(item), rect.Width, ""))
		x = rect.Right()
	}
	return strings.Split(m.style.Render(content.String()), "\n")
}

func (m *Menu) invalidate() {
	m.version++
	m.arrange()
}

func (m *Menu) arrange() {
	children := make([]Node, len(m.items))
	for i, item := range m.items {
		children[i] = Text(item.ID, item.Label)
	}
	root := Box(m.id, m.style, Row(m.id+".items", children...))
	metrics := Measure(root, Constraints{Max: Size{Width: m.width, Height: 1 << 20}})
	if m.width < metrics.Min.Width {
		m.plan, m.err = Plan{Version: m.version}, nil
		return
	}
	left := (m.width - metrics.Preferred.Width) / 2
	available := Rect{
		X:      left,
		Y:      m.top,
		Width:  metrics.Preferred.Width,
		Height: metrics.Preferred.Height,
	}
	m.plan, m.err = Arrange(root, available, m.version)
}
