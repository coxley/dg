package chrome

import (
	"testing"

	"github.com/stretchr/testify/require"
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
