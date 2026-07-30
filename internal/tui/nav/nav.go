// Package nav implements the floating editor tool navigation.
package nav

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/internal/tui/chrome"
)

// Tool identifies an editor tool.
type Tool uint8

const (
	Cursor Tool = iota
	Rectangle
	Line
)

// Item declares one application-owned tool entry.
type Item struct {
	ID    chrome.ID
	Tool  Tool
	Label string
}

// Styles defines the floating navigation appearance.
type Styles struct {
	Container lipgloss.Style
	Item      lipgloss.Style
	Active    lipgloss.Style
	Hover     lipgloss.Style
}

// Model contains floating navigation state and rendered-content caches.
type Model struct {
	styles Styles
	menu   *chrome.Menu
	items  []Item
	active Tool
	hover  Tool

	hovered bool

	lines []string
}

// ActivateMsg requests activation of Tool.
type ActivateMsg struct {
	Tool Tool
}

// Activate returns a command that activates tool through Model.Update.
func Activate(tool Tool) tea.Cmd {
	return func() tea.Msg {
		return ActivateMsg{Tool: tool}
	}
}

// New returns a floating navigation model.
func New(styles Styles, items []Item) Model {
	model := Model{items: append([]Item(nil), items...)}
	menuItems := make([]chrome.MenuItem, len(items))
	for i, item := range items {
		menuItems[i] = chrome.MenuItem{ID: item.ID, Label: item.Label}
	}
	model.menu = chrome.NewMenu("navigation", styles.Container, menuItems)
	model.SetStyles(styles)
	return model
}

// SetStyles replaces all navigation styles and invalidates rendered content.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
	if m.menu != nil {
		m.menu.SetStyle(styles.Container)
	}
	m.render()
}

// SetWidth sets the terminal width used to center the navigation.
func (m *Model) SetWidth(width int) {
	m.menu.SetViewport(width, 1)
	m.render()
}

// Active returns the active tool.
func (m Model) Active() Tool {
	return m.active
}

// Update handles navigation activation and mouse hover.
func (m Model) Update(message tea.Msg) (Model, tea.Cmd) {
	switch message := message.(type) {
	case ActivateMsg:
		if m.active != message.Tool {
			m.active = message.Tool
			m.render()
		}
	case tea.MouseMotionMsg:
		tool, ok := m.toolAt(message.X, message.Y)
		if m.hovered != ok || ok && m.hover != tool {
			m.hover, m.hovered = tool, ok
			m.render()
		}
	case tea.MouseClickMsg:
		if message.Button == tea.MouseLeft {
			if tool, ok := m.toolAt(message.X, message.Y); ok {
				return m, Activate(tool)
			}
		}
	}
	return m, nil
}

// View returns the rendered floating navigation.
func (m Model) View() string {
	return strings.Join(m.lines, "\n")
}

// Lines returns cached rendered rows for cell-aware canvas composition.
func (m Model) Lines() []string {
	return m.lines
}

// LinesFor returns rendered rows with active highlighted without mutating state.
func (m Model) LinesFor(active Tool) []string {
	if active == m.active {
		return m.lines
	}
	return m.renderLines(active)
}

// Width returns the rendered cell width.
func (m Model) Width() int {
	return m.menu.Bounds().Width
}

// Height returns the rendered cell height.
func (m Model) Height() int {
	return m.menu.Bounds().Height
}

// Top returns the first rendered row.
func (m Model) Top() int {
	return m.menu.Bounds().Y
}

// Bounds returns the rectangle used for rendering and pointer input.
func (m Model) Bounds() chrome.Rect {
	return m.menu.Bounds()
}

// Contains reports whether a terminal cell falls inside the navigation.
func (m Model) Contains(x, y int) bool {
	return m.menu.Contains(chrome.Point{X: x, Y: y})
}

// Cell returns the first terminal cell occupied by tool's label.
func (m Model) Cell(tool Tool) (x, y int, ok bool) {
	for _, item := range m.items {
		if item.Tool == tool {
			rect, found := m.menu.ItemRect(item.ID)
			return rect.X, rect.Y, found
		}
	}
	return 0, 0, false
}

func (m Model) toolAt(x, y int) (Tool, bool) {
	menuItem, ok := m.menu.ItemAt(chrome.Point{X: x, Y: y})
	if !ok {
		return 0, false
	}
	for _, item := range m.items {
		if item.ID == menuItem.ID {
			return item.Tool, true
		}
	}
	return 0, false
}

func (m *Model) render() {
	m.lines = m.renderLines(m.active)
}

func (m Model) renderLines(active Tool) []string {
	return m.menu.Lines(func(item chrome.MenuItem) string {
		declared := m.item(item.ID)
		text := m.styles.Item.Render(declared.Label)
		switch {
		case active == declared.Tool:
			text = m.styles.Active.Render(declared.Label)
		case m.hovered && m.hover == declared.Tool:
			text = m.styles.Hover.Render(declared.Label)
		}
		return text
	})
}

func (m Model) item(id chrome.ID) Item {
	for _, item := range m.items {
		if item.ID == id {
			return item
		}
	}
	return Item{}
}
