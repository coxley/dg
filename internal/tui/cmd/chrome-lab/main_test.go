package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestLabHandlesCompletedPhaseInteractions(t *testing.T) {
	t.Parallel()

	model := newLabModel("layout")
	updateLab(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})
	require.Contains(t, model.View().Content, "scenario: layout")

	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: '2', Text: "2"}))
	require.Contains(t, model.View().Content, "scenario: pane")

	updateLab(t, model, tea.MouseWheelMsg{
		X:      10,
		Y:      8,
		Button: tea.MouseWheelDown,
	})
	updateLab(t, model, tea.MouseClickMsg{
		X:      10,
		Y:      8,
		Button: tea.MouseLeft,
	})
	updateLab(t, model, tea.MouseMotionMsg{
		X:      14,
		Y:      9,
		Button: tea.MouseLeft,
	})
	require.Contains(t, model.View().Content, "pointer-capture: scenario")
	updateLab(t, model, tea.MouseReleaseMsg{
		X:      14,
		Y:      9,
		Button: tea.MouseLeft,
	})
	require.Contains(t, model.View().Content, "pointer-capture: none")
	require.Contains(t, model.View().Content, "events: click=1")
}

func TestLabReflowsBeforeView(t *testing.T) {
	t.Parallel()

	model := newLabModel("overflow")
	updateLab(t, model, tea.WindowSizeMsg{Width: 100, Height: 30})
	wide := model.contentPane.Plan().Body
	updateLab(t, model, tea.WindowSizeMsg{Width: 60, Height: 12})
	compact := model.contentPane.Plan().Body

	require.NotEqual(t, wide, compact)
	require.Equal(t, 60, compact.Width)
	require.Equal(t, 12, strings.Count(model.View().Content, "\n")+1)
}

func updateLab(t testing.TB, model *labModel, message tea.Msg) {
	t.Helper()
	next, _ := model.Update(message)
	require.Same(t, model, next)
}
