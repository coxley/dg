package preferences

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/internal/tui/directorypicker"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestModelUpdatesRouterCost(t *testing.T) {
	t.Parallel()

	router := layout.DefaultRouter()
	model := New(Value{Router: router}, 64, 10, testStyles())
	next, command := model.Update(keyPress(tea.KeyRight, ""))

	require.Same(t, model, next)
	require.NotNil(t, command)
	require.Equal(t, router.Costs.Step+1, model.Value().Router.Costs.Step)
	require.Equal(t, 1, model.FieldFlash(0))
}

func TestEnterSubmitsSaveByDefault(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
	dispatch(t, model, keyPress(tea.KeyEnter, ""))

	action, completed := model.Completed()
	require.True(t, completed)
	require.Equal(t, ActionSave, action)
}

func TestKeyboardTraversalVisitsEveryActionAndWraps(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
	require.True(t, model.Focus(fieldKeyProfile))

	_, _ = model.Update(keyPress(tea.KeyTab, ""))
	require.Equal(t, fieldDarkTint, model.FocusID())
	_, _ = model.Update(keyPress(tea.KeyTab, ""))
	require.Equal(t, fieldLightTint, model.FocusID())
	_, _ = model.Update(keyPress(tea.KeyTab, ""))
	require.Equal(t, actionSave, model.FocusID())
	_, _ = model.Update(keyPress(tea.KeyTab, ""))
	require.Equal(t, actionSaveDefaults, model.FocusID())
	_, _ = model.Update(keyPress(tea.KeyTab, ""))
	require.Equal(t, actionCancel, model.FocusID())
	_, _ = model.Update(keyPress(tea.KeyTab, ""))
	require.Equal(t, fieldStep, model.FocusID())
	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	require.Equal(t, actionCancel, model.FocusID())
}

func TestConstrainedFormRevealsActions(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 5, testStyles())
	require.NotContains(t, model.View().Content, "Save as Defaults")

	for range len(preferenceDeclaration(Value{Router: layout.DefaultRouter()}).Fields) + 1 {
		_, _ = model.Update(ScrollMsg{Delta: 1})
	}

	require.Contains(t, model.View().Content, "Save as Defaults")
	require.False(t, model.DirectoryOpen())
}

func TestModelImplementsTeaModel(t *testing.T) {
	t.Parallel()

	var model tea.Model = New(
		Value{Router: layout.DefaultRouter()},
		64,
		10,
		testStyles(),
	)
	require.NotEmpty(t, model.View().Content)
}

func TestFieldsJustifyLabelsAndValuesAcrossWidth(t *testing.T) {
	t.Parallel()

	const width = 48
	model := New(Value{Router: layout.DefaultRouter()}, width, 20, testStyles())
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")

	requireRow := func(label, suffix string) {
		t.Helper()
		for _, line := range lines {
			if !strings.HasPrefix(line, label) {
				continue
			}
			require.True(t, strings.HasSuffix(line, suffix), line)
			require.Equal(t, width, ansi.StringWidth(line), line)
			return
		}
		require.Fail(t, "preference row not rendered", label)
	}
	requireRow("Step cost", "⇽ 10 ⇾")
	requireRow("Shared-step cost", "2  ")
	requireRow("Default save directory", "[ browse ]")
	requireRow("Preferred comments", "//  ")
	requireRow("Shortcut style", "Auto  ")
}

func TestFieldsFollowFormWidthChanges(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 48, 20, testStyles())
	model.SetWidth(64)

	for _, line := range strings.Split(ansi.Strip(model.View().Content), "\n") {
		if strings.HasPrefix(line, "Default save directory") {
			require.Equal(t, 64, ansi.StringWidth(line))
			require.True(t, strings.HasSuffix(line, "[ browse ]"))
			return
		}
	}
	require.Fail(t, "directory row not rendered")
}

func TestDirectoryBrowserOpensOnlyOnExplicitActivation(t *testing.T) {
	t.Parallel()

	for _, key := range []tea.Key{
		{Code: tea.KeyRight},
		{Code: 'l', Text: "l"},
		{Code: tea.KeyEnter},
	} {
		model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
		require.True(t, model.Focus(fieldDirectory))

		dispatch(t, model, tea.KeyPressMsg(key))

		require.True(t, model.DirectoryOpen())
		require.NotContains(t, model.View().Content, "Preferred comments")
	}
}

func TestCollapsedDirectoryUsesFormNavigation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		key  tea.Key
		want chrome.ID
	}{
		{name: "up", key: tea.Key{Code: tea.KeyUp}, want: fieldComment},
		{name: "k", key: tea.Key{Code: 'k', Text: "k"}, want: fieldComment},
		{name: "down", key: tea.Key{Code: tea.KeyDown}, want: fieldKeyProfile},
		{name: "j", key: tea.Key{Code: 'j', Text: "j"}, want: fieldKeyProfile},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
			require.True(t, model.Focus(fieldDirectory))

			_, _ = model.Update(tea.KeyPressMsg(test.key))

			require.False(t, model.DirectoryOpen())
			require.Equal(t, test.want, model.FocusID())
		})
	}
}

func TestQClosesOpenDirectoryAndKeepsFormOpen(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
	require.True(t, model.Focus(fieldDirectory))
	dispatch(t, model, keyPress(tea.KeyEnter, ""))
	require.True(t, model.DirectoryOpen())

	_, _ = model.Update(keyPress('q', "q"))

	require.False(t, model.DirectoryOpen())
	require.Equal(t, fieldDirectory, model.FocusID())
	require.Contains(t, model.View().Content, "Preferred comments")
}

func TestVimKeysNavigateAndEditForm(t *testing.T) {
	t.Parallel()

	router := layout.DefaultRouter()
	model := New(Value{Router: router}, 64, 20, testStyles())
	_, _ = model.Update(keyPress('l', "l"))
	require.Equal(t, router.Costs.Step+1, model.Value().Router.Costs.Step)

	_, _ = model.Update(keyPress('j', "j"))
	_, _ = model.Update(keyPress('h', "h"))
	require.Equal(t, router.Costs.SharedStep-1, model.Value().Router.Costs.SharedStep)

	_, _ = model.Update(keyPress('k', "k"))
	_, _ = model.Update(keyPress('h', "h"))
	require.Equal(t, router.Costs.Step, model.Value().Router.Costs.Step)
}

func TestKeyProfileSelectorUpdatesValue(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
	require.True(t, model.Focus(fieldKeyProfile))

	_, _ = model.Update(keyPress(tea.KeyRight, ""))
	require.Equal(t, chrome.ProfileMac, model.Value().KeyProfile)
	_, _ = model.Update(keyPress(tea.KeyRight, ""))
	require.Equal(t, chrome.ProfileStandard, model.Value().KeyProfile)
}

func TestTintSelectorsUseIndependentChoices(t *testing.T) {
	t.Parallel()

	model := New(
		Value{
			Router:    layout.DefaultRouter(),
			DarkTint:  "dark-a",
			LightTint: "light-a",
		},
		64,
		20,
		testStyles(),
		WithTints(
			[]TintOption{
				{ID: "dark-a", Label: "Dark A"},
				{ID: "dark-b", Label: "Dark B"},
			},
			[]TintOption{
				{ID: "light-a", Label: "Light A"},
				{ID: "light-b", Label: "Light B"},
				{ID: "light-c", Label: "Light C"},
			},
		),
	)

	require.True(t, model.Focus(fieldDarkTint))
	_, _ = model.Update(keyPress(tea.KeyRight, ""))
	require.Equal(t, "dark-b", model.Value().DarkTint)
	require.Equal(t, "light-a", model.Value().LightTint)

	require.True(t, model.Focus(fieldLightTint))
	_, _ = model.Update(keyPress(tea.KeyRight, ""))
	require.Equal(t, "dark-b", model.Value().DarkTint)
	require.Equal(t, "light-b", model.Value().LightTint)
}

func TestInvalidKeyProfileDefaultsToAuto(t *testing.T) {
	t.Parallel()

	model := New(Value{
		Router:     layout.DefaultRouter(),
		KeyProfile: chrome.KeyProfile(255),
	}, 64, 20, testStyles())

	require.Equal(t, chrome.ProfileAuto, model.Value().KeyProfile)
}

func TestActionClickSubmitsSelectedAction(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, borderedStyles())
	action := model.form.Plan().Buttons[1]

	next, command := model.Update(ClickMsg{
		X: action.Rect.X + 1,
		Y: action.Rect.Y + 1,
	})

	require.Same(t, model, next)
	require.Nil(t, command)
	got, completed := model.Completed()
	require.True(t, completed)
	require.Equal(t, ActionSaveDefaults, got)
}

func TestTakeCompletedConsumesSelectedAction(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, borderedStyles())
	model.submit(actionSave)

	action, completed := model.TakeCompleted()

	require.True(t, completed)
	require.Equal(t, ActionSave, action)
	action, completed = model.TakeCompleted()
	require.False(t, completed)
	require.Equal(t, ActionNone, action)
}

func TestActionsUseDeclaredBottomLeftGeometry(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 24, borderedStyles())
	plan := model.form.Plan()

	require.Equal(t, preferenceSpacer, plan.SpacerID)
	require.Equal(t, preferenceActions, plan.ButtonListID)
	require.Equal(t, plan.Bounds.X, plan.Buttons[0].Rect.X)
	require.Equal(t, plan.Bounds.Bottom(), plan.Buttons[0].Rect.Bottom())
	require.Positive(t, plan.Spacer.Height)
}

func TestRepresentativeFieldRequiresOnlyDeclaration(t *testing.T) {
	t.Parallel()

	declaration := preferenceDeclaration(Value{Router: layout.DefaultRouter()})
	declaration.Fields = append(declaration.Fields, chrome.FormField{
		ID:    "representative",
		Label: "Representative field",
		Kind:  chrome.SelectField,
		Options: []chrome.FormOption{
			{Label: "Off", Value: "off"},
			{Label: "On", Value: "on"},
		},
	})
	form := chrome.NewForm(declaration, testStyles().Form)
	form.SetBounds(chrome.Rect{Width: 64})

	require.Contains(t, ansi.Strip(form.View().Content), "Representative field")
	require.Contains(t, form.AccessibleLines(), "Representative field:   Off  ")
	require.True(t, form.Focus("representative"))
}

func dispatch(t *testing.T, model *Model, message tea.Msg) {
	t.Helper()

	_, command := model.Update(message)
	require.NotNil(t, command)
	_, followup := model.Update(command())
	require.Nil(t, followup)
}

func keyPress(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}

func borderedStyles() Styles {
	styles := testStyles()
	styles.Form.Buttons.Button = styles.Form.Buttons.Button.Border(lipgloss.NormalBorder())
	styles.Form.Buttons.FocusedButton = styles.Form.Buttons.FocusedButton.
		Border(lipgloss.DoubleBorder())
	return styles
}

func testStyles() Styles {
	return Styles{
		Picker: directorypicker.Styles{},
		Form: chrome.FormStyles{
			Label:        lipgloss.NewStyle(),
			HoveredLabel: lipgloss.NewStyle(),
			FocusedLabel: lipgloss.NewStyle().Bold(true),
			Value:        lipgloss.NewStyle(),
			HoveredValue: lipgloss.NewStyle(),
			FocusedValue: lipgloss.NewStyle().Bold(true),
			Number: chrome.NumberFieldStyles{
				Value:            lipgloss.NewStyle(),
				HoveredValue:     lipgloss.NewStyle(),
				FocusedValue:     lipgloss.NewStyle().Bold(true),
				FocusedDecrement: lipgloss.NewStyle().Bold(true),
				ActiveDecrement:  lipgloss.NewStyle().Reverse(true),
				FocusedIncrement: lipgloss.NewStyle().Bold(true),
				ActiveIncrement:  lipgloss.NewStyle().Reverse(true),
			},
			Buttons: chrome.ButtonListStyles{
				Button:        lipgloss.NewStyle().Padding(0, 1),
				HoveredButton: lipgloss.NewStyle().Padding(0, 1),
				FocusedButton: lipgloss.NewStyle().Bold(true).Padding(0, 1),
			},
		},
	}
}
