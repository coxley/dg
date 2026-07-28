// Package preferences implements the editor preferences form.
package preferences

import (
	"math"
	"os"

	keybinding "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
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
	step          uint32
	sharedStep    uint32
	bend          uint32
	crossing      uint32
	endpoint      uint32
	reroutePasses uint8
	applyToFuture bool
	saveDirectory string
	commentPrefix string
	save          bool
}

type numericField interface {
	huh.Field
	HandleFlash(numinput.FlashExpiredMsg) bool
	SetStyles(numinput.Styles)
	Flash() int
}

// UpdateMsg routes a child form command back to Model.Update.
type UpdateMsg struct {
	message tea.Msg
}

// Model owns the preferences form and its editable value.
type Model struct {
	value         Value
	input         formValue
	form          *huh.Form
	fields        []numericField
	width         int
	height        int
	naturalHeight int
	styles        Styles
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
	m.form, m.fields, m.naturalHeight = newForm(
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
	if height <= 0 {
		height = m.naturalHeight
	}
	m.form.WithHeight(min(height, m.naturalHeight))
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
	router.Costs.Step = m.input.step
	router.Costs.SharedStep = m.input.sharedStep
	router.Costs.Bend = m.input.bend
	router.Costs.Crossing = m.input.crossing
	router.Costs.EndpointStep = m.input.endpoint
	router.ReroutePasses = m.input.reroutePasses
	m.value = Value{
		Router:        router,
		ApplyToFuture: m.input.applyToFuture,
		SaveDirectory: m.input.saveDirectory,
		CommentPrefix: NormalizeCommentPrefix(m.input.commentPrefix),
	}
}

func formValueFrom(value Value) formValue {
	return formValue{
		step:          value.Router.Costs.Step,
		sharedStep:    value.Router.Costs.SharedStep,
		bend:          value.Router.Costs.Bend,
		crossing:      value.Router.Costs.Crossing,
		endpoint:      value.Router.Costs.EndpointStep,
		reroutePasses: value.Router.ReroutePasses,
		applyToFuture: value.ApplyToFuture,
		saveDirectory: value.SaveDirectory,
		commentPrefix: NormalizeCommentPrefix(value.CommentPrefix),
		save:          true,
	}
}

func newForm(
	value *formValue,
	width, height int,
	styles Styles,
) (*huh.Form, []numericField, int) {
	keymap := keyMap()
	inputs := []numericField{
		numinput.NewField("Step cost", &value.step, uint32(math.MaxUint32), styles.NumInput),
		numinput.NewField("Shared-step cost", &value.sharedStep, uint32(math.MaxUint32), styles.NumInput),
		numinput.NewField("Bend cost", &value.bend, uint32(math.MaxUint32), styles.NumInput),
		numinput.NewField("Crossing cost", &value.crossing, uint32(math.MaxUint32), styles.NumInput),
		numinput.NewField("Endpoint cost", &value.endpoint, uint32(math.MaxUint32), styles.NumInput),
		numinput.NewField("Reroute passes", &value.reroutePasses, uint8(math.MaxUint8), styles.NumInput),
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
		WithShowHelp(false).
		WithKeyMap(keymap).
		WithTheme(styles.Form)
	_ = form.Init()
	naturalHeight := lipgloss.Height(form.View())
	if height <= 0 {
		height = naturalHeight
	}
	form.WithHeight(min(height, naturalHeight))
	return form, inputs, naturalHeight
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

// NormalizeCommentPrefix returns a supported comment preference.
func NormalizeCommentPrefix(prefix string) string {
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
