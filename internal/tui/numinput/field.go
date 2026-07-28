package numinput

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// Field adapts Model for use in a Huh form.
type Field struct {
	input    *Model
	width    int
	height   int
	position huh.FieldPosition
	previous key.Binding
	next     key.Binding
}

// NewField returns a Huh field backed by an independent numeric model.
func NewField(title string, value *string, bits int, styles Styles) *Field {
	return &Field{
		input: New(title, value, bits, styles),
		previous: key.NewBinding(
			key.WithKeys("up", "shift+tab"),
			key.WithHelp("↑", "previous"),
		),
		next: key.NewBinding(
			key.WithKeys("down", "enter", "tab"),
			key.WithHelp("↓", "next"),
		),
	}
}

// HandleFlash routes delayed feedback to this field's model.
func (f *Field) HandleFlash(message FlashExpiredMsg) bool {
	return f.input.HandleFlash(message)
}

// SetStyles replaces the field's visual styles.
func (f *Field) SetStyles(styles Styles) {
	f.input.SetStyles(styles)
}

// Flash reports the active input direction.
func (f *Field) Flash() int {
	return f.input.Flash()
}

func (f *Field) Init() tea.Cmd {
	return f.input.Init()
}

func (f *Field) Update(message tea.Msg) (huh.Model, tea.Cmd) {
	keyPress, ok := message.(tea.KeyPressMsg)
	if !ok {
		_, command := f.input.Update(message)
		return f, command
	}
	switch {
	case key.Matches(keyPress, f.previous):
		return f, huh.PrevField
	case key.Matches(keyPress, f.next):
		return f, huh.NextField
	default:
		_, command := f.input.Update(message)
		return f, command
	}
}

func (f *Field) View() string {
	return f.input.Render()
}

func (f *Field) Focus() tea.Cmd {
	f.input.SetFocused(true)
	return nil
}

func (f *Field) Blur() tea.Cmd {
	f.input.SetFocused(false)
	return nil
}

func (*Field) Error() error {
	return nil
}

func (*Field) Run() error {
	return nil
}

func (f *Field) RunAccessible(writer io.Writer, _ io.Reader) error {
	_, err := fmt.Fprintf(writer, "%s: %v\n", f.input.title, f.GetValue())
	return err
}

func (*Field) Skip() bool {
	return false
}

func (*Field) Zoom() bool {
	return false
}

func (f *Field) KeyBinds() []key.Binding {
	return []key.Binding{f.previous, f.next}
}

func (f *Field) WithTheme(huh.Theme) huh.Field {
	return f
}

func (f *Field) WithKeyMap(*huh.KeyMap) huh.Field {
	return f
}

func (f *Field) WithWidth(width int) huh.Field {
	f.width = width
	return f
}

func (f *Field) WithHeight(height int) huh.Field {
	f.height = height
	return f
}

func (f *Field) WithPosition(position huh.FieldPosition) huh.Field {
	f.position = position
	f.previous.SetEnabled(!position.IsFirst())
	f.next.SetEnabled(!position.IsLast())
	return f
}

func (f *Field) GetKey() string {
	return f.input.title
}

func (f *Field) GetValue() any {
	return *f.input.value
}
