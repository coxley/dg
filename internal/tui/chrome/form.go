package chrome

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const formFlashDuration = 120 * time.Millisecond

// FieldKind selects one declarative form control.
type FieldKind uint8

const (
	// NumberField edits an unsigned integer within an inclusive bound.
	NumberField FieldKind = iota
	// SelectField chooses one declared option.
	SelectField
	// DirectoryField requests an application-owned directory picker.
	DirectoryField
	// TextField edits one single-line string.
	TextField
)

// FormOption declares one Select value.
type FormOption struct {
	Label string
	Value string
}

// FormField declares one application-owned field.
type FormField struct {
	ID          ID
	Label       string
	Kind        FieldKind
	Number      uint64
	Maximum     uint64
	NumberText  func(uint64) string
	Options     []FormOption
	Selected    int
	Directory   string
	Text        string
	Placeholder string
	TextWidth   int
}

// FormSpacer declares flexible space before the button list.
type FormSpacer struct {
	ID   ID
	Grow int
}

// FormDeclaration contains application-owned form content.
type FormDeclaration struct {
	Fields        []FormField
	Spacer        FormSpacer
	Actions       ButtonListDeclaration
	DefaultAction ID
}

// FormStyles defines geometry-stable semantic form states.
type FormStyles struct {
	Label          lipgloss.Style
	HoveredLabel   lipgloss.Style
	FocusedLabel   lipgloss.Style
	AttentionLabel lipgloss.Style
	Value          lipgloss.Style
	HoveredValue   lipgloss.Style
	FocusedValue   lipgloss.Style
	AttentionValue lipgloss.Style
	Number         NumberFieldStyles
	Buttons        ButtonListStyles
	TextInput      TextInputStyles
}

// NumberFieldStyles defines number text and shared directional-control states.
// Directional controls only appear while a number or select field has focus.
type NumberFieldStyles struct {
	Value            lipgloss.Style
	HoveredValue     lipgloss.Style
	FocusedValue     lipgloss.Style
	FocusedDecrement lipgloss.Style
	ActiveDecrement  lipgloss.Style
	FocusedIncrement lipgloss.Style
	ActiveIncrement  lipgloss.Style
}

// FormControlPlan records one visible field or button hit target.
type FormControlPlan struct {
	ID     ID
	Kind   FieldKind
	Rect   Rect
	ValueX int
}

// FormPlan is one retained form arrangement.
type FormPlan struct {
	Version      uint64
	Bounds       Rect
	Content      Rect
	Offset       int
	SpacerID     ID
	Spacer       Rect
	ButtonListID ID
	Fields       []FormControlPlan
	Buttons      []FormControlPlan
}

// FormActivateMsg requests application handling for a field.
type FormActivateMsg struct {
	ID ID
}

// FormSubmitMsg reports one selected action.
type FormSubmitMsg struct {
	ID ID
}

// FormFlashExpiredMsg clears transient feedback for one form generation.
type FormFlashExpiredMsg struct {
	form       *Form
	generation uint64
}

// Form retains declarative values, focus, arrangement, and render data.
type Form struct {
	declaration FormDeclaration
	styles      FormStyles
	bounds      Rect
	hugHeight   bool
	version     uint64
	focus       int
	hovered     bool
	hover       int
	offset      int
	flashID     ID
	flash       int
	generation  uint64
	plan        FormPlan
	lines       []string
	inputs      map[ID]*TextInput
	buttons     *ButtonList
}

// NewForm returns a retained declarative form.
func NewForm(declaration FormDeclaration, styles FormStyles) *Form {
	f := &Form{
		declaration: cloneFormDeclaration(declaration),
		styles:      styles,
		inputs:      make(map[ID]*TextInput),
	}
	f.buttons = NewButtonList(f.declaration.Actions, styles.Buttons)
	f.buttons.SetFocused(false)
	f.syncInputs()
	f.normalize()
	f.arrange()
	return f
}

// Init implements tea.Model.
func (*Form) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (f *Form) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		return f, f.updateKey(message)
	case tea.MouseMotionMsg:
		f.hoverAt(Point{X: message.X, Y: message.Y})
	case tea.PasteMsg:
		f.updateTextInput(message)
	case FormFlashExpiredMsg:
		if message.form == f && message.generation == f.generation {
			f.flash = 0
			f.flashID = ""
			f.arrange()
		}
	}
	return f, nil
}

// View implements tea.Model.
func (f *Form) View() tea.View {
	return tea.NewView(strings.Join(f.lines, "\n"))
}

// SetBounds arranges the form and its hit targets immediately.
func (f *Form) SetBounds(bounds Rect) {
	bounds.Width = max(bounds.Width, 0)
	bounds.Height = max(bounds.Height, 0)
	hugHeight := bounds.Height == 0
	if f.bounds == bounds && f.hugHeight == hugHeight {
		return
	}
	f.bounds = bounds
	f.hugHeight = hugHeight
	f.invalidate()
}

// SetStyles replaces semantic visual states.
func (f *Form) SetStyles(styles FormStyles) {
	f.styles = styles
	f.buttons.SetStyles(styles.Buttons)
	for _, input := range f.inputs {
		input.SetStyles(styles.TextInput)
	}
	f.invalidate()
}

// Plan returns an immutable copy of the current arrangement.
func (f *Form) Plan() FormPlan {
	plan := f.plan
	plan.Fields = append([]FormControlPlan(nil), plan.Fields...)
	plan.Buttons = append([]FormControlPlan(nil), plan.Buttons...)
	return plan
}

// MoveFocus traverses every field and button with wrapping.
func (f *Form) MoveFocus(delta int) {
	count := len(f.declaration.Fields) + len(f.declaration.Actions.Buttons)
	if count == 0 || delta == 0 {
		return
	}
	f.focus = wrappedIndex(f.focus, delta, count)
	f.syncButtonFocus()
	f.revealFocus()
	f.invalidate()
}

// Focus moves focus to one declared field or action.
func (f *Form) Focus(id ID) bool {
	for i, field := range f.declaration.Fields {
		if field.ID == id {
			f.focus = i
			f.revealFocus()
			f.invalidate()
			return true
		}
	}
	for i, button := range f.declaration.Actions.Buttons {
		if button.ID == id {
			f.focus = len(f.declaration.Fields) + i
			f.buttons.Focus(id)
			f.revealFocus()
			f.invalidate()
			return true
		}
	}
	return false
}

// Click focuses or activates the control at point.
func (f *Form) Click(point Point) tea.Cmd {
	for index, button := range f.plan.Buttons {
		if !button.Rect.Contains(point) {
			continue
		}
		f.focus = len(f.declaration.Fields) + index
		f.buttons.Focus(button.ID)
		f.invalidate()
		return emitFormMessage(FormSubmitMsg{ID: button.ID})
	}
	for i, field := range f.plan.Fields {
		if !field.Rect.Contains(point) {
			continue
		}
		f.focus = i
		f.invalidate()
		if f.declaration.Fields[i].Kind == DirectoryField {
			return emitFormMessage(FormActivateMsg{ID: field.ID})
		}
		if f.declaration.Fields[i].Kind == TextField {
			f.clickTextInput(i, point.X)
		}
		return nil
	}
	return nil
}

// Number returns one current Number value.
func (f *Form) Number(id ID) (uint64, bool) {
	field, ok := f.field(id, NumberField)
	if !ok {
		return 0, false
	}
	return field.Number, true
}

// SetNumber replaces one Number value, bound, and optional display formatter.
func (f *Form) SetNumber(
	id ID,
	number, maximum uint64,
	numberText func(uint64) string,
) bool {
	for i := range f.declaration.Fields {
		field := &f.declaration.Fields[i]
		if field.ID == id && field.Kind == NumberField {
			field.Number = min(number, maximum)
			field.Maximum = maximum
			field.NumberText = numberText
			f.invalidate()
			return true
		}
	}
	return false
}

// Selected returns one current Select option value.
func (f *Form) Selected(id ID) (string, bool) {
	field, ok := f.field(id, SelectField)
	if !ok || len(field.Options) == 0 {
		return "", false
	}
	return field.Options[field.Selected].Value, true
}

// Directory returns one current Directory value.
func (f *Form) Directory(id ID) (string, bool) {
	field, ok := f.field(id, DirectoryField)
	if !ok {
		return "", false
	}
	return field.Directory, true
}

// Text returns one current Text value.
func (f *Form) Text(id ID) (string, bool) {
	field, ok := f.field(id, TextField)
	if !ok {
		return "", false
	}
	return field.Text, true
}

// SetDirectory replaces one Directory value.
func (f *Form) SetDirectory(id ID, directory string) bool {
	for i := range f.declaration.Fields {
		field := &f.declaration.Fields[i]
		if field.ID == id && field.Kind == DirectoryField {
			field.Directory = directory
			f.invalidate()
			return true
		}
	}
	return false
}

// FocusID returns the focused field or action.
func (f *Form) FocusID() ID {
	if f.focus < len(f.declaration.Fields) {
		return f.declaration.Fields[f.focus].ID
	}
	return f.buttons.FocusID()
}

// Flash reports the active direction for id.
func (f *Form) Flash(id ID) int {
	if f.flashID == id {
		return f.flash
	}
	return 0
}

// Highlight briefly marks one form row without moving focus.
func (f *Form) Highlight(id ID, duration time.Duration) tea.Cmd {
	if _, ok := f.fieldByID(id); !ok {
		return nil
	}
	f.flashID = id
	f.flash = 0
	f.generation++
	generation := f.generation
	f.invalidate()
	return tea.Tick(duration, func(time.Time) tea.Msg {
		return FormFlashExpiredMsg{form: f, generation: generation}
	})
}

// Highlighted reports whether id has transient row feedback.
func (f *Form) Highlighted(id ID) bool {
	return f.flashID == id && f.flash == 0
}

// AccessibleLines describes every field and executable action.
func (f *Form) AccessibleLines() []string {
	lines := make([]string, 0, len(f.declaration.Fields)+len(f.declaration.Actions.Buttons))
	for _, field := range f.declaration.Fields {
		value := f.fieldText(field, false)
		if field.Kind == TextField {
			value = field.Text
		}
		lines = append(lines, field.Label+": "+value)
	}
	for _, button := range f.declaration.Actions.Buttons {
		lines = append(lines, "action: "+button.Label)
	}
	return lines
}

// RunAccessible writes stable labels for screen-reader execution paths.
func (f *Form) RunAccessible(writer io.Writer) error {
	for _, line := range f.AccessibleLines() {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return fmt.Errorf("write accessible form: %w", err)
		}
	}
	return nil
}

func (f *Form) updateKey(message tea.KeyPressMsg) tea.Cmd {
	textEntry := f.focus < len(f.declaration.Fields) &&
		f.declaration.Fields[f.focus].Kind == TextField
	intent := ResolveControlIntent(message, textEntry)
	if f.focus >= len(f.declaration.Fields) {
		switch intent {
		case FocusPrevious:
			f.MoveFocus(-1)
		case FocusNext:
			f.MoveFocus(1)
		case NavigateLeft, NavigateRight:
			f.buttons.applyIntent(intent)
			f.focus = len(f.declaration.Fields) + f.buttonFocus()
			f.invalidate()
		case Activate:
			if id := f.buttons.FocusID(); id != "" {
				return emitFormMessage(FormSubmitMsg{ID: id})
			}
		case NoControlIntent:
		}
		return nil
	}

	field := &f.declaration.Fields[f.focus]
	if intent == Activate && field.Kind != DirectoryField {
		if id, ok := f.defaultAction(); ok {
			return emitFormMessage(FormSubmitMsg{ID: id})
		}
	}
	if field.Kind == TextField {
		return f.updateTextField(message, intent)
	}
	switch intent {
	case FocusPrevious:
		f.MoveFocus(-1)
	case FocusNext:
		f.MoveFocus(1)
	case NavigateLeft:
		return f.changeField(field, -1)
	case NavigateRight:
		if field.Kind == DirectoryField {
			return emitFormMessage(FormActivateMsg{ID: field.ID})
		}
		return f.changeField(field, 1)
	case Activate:
		if field.Kind == DirectoryField {
			return emitFormMessage(FormActivateMsg{ID: field.ID})
		}
		f.MoveFocus(1)
	case NoControlIntent:
	}
	return nil
}

func (f *Form) defaultAction() (ID, bool) {
	for _, button := range f.declaration.Actions.Buttons {
		if button.ID == f.declaration.DefaultAction {
			return button.ID, true
		}
	}
	return "", false
}

func (f *Form) updateTextField(message tea.KeyPressMsg, intent ControlIntent) tea.Cmd {
	switch intent {
	case FocusPrevious:
		f.MoveFocus(-1)
	case FocusNext, Activate:
		f.MoveFocus(1)
	default:
		f.updateTextInputKey(message)
	}
	return nil
}

func (f *Form) updateTextInput(message tea.Msg) {
	if f.focus < 0 || f.focus >= len(f.declaration.Fields) {
		return
	}
	field := &f.declaration.Fields[f.focus]
	input := f.inputs[field.ID]
	if field.Kind != TextField || input == nil {
		return
	}
	input.Update(message)
	field.Text = input.Value()
	f.invalidate()
}

func (f *Form) updateTextInputKey(message tea.KeyPressMsg) {
	if f.focus < 0 || f.focus >= len(f.declaration.Fields) {
		return
	}
	field := &f.declaration.Fields[f.focus]
	input := f.inputs[field.ID]
	if field.Kind != TextField || input == nil {
		return
	}
	input.updateKey(message)
	field.Text = input.Value()
	f.invalidate()
}

func (f *Form) changeField(field *FormField, delta int) tea.Cmd {
	switch field.Kind {
	case NumberField:
		if delta < 0 {
			if field.Number != 0 {
				field.Number--
			}
		} else if field.Number != field.Maximum {
			field.Number++
		}
	case SelectField:
		if len(field.Options) == 0 {
			return nil
		}
		field.Selected = (field.Selected + delta%len(field.Options) + len(field.Options)) %
			len(field.Options)
	case DirectoryField, TextField:
		return nil
	}
	f.flashID = field.ID
	f.flash = delta
	f.generation++
	generation := f.generation
	f.invalidate()
	return tea.Tick(formFlashDuration, func(time.Time) tea.Msg {
		return FormFlashExpiredMsg{form: f, generation: generation}
	})
}

func (f *Form) field(id ID, kind FieldKind) (FormField, bool) {
	for _, field := range f.declaration.Fields {
		if field.ID == id && field.Kind == kind {
			return field, true
		}
	}
	return FormField{}, false
}

func (f *Form) fieldByID(id ID) (FormField, bool) {
	for _, field := range f.declaration.Fields {
		if field.ID == id {
			return field, true
		}
	}
	return FormField{}, false
}

func (f *Form) normalize() {
	f.syncInputs()
	for i := range f.declaration.Fields {
		field := &f.declaration.Fields[i]
		field.Number = min(field.Number, field.Maximum)
		if len(field.Options) == 0 {
			field.Selected = 0
		} else {
			field.Selected = min(max(field.Selected, 0), len(field.Options)-1)
		}
	}
	count := len(f.declaration.Fields) + len(f.declaration.Actions.Buttons)
	f.focus = min(max(f.focus, 0), max(count-1, 0))
	f.syncButtonFocus()
}

func (f *Form) invalidate() {
	f.version++
	f.arrange()
}

func (f *Form) arrange() {
	f.normalize()
	bounds := f.bounds
	f.buttons.SetBounds(Rect{Width: bounds.Width})
	buttonHeight := f.buttons.Plan().Bounds.Height
	intrinsicHeight := len(f.declaration.Fields) + buttonHeight
	if f.hugHeight {
		bounds.Height = intrinsicHeight
	}
	contentHeight := intrinsicHeight
	if f.declaration.Spacer.Grow > 0 {
		contentHeight = max(contentHeight, bounds.Height)
	}
	spacerHeight := max(contentHeight-intrinsicHeight, 0)
	maxOffset := max(contentHeight-bounds.Height, 0)
	f.offset = min(max(f.offset, 0), maxOffset)
	buttonY := len(f.declaration.Fields) + spacerHeight
	valueColumn := f.valueColumn(bounds.Width)

	plan := FormPlan{
		Version:      f.version,
		Bounds:       bounds,
		SpacerID:     f.declaration.Spacer.ID,
		ButtonListID: f.declaration.Actions.ID,
		Content: Rect{
			X:      bounds.X,
			Y:      bounds.Y,
			Width:  bounds.Width,
			Height: contentHeight,
		},
		Offset: f.offset,
	}
	plan.Spacer = intersectRect(Rect{
		X:      bounds.X,
		Y:      bounds.Y + len(f.declaration.Fields) - f.offset,
		Width:  bounds.Width,
		Height: spacerHeight,
	}, bounds)
	lines := make([]string, bounds.Height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", bounds.Width)
	}
	for i, field := range f.declaration.Fields {
		screenY := bounds.Y + i - f.offset
		rect := intersectRect(
			Rect{X: bounds.X, Y: screenY, Width: bounds.Width, Height: 1},
			bounds,
		)
		plan.Fields = append(plan.Fields, FormControlPlan{
			ID: field.ID, Kind: field.Kind, Rect: rect,
			ValueX: bounds.X + valueColumn,
		})
		if rect.Width != 0 && rect.Height != 0 {
			lines[screenY-bounds.Y] = f.renderField(
				field,
				i == f.focus,
				bounds.Width,
				valueColumn,
			)
		}
	}
	buttonScreenY := bounds.Y + buttonY - f.offset
	f.buttons.SetBounds(Rect{
		X: bounds.X, Y: buttonScreenY, Width: bounds.Width, Height: buttonHeight,
	})
	if buttonHeight > 0 {
		for row, line := range strings.Split(f.buttons.View().Content, "\n") {
			y := buttonScreenY + row
			if y >= bounds.Y && y < bounds.Bottom() {
				lines[y-bounds.Y] = padLine(ansi.Truncate(line, bounds.Width, ""), bounds.Width)
			}
		}
	}
	for _, button := range f.buttons.Plan().Buttons {
		plan.Buttons = append(plan.Buttons, FormControlPlan{
			ID: button.ID, Rect: intersectRect(button.Rect, bounds),
		})
	}
	f.plan = plan
	f.lines = lines
}

func (f *Form) revealFocus() {
	var top, height int
	if f.focus < len(f.declaration.Fields) {
		top = f.focus
		height = 1
	} else {
		buttonHeight := max(f.buttons.Plan().Bounds.Height, 1)
		top = f.plan.Content.Height - buttonHeight
		height = buttonHeight
	}
	if top < f.offset {
		f.offset = top
	} else if top+height > f.offset+f.plan.Bounds.Height {
		f.offset = top + height - f.plan.Bounds.Height
	}
}

func (f *Form) syncButtonFocus() {
	index := f.focus - len(f.declaration.Fields)
	f.buttons.SetFocused(index >= 0 && index < len(f.declaration.Actions.Buttons))
	if index >= 0 && index < len(f.declaration.Actions.Buttons) {
		f.buttons.Focus(f.declaration.Actions.Buttons[index].ID)
	}
}

func (f *Form) buttonFocus() int {
	id := f.buttons.FocusID()
	for index, button := range f.declaration.Actions.Buttons {
		if button.ID == id {
			return index
		}
	}
	return 0
}

func (f *Form) renderField(field FormField, focused bool, width, valueColumn int) string {
	labelStyle, valueStyle := f.styles.Label, f.styles.Value
	hovered := f.hovered && f.declaration.Fields[f.hover].ID == field.ID
	switch {
	case f.Highlighted(field.ID):
		labelStyle, valueStyle = f.styles.AttentionLabel, f.styles.AttentionValue
	case focused:
		labelStyle, valueStyle = f.styles.FocusedLabel, f.styles.FocusedValue
	case hovered:
		labelStyle, valueStyle = f.styles.HoveredLabel, f.styles.HoveredValue
	}
	label := labelStyle.Render(field.Label)
	if field.Kind == TextField {
		input := f.inputs[field.ID]
		input.SetWidth(textFieldWidth(field, valueColumn, width))
		input.SetHovered(hovered)
		if focused {
			input.Focus()
		} else {
			input.Blur()
		}
		return renderFormRow(width, valueColumn, label, input.View())
	}
	value := f.renderFieldValue(field, focused, hovered, valueStyle)
	return renderFormRow(width, valueColumn, label, value)
}

func (f *Form) renderFieldValue(
	field FormField,
	focused, hovered bool,
	style lipgloss.Style,
) string {
	if field.Kind != NumberField && field.Kind != SelectField {
		return style.Render(f.fieldText(field, focused))
	}

	var value string
	if field.Kind == NumberField {
		value = numberFieldText(field)
	} else if len(field.Options) != 0 {
		value = field.Options[field.Selected].Label
	}
	if f.Highlighted(field.ID) {
		return style.Render("  " + value + "  ")
	}
	if !focused {
		if field.Kind == SelectField {
			return style.Render("  " + value + "  ")
		}
		numberStyle := f.styles.Number.Value
		if hovered {
			numberStyle = f.styles.Number.HoveredValue
		}
		return numberStyle.Render("  " + value + "  ")
	}
	decrement := f.styles.Number.FocusedDecrement
	increment := f.styles.Number.FocusedIncrement
	if f.flashID == field.ID {
		if f.flash < 0 {
			decrement = f.styles.Number.ActiveDecrement
		} else if f.flash > 0 {
			increment = f.styles.Number.ActiveIncrement
		}
	}
	valueStyle := f.styles.Number.FocusedValue
	if field.Kind == SelectField {
		valueStyle = style
	}
	return decrement.Render("❮") +
		valueStyle.Render(" "+value+" ") +
		increment.Render("❯")
}

func (f *Form) hoverAt(point Point) {
	for index, field := range f.plan.Fields {
		if !field.Rect.Contains(point) {
			continue
		}
		buttonChanged := f.buttons.ClearHover()
		if f.hovered && f.hover == index {
			if buttonChanged {
				f.invalidate()
			}
			return
		}
		f.hovered = true
		f.hover = index
		f.invalidate()
		return
	}
	wasHovered := f.hovered
	f.hovered = false
	buttonChanged := f.buttons.Hover(point)
	if wasHovered || buttonChanged {
		f.invalidate()
	}
}

func (f *Form) fieldText(field FormField, focused bool) string {
	var value string
	switch field.Kind {
	case NumberField:
		value = numberFieldText(field)
	case SelectField:
		if len(field.Options) != 0 {
			value = field.Options[field.Selected].Label
		}
	case DirectoryField:
		value = "browse"
	case TextField:
		if input := f.inputs[field.ID]; input != nil {
			value = input.View()
		}
	}
	if field.Kind == DirectoryField {
		return "[ " + value + " ]"
	}
	if !focused {
		return "  " + value + "  "
	}
	return "❮ " + value + " ❯"
}

func numberFieldText(field FormField) string {
	if field.NumberText != nil {
		return field.NumberText(field.Number)
	}
	return strconv.FormatUint(field.Number, 10)
}

func (f *Form) syncInputs() {
	for i := range f.declaration.Fields {
		field := &f.declaration.Fields[i]
		if field.Kind != TextField {
			continue
		}
		input := f.inputs[field.ID]
		if input == nil {
			input = NewTextInput(field.Text, field.Placeholder, f.styles.TextInput)
			f.inputs[field.ID] = input
		} else if input.Value() != field.Text {
			input.SetValue(field.Text)
		}
	}
}

func (f *Form) clickTextInput(index, x int) {
	field := f.declaration.Fields[index]
	input := f.inputs[field.ID]
	if input == nil {
		return
	}
	rect := f.plan.Fields[index].Rect
	valueX := f.plan.Fields[index].ValueX
	inputWidth := textFieldWidth(field, valueX-rect.X, rect.Width)
	input.SetWidth(inputWidth)
	input.Focus()
	input.Click(x - valueX)
}

func textFieldWidth(field FormField, valueColumn, width int) int {
	width = max(width-valueColumn, 0)
	if field.TextWidth > 0 {
		width = min(width, field.TextWidth)
	}
	return width
}

func cloneFormDeclaration(declaration FormDeclaration) FormDeclaration {
	clone := declaration
	clone.Fields = append([]FormField(nil), declaration.Fields...)
	for i := range clone.Fields {
		clone.Fields[i].Options = append([]FormOption(nil), clone.Fields[i].Options...)
	}
	clone.Actions = cloneButtonListDeclaration(declaration.Actions)
	return clone
}

func (f *Form) valueColumn(width int) int {
	labelWidth := 0
	for _, field := range f.declaration.Fields {
		for _, style := range [...]lipgloss.Style{
			f.styles.Label,
			f.styles.HoveredLabel,
			f.styles.FocusedLabel,
			f.styles.AttentionLabel,
		} {
			labelWidth = max(labelWidth, ansi.StringWidth(style.Render(field.Label)))
		}
	}
	return min(labelWidth+1, width)
}

func renderFormRow(width, valueColumn int, label, value string) string {
	valueColumn = min(max(valueColumn, 0), width)
	label = ansi.Truncate(label, max(valueColumn-1, 0), "")
	gap := max(valueColumn-ansi.StringWidth(label), 0)
	value = ansi.Truncate(value, max(width-valueColumn, 0), "")
	return padLine(label+strings.Repeat(" ", gap)+value, width)
}

func intersectRect(rect, bounds Rect) Rect {
	left := max(rect.X, bounds.X)
	top := max(rect.Y, bounds.Y)
	right := min(rect.Right(), bounds.Right())
	bottom := min(rect.Bottom(), bounds.Bottom())
	return Rect{
		X: left, Y: top,
		Width: max(right-left, 0), Height: max(bottom-top, 0),
	}
}

func emitFormMessage(message tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return message
	}
}
