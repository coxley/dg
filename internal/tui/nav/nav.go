// Package nav implements the floating editor tool navigation.
package nav

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Tool identifies an editor tool.
type Tool uint8

const (
	Cursor Tool = iota
	Rectangle
	Line
)

const top = 1

var items = [...]struct {
	tool  Tool
	label string
}{
	{Cursor, " Cursor "},
	{Rectangle, " Rectangle "},
	{Line, " Line "},
}

// Styles defines the floating navigation appearance.
type Styles struct {
	Container lipgloss.Style
	Active    lipgloss.Style
	Hover     lipgloss.Style
}

type geometry struct {
	frameWidth  int
	frameHeight int
	contentLeft int
	contentTop  int
	toolsWidth  int
}

// Model contains floating navigation state and rendered-content caches.
type Model struct {
	styles  Styles
	geo     geometry
	width   int
	active  Tool
	hover   Tool
	hovered bool

	lines         []string
	renderActive  Tool
	renderHover   Tool
	renderHovered bool
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
func New(styles Styles) Model {
	var model Model
	model.SetStyles(styles)
	return model
}

// SetStyles replaces all navigation styles and invalidates rendered content.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
	m.geo = geometry{
		frameWidth:  styles.Container.GetHorizontalFrameSize(),
		frameHeight: styles.Container.GetVerticalFrameSize(),
		contentLeft: styles.Container.GetBorderLeftSize() +
			styles.Container.GetPaddingLeft(),
		contentTop: styles.Container.GetBorderTopSize() +
			styles.Container.GetPaddingTop(),
	}
	for _, item := range items {
		m.geo.toolsWidth += lipgloss.Width(item.label)
	}
	m.lines = nil
}

// SetWidth sets the terminal width used to center the navigation.
func (m *Model) SetWidth(width int) {
	m.width = max(width, 0)
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
			m.lines = nil
		}
	case tea.MouseMotionMsg:
		tool, ok := m.toolAt(message.X, message.Y)
		if m.hovered != ok || ok && m.hover != tool {
			m.hover, m.hovered = tool, ok
			m.lines = nil
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
func (m *Model) View() string {
	if m.lines == nil ||
		m.renderActive != m.active ||
		m.renderHovered != m.hovered ||
		m.hovered && m.renderHover != m.hover {
		m.render()
	}
	return strings.Join(m.lines, "\n")
}

// Lines returns cached rendered rows for cell-aware canvas composition.
func (m *Model) Lines() []string {
	if m.lines == nil {
		m.render()
	}
	return m.lines
}

// Width returns the rendered cell width.
func (m Model) Width() int {
	return m.geo.toolsWidth + m.geo.frameWidth
}

// Height returns the rendered cell height.
func (m Model) Height() int {
	return 1 + m.geo.frameHeight
}

// Top returns the first rendered row.
func (m Model) Top() int {
	return top
}

// Contains reports whether a terminal cell falls inside the navigation.
func (m Model) Contains(x, y int) bool {
	width := m.Width()
	if m.width < width || y < top || y >= top+m.Height() {
		return false
	}
	left := (m.width - width) / 2
	return x >= left && x < left+width
}

// Cell returns the first terminal cell occupied by tool's label.
func (m Model) Cell(tool Tool) (x, y int, ok bool) {
	if m.width < m.Width() {
		return 0, 0, false
	}
	x = (m.width-m.Width())/2 + m.geo.contentLeft
	for _, item := range items {
		if item.tool == tool {
			return x, top + m.geo.contentTop, true
		}
		x += lipgloss.Width(item.label)
	}
	return 0, 0, false
}

func (m Model) toolAt(x, y int) (Tool, bool) {
	if !m.Contains(x, y) || y != top+m.geo.contentTop {
		return 0, false
	}
	x -= (m.width-m.Width())/2 + m.geo.contentLeft
	for _, item := range items {
		width := lipgloss.Width(item.label)
		if x >= 0 && x < width {
			return item.tool, true
		}
		x -= width
	}
	return 0, false
}

func (m *Model) render() {
	var content strings.Builder
	content.Grow(m.geo.toolsWidth)
	for _, item := range items {
		text := item.label
		switch {
		case m.active == item.tool:
			text = m.styles.Active.Render(text)
		case m.hovered && m.hover == item.tool:
			text = m.styles.Hover.Render(text)
		}
		content.WriteString(text)
	}
	m.lines = strings.Split(m.styles.Container.Render(content.String()), "\n")
	m.renderActive = m.active
	m.renderHover = m.hover
	m.renderHovered = m.hovered
}
