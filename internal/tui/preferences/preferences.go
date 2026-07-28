// Package preferences implements the editor preferences form.
package preferences

import (
	"os"
	"strconv"

	keybinding "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/coxley/dg/internal/tui/numinput"
	"github.com/coxley/dg/layout"
)

// Value contains editable user preferences.
type Value struct {
	Router        layout.Router
	ApplyToFuture bool
	SaveDirectory string
	CommentPrefix string
}

// Styles defines all preferences-owned appearance.
type Styles struct {
	Form     huh.Theme
	NumInput numinput.Styles
}

type formValue struct {
	step          string
	sharedStep    string
	bend          string
	crossing      string
	endpoint      string
	reroutePasses string
	applyToFuture bool
	saveDirectory string
	commentPrefix string
	save          bool
}

// UpdateMsg routes a child form command back to Model.Update.
type UpdateMsg struct {
	message tea.Msg
}

// Model owns the preferences form and its editable value.
type Model struct {
	value  Value
	input  formValue
	form   *huh.Form
	fields []*numinput.Field
	width  int
	height int
	styles Styles
}

// New returns a preferences model.
func New(value Value, width, height int, styles Styles) *Model {
	model := &Model{
		width:  width,
		height: height,
		styles: styles,
	}
	model.Reset(value)
	return model
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return m.form.Init()
}

// Update implements tea.Model.
func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if update, ok := message.(UpdateMsg); ok {
		message = update.message
	}
	if flash, ok := message.(numinput.FlashExpiredMsg); ok {
		for _, field := range m.fields {
			if field.HandleFlash(flash) {
				return m, nil
			}
		}
	}
	form, command := m.form.Update(message)
	m.form = form.(*huh.Form)
	m.sync()
	return m, wrap(command)
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	return tea.NewView(m.form.View())
}

// Reset replaces the editable value and creates a fresh form.
func (m *Model) Reset(value Value) {
	m.value = value
	m.input = formValueFrom(value)
	m.form, m.fields = newForm(
		&m.input,
		m.width,
		m.height,
		m.styles,
	)
}

// Value returns the current form value.
func (m *Model) Value() Value {
	return m.value
}

// Completed reports whether the form finished and whether Save was selected.
func (m *Model) Completed() (save, completed bool) {
	return m.input.save, m.form.State == huh.StateCompleted
}

// SetHeight replaces the available form height.
func (m *Model) SetHeight(height int) {
	m.height = height
	m.form.WithHeight(height)
}

// SetStyles replaces all visual styles.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
	m.form.WithTheme(styles.Form)
	for _, field := range m.fields {
		field.SetStyles(styles.NumInput)
	}
}

// FieldFlash reports a numeric field's active direction for tests and diagnostics.
func (m *Model) FieldFlash(index int) int {
	if index < 0 || index >= len(m.fields) {
		return 0
	}
	return m.fields[index].Flash()
}

func (m *Model) sync() {
	router := m.value.Router
	parse32 := func(text string, destination *uint32) {
		if value, err := strconv.ParseUint(text, 10, 32); err == nil {
			*destination = uint32(value)
		}
	}
	parse32(m.input.step, &router.Costs.Step)
	parse32(m.input.sharedStep, &router.Costs.SharedStep)
	parse32(m.input.bend, &router.Costs.Bend)
	parse32(m.input.crossing, &router.Costs.Crossing)
	parse32(m.input.endpoint, &router.Costs.EndpointStep)
	if value, err := strconv.ParseUint(m.input.reroutePasses, 10, 8); err == nil {
		router.ReroutePasses = uint8(value)
	}
	m.value = Value{
		Router:        router,
		ApplyToFuture: m.input.applyToFuture,
		SaveDirectory: m.input.saveDirectory,
		CommentPrefix: normalizeCommentPrefix(m.input.commentPrefix),
	}
}

func formValueFrom(value Value) formValue {
	return formValue{
		step:          strconv.FormatUint(uint64(value.Router.Costs.Step), 10),
		sharedStep:    strconv.FormatUint(uint64(value.Router.Costs.SharedStep), 10),
		bend:          strconv.FormatUint(uint64(value.Router.Costs.Bend), 10),
		crossing:      strconv.FormatUint(uint64(value.Router.Costs.Crossing), 10),
		endpoint:      strconv.FormatUint(uint64(value.Router.Costs.EndpointStep), 10),
		reroutePasses: strconv.FormatUint(uint64(value.Router.ReroutePasses), 10),
		applyToFuture: value.ApplyToFuture,
		saveDirectory: value.SaveDirectory,
		commentPrefix: normalizeCommentPrefix(value.CommentPrefix),
		save:          true,
	}
}

func newForm(
	value *formValue,
	width, height int,
	styles Styles,
) (*huh.Form, []*numinput.Field) {
	keymap := keyMap()
	inputs := []*numinput.Field{
		numinput.NewField("Step cost", &value.step, 32, styles.NumInput),
		numinput.NewField("Shared-step cost", &value.sharedStep, 32, styles.NumInput),
		numinput.NewField("Bend cost", &value.bend, 32, styles.NumInput),
		numinput.NewField("Crossing cost", &value.crossing, 32, styles.NumInput),
		numinput.NewField("Endpoint cost", &value.endpoint, 32, styles.NumInput),
		numinput.NewField("Reroute passes", &value.reroutePasses, 8, styles.NumInput),
	}
	directory := value.saveDirectory
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		directory, _ = os.UserHomeDir()
	}
	fields := []huh.Field{
		inputs[0],
		inputs[1],
		inputs[2],
		inputs[3],
		inputs[4],
		inputs[5],
		huh.NewSelect[bool]().
			Options(
				huh.NewOption(option("Apply to future diagrams?", "Yes"), true),
				huh.NewOption(option("Apply to future diagrams?", "No"), false),
			).
			Inline(true).
			Value(&value.applyToFuture),
		huh.NewFilePicker().
			Title("Default save directory").
			DirAllowed(true).
			FileAllowed(false).
			ShowHidden(true).
			CurrentDirectory(directory).
			Picking(true).
			Value(&value.saveDirectory),
		huh.NewSelect[string]().
			Options(
				huh.NewOption(option("Preferred comments", "//"), "// "),
				huh.NewOption(option("Preferred comments", "#"), "# "),
				huh.NewOption(option("Preferred comments", "/* */"), "/* */"),
			).
			Inline(true).
			Value(&value.commentPrefix),
		huh.NewConfirm().
			Affirmative("Save").
			Negative("Cancel").
			Value(&value.save),
	}
	form := huh.NewForm(huh.NewGroup(fields...)).
		WithWidth(width).
		WithHeight(height).
		WithShowHelp(false).
		WithKeyMap(keymap).
		WithTheme(styles.Form)
	_ = form.Init()
	return form, inputs
}

func keyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Input.Prev = keybinding.NewBinding(
		keybinding.WithKeys("up", "shift+tab"),
		keybinding.WithHelp("↑", "previous"),
	)
	keymap.Input.Next = keybinding.NewBinding(
		keybinding.WithKeys("down", "enter", "tab"),
		keybinding.WithHelp("↓", "next"),
	)
	keymap.Confirm.Prev = keymap.Input.Prev
	keymap.Confirm.Next = keymap.Input.Next
	keymap.Select.Prev = keymap.Input.Prev
	keymap.Select.Next = keymap.Input.Next
	keymap.Select.Up = keybinding.NewBinding(
		keybinding.WithKeys("left"),
		keybinding.WithHelp("←", "choice"),
	)
	keymap.Select.Down = keybinding.NewBinding(
		keybinding.WithKeys("right"),
		keybinding.WithHelp("→", "choice"),
	)
	return keymap
}

func option(title, value string) string {
	return title + "  " + value
}

func normalizeCommentPrefix(prefix string) string {
	switch prefix {
	case "# ", "/* */":
		return prefix
	default:
		return "// "
	}
}

func wrap(command tea.Cmd) tea.Cmd {
	if command == nil {
		return nil
	}
	return func() tea.Msg {
		message := command()
		if message == nil {
			return nil
		}
		if batch, ok := message.(tea.BatchMsg); ok {
			for i := range batch {
				batch[i] = wrap(batch[i])
			}
			return batch
		}
		return UpdateMsg{message: message}
	}
}
