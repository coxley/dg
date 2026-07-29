package chrome

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Button declares one semantic button.
type Button struct {
	ID    ID
	Label string
}

// ButtonListDeclaration declares one horizontal button list.
type ButtonListDeclaration struct {
	ID      ID
	Buttons []Button
}

// ButtonListStyles defines geometry-stable button states.
type ButtonListStyles struct {
	Button        lipgloss.Style
	FocusedButton lipgloss.Style
}

// ButtonPlan records one arranged button hit target.
type ButtonPlan struct {
	ID   ID
	Rect Rect
}

// ButtonListPlan is one retained horizontal button arrangement.
type ButtonListPlan struct {
	Version uint64
	ID      ID
	Bounds  Rect
	Buttons []ButtonPlan
}

// ButtonPressMsg reports one activated button.
type ButtonPressMsg struct {
	ID ID
}

// ButtonList retains horizontal button focus, arrangement, and render data.
type ButtonList struct {
	declaration ButtonListDeclaration
	styles      ButtonListStyles
	bounds      Rect
	hugHeight   bool
	focused     bool
	focus       int
	version     uint64
	plan        ButtonListPlan
	lines       []string
}

// NewButtonList returns a horizontal button list.
func NewButtonList(
	declaration ButtonListDeclaration,
	styles ButtonListStyles,
) *ButtonList {
	list := &ButtonList{
		declaration: cloneButtonListDeclaration(declaration),
		styles:      styles,
		focused:     true,
	}
	list.normalize()
	list.arrange()
	return list
}

// Init implements tea.Model.
func (*ButtonList) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (l *ButtonList) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return l, nil
	}
	id, pressed := l.applyIntent(ResolveControlIntent(key, false))
	if !pressed {
		return l, nil
	}
	return l, emitButtonMessage(ButtonPressMsg{ID: id})
}

// View implements tea.Model.
func (l *ButtonList) View() tea.View {
	return tea.NewView(strings.Join(l.lines, "\n"))
}

// SetBounds arranges the list and its hit targets immediately.
func (l *ButtonList) SetBounds(bounds Rect) {
	bounds.Width = max(bounds.Width, 0)
	bounds.Height = max(bounds.Height, 0)
	hugHeight := bounds.Height == 0
	if l.bounds == bounds && l.hugHeight == hugHeight {
		return
	}
	l.bounds = bounds
	l.hugHeight = hugHeight
	l.invalidate()
}

// SetStyles replaces semantic visual states.
func (l *ButtonList) SetStyles(styles ButtonListStyles) {
	l.styles = styles
	l.invalidate()
}

// SetFocused controls whether the selected button displays focus.
func (l *ButtonList) SetFocused(focused bool) {
	if l.focused == focused {
		return
	}
	l.focused = focused
	l.invalidate()
}

// Plan returns an immutable copy of the current arrangement.
func (l *ButtonList) Plan() ButtonListPlan {
	plan := l.plan
	plan.Buttons = append([]ButtonPlan(nil), plan.Buttons...)
	return plan
}

// MoveFocus traverses buttons with wrapping.
func (l *ButtonList) MoveFocus(delta int) {
	if len(l.declaration.Buttons) == 0 || delta == 0 {
		return
	}
	l.moveFocus(delta, true)
	l.invalidate()
}

// Focus moves focus to one declared button.
func (l *ButtonList) Focus(id ID) bool {
	for index, button := range l.declaration.Buttons {
		if button.ID != id {
			continue
		}
		if l.focus == index {
			return true
		}
		l.focus = index
		l.invalidate()
		return true
	}
	return false
}

// FocusID returns the focused button.
func (l *ButtonList) FocusID() ID {
	if len(l.declaration.Buttons) == 0 {
		return ""
	}
	return l.declaration.Buttons[l.focus].ID
}

// Click focuses and activates the button at point.
func (l *ButtonList) Click(point Point) tea.Cmd {
	id, pressed := l.press(point)
	if !pressed {
		return nil
	}
	return emitButtonMessage(ButtonPressMsg{ID: id})
}

func (l *ButtonList) applyIntent(intent ControlIntent) (ID, bool) {
	switch intent {
	case NavigateLeft:
		l.moveFocus(-1, false)
		l.invalidate()
	case NavigateRight:
		l.moveFocus(1, false)
		l.invalidate()
	case FocusPrevious:
		l.MoveFocus(-1)
	case FocusNext:
		l.MoveFocus(1)
	case Activate:
		if id := l.FocusID(); id != "" {
			return id, true
		}
	case NoControlIntent:
	}
	return "", false
}

func (l *ButtonList) moveFocus(delta int, wrap bool) {
	if len(l.declaration.Buttons) == 0 || delta == 0 {
		return
	}
	if wrap {
		l.focus = wrappedIndex(l.focus, delta, len(l.declaration.Buttons))
		return
	}
	l.focus = min(max(l.focus+delta, 0), len(l.declaration.Buttons)-1)
}

func (l *ButtonList) press(point Point) (ID, bool) {
	for index, button := range l.plan.Buttons {
		if !button.Rect.Contains(point) {
			continue
		}
		l.focus = index
		l.invalidate()
		return button.ID, true
	}
	return "", false
}

func (l *ButtonList) normalize() {
	l.focus = min(max(l.focus, 0), max(len(l.declaration.Buttons)-1, 0))
}

func (l *ButtonList) invalidate() {
	l.version++
	l.arrange()
}

func (l *ButtonList) arrange() {
	l.normalize()
	views := make([]string, len(l.declaration.Buttons))
	widths := make([]int, len(views))
	for index, button := range l.declaration.Buttons {
		style := l.styles.Button
		if l.focused && index == l.focus {
			style = l.styles.FocusedButton
		}
		views[index] = style.Render(button.Label)
		widths[index] = lipgloss.Width(views[index])
	}
	rendered := lipgloss.JoinHorizontal(lipgloss.Top, views...)
	renderedLines := strings.Split(rendered, "\n")
	if len(views) == 0 {
		renderedLines = nil
	}
	bounds := l.bounds
	if l.hugHeight {
		bounds.Height = len(renderedLines)
	}
	lines := make([]string, bounds.Height)
	for row := range lines {
		line := ""
		if row < len(renderedLines) {
			line = ansi.Truncate(renderedLines[row], bounds.Width, "")
		}
		lines[row] = padLine(line, bounds.Width)
	}
	plan := ButtonListPlan{
		Version: l.version,
		ID:      l.declaration.ID,
		Bounds:  bounds,
		Buttons: make([]ButtonPlan, 0, len(l.declaration.Buttons)),
	}
	left := bounds.X
	for index, button := range l.declaration.Buttons {
		plan.Buttons = append(plan.Buttons, ButtonPlan{
			ID: button.ID,
			Rect: intersectRect(Rect{
				X: left, Y: bounds.Y, Width: widths[index], Height: len(renderedLines),
			}, bounds),
		})
		left += widths[index]
	}
	l.plan = plan
	l.lines = lines
}

func cloneButtonListDeclaration(declaration ButtonListDeclaration) ButtonListDeclaration {
	clone := declaration
	clone.Buttons = append([]Button(nil), declaration.Buttons...)
	return clone
}

func wrappedIndex(index, delta, count int) int {
	return (index + delta%count + count) % count
}

func emitButtonMessage(message tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return message
	}
}
