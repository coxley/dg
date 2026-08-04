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

const (
	testCanvasScope   chrome.ScopeID   = "canvas"
	testGlobalScope   chrome.ScopeID   = "global"
	testCanvasLabel                    = "Canvas"
	testConflictChord chrome.Chord     = "ctrl+x"
	testControlChord                   = "ctrl+p"
	testLeftCommand   chrome.CommandID = "left"
)

func TestGeneralFieldsUseRequestedOrderAndTitleCase(t *testing.T) {
	t.Parallel()

	model := testModel()
	view := ansi.Strip(model.View().Content)
	labels := []string{
		"Theme",
		"Dark Theme",
		"Light Theme",
		"Background",
		"Comment Style",
		"Save Directory…",
	}
	previous := -1
	for _, label := range labels {
		index := strings.Index(view, label)
		require.Greater(t, index, previous, label)
		previous = index
	}
	require.NotContains(t, view, "Shortcut style")
	require.NotContains(t, view, "Step Cost")
}

func TestTabsSwitchInRequestedOrder(t *testing.T) {
	t.Parallel()

	model := testModel()
	require.Equal(t, GeneralTab, model.ActiveTab())

	_, _ = model.Update(ctrlTab(false))
	require.Equal(t, KeybindsTab, model.ActiveTab())
	keybinds := ansi.Strip(model.View().Content)
	require.Contains(t, keybinds, "Global")
	require.Contains(t, keybinds, "Open Preferences")
	require.LessOrEqual(t, lipgloss.Width(model.View().Content), 72)

	_, _ = model.Update(ctrlTab(false))
	require.Equal(t, LinkRoutingTab, model.ActiveTab())
	require.Contains(t, ansi.Strip(model.View().Content), "Step Cost")

	_, _ = model.Update(ctrlTab(false))
	require.Equal(t, GeneralTab, model.ActiveTab())
	_, _ = model.Update(ctrlTab(true))
	require.Equal(t, LinkRoutingTab, model.ActiveTab())
}

func TestGeneralThemeAndBackgroundUpdateValue(t *testing.T) {
	t.Parallel()

	model := testModel()
	require.True(t, model.Focus(fieldTheme))
	_, _ = model.Update(keyPress(tea.KeyRight, ""))
	require.Equal(t, ThemeDark, model.Value().Theme)

	require.True(t, model.Focus(fieldBackground))
	_, _ = model.Update(keyPress(tea.KeyRight, ""))
	require.True(t, model.Value().OpaqueBackground)
}

func TestLinkRoutingUpdatesRouterCost(t *testing.T) {
	t.Parallel()

	model := testModel()
	router := model.Value().Router
	require.True(t, model.Focus(fieldStep))
	require.Equal(t, LinkRoutingTab, model.ActiveTab())

	_, command := model.Update(keyPress(tea.KeyRight, ""))

	require.NotNil(t, command)
	require.Equal(t, router.Costs.Step+1, model.Value().Router.Costs.Step)
	require.Equal(t, 1, model.FieldFlash(0))
}

func TestKeybindNavigationRemapCancelAndDelete(t *testing.T) {
	t.Parallel()

	model := testModel()
	model.SetTab(KeybindsTab)
	require.Equal(t, 0, model.keybinds.row)
	require.Equal(t, 0, model.keybinds.cell)

	_, _ = model.Update(keyPress(tea.KeyTab, ""))
	require.Equal(t, 1, model.keybinds.cell)
	_, _ = model.Update(keyPress(tea.KeyDown, ""))
	require.Equal(t, 1, model.keybinds.row)

	before := model.Value().Keybinds[1].Mappings[1]
	_, _ = model.Update(keyPress(tea.KeyEnter, ""))
	require.True(t, model.CapturesKey())
	require.Contains(t, ansi.Strip(model.View().Content), "...")
	_, _ = model.Update(keyPress(tea.KeyEscape, ""))
	require.False(t, model.CapturesKey())
	require.Equal(t, before, model.Value().Keybinds[1].Mappings[1])

	_, _ = model.Update(keyPress(tea.KeyEnter, ""))
	_, _ = model.Update(keyPress('z', "z"))
	require.Equal(t, chrome.Chord("z"), model.Value().Keybinds[1].Mappings[1])
	_, _ = model.Update(keyPress(tea.KeyBackspace, ""))
	require.Empty(t, model.Value().Keybinds[1].Mappings[1])
}

func TestClickingKeybindStartsRemapping(t *testing.T) {
	t.Parallel()

	model := testModel()
	model.SetTab(KeybindsTab)
	_ = model.View()
	plan := model.keybinds.plans[0]

	point := chrome.Point{
		X: plan.rect.Right() - 2,
		Y: plan.rect.Y + plan.rect.Height/2,
	}
	require.True(t, model.PointerOccupied(point.X, point.Y))
	_, _ = model.Update(ClickMsg{X: point.X, Y: point.Y})

	require.True(t, model.CapturesKey())
	require.Equal(t, plan.row, model.keybinds.row)
	require.Equal(t, plan.cell, model.keybinds.cell)
	require.Contains(t, ansi.Strip(model.View().Content), "...")
}

func TestKeybindMappingPlansMatchRenderedPills(t *testing.T) {
	t.Parallel()

	model := testModel()
	model.SetTab(KeybindsTab)
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	for _, plan := range model.keybinds.plans[:mappingLimit] {
		top := []rune(lines[plan.rect.Y])
		bottom := []rune(lines[plan.rect.Bottom()-1])
		require.Equal(t, '╭', top[plan.rect.X])
		require.Equal(t, '╮', top[plan.rect.Right()-1])
		require.Equal(t, '╰', bottom[plan.rect.X])
		require.Equal(t, '╯', bottom[plan.rect.Right()-1])
	}
}

func TestKeybindConflictsStayWithinScope(t *testing.T) {
	t.Parallel()

	actions := []KeybindAction{
		{Scope: testCanvasScope, ScopeLabel: testCanvasLabel, Command: "a", Label: "Action A"},
		{Scope: testCanvasScope, ScopeLabel: testCanvasLabel, Command: "b", Label: "Action B"},
		{Scope: testGlobalScope, ScopeLabel: "Global", Command: "c", Label: "Action C"},
	}
	values := []Keybind{
		{Scope: testCanvasScope, Command: "a", Mappings: [3]chrome.Chord{testConflictChord}},
		{Scope: testCanvasScope, Command: "b", Mappings: [3]chrome.Chord{testConflictChord}},
		{Scope: testGlobalScope, Command: "c", Mappings: [3]chrome.Chord{testConflictChord}},
	}
	model := New(
		Value{Router: layout.DefaultRouter(), Keybinds: values},
		72,
		18,
		testStyles(),
		WithKeybindActions(actions),
	)
	conflicts := model.keybinds.conflicts()

	require.True(t, conflicts[[2]string{string(testCanvasScope), string(testConflictChord)}])
	require.False(t, conflicts[[2]string{string(testGlobalScope), string(testConflictChord)}])
}

func TestKeybindPillsExposeHoverEmptyAndConflictStyles(t *testing.T) {
	t.Parallel()

	styles := testStyles().Mapping
	pill := NewMappingPill(string(testConflictChord), styles)
	pill.SetState(string(testConflictChord), false, false, false, true)
	require.Equal(t, styles.Hovered, pill.style())

	pill.SetState("", false, false, false, true)
	require.Equal(t, styles.EmptyHovered, pill.style())

	pill.SetState(string(testConflictChord), false, false, true, true)
	require.Equal(t, styles.ConflictHovered, pill.style())
}

func TestMappingPillStatesPreserveRequestedWidth(t *testing.T) {
	t.Parallel()

	styles := testStyles().Mapping
	states := []struct {
		value                              string
		focused, active, conflict, hovered bool
	}{
		{value: testControlChord},
		{value: testControlChord, hovered: true},
		{value: testControlChord, focused: true},
		{value: testControlChord, active: true},
		{},
		{hovered: true},
		{value: testControlChord, conflict: true},
		{value: testControlChord, conflict: true, hovered: true},
		{value: testControlChord, focused: true, conflict: true},
	}
	for _, state := range states {
		pill := NewMappingPill(state.value, styles)
		pill.SetState(
			state.value,
			state.focused,
			state.active,
			state.conflict,
			state.hovered,
		)
		require.Equal(t, 15, lipgloss.Width(pill.View(15)))
	}
}

func TestMappingLabelUsesCmdOnlyOnMacOS(t *testing.T) {
	t.Parallel()

	require.Equal(t, "cmd+shift+p", mappingLabel("super+shift+p", "darwin"))
	require.Equal(t, "super+shift+p", mappingLabel("super+shift+p", "linux"))
}

func TestDirectoryBrowserOpensOnlyOnExplicitActivation(t *testing.T) {
	t.Parallel()

	for _, key := range []tea.Key{
		{Code: tea.KeyRight},
		{Code: 'l', Text: "l"},
		{Code: tea.KeyEnter},
	} {
		model := testModel()
		require.True(t, model.Focus(fieldDirectory))

		dispatch(t, model, tea.KeyPressMsg(key))

		require.True(t, model.DirectoryOpen())
		require.NotContains(t, model.View().Content, "Comment Style")
	}
}

func TestEnterSubmitsSaveFromGeneralAndRouting(t *testing.T) {
	t.Parallel()

	for _, tab := range []Tab{GeneralTab, LinkRoutingTab} {
		model := testModel()
		model.SetTab(tab)
		dispatch(t, model, keyPress(tea.KeyEnter, ""))

		action, completed := model.Completed()
		require.True(t, completed)
		require.Equal(t, ActionSave, action)
	}
}

func TestActionClickSubmitsSelectedAction(t *testing.T) {
	t.Parallel()

	model := testModelWithStyles(borderedStyles())
	action := model.general.Plan().Buttons[1]

	next, command := model.Update(ClickMsg{
		X: action.Rect.X + 1,
		Y: action.Rect.Y + 1,
	})

	require.Same(t, model, next)
	require.Nil(t, command)
	got, completed := model.Completed()
	require.True(t, completed)
	require.Equal(t, ActionCancel, got)
}

func TestActionsStayAtBottomOfGeneralAndRouting(t *testing.T) {
	t.Parallel()

	model := testModelWithStyles(borderedStyles())
	for _, form := range []*chrome.Form{model.general, model.routing} {
		plan := form.Plan()
		require.Equal(t, preferenceSpacer, plan.SpacerID)
		require.Equal(t, preferenceActions, plan.ButtonListID)
		require.Equal(t, plan.Bounds.Bottom(), plan.Buttons[0].Rect.Bottom())
		require.Positive(t, plan.Spacer.Height)
	}
}

func TestModelImplementsTeaModel(t *testing.T) {
	t.Parallel()

	var model tea.Model = testModel()
	require.NotEmpty(t, model.View().Content)
}

func testModel() *Model {
	return testModelWithStyles(testStyles())
}

func testModelWithStyles(styles Styles) *Model {
	actions := []KeybindAction{
		{Scope: testGlobalScope, ScopeLabel: "Global", Command: "preferences", Label: "Open Preferences"},
		{Scope: testCanvasScope, ScopeLabel: testCanvasLabel, Command: testLeftCommand, Label: "Navigate Left"},
	}
	values := []Keybind{
		{Scope: testGlobalScope, Command: "preferences", Mappings: [3]chrome.Chord{"ctrl+p", "super+p"}},
		{Scope: testCanvasScope, Command: testLeftCommand, Mappings: [3]chrome.Chord{chrome.Chord(testLeftCommand)}},
	}
	return New(
		Value{
			Router:        layout.DefaultRouter(),
			Theme:         ThemeAuto,
			DarkTint:      "dark",
			LightTint:     "light",
			CommentPrefix: commentSlash,
			Keybinds:      values,
		},
		72,
		20,
		styles,
		WithTints(
			[]TintOption{{ID: "dark", Label: "Dark"}},
			[]TintOption{{ID: "light", Label: "Light"}},
		),
		WithKeybindActions(actions),
	)
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

func ctrlTab(shift bool) tea.KeyPressMsg {
	mod := tea.ModCtrl
	if shift {
		mod |= tea.ModShift
	}
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: mod})
}

func borderedStyles() Styles {
	styles := testStyles()
	styles.Form.Buttons.Button = styles.Form.Buttons.Button.Border(lipgloss.NormalBorder())
	styles.Form.Buttons.FocusedButton = styles.Form.Buttons.FocusedButton.
		Border(lipgloss.DoubleBorder())
	return styles
}

func testStyles() Styles {
	plain := lipgloss.NewStyle()
	mapping := plain.Border(lipgloss.RoundedBorder())
	return Styles{
		Picker: directorypicker.Styles{},
		Scope:  plain.Bold(true),
		Action: plain,
		Mapping: MappingPillStyles{
			Normal:          mapping,
			Hovered:         mapping.Foreground(lipgloss.Color("1")),
			Focused:         mapping.Foreground(lipgloss.Color("2")),
			Active:          mapping.Foreground(lipgloss.Color("3")),
			Empty:           mapping.Foreground(lipgloss.Color("4")),
			EmptyHovered:    mapping.Foreground(lipgloss.Color("5")),
			Conflict:        mapping.Foreground(lipgloss.Color("6")),
			ConflictHovered: mapping.Foreground(lipgloss.Color("7")),
			ConflictFocused: mapping.Foreground(lipgloss.Color("8")),
		},
		Form: chrome.FormStyles{
			Label: plain, HoveredLabel: plain, FocusedLabel: plain.Bold(true),
			Value: plain, HoveredValue: plain, FocusedValue: plain.Bold(true),
			Number: chrome.NumberFieldStyles{
				Value: plain, HoveredValue: plain, FocusedValue: plain.Bold(true),
				FocusedDecrement: plain.Bold(true), ActiveDecrement: plain.Reverse(true),
				FocusedIncrement: plain.Bold(true), ActiveIncrement: plain.Reverse(true),
			},
			Buttons: chrome.ButtonListStyles{
				Button: plain.Padding(0, 1), HoveredButton: plain.Padding(0, 1),
				FocusedButton: plain.Bold(true).Padding(0, 1),
			},
		},
	}
}
