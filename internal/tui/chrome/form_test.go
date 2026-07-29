package chrome

import (
	"bytes"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	require.Equal(t, "two", selected)
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
	require.Equal(t, ID("actions"), plan.ActionBarID)
	require.Len(t, plan.Fields, 3)
	require.Len(t, plan.Actions, 2)
	require.Equal(t, ID("directory"), plan.Fields[2].ID)
	require.Equal(t, plan.Bounds.X, plan.Actions[0].Rect.X)
	require.Equal(t, plan.Bounds.Bottom(), plan.Actions[0].Rect.Bottom())
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
	require.NotZero(t, plan.Actions[1].Rect.Height)
	require.Contains(t, form.View().Content, "Cancel")
}

func TestFormPointerEmitsSemanticMessages(t *testing.T) {
	t.Parallel()

	form := newTestForm()
	form.SetBounds(Rect{Width: 40, Height: 8})
	plan := form.Plan()

	directory := plan.Fields[2].Rect
	message := form.Click(Point{X: directory.X, Y: directory.Y})()
	require.Equal(t, FormActivateMsg{ID: "directory"}, message)

	save := plan.Actions[0].Rect
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
	declaration.Actions.Actions[0].Label = "Changed"
	form.SetBounds(Rect{Width: 40})

	require.Contains(t, form.View().Content, "One")
	require.Contains(t, form.View().Content, "Save")
	require.NotContains(t, form.View().Content, "Changed")
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
					{Label: "One", Value: "one"},
					{Label: "Two", Value: "two"},
				},
			},
			{ID: "directory", Label: "Directory", Kind: DirectoryField},
		},
		Spacer: FormSpacer{ID: "spacer", Grow: 1},
		Actions: ActionBar{
			ID: "actions",
			Actions: []FormAction{
				{ID: testFormSave, Label: "Save"},
				{ID: "cancel", Label: "Cancel"},
			},
		},
	}
}

func testFormStyles() FormStyles {
	return FormStyles{
		Label:          lipgloss.NewStyle(),
		FocusedLabel:   lipgloss.NewStyle().Bold(true),
		Value:          lipgloss.NewStyle(),
		FocusedValue:   lipgloss.NewStyle().Bold(true),
		ActiveValue:    lipgloss.NewStyle().Bold(true),
		Action:         lipgloss.NewStyle().Padding(0, 1),
		SelectedAction: lipgloss.NewStyle().Bold(true).Padding(0, 1),
	}
}

func formKey(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}
