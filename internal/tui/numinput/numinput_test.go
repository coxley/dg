package numinput

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func TestModelStepsWithinBitLimit(t *testing.T) {
	t.Parallel()

	value := "0"
	model := New("Cost", &value, 8, testStyles())
	next, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	require.Same(t, model, next)
	require.NotNil(t, command)
	require.Equal(t, "0", value)
	require.Equal(t, -1, model.Flash())

	next, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	require.Same(t, model, next)
	require.NotNil(t, command)
	require.Equal(t, "1", value)
	require.Equal(t, 1, model.Flash())
}

func TestModelIgnoresStaleFlash(t *testing.T) {
	t.Parallel()

	value := "1"
	model := New("Cost", &value, 8, testStyles())
	_, first := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	_, second := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))

	require.True(t, model.HandleFlash(first().(FlashExpiredMsg)))
	require.Equal(t, 1, model.Flash())
	require.True(t, model.HandleFlash(second().(FlashExpiredMsg)))
	require.Zero(t, model.Flash())
}

func TestModelImplementsTeaModel(t *testing.T) {
	t.Parallel()

	value := "1"
	var model tea.Model = New("Cost", &value, 8, testStyles())
	require.Contains(t, model.View().Content, "Cost")
}

func testStyles() Styles {
	return Styles{
		Title:        lipgloss.NewStyle(),
		FocusedTitle: lipgloss.NewStyle().Bold(true),
		Button:       lipgloss.NewStyle(),
		ActiveButton: lipgloss.NewStyle().Bold(true),
	}
}
