package chrome

import (
	"bytes"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

const testFormSave ID = "save"

func TestFormEditsDeclaredFieldsWithinBounds(t *testing.T) {
	t.Parallel()

	form := newTestForm()
	form.SetBounds(Rect{Width: 40, Height: 8})

	_, command := form.Update(formKey(tea.KeyRight, ""))
	require.NotNil(t, command)
	value, ok := form.Number("number")
	require.True(t, ok)
	require.Equal(t, uint64(2), value)

	_, _ = form.Update(formKey(tea.KeyRight, ""))
	value, _ = form.Number("number")
	require.Equal(t, uint64(2), value)

	require.True(t, form.Focus("select"))
	_, _ = form.Update(formKey(tea.KeyLeft, ""))
	selected, ok := form.Selected("select")
	require.True(t, ok)
	require.Equal(t, viewportTwo, selected)
}

func TestFormNumberFlashDoesNotExposeANSIMarkup(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		key   rune
		value string
	}{
		{name: "decrement", key: tea.KeyLeft, value: "0"},
		{name: "increment", key: tea.KeyRight, value: "2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			styles := testFormStyles()
			styles.Number.FocusedValue = styles.Number.FocusedValue.Underline(true)
			styles.Number.ActiveDecrement = lipgloss.NewStyle().Reverse(true)
			styles.Number.ActiveIncrement = lipgloss.NewStyle().Reverse(true)
			form := NewForm(testFormDeclaration(), styles)
			form.SetBounds(Rect{Width: 40, Height: 8})

			_, _ = form.Update(formKey(test.key, ""))

			line := strings.Split(ansi.Strip(form.View().Content), "\n")[0]
			require.Equal(t, "Number"+strings.Repeat(" ", 29)+"⇽ "+test.value+" ⇾", line)
		})
	}
}

func TestFormRendersIndependentNumberControlStates(t *testing.T) {
	t.Parallel()

	styles := testFormStyles()
	styles.Number.FocusedDecrement = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#111111"))
	styles.Number.ActiveDecrement = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#222222"))
	styles.Number.FocusedIncrement = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#333333"))
	styles.Number.ActiveIncrement = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444444"))

	form := NewForm(testFormDeclaration(), styles)
	form.SetBounds(Rect{Width: 40, Height: 8})
	require.Contains(t, form.View().Content, styles.Number.FocusedDecrement.Render("⇽"))
	require.Contains(t, form.View().Content, styles.Number.FocusedIncrement.Render("⇾"))

	_, _ = form.Update(formKey(tea.KeyLeft, ""))
	require.Contains(t, form.View().Content, styles.Number.ActiveDecrement.Render("⇽"))
	require.Contains(t, form.View().Content, styles.Number.FocusedIncrement.Render("⇾"))

	form = NewForm(testFormDeclaration(), styles)
	form.SetBounds(Rect{Width: 40, Height: 8})
	_, _ = form.Update(formKey(tea.KeyRight, ""))
	require.Contains(t, form.View().Content, styles.Number.FocusedDecrement.Render("⇽"))
	require.Contains(t, form.View().Content, styles.Number.ActiveIncrement.Render("⇾"))
}

func TestFormRendersPointerHoverStates(t *testing.T) {
	t.Parallel()

	styles := testFormStyles()
	styles.HoveredLabel = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#111111"))
	styles.HoveredValue = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#222222"))
	styles.Buttons.HoveredButton = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#333333")).
		Padding(0, 1)
	form := NewForm(testFormDeclaration(), styles)
	form.SetBounds(Rect{Width: 40, Height: 8})

	field := form.Plan().Fields[1].Rect
	_, _ = form.Update(tea.MouseMotionMsg{X: field.X, Y: field.Y})
	require.Contains(t, form.View().Content, styles.HoveredLabel.Render("Select"))
	require.Contains(t, form.View().Content, styles.HoveredValue.Render("  One  "))

	button := form.Plan().Buttons[0].Rect
	_, _ = form.Update(tea.MouseMotionMsg{X: button.X, Y: button.Y})
	require.Contains(t, form.View().Content, styles.Buttons.HoveredButton.Render("Save"))
}

func TestFormFlashExpiresByGeneration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		form := newTestForm()
		_, command := form.Update(formKey(tea.KeyRight, ""))
		require.NotNil(t, command)
		require.Equal(t, 1, form.Flash("number"))

		message := command()
		_, _ = form.Update(message)

		require.Zero(t, form.Flash("number"))
	})
}

func TestFormPlanRetainsSemanticGeometry(t *testing.T) {
	t.Parallel()

	form := newTestForm()
	form.SetBounds(Rect{X: 3, Y: 2, Width: 40, Height: 9})
	plan := form.Plan()

	require.Equal(t, ID("spacer"), plan.SpacerID)
	require.Equal(t, ID("actions"), plan.ButtonListID)
	require.Len(t, plan.Fields, 3)
	require.Len(t, plan.Buttons, 2)
	require.Equal(t, ID("directory"), plan.Fields[2].ID)
	require.Equal(t, plan.Bounds.X, plan.Buttons[0].Rect.X)
	require.Equal(t, plan.Bounds.Bottom(), plan.Buttons[0].Rect.Bottom())
	require.Positive(t, plan.Spacer.Height)

	plan.Fields[0].ID = "mutated"
	require.Equal(t, ID("number"), form.Plan().Fields[0].ID)
}

func TestFormRevealsFocusedControlInConstrainedBounds(t *testing.T) {
	t.Parallel()

	form := newTestForm()
	form.SetBounds(Rect{Width: 40, Height: 2})
	require.True(t, form.Focus("cancel"))

	plan := form.Plan()
	require.Positive(t, plan.Offset)
	require.NotZero(t, plan.Buttons[1].Rect.Height)
	require.Contains(t, form.View().Content, "Cancel")
}

func TestFormTraversalIncludesEveryButtonAndWraps(t *testing.T) {
	t.Parallel()

	form := newTestForm()
	require.True(t, form.Focus("directory"))

	_, _ = form.Update(formKey(tea.KeyTab, ""))
	require.Equal(t, testFormSave, form.FocusID())
	_, _ = form.Update(formKey(tea.KeyTab, ""))
	require.Equal(t, ID("cancel"), form.FocusID())
	_, _ = form.Update(formKey(tea.KeyTab, ""))
	require.Equal(t, ID("number"), form.FocusID())
	_, _ = form.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	require.Equal(t, ID("cancel"), form.FocusID())
}

func TestFormPointerEmitsSemanticMessages(t *testing.T) {
	t.Parallel()

	form := newTestForm()
	form.SetBounds(Rect{Width: 40, Height: 8})
	plan := form.Plan()

	directory := plan.Fields[2].Rect
	message := form.Click(Point{X: directory.X, Y: directory.Y})()
	require.Equal(t, FormActivateMsg{ID: "directory"}, message)

	save := plan.Buttons[0].Rect
	message = form.Click(Point{X: save.X, Y: save.Y})()
	require.Equal(t, FormSubmitMsg{ID: testFormSave}, message)
}

func TestFormAccessibleExecutionIncludesEveryControl(t *testing.T) {
	t.Parallel()

	form := newTestForm()
	var output bytes.Buffer

	require.NoError(t, form.RunAccessible(&output))
	require.Equal(t, []string{
		"Number:   1  ",
		"Select:   One  ",
		"Directory: [ browse ]",
		"action: Save",
		"action: Cancel",
	}, form.AccessibleLines())
	require.Equal(
		t,
		"Number:   1  \nSelect:   One  \nDirectory: [ browse ]\n"+
			"action: Save\naction: Cancel\n",
		output.String(),
	)
}

func TestFormClonesApplicationDeclarations(t *testing.T) {
	t.Parallel()

	declaration := testFormDeclaration()
	form := NewForm(declaration, testFormStyles())
	declaration.Fields[1].Options[0].Label = "Changed"
	declaration.Actions.Buttons[0].Label = "Changed"
	form.SetBounds(Rect{Width: 40})

	require.Contains(t, form.View().Content, "One")
	require.Contains(t, form.View().Content, "Save")
	require.NotContains(t, form.View().Content, "Changed")
}

func TestFormTextFieldTypesPastesClicksAndStaysAccessible(t *testing.T) {
	t.Parallel()

	declaration := FormDeclaration{
		Fields: []FormField{{
			ID: "name", Label: "File name", Kind: TextField,
			Text: "diagram", Placeholder: "diagram.json",
		}},
		Actions: ButtonListDeclaration{
			ID:      "actions",
			Buttons: []Button{{ID: testFormSave, Label: "Save"}},
		},
	}
	form := NewForm(declaration, testFormStyles())
	form.SetBounds(Rect{Width: 30, Height: 2})
	_, _ = form.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
	_, _ = form.Update(formKey('x', "x"))
	_, _ = form.Update(tea.PasteMsg{Content: ".json\n"})
	value, ok := form.Text("name")
	require.True(t, ok)
	require.Equal(t, "diagramx.json", value)

	require.Nil(t, form.Click(Point{X: 15, Y: 0}))
	require.Equal(t, ID("name"), form.FocusID())
	require.Contains(t, form.AccessibleLines(), "File name: diagramx.json")
	require.Equal(t, 30, ansi.StringWidth(strings.Split(form.View().Content, "\n")[0]))
}

func TestFormTextFieldTypesNavigationAliasesAndUsesArrowsForCaret(t *testing.T) {
	t.Parallel()

	form := NewForm(FormDeclaration{
		Fields: []FormField{{ID: "name", Label: "Name", Kind: TextField}},
	}, testFormStyles())
	form.SetBounds(Rect{Width: 20, Height: 1})

	_, _ = form.Update(formKey('h', "h"))
	_, _ = form.Update(formKey('l', "l"))
	_, _ = form.Update(formKey(tea.KeyLeft, ""))
	_, _ = form.Update(formKey('x', "x"))

	value, ok := form.Text("name")
	require.True(t, ok)
	require.Equal(t, "hxl", value)
}

func newTestForm() *Form {
	return NewForm(testFormDeclaration(), testFormStyles())
}

func testFormDeclaration() FormDeclaration {
	return FormDeclaration{
		Fields: []FormField{
			{ID: "number", Label: "Number", Kind: NumberField, Number: 1, Maximum: 2},
			{
				ID: "select", Label: "Select", Kind: SelectField,
				Options: []FormOption{
					{Label: "One", Value: viewportOne},
					{Label: "Two", Value: viewportTwo},
				},
			},
			{ID: "directory", Label: "Directory", Kind: DirectoryField},
		},
		Spacer: FormSpacer{ID: "spacer", Grow: 1},
		Actions: ButtonListDeclaration{
			ID: "actions",
			Buttons: []Button{
				{ID: testFormSave, Label: "Save"},
				{ID: "cancel", Label: "Cancel"},
			},
		},
	}
}

func testFormStyles() FormStyles {
	return FormStyles{
		Label:        lipgloss.NewStyle(),
		HoveredLabel: lipgloss.NewStyle(),
		FocusedLabel: lipgloss.NewStyle().Bold(true),
		Value:        lipgloss.NewStyle(),
		HoveredValue: lipgloss.NewStyle(),
		FocusedValue: lipgloss.NewStyle().Bold(true),
		Number: NumberFieldStyles{
			Value:            lipgloss.NewStyle(),
			HoveredValue:     lipgloss.NewStyle(),
			FocusedValue:     lipgloss.NewStyle().Bold(true),
			FocusedDecrement: lipgloss.NewStyle().Bold(true),
			ActiveDecrement:  lipgloss.NewStyle().Reverse(true),
			FocusedIncrement: lipgloss.NewStyle().Bold(true),
			ActiveIncrement:  lipgloss.NewStyle().Reverse(true),
		},
		Buttons: ButtonListStyles{
			Button:        lipgloss.NewStyle().Padding(0, 1),
			HoveredButton: lipgloss.NewStyle().Padding(0, 1),
			FocusedButton: lipgloss.NewStyle().Bold(true).Padding(0, 1),
		},
		TextInput: testTextInputStyles(),
	}
}

func formKey(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}
