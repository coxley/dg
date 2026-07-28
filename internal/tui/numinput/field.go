package numinput

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"golang.org/x/exp/constraints"
)

// Field adapts Model for use in a Huh form.
type Field[T constraints.Integer] struct {
	input    *Model[T]
	width    int
	height   int
	position huh.FieldPosition
	previous key.Binding
	next     key.Binding
}

// NewField returns a Huh field backed by an independent numeric model.
func NewField[T constraints.Integer](
	title string,
	value *T,
	maxValue T,
	styles Styles,
) *Field[T] {
	return &Field[T]{
		input: New(title, value, maxValue, styles),
		previous: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "previous"),
		),
		next: key.NewBinding(
			key.WithKeys("down", "j", "enter"),
			key.WithHelp("↓", "next"),
		),
	}
}

// HandleFlash routes delayed feedback to this field's model.
func (f *Field[T]) HandleFlash(message FlashExpiredMsg) bool {
	return f.input.HandleFlash(message)
}

// SetStyles replaces the field's visual styles.
func (f *Field[T]) SetStyles(styles Styles) {
	f.input.SetStyles(styles)
}

// Flash reports the active input direction.
func (f *Field[T]) Flash() int {
	return f.input.Flash()
}

func (f *Field[T]) Init() tea.Cmd {
	return f.input.Init()
}

func (f *Field[T]) Update(message tea.Msg) (huh.Model, tea.Cmd) {
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

func (f *Field[T]) View() string {
	return f.input.Render()
}

func (f *Field[T]) Focus() tea.Cmd {
	f.input.SetFocused(true)
	return nil
}

func (f *Field[T]) Blur() tea.Cmd {
	f.input.SetFocused(false)
	return nil
}

func (*Field[T]) Error() error {
	return nil
}

func (*Field[T]) Run() error {
	return nil
}

func (f *Field[T]) RunAccessible(writer io.Writer, _ io.Reader) error {
	_, err := fmt.Fprintf(writer, "%s: %v\n", f.input.title, f.GetValue())
	return err
}

func (*Field[T]) Skip() bool {
	return false
}

func (*Field[T]) Zoom() bool {
	return false
}

func (f *Field[T]) KeyBinds() []key.Binding {
	return []key.Binding{f.previous, f.next}
}

func (f *Field[T]) WithTheme(huh.Theme) huh.Field {
	return f
}

func (f *Field[T]) WithKeyMap(*huh.KeyMap) huh.Field {
	return f
}

func (f *Field[T]) WithWidth(width int) huh.Field {
	f.width = width
	f.input.SetWidth(width)
	return f
}

func (f *Field[T]) WithHeight(height int) huh.Field {
	f.height = height
	return f
}

func (f *Field[T]) WithPosition(position huh.FieldPosition) huh.Field {
	f.position = position
	f.previous.SetEnabled(!position.IsFirst())
	f.next.SetEnabled(!position.IsLast())
	return f
}

func (f *Field[T]) GetKey() string {
	return f.input.title
}

func (f *Field[T]) GetValue() any {
	return *f.input.value
}
