package chrome

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const workspaceMotionDelta = time.Second / 60

func TestWorkspaceRoutesZOrderModalAndCapture(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	workspace.SetTerminal(Size{Width: 80, Height: 24})
	workspace.SetFooter(1)
	require.NoError(t, workspace.SetSurfaces([]Surface{
		{ID: "help", Role: SurfacePassive, Requested: Rect{X: 50, Y: 10, Width: 30, Height: 10}, Visible: true, Priority: 1},
		{
			ID: "modal", Role: SurfaceModal,
			Requested: Rect{X: 20, Y: 5, Width: 40, Height: 12},
			Visible:   true, Priority: 2, DismissOutside: true, DismissBack: true,
		},
	}))
	require.Equal(t, Rect{Width: 80, Height: 23}, workspace.Plan().Canvas)
	id, ok := workspace.SurfaceAt(Point{X: 30, Y: 8})
	require.True(t, ok)
	require.Equal(t, SurfaceID("modal"), id)
	_, ok = workspace.SurfaceAt(Point{X: 1, Y: 1})
	require.False(t, ok)

	workspace.Capture("modal")
	id, ok = workspace.SurfaceAt(Point{})
	require.True(t, ok)
	require.Equal(t, SurfaceID("modal"), id)
	workspace.Release()
	require.Empty(t, workspace.CaptureID())
	id, ok = workspace.DismissAt(Point{X: 1, Y: 1})
	require.True(t, ok)
	require.Equal(t, SurfaceID("modal"), id)
	id, ok = workspace.Back()
	require.True(t, ok)
	require.Equal(t, SurfaceID("modal"), id)
}

func TestWorkspacePreservesRequestedGeometryAndCanvasTransform(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	workspace.SetTerminal(Size{Width: 40, Height: 12})
	require.NoError(t, workspace.SetSurfaces([]Surface{
		{ID: "dock", Role: SurfaceDock, Requested: Rect{Width: 10, Height: 20}, Visible: true},
		{ID: "floating", Role: SurfaceFloating, Requested: Rect{X: 35, Y: 10, Width: 20, Height: 10}, Visible: true},
	}))
	plan := workspace.Plan()
	require.Equal(t, Rect{X: 10, Width: 30, Height: 12}, plan.Canvas)
	require.Equal(t, Rect{X: 20, Y: 2, Width: 20, Height: 10}, plan.Surfaces[1].Rect)
	require.Equal(t, Rect{X: 35, Y: 10, Width: 20, Height: 10}, plan.Surfaces[1].Surface.Requested)
	canvas, ok := workspace.ScreenToCanvas(Point{X: 12, Y: 3})
	require.True(t, ok)
	require.Equal(t, Point{X: 2, Y: 3}, canvas)
	screen, ok := workspace.CanvasToScreen(canvas)
	require.True(t, ok)
	require.Equal(t, Point{X: 12, Y: 3}, screen)
}

func TestWorkspaceAnchorsLifecycleAndDuplicateIDs(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	workspace.SetTerminal(Size{Width: 50, Height: 20})
	require.NoError(t, workspace.SetSurfaces([]Surface{
		{
			ID: "dock", Role: SurfaceDock, Dock: DockRight,
			Requested: Rect{Width: 12}, Visible: true,
		},
		{
			ID: "canvas-menu", Role: SurfaceFloating, Anchor: AnchorCanvas,
			Requested: Rect{X: 2, Y: 1, Width: 10, Height: 2},
			Visible:   true, Priority: 1, FocusOnOpen: true,
		},
	}))
	plan := workspace.Plan()
	require.Equal(t, Rect{Width: 38, Height: 20}, plan.Canvas)
	require.Equal(t, Rect{X: 38, Width: 12, Height: 20}, plan.Dock)
	require.Equal(t, Rect{X: 2, Y: 1, Width: 10, Height: 2}, plan.Surfaces[1].Rect)
	focused, ok := workspace.FocusOnOpen()
	require.True(t, ok)
	require.Equal(t, SurfaceID("canvas-menu"), focused)

	err := workspace.SetSurfaces([]Surface{
		{ID: "same", Visible: true},
		{ID: "same", Visible: true},
	})
	require.ErrorIs(t, err, ErrDuplicateSurface)
}

func TestWorkspaceCoordinatesAnimatedDockAndCanvasBoundary(t *testing.T) {
	t.Parallel()

	const sidebarWidth = 24
	var workspace Workspace
	workspace.SetTerminal(Size{Width: 80, Height: 20})
	require.True(t, workspace.RetargetSurface("sidebar", sidebarWidth))
	require.NoError(t, workspace.SetSurfaces([]Surface{{
		ID: "sidebar", Role: SurfaceDock, Dock: DockLeft,
		Requested: Rect{Width: sidebarWidth}, Visible: true, Animated: true,
	}}))

	var boundaries []int
	for workspace.SurfaceMoving("sidebar") {
		if !workspace.AdvanceSurface("sidebar", workspaceMotionDelta) {
			continue
		}
		plan := workspace.Plan()
		extent := workspace.SurfacePosition("sidebar")
		require.GreaterOrEqual(t, extent, 0)
		require.LessOrEqual(t, extent, sidebarWidth)
		sidebar, ok := workspace.Surface("sidebar")
		require.True(t, ok)
		require.Equal(t, Rect{
			X:     -sidebarWidth + extent,
			Width: sidebarWidth, Height: 20,
		}, sidebar.Content)
		require.Equal(t, Rect{Width: extent, Height: 20}, sidebar.Rect)
		require.Equal(t, sidebar.Rect.Right(), plan.Canvas.X)
		require.Equal(t, 80-sidebar.Rect.Width, plan.Canvas.Width)
		require.Equal(t, plan.Main.Width, sidebar.Rect.Width+plan.Canvas.Width)
		id, ok := workspace.SurfaceAt(Point{X: extent - 1, Y: 2})
		require.True(t, ok)
		require.Equal(t, SurfaceID("sidebar"), id)
		_, ok = workspace.SurfaceAt(Point{X: extent, Y: 2})
		require.False(t, ok)
		canvas, ok := workspace.ScreenToCanvas(Point{X: plan.Canvas.X + 3, Y: 2})
		require.True(t, ok)
		screen, ok := workspace.CanvasToScreen(canvas)
		require.True(t, ok)
		require.Equal(t, Point{X: plan.Canvas.X + 3, Y: 2}, screen)
		boundaries = append(boundaries, plan.Canvas.X)
	}
	require.Equal(t, sidebarWidth, boundaries[len(boundaries)-1])

	require.True(t, workspace.RetargetSurface("sidebar", 0))
	for workspace.SurfaceMoving("sidebar") {
		workspace.AdvanceSurface("sidebar", workspaceMotionDelta)
	}
	plan := workspace.Plan()
	sidebar, ok := workspace.Surface("sidebar")
	require.True(t, ok)
	require.Equal(t, Rect{X: -sidebarWidth, Width: sidebarWidth, Height: 20}, sidebar.Content)
	require.Equal(t, Rect{}, sidebar.Rect)
	require.Equal(t, plan.Main, plan.Canvas)
	_, ok = workspace.SurfaceAt(Point{Y: 2})
	require.False(t, ok)
}

func TestWorkspaceDrawerMovesContentAndKeepsCanvasFixed(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	workspace.SetTerminal(Size{Width: 40, Height: 12})
	require.True(t, workspace.RetargetSurface("sidebar", 18))
	require.NoError(t, workspace.SetSurfaces([]Surface{{
		ID: "sidebar", Role: SurfaceDrawer, Dock: DockLeft,
		Requested: Rect{Width: 18, Height: 12},
		Visible:   true, Animated: true, DismissOutside: true,
	}}))

	require.True(t, workspace.AdvanceSurface("sidebar", workspaceMotionDelta))
	plan := workspace.Plan()
	require.Equal(t, Rect{Width: 40, Height: 12}, plan.Canvas)
	sidebar, ok := workspace.Surface("sidebar")
	require.True(t, ok)
	require.Equal(t, 18, sidebar.Content.Width)
	require.Less(t, sidebar.Content.X, 0)
	require.Equal(t, 0, sidebar.Rect.X)
	require.Equal(t, sidebar.Content.Right(), sidebar.Rect.Right())
	require.Equal(t, workspace.SurfacePosition("sidebar"), sidebar.Rect.Width)
	id, ok := workspace.DismissAt(Point{X: sidebar.Rect.Right() + 1, Y: 2})
	require.True(t, ok)
	require.Equal(t, SurfaceID("sidebar"), id)
}

func TestWorkspaceAnimatedSurfaceRetargetsReversesAndDisablesMotion(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	require.True(t, workspace.RetargetSurface("sidebar", 24))
	plan := workspace.Plan()
	require.False(t, workspace.AdvanceSurface("sidebar", workspaceMotionDelta/2))
	require.Equal(t, plan, workspace.Plan())
	require.True(t, workspace.SurfaceMoving("sidebar"))
	require.True(t, workspace.AdvanceSurface(
		"sidebar",
		workspaceMotionDelta-workspaceMotionDelta/2,
	))
	current := workspace.SurfacePosition("sidebar")
	require.True(t, workspace.RetargetSurface("sidebar", 16))
	require.Equal(t, current, workspace.SurfacePosition("sidebar"))
	require.True(t, workspace.RetargetSurface("sidebar", 0))
	require.Equal(t, current, workspace.SurfacePosition("sidebar"))
	for range 240 {
		workspace.AdvanceSurface("sidebar", workspaceMotionDelta)
		if workspace.SurfacePosition("sidebar") != current {
			break
		}
	}
	require.Less(t, workspace.SurfacePosition("sidebar"), current)

	workspace.SetMotionEnabled(false)
	require.Zero(t, workspace.SurfacePosition("sidebar"))
	require.False(t, workspace.SurfaceMoving("sidebar"))
	require.False(t, workspace.RetargetSurface("sidebar", 20))
	require.Equal(t, 20, workspace.SurfacePosition("sidebar"))
	require.False(t, workspace.SurfaceMoving("sidebar"))
}
