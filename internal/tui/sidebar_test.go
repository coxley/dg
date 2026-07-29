package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/stretchr/testify/require"
)

func TestSidebarAdaptsFocusAndBackToPlacement(t *testing.T) {
	t.Parallel()

	t.Run("docked", func(t *testing.T) {
		t.Parallel()

		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
		updateModelCommand(t, model, sidebarKey())
		require.True(t, model.sidebar.open)
		require.True(t, model.sidebar.focused)
		require.Equal(t, sidebarDocked, model.sidebar.placement)
		advanceSidebar(t, model)
		require.Equal(t, sidebarPreferredWidth, model.workspace.Geometry().Canvas.X)

		updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		require.True(t, model.sidebar.open)
		require.False(t, model.sidebar.focused)
		require.Equal(t, sidebarPreferredWidth, model.workspace.SurfacePosition(surfaceSidebar))
		scope, focus := model.sidebar.focus.Current()
		require.Equal(t, scopeCanvas, scope)
		require.Equal(t, chrome.FocusID("canvas"), focus)

		updateModelCommand(t, model, sidebarKey())
		require.True(t, model.sidebar.focused)
		updateModelCommand(t, model, sidebarKey())
		require.False(t, model.sidebar.open)
		require.False(t, model.sidebar.focused)
		advanceSidebar(t, model)
		require.Zero(t, model.workspace.SurfacePosition(surfaceSidebar))
	})

	t.Run("drawer", func(t *testing.T) {
		t.Parallel()

		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 16})
		updateModelCommand(t, model, sidebarKey())
		require.Equal(t, sidebarDrawer, model.sidebar.placement)
		advanceSidebar(t, model)
		require.Zero(t, model.workspace.Geometry().Canvas.X)

		surface, ok := model.surfacePlan(surfaceSidebar)
		require.True(t, ok)
		require.Equal(t, sidebarPreferredWidth, surface.Rect.Width)
		command := updateModelCommand(t, model, tea.MouseClickMsg{
			X:      surface.Rect.Right() + 2,
			Y:      4,
			Button: tea.MouseLeft,
		})
		require.NotNil(t, command)
		require.False(t, model.sidebar.open)
		require.False(t, model.sidebar.focused)
		require.Equal(t, []chrome.ScopeID{scopeCanvas, scopeGlobal}, model.activeBindingScopes())
	})

	t.Run("compact boundary", func(t *testing.T) {
		t.Parallel()

		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})
		updateModelCommand(t, model, sidebarKey())
		require.Equal(t, sidebarDrawer, model.sidebar.placement)
	})
}

func TestDockedSidebarUsesOneBoundaryForRenderInputAndCursor(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	viewport := model.viewport
	updateModelCommand(t, model, sidebarKey())

	var boundaries []int
	for model.workspace.SurfaceMoving(surfaceSidebar) {
		updateModelCommand(t, model, sidebarMotionMsg{generation: model.sidebar.generation})
		geometry := model.workspace.Geometry()
		position := model.workspace.SurfacePosition(surfaceSidebar)
		require.Equal(t, position, geometry.Canvas.X)
		require.Equal(t, 100-position, geometry.Canvas.Width)
		require.Equal(t, viewport, model.viewport)

		canvasPoint := chrome.Point{X: min(3, geometry.Canvas.Width-1), Y: 2}
		screen, ok := model.workspace.CanvasToScreen(canvasPoint)
		require.True(t, ok)
		roundTrip, ok := model.workspace.ScreenToCanvas(screen)
		require.True(t, ok)
		require.Equal(t, canvasPoint, roundTrip)

		x, _, ok := model.cursorPosition()
		require.True(t, ok)
		require.Equal(t, geometry.Canvas.X+int(model.cursor.X-model.viewport.X), x)
		navigation, ok := model.surfacePlan(surfaceNavigation)
		require.True(t, ok)
		require.GreaterOrEqual(t, navigation.Rect.X, geometry.Canvas.X)

		lines := strings.Split(strings.TrimSuffix(ansi.Strip(model.View().Content), "\n"), "\n")
		require.Len(t, lines, 20)
		for _, line := range lines {
			require.LessOrEqual(t, ansi.StringWidth(line), 100)
		}
		boundaries = append(boundaries, position)
	}
	require.NotEmpty(t, boundaries)
	require.Equal(t, sidebarPreferredWidth, boundaries[len(boundaries)-1])
}

func TestSidebarRetargetsAcrossResizeAndReversesFromCurrentCell(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	updateModelCommand(t, model, sidebarKey())
	updateModelCommand(t, model, sidebarMotionMsg{generation: model.sidebar.generation})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	_, focused := model.sidebar.focus.Current()
	current := model.workspace.SurfacePosition(surfaceSidebar)

	updateModelCommand(t, model, tea.WindowSizeMsg{Width: 60, Height: 16})
	require.Equal(t, sidebarDrawer, model.sidebar.placement)
	require.Equal(t, current, model.workspace.SurfacePosition(surfaceSidebar))
	require.Zero(t, model.workspace.Geometry().Canvas.X)
	_, resizedFocus := model.sidebar.focus.Current()
	require.Equal(t, focused, resizedFocus)

	updateModelCommand(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	require.Equal(t, sidebarDocked, model.sidebar.placement)
	require.Equal(t, current, model.workspace.SurfacePosition(surfaceSidebar))
	require.Equal(t, current, model.workspace.Geometry().Canvas.X)

	updateModelCommand(t, model, sidebarKey())
	updateModelCommand(t, model, sidebarMotionMsg{generation: model.sidebar.generation})
	closing := model.workspace.SurfacePosition(surfaceSidebar)
	require.Less(t, closing, current)
	updateModelCommand(t, model, sidebarKey())
	require.Equal(t, closing, model.workspace.SurfacePosition(surfaceSidebar))
	updateModelCommand(t, model, sidebarMotionMsg{generation: model.sidebar.generation})
	require.Greater(t, model.workspace.SurfacePosition(surfaceSidebar), closing)
}

func TestSidebarDisabledMotionSnapsWithoutChangingPlacementPolicy(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.setMotionEnabled(false)
	command := updateModelCommand(t, model, sidebarKey())
	require.Nil(t, command)
	require.Equal(t, sidebarPreferredWidth, model.workspace.SurfacePosition(surfaceSidebar))
	require.Equal(t, sidebarPreferredWidth, model.workspace.Geometry().Canvas.X)
	require.False(t, model.workspace.SurfaceMoving(surfaceSidebar))
}

func TestSidebarPaneUsesApplicationHeaderBodyAndFooter(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 12})
	model.setMotionEnabled(false)
	updateModel(t, model, sidebarKey())
	surface, ok := model.surfacePlan(surfaceSidebar)
	require.True(t, ok)
	lines := model.sidebar.lines(surface)

	require.Contains(t, ansi.Strip(lines[0]), "SIDEBAR")
	require.Contains(t, ansi.Strip(strings.Join(lines[1:len(lines)-1], "\n")), "Overview")
	require.Contains(t, ansi.Strip(lines[len(lines)-1]), "Esc canvas")
}

func sidebarKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'b', Mod: tea.ModCtrl})
}

func advanceSidebar(t testing.TB, model *Model) {
	t.Helper()
	for model.workspace.SurfaceMoving(surfaceSidebar) {
		updateModelCommand(t, model, sidebarMotionMsg{generation: model.sidebar.generation})
	}
}
