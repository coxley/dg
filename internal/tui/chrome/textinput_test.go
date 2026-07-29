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

func testTextInputStyles() TextInputStyles {
	return TextInputStyles{
		Text:        lipgloss.NewStyle(),
		FocusedText: lipgloss.NewStyle().Bold(true),
		Placeholder: lipgloss.NewStyle().Faint(true),
		Cursor:      lipgloss.NewStyle().Reverse(true),
	}
}
