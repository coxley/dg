package preferences

import (
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
)

type rowField struct {
	field   huh.Field
	title   string
	control func(any, bool) string
	width   int
	focused bool
	styles  Styles
}

type directoryField struct {
	*rowField
	picker *huh.FilePicker
}

func newRowField(
	field huh.Field,
	title string,
	control func(any, bool) string,
	styles Styles,
) *rowField {
	return &rowField{
		field:   field,
		title:   title,
		control: control,
		styles:  styles,
	}
}

func (f *rowField) Init() tea.Cmd {
	return f.field.Init()
}

func (f *rowField) Update(message tea.Msg) (huh.Model, tea.Cmd) {
	model, command := f.field.Update(message)
	f.field = model.(huh.Field)
	return f, command
}

func (f *rowField) View() string {
	if f.field.Zoom() {
		return f.field.View()
	}
	titleStyle := f.styles.Title
	valueStyle := f.styles.Value
	if f.focused {
		titleStyle = f.styles.FocusedTitle
		valueStyle = f.styles.FocusedValue
	}
	title := titleStyle.Render(f.title)
	control := valueStyle.Render(
		f.control(f.field.GetValue(), f.focused),
	)
	return justifyApart(title, control, f.width)
}

func (f *rowField) Focus() tea.Cmd {
	f.focused = true
	return f.field.Focus()
}

func (f *rowField) Blur() tea.Cmd {
	f.focused = false
	return f.field.Blur()
}

func (f *rowField) Error() error {
	return f.field.Error()
}

func (f *rowField) Run() error {
	return f.field.Run()
}

func (f *rowField) RunAccessible(writer io.Writer, reader io.Reader) error {
	return f.field.RunAccessible(writer, reader)
}

func (f *rowField) Skip() bool {
	return f.field.Skip()
}

func (f *rowField) Zoom() bool {
	return f.field.Zoom()
}

func (f *rowField) KeyBinds() []key.Binding {
	return f.field.KeyBinds()
}

func (f *rowField) WithTheme(theme huh.Theme) huh.Field {
	f.field = f.field.WithTheme(theme)
	return f
}

func (f *rowField) WithKeyMap(keymap *huh.KeyMap) huh.Field {
	f.field = f.field.WithKeyMap(keymap)
	return f
}

func (f *rowField) WithWidth(width int) huh.Field {
	f.width = max(width, 0)
	f.field = f.field.WithWidth(width)
	return f
}

func (f *rowField) WithHeight(height int) huh.Field {
	f.field = f.field.WithHeight(height)
	return f
}

func (f *rowField) WithPosition(position huh.FieldPosition) huh.Field {
	f.field = f.field.WithPosition(position)
	return f
}

func (f *rowField) GetKey() string {
	return f.field.GetKey()
}

func (f *rowField) GetValue() any {
	return f.field.GetValue()
}

func (f *rowField) SetStyles(styles Styles) {
	f.styles = styles
}

func newDirectoryField(
	picker *huh.FilePicker,
	styles Styles,
) *directoryField {
	return &directoryField{
		rowField: newRowField(
			picker,
			"Default save directory",
			browseControl,
			styles,
		),
		picker: picker,
	}
}

func (f *directoryField) close() {
	f.picker.Picking(false)
}

func browseControl(any, bool) string {
	return "[ browse ]"
}

func choiceControl(value any, focused bool) string {
	choice := strings.TrimSpace(value.(string))
	if focused {
		return "⇽ " + choice + " ⇾"
	}
	return choice
}

func justifyApart(left, right string, width int) string {
	if width <= 0 {
		return left + "  " + right
	}
	rightWidth := ansi.StringWidth(right)
	leftWidth := max(width-rightWidth-1, 0)
	left = ansi.Truncate(left, leftWidth, "")
	return left +
		strings.Repeat(" ", max(width-ansi.StringWidth(left)-rightWidth, 0)) +
		right
}
