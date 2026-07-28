// Package preferences implements the editor preferences form.
package preferences

import (
	"math"
	"os"
	"strings"

	keybinding "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/numinput"
	"github.com/coxley/dg/layout"
)

// Value contains editable user preferences.
type Value struct {
	Router        layout.Router
	SaveDirectory string
	CommentPrefix string
}

// Action identifies how an edited preference form should close.
type Action uint8

const (
	ActionNone Action = iota
	ActionSave
	ActionSaveDefaults
	ActionCancel
)

var actionLabels = [...]string{
	ActionSave:         "Save",
	ActionSaveDefaults: "Save as Defaults",
	ActionCancel:       "Cancel",
}

// Styles defines all preferences-owned appearance.
type Styles struct {
	Form           huh.Theme
	NumInput       numinput.Styles
	Title          lipgloss.Style
	FocusedTitle   lipgloss.Style
	Value          lipgloss.Style
	FocusedValue   lipgloss.Style
	Action         lipgloss.Style
	SelectedAction lipgloss.Style
}

type formValue struct {
	step          uint32
	sharedStep    uint32
	bend          uint32
	crossing      uint32
	endpoint      uint32
	reroutePasses uint8
	saveDirectory string
	commentPrefix string
	action        Action
	submitted     bool
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
	rows          []*rowField
	directory     *directoryField
	actions       *actionField
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
	switch message := message.(type) {
	case ClickMsg:
		m.click(message.X, message.Y)
		return m, nil
	case ScrollMsg:
		return m, m.scroll(message.Delta)
	case tea.KeyPressMsg:
		if handled, command := m.updateCollapsedDirectory(message); handled {
			return m, command
		}
		if message.Code == tea.KeyEscape && m.directory.Zoom() {
			m.directory.close()
			form, command := m.form.Update(refreshMsg{})
			m.form = form.(*huh.Form)
			return m, wrap(command)
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
	m.form, m.fields, m.rows, m.directory, m.actions, m.naturalHeight = newForm(
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

// Completed reports the submitted form action.
func (m *Model) Completed() (Action, bool) {
	return m.input.action, m.input.submitted
}

// NaturalHeight returns the unconstrained form height.
func (m *Model) NaturalHeight() int {
	return m.naturalHeight
}

// DirectoryOpen reports whether the directory browser replaces the form.
func (m *Model) DirectoryOpen() bool {
	return m.directory.Zoom()
}

// SetHeight replaces the available form height.
func (m *Model) SetHeight(height int) {
	m.height = height
	if height <= 0 {
		height = m.naturalHeight
	}
	m.form.WithHeight(min(height, m.naturalHeight))
}

// SetWidth replaces the available form width.
func (m *Model) SetWidth(width int) {
	m.width = max(width, 0)
	m.form.WithWidth(m.width)
	form, _ := m.form.Update(refreshMsg{})
	m.form = form.(*huh.Form)
}

// SetStyles replaces all visual styles.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
	m.form.WithTheme(styles.Form)
	for _, field := range m.fields {
		field.SetStyles(styles.NumInput)
	}
	for _, row := range m.rows {
		row.SetStyles(styles)
	}
	m.actions.styles = styles
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
		saveDirectory: value.SaveDirectory,
		commentPrefix: NormalizeCommentPrefix(value.CommentPrefix),
		action:        ActionSave,
	}
}

func newForm(
	value *formValue,
	width, height int,
	styles Styles,
) (
	*huh.Form,
	[]numericField,
	[]*rowField,
	*directoryField,
	*actionField,
	int,
) {
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
	filePicker := huh.NewFilePicker().
		Title("Default save directory").
		DirAllowed(true).
		FileAllowed(false).
		ShowHidden(false).
		CurrentDirectory(directory).
		Picking(false).
		Value(&value.saveDirectory)
	directoryField := newDirectoryField(
		filePicker,
		styles,
	)
	commentSelect := huh.NewSelect[string]().
		Title("Preferred comments").
		Options(
			huh.NewOption("//", "// "),
			huh.NewOption("#", "# "),
			huh.NewOption("/* */", "/* */"),
		).
		Inline(true).
		Value(&value.commentPrefix)
	commentField := newRowField(
		commentSelect,
		"Preferred comments",
		choiceControl,
		styles,
	)
	actions := newActionField(&value.action, &value.submitted, styles)
	fields := []huh.Field{
		inputs[0],
		inputs[1],
		inputs[2],
		inputs[3],
		inputs[4],
		inputs[5],
		commentField,
		directoryField,
		actions,
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
	return form, inputs, []*rowField{
		commentField,
		directoryField.rowField,
	}, directoryField, actions, naturalHeight
}

func keyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Input.Prev = keybinding.NewBinding(
		keybinding.WithKeys("up", "k"),
		keybinding.WithHelp("↑", "previous"),
	)
	keymap.Input.Next = keybinding.NewBinding(
		keybinding.WithKeys("down", "j", "enter"),
		keybinding.WithHelp("↓", "next"),
	)
	keymap.Confirm.Prev = keymap.Input.Prev
	keymap.Confirm.Next = keymap.Input.Next
	keymap.Select.Prev = keymap.Input.Prev
	keymap.Select.Next = keymap.Input.Next
	keymap.Select.Up = keybinding.NewBinding(
		keybinding.WithKeys("left", "h"),
		keybinding.WithHelp("←", "choice"),
	)
	keymap.Select.Down = keybinding.NewBinding(
		keybinding.WithKeys("right", "l"),
		keybinding.WithHelp("→", "choice"),
	)
	return keymap
}

// ClickMsg identifies a form-local pointer click.
type ClickMsg struct {
	X int
	Y int
}

// ScrollMsg moves form focus without activating the focused field.
type ScrollMsg struct {
	Delta int
}

type refreshMsg struct{}

func (m *Model) click(x, y int) {
	lines := strings.Split(ansi.Strip(m.form.View()), "\n")
	actionView := ansi.Strip(m.actions.View())
	for row, line := range lines {
		start := strings.Index(line, actionView)
		if row == y && start >= 0 {
			m.actions.hit(x - start)
			return
		}
	}
}

func (m *Model) scroll(delta int) tea.Cmd {
	if delta == 0 || m.directory.Zoom() {
		return nil
	}
	if m.form.GetFocusedField() == m.actions && delta > 0 {
		return nil
	}
	message := huh.PrevField()
	if delta > 0 {
		message = huh.NextField()
	}
	form, command := m.form.Update(message)
	m.form = form.(*huh.Form)
	return wrap(command)
}

func (m *Model) updateCollapsedDirectory(message tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.directory.Zoom() || m.form.GetFocusedField() != m.directory {
		return false, nil
	}
	switch message.Text {
	case "j":
		return true, m.scroll(1)
	case "k":
		return true, m.scroll(-1)
	case "h":
		return true, m.scroll(-1)
	}
	return false, nil
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
