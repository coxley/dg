package modal

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func TestModelSwitchesTabsThroughCommands(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(
		80, 24, 6, 40, "body", Standard,
		[]Tab{{ID: 1, Label: "One"}, {ID: 2, Label: "Two"}},
		1,
	)
	overlay := model.Overlay()
	next, command := model.Update(tea.MouseClickMsg{
		X:      overlay.ContentLeft + lipgloss.Width(model.styles.ActiveTab.Render("One")),
		Y:      overlay.ContentTop,
		Button: tea.MouseLeft,
	})
	require.NotNil(t, command)
	next, command = next.Update(command())
	require.Nil(t, command)
	require.Equal(t, TabID(2), next.ActiveTab())
}

func TestModelKeepsModalBelowAvoidedRows(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(80, 12, 6, 40, "body", Standard, nil, 0)
	require.GreaterOrEqual(t, model.Overlay().Top, 6)
}

func TestModelUsesFullScreenWhenLongContentCannotFit(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(
		40, 8, 6, 30,
		"one\ntwo\nthree\nfour",
		Standard,
		nil,
		0,
	)
	overlay := model.Overlay()
	require.Equal(t, 0, overlay.Left)
	require.Equal(t, 0, overlay.Top)
	require.Equal(t, 40, overlay.Width)
	require.Equal(t, 8, overlay.Height)
}

func testStyles() Styles {
	return Styles{
		Container: lipgloss.NewStyle().Border(lipgloss.NormalBorder()),
		Notice:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
		Body:      lipgloss.NewStyle().PaddingTop(1),
		Tab:       lipgloss.NewStyle().Padding(0, 1),
		ActiveTab: lipgloss.NewStyle().Bold(true).Padding(0, 1),
	}
}
