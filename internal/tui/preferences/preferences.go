// Package preferences implements the editor preferences form.
package preferences

import (
	"math"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/internal/tui/directorypicker"
	"github.com/coxley/dg/layout"
)

const (
	fieldTheme         chrome.ID = "theme"
	fieldDarkTint      chrome.ID = "dark-tint"
	fieldLightTint     chrome.ID = "light-tint"
	fieldBackground    chrome.ID = "background"
	fieldComment       chrome.ID = "comment"
	fieldDirectory     chrome.ID = "directory"
	fieldStep          chrome.ID = "step"
	fieldSharedStep    chrome.ID = "shared-step"
	fieldBend          chrome.ID = "bend"
	fieldCrossing      chrome.ID = "crossing"
	fieldEndpoint      chrome.ID = "endpoint"
	fieldReroutePasses chrome.ID = "reroute-passes"
	fieldKeybinds      chrome.ID = "keybinds"
	preferenceSpacer   chrome.ID = "preference-spacer"
	preferenceActions  chrome.ID = "preference-actions"
	actionSave         chrome.ID = "save"
	actionCancel       chrome.ID = "cancel"
	commentSlash                 = "// "
	commentHash                  = "# "
	commentBlock                 = "/* */"
	backgroundTerminal           = "terminal"
	backgroundOpaque             = "opaque"
)

const (
	// ThemeAuto follows the terminal color scheme.
	ThemeAuto = "auto"
	// ThemeDark always uses the configured dark theme.
	ThemeDark = "dark"
	// ThemeLight always uses the configured light theme.
	ThemeLight = "light"
)

// Tab identifies one preferences section.
type Tab uint8

const (
	// GeneralTab contains application appearance and storage settings.
	GeneralTab Tab = iota
	// KeybindsTab contains scoped action mappings.
	KeybindsTab
	// LinkRoutingTab contains orthogonal router costs.
	LinkRoutingTab
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
	Router           layout.Router
	SaveDirectory    string
	CommentPrefix    string
	Theme            string
	Keybinds         []Keybind
	OpaqueBackground bool
	DarkTint         string
	LightTint        string
}

// Equal reports whether two preference values contain the same settings.
func Equal(a, b Value) bool {
	return a.Router == b.Router &&
		a.SaveDirectory == b.SaveDirectory &&
		a.CommentPrefix == b.CommentPrefix &&
		a.Theme == b.Theme &&
		a.OpaqueBackground == b.OpaqueBackground &&
		a.DarkTint == b.DarkTint &&
		a.LightTint == b.LightTint &&
		slices.Equal(a.Keybinds, b.Keybinds)
}

// TintOption declares one application-provided semantic tint choice.
type TintOption struct {
	ID    string
	Label string
}

// Option configures a preferences model.
type Option func(*Model)

// WithTints supplies independent dark and light tint choices.
func WithTints(dark, light []TintOption) Option {
	return func(model *Model) {
		model.darkTints = append([]TintOption(nil), dark...)
		model.lightTints = append([]TintOption(nil), light...)
	}
}

// WithKeybindActions supplies configurable action declarations.
func WithKeybindActions(actions []KeybindAction) Option {
	return func(model *Model) {
		model.keybindActions = append([]KeybindAction(nil), actions...)
	}
}

// Action identifies how an edited preference form should close.
type Action uint8

const (
	ActionNone Action = iota
	ActionSave
	ActionCancel
)

// Styles defines preferences appearance.
type Styles struct {
	Form    chrome.FormStyles
	Picker  directorypicker.Styles
	Mapping MappingPillStyles
	Scope   lipgloss.Style
	Action  lipgloss.Style
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

// ScrollMsg moves focus without activating the focused control.
type ScrollMsg struct {
	Delta int
}

// Model owns the preference sections, editable value, and picker adapter.
type Model struct {
	value          Value
	general        *chrome.Form
	routing        *chrome.Form
	keybindButtons *chrome.Form
	keybinds       *keybindModel
	picker         *directorypicker.Model
	pickerOpen     bool
	tab            Tab
	action         Action
	completed      bool
	width          int
	height         int
	styles         Styles
	darkTints      []TintOption
	lightTints     []TintOption
	keybindActions []KeybindAction
	keybindHeight  int
}

// New returns a tabbed preferences model.
func New(value Value, width, height int, styles Styles, options ...Option) *Model {
	m := &Model{width: max(width, 0), height: max(height, 0), styles: styles}
	for _, option := range options {
		option(m)
	}
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
	if key, ok := message.(tea.KeyPressMsg); ok && m.updateTabKey(key) {
		return m, nil
	}
	switch message := message.(type) {
	case ClickMsg:
		return m, m.click(chrome.Point{X: message.X, Y: message.Y})
	case ScrollMsg:
		m.scroll(message.Delta)
		return m, nil
	case tea.MouseMotionMsg:
		if m.tab == KeybindsTab {
			m.keybinds.hover(chrome.Point{X: message.X, Y: message.Y})
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
	}
	if m.tab == KeybindsTab {
		if key, ok := message.(tea.KeyPressMsg); ok && m.keybinds.updateKey(key) {
			m.value.Keybinds = m.keybinds.value()
		}
		return m, nil
	}
	form := m.activeForm()
	updated, command := form.Update(message)
	m.setActiveForm(updated.(*chrome.Form))
	m.sync()
	return m, wrap(command)
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	if m.pickerOpen {
		return m.picker.View()
	}
	if m.tab != KeybindsTab {
		return m.activeForm().View()
	}
	content := m.keybinds.render()
	actions := m.keybindButtons.View().Content
	if actions != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, content, actions)
	}
	return tea.NewView(content)
}

// Reset replaces the editable value and declarations.
func (m *Model) Reset(value Value) {
	value.CommentPrefix = NormalizeCommentPrefix(value.CommentPrefix)
	value.Theme = NormalizeTheme(value.Theme)
	value.Keybinds = append([]Keybind(nil), value.Keybinds...)
	m.value = value
	m.tab = GeneralTab
	m.action = ActionNone
	m.completed = false
	m.pickerOpen = false
	m.general = chrome.NewForm(generalDeclaration(value, m.darkTints, m.lightTints), m.styles.Form)
	m.routing = chrome.NewForm(routingDeclaration(value), m.styles.Form)
	m.keybindButtons = chrome.NewForm(actionDeclaration(false), m.styles.Form)
	m.keybinds = newKeybindModel(m.keybindActions, value.Keybinds, m.styles)
	m.setBounds()
	m.resetPicker(value.SaveDirectory)
}

// Value returns the current form value.
func (m *Model) Value() Value {
	value := m.value
	value.Keybinds = append([]Keybind(nil), value.Keybinds...)
	return value
}

// ActiveTab returns the visible preferences section.
func (m *Model) ActiveTab() Tab {
	return m.tab
}

// SetTab selects one preferences section.
func (m *Model) SetTab(tab Tab) {
	if tab > LinkRoutingTab {
		return
	}
	m.tab = tab
	m.setBounds()
}

// CapturesKey reports whether a mapping is waiting for a replacement chord.
func (m *Model) CapturesKey() bool {
	return m.tab == KeybindsTab && m.keybinds.capturesKey()
}

// PointerOccupied reports whether a pointer cell belongs to an interactive
// mapping pill.
func (m *Model) PointerOccupied(x, y int) bool {
	if m.pickerOpen || m.tab != KeybindsTab || y >= m.keybinds.height {
		return false
	}
	_, ok := m.keybinds.mappingAt(chrome.Point{X: x, Y: y})
	return ok
}

// SubmitSave completes the form with its primary save action.
func (m *Model) SubmitSave() {
	m.submit(actionSave)
}

// Completed reports the submitted form action.
func (m *Model) Completed() (Action, bool) {
	return m.action, m.completed
}

// TakeCompleted returns and clears the submitted form action.
func (m *Model) TakeCompleted() (Action, bool) {
	action, completed := m.action, m.completed
	m.action = ActionNone
	m.completed = false
	return action, completed
}

// DirectoryOpen reports whether the bounded directory picker replaces the form.
func (m *Model) DirectoryOpen() bool {
	return m.pickerOpen
}

// SetHeight replaces the available form height; zero hugs content.
func (m *Model) SetHeight(height int) {
	m.SetBounds(m.width, height)
}

// SetWidth replaces the available form width.
func (m *Model) SetWidth(width int) {
	m.SetBounds(width, m.height)
}

// SetBounds replaces the available preferences dimensions.
func (m *Model) SetBounds(width, height int) {
	width = max(width, 0)
	height = max(height, 0)
	if m.width == width && m.height == height {
		return
	}
	m.width = width
	m.height = height
	m.setBounds()
	m.picker.SetBounds(m.width, m.height)
}

// SetStyles replaces form, mapping, and picker styles.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
	m.general.SetStyles(styles.Form)
	m.routing.SetStyles(styles.Form)
	m.keybindButtons.SetStyles(styles.Form)
	m.keybinds.setStyles(styles)
	m.picker.SetStyles(styles.Picker)
	m.setBounds()
}

// FieldFlash reports one numeric field's active direction.
func (m *Model) FieldFlash(index int) int {
	if index < 0 || index >= len(numericFieldIDs) {
		return 0
	}
	return m.routing.Flash(numericFieldIDs[index])
}

// FocusID returns the semantic focus in the visible section.
func (m *Model) FocusID() chrome.ID {
	if m.pickerOpen {
		return fieldDirectory
	}
	if m.tab == KeybindsTab {
		return fieldKeybinds
	}
	return m.activeForm().FocusID()
}

// Focus moves focus to one semantic form control and reveals its section.
func (m *Model) Focus(id chrome.ID) bool {
	if m.pickerOpen {
		m.closePicker()
	}
	if m.general.Focus(id) {
		m.SetTab(GeneralTab)
		return true
	}
	if m.routing.Focus(id) {
		m.SetTab(LinkRoutingTab)
		return true
	}
	if id == fieldKeybinds {
		m.SetTab(KeybindsTab)
		return true
	}
	return false
}

func (m *Model) activeForm() *chrome.Form {
	if m.tab == LinkRoutingTab {
		return m.routing
	}
	return m.general
}

func (m *Model) setActiveForm(form *chrome.Form) {
	if m.tab == LinkRoutingTab {
		m.routing = form
	} else {
		m.general = form
	}
}

func (m *Model) setBounds() {
	bounds := chrome.Rect{Width: m.width, Height: m.height}
	m.general.SetBounds(bounds)
	m.routing.SetBounds(bounds)
	m.keybindButtons.SetBounds(chrome.Rect{Width: m.width})
	m.keybindHeight = lipgloss.Height(m.keybindButtons.View().Content)
	m.keybinds.setBounds(m.width, max(m.height-m.keybindHeight, 1))
}

func (m *Model) updateTabKey(message tea.KeyPressMsg) bool {
	if message.Code != tea.KeyTab || !message.Mod.Contains(tea.ModCtrl) {
		return false
	}
	delta := 1
	if message.Mod.Contains(tea.ModShift) {
		delta = -1
	}
	m.tab = Tab((int(m.tab) + 3 + delta) % 3)
	m.setBounds()
	return true
}

func (m *Model) click(point chrome.Point) tea.Cmd {
	if m.tab == KeybindsTab {
		if point.Y < m.keybinds.height && m.keybinds.click(point) {
			return nil
		}
		point.Y -= m.keybinds.height
		command := m.keybindButtons.Click(point)
		if command != nil {
			_, followup := m.Update(command())
			return followup
		}
		return nil
	}
	command := m.activeForm().Click(point)
	if command == nil {
		return nil
	}
	_, followup := m.Update(command())
	return followup
}

func (m *Model) scroll(delta int) {
	if delta == 0 {
		return
	}
	if m.tab == KeybindsTab {
		m.keybinds.moveRow(delta)
		return
	}
	m.activeForm().MoveFocus(delta)
}

func (m *Model) updatePicker(message tea.Msg) tea.Cmd {
	if key, ok := message.(tea.KeyPressMsg); ok &&
		(key.Code == tea.KeyEscape || key.Code == 'q' && key.Mod == 0) {
		m.closePicker()
		return nil
	}
	picker, command := m.picker.Update(message)
	m.picker = picker.(*directorypicker.Model)
	m.value.SaveDirectory = m.picker.Value()
	if !m.picker.Opened() {
		m.closePicker()
	}
	return wrap(command)
}

func (m *Model) openPicker() {
	m.picker.SetValue(m.value.SaveDirectory)
	m.picker.SetBounds(m.width, m.height)
	m.picker.Open()
	m.pickerOpen = true
}

func (m *Model) closePicker() {
	m.picker.Close()
	m.pickerOpen = false
	m.value.SaveDirectory = m.picker.Value()
	m.general.SetDirectory(fieldDirectory, m.value.SaveDirectory)
}

func (m *Model) resetPicker(directory string) {
	m.picker = directorypicker.New(directorypicker.Config{Value: directory}, m.styles.Picker)
	m.picker.SetBounds(m.width, m.height)
}

func (m *Model) sync() {
	if m.tab == GeneralTab {
		m.value.Theme = NormalizeTheme(m.mustSelected(m.general, fieldTheme))
		m.value.DarkTint = m.mustSelected(m.general, fieldDarkTint)
		m.value.LightTint = m.mustSelected(m.general, fieldLightTint)
		m.value.OpaqueBackground = m.mustSelected(m.general, fieldBackground) == backgroundOpaque
		m.value.CommentPrefix = NormalizeCommentPrefix(m.mustSelected(m.general, fieldComment))
		m.value.SaveDirectory, _ = m.general.Directory(fieldDirectory)
		return
	}
	router := m.value.Router
	router.Costs.Step = uint32(m.mustNumber(fieldStep))
	router.Costs.SharedStep = uint32(m.mustNumber(fieldSharedStep))
	router.Costs.Bend = uint32(m.mustNumber(fieldBend))
	router.Costs.Crossing = uint32(m.mustNumber(fieldCrossing))
	router.Costs.EndpointStep = uint32(m.mustNumber(fieldEndpoint))
	router.ReroutePasses = uint8(m.mustNumber(fieldReroutePasses))
	m.value.Router = router
}

func (m *Model) mustNumber(id chrome.ID) uint64 {
	value, _ := m.routing.Number(id)
	return value
}

func (*Model) mustSelected(form *chrome.Form, id chrome.ID) string {
	value, _ := form.Selected(id)
	return value
}

func (m *Model) submit(id chrome.ID) {
	switch id {
	case actionSave:
		m.action = ActionSave
	case actionCancel:
		m.action = ActionCancel
	default:
		return
	}
	m.completed = true
}

func generalDeclaration(value Value, darkTints, lightTints []TintOption) chrome.FormDeclaration {
	background := 0
	if value.OpaqueBackground {
		background = 1
	}
	return declaration([]chrome.FormField{
		{
			ID: fieldTheme, Label: "Theme", Kind: chrome.SelectField,
			Options: []chrome.FormOption{
				{Label: "Auto", Value: ThemeAuto},
				{Label: "Dark", Value: ThemeDark},
				{Label: "Light", Value: ThemeLight},
			},
			Selected: optionIndex([]string{ThemeAuto, ThemeDark, ThemeLight}, NormalizeTheme(value.Theme)),
		},
		tintField(fieldDarkTint, "Dark Theme", value.DarkTint, darkTints),
		tintField(fieldLightTint, "Light Theme", value.LightTint, lightTints),
		{
			ID: fieldBackground, Label: "Background", Kind: chrome.SelectField,
			Options: []chrome.FormOption{
				{Label: "Terminal", Value: backgroundTerminal},
				{Label: "Opaque", Value: backgroundOpaque},
			},
			Selected: background,
		},
		{
			ID: fieldComment, Label: "Comment Style", Kind: chrome.SelectField,
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
			ID: fieldDirectory, Label: "Save Directory…",
			Kind: chrome.DirectoryField, Directory: value.SaveDirectory,
		},
	})
}

func routingDeclaration(value Value) chrome.FormDeclaration {
	return declaration([]chrome.FormField{
		numberField(fieldStep, "Step Cost", uint64(value.Router.Costs.Step), math.MaxUint32),
		numberField(fieldSharedStep, "Shared-Step Cost", uint64(value.Router.Costs.SharedStep), math.MaxUint32),
		numberField(fieldBend, "Bend Cost", uint64(value.Router.Costs.Bend), math.MaxUint32),
		numberField(fieldCrossing, "Crossing Cost", uint64(value.Router.Costs.Crossing), math.MaxUint32),
		numberField(fieldEndpoint, "Endpoint Cost", uint64(value.Router.Costs.EndpointStep), math.MaxUint32),
		numberField(fieldReroutePasses, "Reroute Passes", uint64(value.Router.ReroutePasses), math.MaxUint8),
	})
}

func declaration(fields []chrome.FormField) chrome.FormDeclaration {
	result := actionDeclaration(true)
	result.Fields = fields
	result.RightAlignValues = true
	return result
}

func actionDeclaration(spacer bool) chrome.FormDeclaration {
	result := chrome.FormDeclaration{
		DefaultAction: actionSave,
		Actions: chrome.ButtonListDeclaration{
			ID: preferenceActions,
			Buttons: []chrome.Button{
				{ID: actionSave, Label: "Save"},
				{ID: actionCancel, Label: "Cancel"},
			},
		},
	}
	if spacer {
		result.Spacer = chrome.FormSpacer{ID: preferenceSpacer, Grow: 1}
	}
	return result
}

func tintField(id chrome.ID, label, selected string, tints []TintOption) chrome.FormField {
	options := make([]chrome.FormOption, len(tints))
	values := make([]string, len(tints))
	for i, tint := range tints {
		options[i] = chrome.FormOption{Label: tint.Label, Value: tint.ID}
		values[i] = tint.ID
	}
	if len(options) == 0 {
		options = []chrome.FormOption{{Label: selected, Value: selected}}
		values = []string{selected}
	}
	return chrome.FormField{
		ID: id, Label: label, Kind: chrome.SelectField,
		Options: options, Selected: optionIndex(values, selected),
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

// NormalizeTheme returns a supported theme preference.
func NormalizeTheme(theme string) string {
	switch strings.ToLower(theme) {
	case ThemeDark:
		return ThemeDark
	case ThemeLight:
		return ThemeLight
	default:
		return ThemeAuto
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
