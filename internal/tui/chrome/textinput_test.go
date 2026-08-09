package chrome

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestTextInputEditsTypesAndPastesSingleLineText(t *testing.T) {
	t.Parallel()

	input := NewTextInput("ac", "name", testTextInputStyles())
	input.SetWidth(8)
	input.Focus()
	input.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	input.Update(tea.KeyPressMsg(tea.Key{Code: 'b', Text: "b"}))
	input.Update(tea.PasteMsg{Content: "d\ne"})
	require.Equal(t, "abdec", input.Value())

	input.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	input.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDelete}))
	require.Equal(t, "abd", input.Value())
	require.Equal(t, 8, ansi.StringWidth(input.View()))
}

func TestTextInputClipsWideCellsAndClickMovesCaret(t *testing.T) {
	t.Parallel()

	input := NewTextInput("A界BC", "", testTextInputStyles())
	input.SetWidth(4)
	input.Focus()
	require.Equal(t, 4, ansi.StringWidth(input.View()))

	input.Click(0)
	input.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	require.Equal(t, "A界xBC", input.Value())
}

func TestTextInputPlaceholderAndBlurredValue(t *testing.T) {
	t.Parallel()

	input := NewTextInput("", "diagram.json", testTextInputStyles())
	input.SetWidth(7)
	require.Equal(t, "diagram", ansi.Strip(input.View()))

	input.SetValue("file")
	require.Equal(t, "file   ", ansi.Strip(input.View()))
}

func TestTextInputDeletesWholeGraphemeClusters(t *testing.T) {
	t.Parallel()

	input := NewTextInput("e\u0301x", "", testTextInputStyles())
	input.SetWidth(4)
	input.Focus()
	input.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	input.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	require.Equal(t, "x", input.Value())
}

func TestTextInputUsesSharedEditingShortcuts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []tea.Key
		want string
	}{
		{
			name: "control-u deletes to line start",
			keys: []tea.Key{{Code: 'u', Mod: tea.ModCtrl}},
			want: "",
		},
		{
			name: "control-w deletes previous word",
			keys: []tea.Key{{Code: 'w', Mod: tea.ModCtrl}},
			want: "one two/",
		},
		{
			name: "alt-b moves to previous word",
			keys: []tea.Key{
				{Code: 'b', Mod: tea.ModAlt},
				{Code: 'X', Text: "X"},
			},
			want: "one two/Xthree",
		},
		{
			name: "control-a and alt-f move forward by word",
			keys: []tea.Key{
				{Code: 'a', Mod: tea.ModCtrl},
				{Code: 'f', Mod: tea.ModAlt},
				{Code: 'X', Text: "X"},
			},
			want: "oneX two/three",
		},
		{
			name: "control-k deletes to line end",
			keys: []tea.Key{
				{Code: 'b', Mod: tea.ModAlt},
				{Code: 'k', Mod: tea.ModCtrl},
			},
			want: "one two/",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := NewTextInput("one two/three", "", testTextInputStyles())
			input.Focus()
			for _, key := range test.keys {
				input.Update(tea.KeyPressMsg(key))
			}
			require.Equal(t, test.want, input.Value())
		})
	}
}

func TestTextInputRendersHoverAndSelectionStates(t *testing.T) {
	t.Parallel()

	styles := testTextInputStyles()
	styles.HoveredText = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#111111"))
	styles.SelectedText = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#222222"))
	input := NewTextInput("value", "", styles)
	input.SetWidth(8)
	input.SetHovered(true)
	require.Contains(t, input.View(), styles.HoveredText.Render("value"))

	input.Focus()
	input.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModSuper}))
	require.Contains(t, input.View(), styles.SelectedText.Render("value"))
}

func testTextInputStyles() TextInputStyles {
	return TextInputStyles{
		Text:               lipgloss.NewStyle(),
		HoveredText:        lipgloss.NewStyle(),
		FocusedText:        lipgloss.NewStyle().Bold(true),
		SelectedText:       lipgloss.NewStyle().Reverse(true),
		Placeholder:        lipgloss.NewStyle().Faint(true),
		HoveredPlaceholder: lipgloss.NewStyle().Faint(true),
		Cursor:             lipgloss.NewStyle().Reverse(true),
	}
}
