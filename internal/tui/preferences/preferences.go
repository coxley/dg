// Package preferences implements the editor preferences form.
package preferences

import (
	"math"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
)

const (
	fieldStep          chrome.ID = "step"
	fieldSharedStep    chrome.ID = "shared-step"
	fieldBend          chrome.ID = "bend"
	fieldCrossing      chrome.ID = "crossing"
	fieldEndpoint      chrome.ID = "endpoint"
	fieldReroutePasses chrome.ID = "reroute-passes"
	fieldComment       chrome.ID = "comment"
	fieldDirectory     chrome.ID = "directory"
	fieldKeyProfile    chrome.ID = "key-profile"
	preferenceSpacer   chrome.ID = "preference-spacer"
	preferenceActions  chrome.ID = "preference-actions"
	actionSave         chrome.ID = "save"
	actionSaveDefaults chrome.ID = "save-defaults"
	actionCancel       chrome.ID = "cancel"
	commentSlash                 = "// "
	commentHash                  = "# "
	commentBlock                 = "/* */"
)

var numericFieldIDs = [...]chrome.ID{
	fieldStep,
	fieldSharedStep,
	fieldBend,
	fieldCrossing,
	fieldEndpoint,
	fieldReroutePasses,
}

// Value contains editable user preferences.
type Value struct {
	Router        layout.Router
	SaveDirectory string
	CommentPrefix string
	KeyProfile    chrome.KeyProfile
}

// Action identifies how an edited preference form should close.
type Action uint8

const (
	ActionNone Action = iota
	ActionSave
	ActionSaveDefaults
	ActionCancel
)

// Styles defines preferences appearance and the bounded picker adapter.
type Styles struct {
	Form   chrome.FormStyles
	Picker huh.Theme
}

// UpdateMsg routes a child form command back to Model.Update.
type UpdateMsg struct {
	message tea.Msg
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

// Model owns the preference declarations, editable value, and picker adapter.
type Model struct {
	value       Value
	form        *chrome.Form
	picker      *huh.FilePicker
	pickerValue string
	pickerOpen  bool
	action      Action
	completed   bool
	width       int
	height      int
	styles      Styles
}

// New returns a declarative preferences model.
func New(value Value, width, height int, styles Styles) *Model {
	m := &Model{width: max(width, 0), height: max(height, 0), styles: styles}
	m.Reset(value)
	return m
}

// Init implements tea.Model.
func (*Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if update, ok := message.(UpdateMsg); ok {
		message = update.message
	}
	if m.pickerOpen {
		return m, m.updatePicker(message)
	}
	switch message := message.(type) {
	case ClickMsg:
		command := m.form.Click(chrome.Point{X: message.X, Y: message.Y})
		if command == nil {
			return m, nil
		}
		return m.Update(command())
	case ScrollMsg:
		if message.Delta < 0 {
			m.form.MoveFocus(-1)
		} else if message.Delta > 0 {
			m.form.MoveFocus(1)
		}
		return m, nil
	case chrome.FormActivateMsg:
		if message.ID == fieldDirectory {
			m.openPicker()
		}
		return m, nil
	case chrome.FormSubmitMsg:
		m.submit(message.ID)
		return m, nil
	default:
		form, command := m.form.Update(message)
		m.form = form.(*chrome.Form)
		m.sync()
		return m, wrap(command)
	}
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	if m.pickerOpen {
		return tea.NewView(m.picker.View())
	}
	return m.form.View()
}

// Reset replaces the editable value and declarations.
func (m *Model) Reset(value Value) {
	value.CommentPrefix = NormalizeCommentPrefix(value.CommentPrefix)
	value.KeyProfile = NormalizeKeyProfile(value.KeyProfile)
	m.value = value
	m.action = ActionNone
	m.completed = false
	m.pickerOpen = false
	m.form = chrome.NewForm(preferenceDeclaration(value), m.styles.Form)
	m.form.SetBounds(chrome.Rect{Width: m.width, Height: m.height})
	m.resetPicker(value.SaveDirectory)
}

// Value returns the current form value.
func (m *Model) Value() Value {
	return m.value
}

// Completed reports the submitted form action.
func (m *Model) Completed() (Action, bool) {
	return m.action, m.completed
}

// DirectoryOpen reports whether the bounded Huh picker replaces the form.
func (m *Model) DirectoryOpen() bool {
	return m.pickerOpen
}

// SetHeight replaces the available form height; zero hugs content.
func (m *Model) SetHeight(height int) {
	m.height = max(height, 0)
	m.form.SetBounds(chrome.Rect{Width: m.width, Height: m.height})
	m.picker.WithHeight(m.height)
}

// SetWidth replaces the available form width.
func (m *Model) SetWidth(width int) {
	m.width = max(width, 0)
	m.form.SetBounds(chrome.Rect{Width: m.width, Height: m.height})
	m.picker.WithWidth(m.width)
}

// SetStyles replaces form and picker styles.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
	m.form.SetStyles(styles.Form)
	m.picker.WithTheme(styles.Picker)
}

// FieldFlash reports one numeric field's active direction.
func (m *Model) FieldFlash(index int) int {
	if index < 0 || index >= len(numericFieldIDs) {
		return 0
	}
	return m.form.Flash(numericFieldIDs[index])
}

// FocusID returns the semantic form focus.
func (m *Model) FocusID() chrome.ID {
	if m.pickerOpen {
		return fieldDirectory
	}
	return m.form.FocusID()
}

// Focus moves focus to one semantic form control.
func (m *Model) Focus(id chrome.ID) bool {
	if m.pickerOpen {
		m.closePicker()
	}
	return m.form.Focus(id)
}

func (m *Model) updatePicker(message tea.Msg) tea.Cmd {
	if key, ok := message.(tea.KeyPressMsg); ok &&
		(key.Code == tea.KeyEscape || key.Code == 'q' && key.Mod == 0) {
		m.closePicker()
		return nil
	}
	picker, command := m.picker.Update(message)
	m.picker = picker.(*huh.FilePicker)
	m.value.SaveDirectory = m.pickerValue
	if !m.picker.Zoom() {
		m.closePicker()
	}
	return wrap(command)
}

func (m *Model) openPicker() {
	m.pickerValue = m.value.SaveDirectory
	m.picker.Picking(true)
	m.pickerOpen = true
}

func (m *Model) closePicker() {
	m.picker.Picking(false)
	m.pickerOpen = false
	m.value.SaveDirectory = m.pickerValue
	m.form.SetDirectory(fieldDirectory, m.pickerValue)
}

func (m *Model) resetPicker(directory string) {
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		directory, _ = os.UserHomeDir()
	}
	m.pickerValue = m.value.SaveDirectory
	m.picker = huh.NewFilePicker().
		Title("Default save directory").
		DirAllowed(true).
		FileAllowed(false).
		ShowHidden(false).
		CurrentDirectory(directory).
		Picking(false).
		Value(&m.pickerValue)
	m.picker.WithTheme(m.styles.Picker)
	m.picker.WithWidth(m.width)
	m.picker.WithHeight(m.height)
}

func (m *Model) sync() {
	router := m.value.Router
	router.Costs.Step = uint32(m.mustNumber(fieldStep))
	router.Costs.SharedStep = uint32(m.mustNumber(fieldSharedStep))
	router.Costs.Bend = uint32(m.mustNumber(fieldBend))
	router.Costs.Crossing = uint32(m.mustNumber(fieldCrossing))
	router.Costs.EndpointStep = uint32(m.mustNumber(fieldEndpoint))
	router.ReroutePasses = uint8(m.mustNumber(fieldReroutePasses))
	m.value.Router = router
	m.value.CommentPrefix = NormalizeCommentPrefix(m.mustSelected(fieldComment))
	m.value.KeyProfile = profileFromValue(m.mustSelected(fieldKeyProfile))
	m.value.SaveDirectory, _ = m.form.Directory(fieldDirectory)
}

func (m *Model) mustNumber(id chrome.ID) uint64 {
	value, _ := m.form.Number(id)
	return value
}

func (m *Model) mustSelected(id chrome.ID) string {
	value, _ := m.form.Selected(id)
	return value
}

func (m *Model) submit(id chrome.ID) {
	switch id {
	case actionSave:
		m.action = ActionSave
	case actionSaveDefaults:
		m.action = ActionSaveDefaults
	case actionCancel:
		m.action = ActionCancel
	default:
		return
	}
	m.completed = true
}

func preferenceDeclaration(value Value) chrome.FormDeclaration {
	return chrome.FormDeclaration{
		Fields: []chrome.FormField{
			numberField(fieldStep, "Step cost", uint64(value.Router.Costs.Step), math.MaxUint32),
			numberField(fieldSharedStep, "Shared-step cost", uint64(value.Router.Costs.SharedStep), math.MaxUint32),
			numberField(fieldBend, "Bend cost", uint64(value.Router.Costs.Bend), math.MaxUint32),
			numberField(fieldCrossing, "Crossing cost", uint64(value.Router.Costs.Crossing), math.MaxUint32),
			numberField(fieldEndpoint, "Endpoint cost", uint64(value.Router.Costs.EndpointStep), math.MaxUint32),
			numberField(fieldReroutePasses, "Reroute passes", uint64(value.Router.ReroutePasses), math.MaxUint8),
			{
				ID: fieldComment, Label: "Preferred comments", Kind: chrome.SelectField,
				Options: []chrome.FormOption{
					{Label: "//", Value: commentSlash},
					{Label: "#", Value: commentHash},
					{Label: commentBlock, Value: commentBlock},
				},
				Selected: optionIndex(
					[]string{commentSlash, commentHash, commentBlock},
					NormalizeCommentPrefix(value.CommentPrefix),
				),
			},
			{
				ID: fieldDirectory, Label: "Default save directory",
				Kind: chrome.DirectoryField, Directory: value.SaveDirectory,
			},
			{
				ID: fieldKeyProfile, Label: "Key profile", Kind: chrome.SelectField,
				Options: []chrome.FormOption{
					{Label: "Auto", Value: "auto"},
					{Label: "Mac", Value: "mac"},
					{Label: "Standard", Value: "standard"},
				},
				Selected: int(NormalizeKeyProfile(value.KeyProfile)),
			},
		},
		Spacer: chrome.FormSpacer{ID: preferenceSpacer, Grow: 1},
		Actions: chrome.ActionBar{
			ID: preferenceActions,
			Actions: []chrome.FormAction{
				{ID: actionSave, Label: "Save"},
				{ID: actionSaveDefaults, Label: "Save as Defaults"},
				{ID: actionCancel, Label: "Cancel"},
			},
		},
	}
}

func numberField(id chrome.ID, label string, value, maximum uint64) chrome.FormField {
	return chrome.FormField{
		ID: id, Label: label, Kind: chrome.NumberField,
		Number: value, Maximum: maximum,
	}
}

func optionIndex(options []string, value string) int {
	for i, option := range options {
		if option == value {
			return i
		}
	}
	return 0
}

func profileFromValue(value string) chrome.KeyProfile {
	switch value {
	case "mac":
		return chrome.ProfileMac
	case "standard":
		return chrome.ProfileStandard
	default:
		return chrome.ProfileAuto
	}
}

// NormalizeKeyProfile returns a supported key profile.
func NormalizeKeyProfile(profile chrome.KeyProfile) chrome.KeyProfile {
	switch profile {
	case chrome.ProfileMac, chrome.ProfileStandard:
		return profile
	default:
		return chrome.ProfileAuto
	}
}

// NormalizeCommentPrefix returns a supported comment preference.
func NormalizeCommentPrefix(prefix string) string {
	switch prefix {
	case commentHash, commentBlock:
		return prefix
	default:
		return commentSlash
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
