package chrome

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

const (
	testButtonFirst  ID = "button-first"
	testButtonSecond ID = "button-second"
	testButtonThird  ID = "button-third"
)

func TestButtonListWrapsAndActivatesFocusedButton(t *testing.T) {
	t.Parallel()

	list := newTestButtonList()
	list.SetBounds(Rect{X: 2, Y: 3, Width: 24})

	_, _ = list.Update(buttonKey(tea.KeyLeft, ""))
	require.Equal(t, testButtonFirst, list.FocusID())
	_, _ = list.Update(buttonKey('l', "l"))
	require.Equal(t, testButtonSecond, list.FocusID())
	_, _ = list.Update(buttonKey(tea.KeyTab, ""))
	require.Equal(t, testButtonThird, list.FocusID())
	_, _ = list.Update(buttonKey(tea.KeyTab, ""))
	require.Equal(t, testButtonFirst, list.FocusID())
	_, _ = list.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	require.Equal(t, testButtonThird, list.FocusID())

	_, command := list.Update(buttonKey(tea.KeyEnter, ""))
	require.NotNil(t, command)
	require.Equal(t, ButtonPressMsg{ID: testButtonThird}, command())
}

func TestButtonListClickFocusesAndActivates(t *testing.T) {
	t.Parallel()

	list := newTestButtonList()
	list.SetBounds(Rect{X: 2, Y: 3, Width: 24})
	target := list.Plan().Buttons[1].Rect

	command := list.Click(Point{X: target.X, Y: target.Y})

	require.NotNil(t, command)
	require.Equal(t, testButtonSecond, list.FocusID())
	require.Equal(t, ButtonPressMsg{ID: testButtonSecond}, command())
}

func TestButtonListPlanRetainsStableButtonIDs(t *testing.T) {
	t.Parallel()

	declaration := testButtonListDeclaration()
	list := NewButtonList(declaration, testButtonListStyles())
	list.SetBounds(Rect{X: 2, Y: 3, Width: 24})
	declaration.Buttons[0].ID = "changed"

	plan := list.Plan()
	require.Equal(t, ID("buttons"), plan.ID)
	require.Equal(t, []ID{testButtonFirst, testButtonSecond, testButtonThird}, []ID{
		plan.Buttons[0].ID,
		plan.Buttons[1].ID,
		plan.Buttons[2].ID,
	})
	require.Equal(t, plan.Bounds.X, plan.Buttons[0].Rect.X)

	plan.Buttons[0].ID = "mutated"
	require.Equal(t, testButtonFirst, list.Plan().Buttons[0].ID)
}

func newTestButtonList() *ButtonList {
	return NewButtonList(testButtonListDeclaration(), testButtonListStyles())
}

func testButtonListDeclaration() ButtonListDeclaration {
	return ButtonListDeclaration{
		ID: "buttons",
		Buttons: []Button{
			{ID: testButtonFirst, Label: "First"},
			{ID: testButtonSecond, Label: "Second"},
			{ID: testButtonThird, Label: "Third"},
		},
	}
}

func testButtonListStyles() ButtonListStyles {
	return ButtonListStyles{
		Button:        lipgloss.NewStyle().Padding(0, 1),
		FocusedButton: lipgloss.NewStyle().Bold(true).Padding(0, 1),
	}
}

func buttonKey(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}
