package nav

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func TestModelActivatesAndHighlightsTools(t *testing.T) {
	t.Parallel()

	model := New(Styles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1),
		Active: lipgloss.NewStyle().Bold(true),
		Hover:  lipgloss.NewStyle().Underline(true),
	})
	model.SetWidth(60)

	left := (60-model.Width())/2 + model.geo.contentLeft
	next, command := model.Update(tea.MouseClickMsg{
		X:      left + len(" Cursor "),
		Y:      top + model.geo.contentTop,
		Button: tea.MouseLeft,
	})
	require.NotNil(t, command)
	message := command()
	next, command = next.Update(message)
	require.Nil(t, command)
	require.Equal(t, Rectangle, next.Active())
	require.Contains(t, next.View(), "Rectangle")
	require.Equal(t, next.Height(), len(strings.Split(next.View(), "\n")))
}
