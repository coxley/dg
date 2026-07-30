package chrome

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	workspaceSidebarID   = "sidebar"
	workspaceMotionDelta = time.Second / 60
)

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
	require.True(t, workspace.RetargetSurface(workspaceSidebarID, sidebarWidth))
	require.NoError(t, workspace.SetSurfaces([]Surface{{
		ID: workspaceSidebarID, Role: SurfaceDock, Dock: DockLeft,
		Requested: Rect{Width: sidebarWidth}, Visible: true, Animated: true,
	}}))

	var boundaries []int
	for workspace.SurfaceMoving(workspaceSidebarID) {
		if !workspace.AdvanceSurface(workspaceSidebarID, workspaceMotionDelta) {
			continue
		}
		plan := workspace.Plan()
		extent := workspace.SurfacePosition(workspaceSidebarID)
		require.GreaterOrEqual(t, extent, 0)
		require.LessOrEqual(t, extent, sidebarWidth)
		sidebar, ok := workspace.Surface(workspaceSidebarID)
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
		require.Equal(t, SurfaceID(workspaceSidebarID), id)
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

	require.True(t, workspace.RetargetSurface(workspaceSidebarID, 0))
	for workspace.SurfaceMoving(workspaceSidebarID) {
		workspace.AdvanceSurface(workspaceSidebarID, workspaceMotionDelta)
	}
	plan := workspace.Plan()
	sidebar, ok := workspace.Surface(workspaceSidebarID)
	require.True(t, ok)
	require.Equal(t, Rect{X: -sidebarWidth, Width: sidebarWidth, Height: 20}, sidebar.Content)
	require.Equal(t, Rect{}, sidebar.Rect)
	require.Equal(t, plan.Main, plan.Canvas)
	_, ok = workspace.SurfaceAt(Point{Y: 2})
	require.False(t, ok)
}

func TestWorkspaceVerticalDockOwnsHeightBesideCanvasFooter(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	workspace.SetTerminal(Size{Width: 80, Height: 20})
	workspace.SetFooter(1)
	workspace.SetMotionEnabled(false)
	require.False(t, workspace.RetargetSurface(workspaceSidebarID, 24))
	require.NoError(t, workspace.SetSurfaces([]Surface{{
		ID: workspaceSidebarID, Role: SurfaceDock, Dock: DockLeft,
		Requested: Rect{Width: 24}, Visible: true, Animated: true,
	}}))

	plan := workspace.Plan()
	require.Equal(t, Rect{Width: 80, Height: 19}, plan.Main)
	require.Equal(t, Rect{X: 24, Width: 56, Height: 19}, plan.Canvas)
	require.Equal(t, Rect{X: 24, Y: 19, Width: 56, Height: 1}, plan.Footer)
	sidebar, ok := workspace.Surface(workspaceSidebarID)
	require.True(t, ok)
	require.Equal(t, Rect{Width: 24, Height: 20}, sidebar.Rect)
	require.Equal(t, sidebar.Rect, sidebar.Content)
}

func TestWorkspaceDrawerMovesContentAndKeepsCanvasFixed(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	workspace.SetTerminal(Size{Width: 40, Height: 12})
	require.True(t, workspace.RetargetSurface(workspaceSidebarID, 18))
	require.NoError(t, workspace.SetSurfaces([]Surface{{
		ID: workspaceSidebarID, Role: SurfaceDrawer, Dock: DockLeft,
		Requested: Rect{Width: 18, Height: 12},
		Visible:   true, Animated: true, DismissOutside: true,
	}}))

	require.True(t, workspace.AdvanceSurface(workspaceSidebarID, workspaceMotionDelta))
	plan := workspace.Plan()
	require.Equal(t, Rect{Width: 40, Height: 12}, plan.Canvas)
	sidebar, ok := workspace.Surface(workspaceSidebarID)
	require.True(t, ok)
	require.Equal(t, 18, sidebar.Content.Width)
	require.Less(t, sidebar.Content.X, 0)
	require.Equal(t, 0, sidebar.Rect.X)
	require.Equal(t, sidebar.Content.Right(), sidebar.Rect.Right())
	require.Equal(t, workspace.SurfacePosition(workspaceSidebarID), sidebar.Rect.Width)
	id, ok := workspace.DismissAt(Point{X: sidebar.Rect.Right() + 1, Y: 2})
	require.True(t, ok)
	require.Equal(t, SurfaceID(workspaceSidebarID), id)
}

func TestWorkspaceAnimatedSurfaceRetargetsReversesAndDisablesMotion(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	require.True(t, workspace.RetargetSurface(workspaceSidebarID, 24))
	plan := workspace.Plan()
	require.False(t, workspace.AdvanceSurface(workspaceSidebarID, workspaceMotionDelta/2))
	require.Equal(t, plan, workspace.Plan())
	require.True(t, workspace.SurfaceMoving(workspaceSidebarID))
	require.True(t, workspace.AdvanceSurface(
		workspaceSidebarID,
		workspaceMotionDelta-workspaceMotionDelta/2,
	))
	current := workspace.SurfacePosition(workspaceSidebarID)
	require.True(t, workspace.RetargetSurface(workspaceSidebarID, 16))
	require.Equal(t, current, workspace.SurfacePosition(workspaceSidebarID))
	require.True(t, workspace.RetargetSurface(workspaceSidebarID, 0))
	require.Equal(t, current, workspace.SurfacePosition(workspaceSidebarID))
	for range 240 {
		workspace.AdvanceSurface(workspaceSidebarID, workspaceMotionDelta)
		if workspace.SurfacePosition(workspaceSidebarID) != current {
			break
		}
	}
	require.Less(t, workspace.SurfacePosition(workspaceSidebarID), current)

	workspace.SetMotionEnabled(false)
	require.Zero(t, workspace.SurfacePosition(workspaceSidebarID))
	require.False(t, workspace.SurfaceMoving(workspaceSidebarID))
	require.False(t, workspace.RetargetSurface(workspaceSidebarID, 20))
	require.Equal(t, 20, workspace.SurfacePosition(workspaceSidebarID))
	require.False(t, workspace.SurfaceMoving(workspaceSidebarID))
}
