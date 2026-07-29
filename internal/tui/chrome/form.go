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
)

// FormOption declares one Select value.
type FormOption struct {
	Label string
	Value string
}

// FormField declares one application-owned field.
type FormField struct {
	ID        ID
	Label     string
	Kind      FieldKind
	Number    uint64
	Maximum   uint64
	Options   []FormOption
	Selected  int
	Directory string
}

// FormAction declares one ActionBar button.
type FormAction struct {
	ID    ID
	Label string
}

// FormSpacer declares flexible space before the action bar.
type FormSpacer struct {
	ID   ID
	Grow int
}

// ActionBar declares one horizontally selectable action group.
type ActionBar struct {
	ID      ID
	Actions []FormAction
}

// FormDeclaration contains application-owned form content.
type FormDeclaration struct {
	Fields  []FormField
	Spacer  FormSpacer
	Actions ActionBar
}

// FormStyles defines geometry-stable semantic form states.
type FormStyles struct {
	Label          lipgloss.Style
	FocusedLabel   lipgloss.Style
	Value          lipgloss.Style
	FocusedValue   lipgloss.Style
	ActiveValue    lipgloss.Style
	Action         lipgloss.Style
	SelectedAction lipgloss.Style
}

// FormControlPlan records one visible field or action hit target.
type FormControlPlan struct {
	ID   ID
	Kind FieldKind
	Rect Rect
}

// FormPlan is one retained form arrangement.
type FormPlan struct {
	Version     uint64
	Bounds      Rect
	Content     Rect
	Offset      int
	SpacerID    ID
	Spacer      Rect
	ActionBarID ID
	Fields      []FormControlPlan
	Actions     []FormControlPlan
}

// FormActivateMsg requests application handling for a field.
type FormActivateMsg struct {
	ID ID
}

// FormSubmitMsg reports one selected action.
type FormSubmitMsg struct {
	ID ID
}

// FormFlashExpiredMsg clears directional feedback for one form generation.
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
	action      int
	offset      int
	flashID     ID
	flash       int
	generation  uint64
	plan        FormPlan
	lines       []string
}

// NewForm returns a retained declarative form.
func NewForm(declaration FormDeclaration, styles FormStyles) *Form {
	f := &Form{
		declaration: cloneFormDeclaration(declaration),
		styles:      styles,
	}
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
	f.invalidate()
}

// Plan returns an immutable copy of the current arrangement.
func (f *Form) Plan() FormPlan {
	plan := f.plan
	plan.Fields = append([]FormControlPlan(nil), plan.Fields...)
	plan.Actions = append([]FormControlPlan(nil), plan.Actions...)
	return plan
}

// MoveFocus traverses enabled fields and the action bar.
func (f *Form) MoveFocus(delta int) {
	count := len(f.declaration.Fields) + boolCell(len(f.declaration.Actions.Actions) != 0)
	if count == 0 || delta == 0 {
		return
	}
	f.focus = min(max(f.focus+delta, 0), count-1)
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
	for i, action := range f.declaration.Actions.Actions {
		if action.ID == id {
			f.focus = len(f.declaration.Fields)
			f.action = i
			f.revealFocus()
			f.invalidate()
			return true
		}
	}
	return false
}

// Click focuses or activates the control at point.
func (f *Form) Click(point Point) tea.Cmd {
	for i, action := range f.plan.Actions {
		if action.Rect.Contains(point) {
			f.focus = len(f.declaration.Fields)
			f.action = i
			f.invalidate()
			return emitFormMessage(FormSubmitMsg{ID: action.ID})
		}
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
	if len(f.declaration.Actions.Actions) != 0 {
		return f.declaration.Actions.Actions[f.action].ID
	}
	return ""
}

// Flash reports the active direction for id.
func (f *Form) Flash(id ID) int {
	if f.flashID == id {
		return f.flash
	}
	return 0
}

// AccessibleLines describes every field and executable action.
func (f *Form) AccessibleLines() []string {
	lines := make([]string, 0, len(f.declaration.Fields)+len(f.declaration.Actions.Actions))
	for _, field := range f.declaration.Fields {
		lines = append(lines, field.Label+": "+f.fieldText(field, false))
	}
	for _, action := range f.declaration.Actions.Actions {
		lines = append(lines, "action: "+action.Label)
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
	if f.focus >= len(f.declaration.Fields) {
		switch {
		case message.Code == tea.KeyUp || message.Code == tea.KeyTab && message.Mod == tea.ModShift ||
			message.Text == "k":
			f.MoveFocus(-1)
		case message.Code == tea.KeyLeft || message.Text == "h":
			f.action = max(f.action-1, 0)
			f.invalidate()
		case message.Code == tea.KeyRight || message.Text == "l":
			f.action = min(f.action+1, len(f.declaration.Actions.Actions)-1)
			f.invalidate()
		case message.Code == tea.KeyEnter:
			return emitFormMessage(FormSubmitMsg{
				ID: f.declaration.Actions.Actions[f.action].ID,
			})
		}
		return nil
	}

	field := &f.declaration.Fields[f.focus]
	switch {
	case message.Code == tea.KeyUp || message.Code == tea.KeyTab && message.Mod == tea.ModShift ||
		message.Text == "k":
		f.MoveFocus(-1)
	case message.Code == tea.KeyDown || message.Code == tea.KeyTab && message.Mod == 0 ||
		message.Text == "j":
		f.MoveFocus(1)
	case message.Code == tea.KeyLeft || message.Text == "h":
		return f.changeField(field, -1)
	case message.Code == tea.KeyRight || message.Text == "l":
		if field.Kind == DirectoryField {
			return emitFormMessage(FormActivateMsg{ID: field.ID})
		}
		return f.changeField(field, 1)
	case message.Code == tea.KeyEnter:
		if field.Kind == DirectoryField {
			return emitFormMessage(FormActivateMsg{ID: field.ID})
		}
		f.MoveFocus(1)
	}
	return nil
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
		f.flashID = field.ID
		f.flash = delta
		f.generation++
		generation := f.generation
		f.invalidate()
		return tea.Tick(formFlashDuration, func(time.Time) tea.Msg {
			return FormFlashExpiredMsg{form: f, generation: generation}
		})
	case SelectField:
		if len(field.Options) != 0 {
			field.Selected = (field.Selected + delta%len(field.Options) + len(field.Options)) %
				len(field.Options)
			f.invalidate()
		}
	case DirectoryField:
	}
	return nil
}

func (f *Form) field(id ID, kind FieldKind) (FormField, bool) {
	for _, field := range f.declaration.Fields {
		if field.ID == id && field.Kind == kind {
			return field, true
		}
	}
	return FormField{}, false
}

func (f *Form) normalize() {
	for i := range f.declaration.Fields {
		field := &f.declaration.Fields[i]
		field.Number = min(field.Number, field.Maximum)
		if len(field.Options) == 0 {
			field.Selected = 0
		} else {
			field.Selected = min(max(field.Selected, 0), len(field.Options)-1)
		}
	}
	f.action = min(max(f.action, 0), max(len(f.declaration.Actions.Actions)-1, 0))
}

func (f *Form) invalidate() {
	f.version++
	f.arrange()
}

func (f *Form) arrange() {
	f.normalize()
	actionLines, widths := f.renderActions()
	actionHeight := len(actionLines)
	intrinsicHeight := len(f.declaration.Fields) + actionHeight
	bounds := f.bounds
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
	actionY := len(f.declaration.Fields) + spacerHeight

	plan := FormPlan{
		Version:     f.version,
		Bounds:      bounds,
		SpacerID:    f.declaration.Spacer.ID,
		ActionBarID: f.declaration.Actions.ID,
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
		})
		if rect.Width != 0 && rect.Height != 0 {
			lines[screenY-bounds.Y] = f.renderField(field, i == f.focus, bounds.Width)
		}
	}
	actionScreenY := bounds.Y + actionY - f.offset
	for row, line := range actionLines {
		y := actionScreenY + row
		if y >= bounds.Y && y < bounds.Bottom() {
			lines[y-bounds.Y] = padLine(ansi.Truncate(line, bounds.Width, ""), bounds.Width)
		}
	}
	left := bounds.X
	for i, action := range f.declaration.Actions.Actions {
		rect := intersectRect(
			Rect{X: left, Y: actionScreenY, Width: widths[i], Height: actionHeight},
			bounds,
		)
		plan.Actions = append(plan.Actions, FormControlPlan{
			ID: action.ID, Rect: rect,
		})
		left += widths[i]
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
		top = f.plan.Content.Height - max(len(f.renderActionLines()), 1)
		height = max(len(f.renderActionLines()), 1)
	}
	if top < f.offset {
		f.offset = top
	} else if top+height > f.offset+f.plan.Bounds.Height {
		f.offset = top + height - f.plan.Bounds.Height
	}
}

func (f *Form) renderField(field FormField, focused bool, width int) string {
	labelStyle, valueStyle := f.styles.Label, f.styles.Value
	if focused {
		labelStyle, valueStyle = f.styles.FocusedLabel, f.styles.FocusedValue
	}
	label := labelStyle.Render(field.Label)
	value := valueStyle.Render(f.fieldText(field, focused))
	return renderFormRow(width, label, value)
}

func (f *Form) fieldText(field FormField, focused bool) string {
	var value string
	switch field.Kind {
	case NumberField:
		value = strconv.FormatUint(field.Number, 10)
	case SelectField:
		if len(field.Options) != 0 {
			value = field.Options[field.Selected].Label
		}
	case DirectoryField:
		value = "browse"
	}
	if field.Kind == DirectoryField {
		return "[ " + value + " ]"
	}
	if !focused {
		return "  " + value + "  "
	}
	left, right := "⇽", "⇾"
	if f.flashID == field.ID && f.flash < 0 {
		left = f.styles.ActiveValue.Render(left)
	}
	if f.flashID == field.ID && f.flash > 0 {
		right = f.styles.ActiveValue.Render(right)
	}
	return left + " " + value + " " + right
}

func (f *Form) renderActions() ([]string, []int) {
	if len(f.declaration.Actions.Actions) == 0 {
		return nil, nil
	}
	views := make([]string, len(f.declaration.Actions.Actions))
	widths := make([]int, len(views))
	for i, action := range f.declaration.Actions.Actions {
		style := f.styles.Action
		if f.focus >= len(f.declaration.Fields) && i == f.action {
			style = f.styles.SelectedAction
		}
		views[i] = style.Render(action.Label)
		widths[i] = lipgloss.Width(views[i])
	}
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, views...), "\n"), widths
}

func (f *Form) renderActionLines() []string {
	lines, _ := f.renderActions()
	return lines
}

func cloneFormDeclaration(declaration FormDeclaration) FormDeclaration {
	clone := declaration
	clone.Fields = append([]FormField(nil), declaration.Fields...)
	for i := range clone.Fields {
		clone.Fields[i].Options = append([]FormOption(nil), clone.Fields[i].Options...)
	}
	clone.Actions.Actions = append([]FormAction(nil), declaration.Actions.Actions...)
	return clone
}

func renderFormRow(width int, label, value string) string {
	valueWidth := ansi.StringWidth(value)
	label = ansi.Truncate(label, max(width-valueWidth, 0), "")
	gap := max(width-ansi.StringWidth(label)-valueWidth, 0)
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
