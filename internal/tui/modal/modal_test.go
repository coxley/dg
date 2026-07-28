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

func TestModelTreatsConfiguredWidthAsOuterWidth(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(
		80, 24, 0, 40, "body", Standard,
		[]Tab{{ID: 1, Label: "One"}},
		1,
	)

	require.Equal(t, 40, model.Overlay().Width)
	require.Equal(t, 38, model.BodyWidth())
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

func TestModelDragsFromEmptyCellsAfterPointerMotion(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(80, 24, 0, 40, "Save", Standard, nil, 0)
	before := model.Overlay()

	model, command := model.Update(tea.MouseClickMsg{
		X:      before.ContentLeft + 10,
		Y:      before.ContentTop,
		Button: tea.MouseLeft,
	})
	require.Nil(t, command)
	require.True(t, model.CapturesPointer())
	require.False(t, model.Dragging())
	require.Equal(t, before, model.Overlay())

	model, command = model.Update(tea.MouseMotionMsg{
		X:      before.ContentLeft + 15,
		Y:      before.ContentTop + 2,
		Button: tea.MouseLeft,
	})
	require.Nil(t, command)
	require.True(t, model.Dragging())
	require.Equal(t, before.Left+5, model.Overlay().Left)
	require.Equal(t, before.Top+2, model.Overlay().Top)
}

func TestModelLeavesRenderedContentInteractive(t *testing.T) {
	t.Parallel()

	button := lipgloss.NewStyle().
		Background(lipgloss.Color("#ffffff")).
		Render(" Save ")
	model := New(testStyles())
	model.Configure(80, 24, 0, 40, button, Standard, nil, 0)
	overlay := model.Overlay()

	for _, offset := range []int{0, 1} {
		next, command := model.Update(tea.MouseClickMsg{
			X:      overlay.ContentLeft + offset,
			Y:      overlay.ContentTop,
			Button: tea.MouseLeft,
		})
		require.Nil(t, command)
		require.False(t, next.CapturesPointer())
	}
}

func TestModelReleasesPendingDragWithoutMoving(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(80, 24, 0, 40, "Save", Standard, nil, 0)
	before := model.Overlay()
	model, _ = model.Update(tea.MouseClickMsg{
		X:      before.ContentLeft + 10,
		Y:      before.ContentTop,
		Button: tea.MouseLeft,
	})
	model, command := model.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft})

	require.Nil(t, command)
	require.False(t, model.CapturesPointer())
	require.Equal(t, before, model.Overlay())
}

func TestModelResizesFromNearestCornerAfterPointerMotion(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(80, 24, 0, 40, "one\ntwo\nthree", Standard, nil, 0)
	before := model.Overlay()
	bodyHeight := model.BodyHeight()
	mouse := tea.Mouse{
		X:      before.Left + before.Width - 4,
		Y:      before.Top + before.Height - 2,
		Button: tea.MouseRight,
	}

	model, command := model.Update(tea.MouseClickMsg(mouse))
	require.Nil(t, command)
	require.True(t, model.CapturesPointer())
	require.False(t, model.Resizing())
	require.Equal(t, before, model.Overlay())

	mouse.X += 5
	mouse.Y += 2
	model, command = model.Update(tea.MouseMotionMsg(mouse))
	require.Nil(t, command)
	require.True(t, model.Resizing())
	require.Equal(t, before.Left, model.Overlay().Left)
	require.Equal(t, before.Top, model.Overlay().Top)
	require.Equal(t, before.Width+5, model.Overlay().Width)
	require.Equal(t, before.Height+2, model.Overlay().Height)
	require.Equal(t, bodyHeight+2, model.BodyHeight())
}

func TestModelNorthwestResizeKeepsOppositeCornerFixed(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(80, 24, 0, 40, "one\ntwo\nthree", Standard, nil, 0)
	before := model.Overlay()
	right := before.Left + before.Width
	bottom := before.Top + before.Height
	mouse := tea.Mouse{
		X:      before.Left,
		Y:      before.Top,
		Button: tea.MouseRight,
	}

	model, _ = model.Update(tea.MouseClickMsg(mouse))
	mouse.X -= 3
	mouse.Y -= 2
	model, _ = model.Update(tea.MouseMotionMsg(mouse))

	after := model.Overlay()
	require.Equal(t, before.Left-3, after.Left)
	require.Equal(t, before.Top-2, after.Top)
	require.Equal(t, right, after.Left+after.Width)
	require.Equal(t, bottom, after.Top+after.Height)
}

func TestModelResizeHonorsMinimumAndSurvivesConfigure(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(
		80, 24, 0, 40,
		"one\ntwo\nthree\nfour\nfive",
		Standard,
		nil,
		0,
	)
	before := model.Overlay()
	mouse := tea.Mouse{
		X:      before.Left + before.Width - 1,
		Y:      before.Top + before.Height - 1,
		Button: tea.MouseRight,
	}
	model, _ = model.Update(tea.MouseClickMsg(mouse))
	model, _ = model.Update(tea.MouseMotionMsg{
		X:      before.Left,
		Y:      before.Top,
		Button: tea.MouseRight,
	})
	resized := model.Overlay()
	require.Equal(t, minimumWidth, resized.Width)
	require.Equal(t, minimumHeight, resized.Height)

	model.Configure(
		80, 24, 0, 40,
		"one\ntwo\nthree\nfour\nfive",
		Standard,
		nil,
		0,
	)
	require.Equal(t, resized, model.Overlay())
}

func TestModelReleasesPendingResizeWithoutChangingSize(t *testing.T) {
	t.Parallel()

	model := New(testStyles())
	model.Configure(80, 24, 0, 40, "body", Standard, nil, 0)
	before := model.Overlay()
	model, _ = model.Update(tea.MouseClickMsg{
		X:      before.ContentLeft,
		Y:      before.ContentTop,
		Button: tea.MouseRight,
	})
	model, command := model.Update(tea.MouseReleaseMsg{Button: tea.MouseRight})

	require.Nil(t, command)
	require.False(t, model.CapturesPointer())
	require.Equal(t, before, model.Overlay())
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
