package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
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

func TestLabSurfacesRouteModalCaptureAndDismissal(t *testing.T) {
	t.Parallel()

	model := newLabModel(scenarioSurfaces)
	updateLab(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})
	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: 'm', Text: "m"}))
	require.True(t, model.modalVisible)
	require.Contains(t, model.View().Content, "legacy modal adapter: open")

	updateLab(t, model, tea.MouseClickMsg{
		X:      30,
		Y:      8,
		Button: tea.MouseLeft,
	})
	require.Equal(t, chrome.SurfaceID("modal"), model.workspace.CaptureID())
	updateLab(t, model, tea.MouseReleaseMsg{
		X:      30,
		Y:      8,
		Button: tea.MouseLeft,
	})
	require.Empty(t, model.workspace.CaptureID())

	updateLab(t, model, tea.MouseClickMsg{
		X:      1,
		Y:      1,
		Button: tea.MouseLeft,
	})
	require.False(t, model.modalVisible)
	require.True(t, model.helpVisible)
}

func TestLabFormsExposeLiveContextAndNestedPicker(t *testing.T) {
	t.Parallel()

	model := newLabModel(scenarioForms)
	updateLab(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})

	command := updateLabCommand(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	require.NotNil(t, command)
	require.Contains(t, model.View().Content, "router-step: 11")

	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	require.Contains(t, model.View().Content, "key-profile: mac")

	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	command = updateLabCommand(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.NotNil(t, command)
	updateLab(t, model, command())
	require.True(t, model.formPicker)
	require.Contains(t, model.View().Content, "NESTED DIRECTORY PICKER")

	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	require.False(t, model.formPicker)
	require.Contains(t, model.View().Content, "Save directory")
}

func TestLabDialogsExerciseLifecycleAndDismissalPolicy(t *testing.T) {
	t.Parallel()

	model := newLabModel(scenarioDialogs)
	updateLab(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})
	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	require.Equal(t, labDialogSave, model.activeDialog)
	require.Contains(t, model.View().Content, "dialog.save")

	updateLab(t, model, tea.MouseClickMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	})
	require.Empty(t, model.activeDialog)

	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	updateLab(t, model, tea.MouseClickMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	})
	require.Equal(t, labDialogNotice, model.activeDialog)
	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	require.Empty(t, model.activeDialog)

	updateLab(t, model, tea.KeyPressMsg(tea.Key{Code: 'c', Text: "c"}))
	require.Equal(t, labDialogConfirm, model.activeDialog)
}

func updateLab(t testing.TB, model *labModel, message tea.Msg) {
	t.Helper()
	next, _ := model.Update(message)
	require.Same(t, model, next)
}

func updateLabCommand(t testing.TB, model *labModel, message tea.Msg) tea.Cmd {
	t.Helper()
	next, command := model.Update(message)
	require.Same(t, model, next)
	return command
}
