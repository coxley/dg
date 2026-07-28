package preferences

import (
	"fmt"
	"io"
	"strings"

	keybinding "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

type actionField struct {
	action    *Action
	submitted *bool
	styles    Styles
	focused   bool
	selected  Action
}

func newActionField(
	action *Action,
	submitted *bool,
	styles Styles,
) *actionField {
	return &actionField{
		action:    action,
		submitted: submitted,
		styles:    styles,
		selected:  ActionSave,
	}
}

func (*actionField) Init() tea.Cmd { return nil }

func (f *actionField) Update(message tea.Msg) (huh.Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}
	switch key.Code {
	case tea.KeyUp:
		return f, huh.PrevField
	case tea.KeyLeft:
		f.selectBy(-1)
	case tea.KeyRight:
		f.selectBy(1)
	case tea.KeyEnter:
		f.submit(f.selected)
	default:
		switch key.Text {
		case "k":
			return f, huh.PrevField
		case "h":
			f.selectBy(-1)
		case "l":
			f.selectBy(1)
		}
	}
	return f, nil
}

func (f *actionField) View() string {
	return f.content()
}

func (f *actionField) content() string {
	views := make([]string, 0, len(actionLabels))
	for action := ActionSave; action <= ActionCancel; action++ {
		views = append(views, f.button(action))
	}
	return lipgloss.JoinHorizontal(1, views...)
}

func (f *actionField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *actionField) Blur() tea.Cmd {
	f.focused = false
	return nil
}

func (*actionField) Error() error { return nil }
func (*actionField) Run() error   { return nil }

func (f *actionField) RunAccessible(writer io.Writer, _ io.Reader) error {
	_, err := fmt.Fprintln(writer, strings.Join(actionLabels[1:], ", "))
	return err
}

func (*actionField) Skip() bool                         { return false }
func (*actionField) Zoom() bool                         { return false }
func (*actionField) KeyBinds() []keybinding.Binding     { return nil }
func (f *actionField) WithTheme(huh.Theme) huh.Field    { return f }
func (f *actionField) WithKeyMap(*huh.KeyMap) huh.Field { return f }
func (f *actionField) WithWidth(int) huh.Field          { return f }
func (f *actionField) WithHeight(int) huh.Field         { return f }
func (f *actionField) WithPosition(huh.FieldPosition) huh.Field {
	return f
}
func (*actionField) GetKey() string  { return "action" }
func (f *actionField) GetValue() any { return *f.action }

func (f *actionField) hit(x, y int) {
	for action := ActionSave; action <= ActionCancel; action++ {
		style := f.buttonStyle(action)
		button := style.Render(actionLabels[action])
		width, height := lipgloss.Width(button), lipgloss.Height(button)
		if x >= style.GetMarginLeft() &&
			x < width-style.GetMarginRight() &&
			y >= style.GetMarginTop() &&
			y < height-style.GetMarginBottom() {
			f.submit(action)
			return
		}
		x -= width
	}
}

func (f *actionField) button(action Action) string {
	return f.buttonStyle(action).Render(actionLabels[action])
}

func (f *actionField) buttonStyle(action Action) lipgloss.Style {
	style := f.styles.Action
	if f.focused && action == f.selected {
		style = f.styles.SelectedAction
	}
	return style
}

func (f *actionField) selectBy(delta int) {
	f.selected = Action(min(
		max(int(f.selected)+delta, int(ActionSave)),
		int(ActionCancel),
	))
}

func (f *actionField) submit(action Action) {
	f.selected = action
	*f.action = action
	*f.submitted = true
}
