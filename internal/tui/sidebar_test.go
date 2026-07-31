package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
	canvasstore "github.com/coxley/dg/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSidebarBuildsCanvasSectionsAndDraftTab(t *testing.T) {
	t.Parallel()

	model, _, store := newStoredTestModel(t, "active draft")
	for _, location := range [][2]string{
		{"", "Architecture"},
		{"", "Databases"},
		{"Interviews", "Candidate 1"},
		{"Interviews", "Candidate 2"},
		{"RFCs", "Proposal.bak"},
	} {
		_, err := store.Create(location[0], location[1], document.New(mustLayoutWithLabel(t, location[1])))
		require.NoError(t, err)
	}
	_, err := store.CreateDraft(document.New(mustLayoutWithLabel(t, "other draft")))
	require.NoError(t, err)
	model.updateCatalog(store.Reconcile(model.catalog))

	labels := sidebarLabels(model)
	require.Equal(t, []string{
		"Architecture",
		"Databases",
		"▾ [Interviews]",
		"  Candidate 1",
		"  Candidate 2",
		"▾ [RFCs]",
		"  Proposal.bak [backup]",
	}, labels)
	wantWidth := model.sidebar.desired
	focusSidebarItem(t, model, "section:Interviews")
	model.activateSidebar()
	require.NotContains(t, sidebarLabels(model), "  Candidate 1")
	require.Equal(t, wantWidth, model.sidebar.desired)

	model.switchSidebarTab(1)
	require.Equal(t, "Canvases  [Drafts]", model.sidebar.declaration.Header)
	require.Equal(t, wantWidth, model.sidebar.desired)
	require.Len(t, model.sidebar.declaration.Items, 3)
	for _, item := range model.sidebar.declaration.Items[:2] {
		require.NotContains(t, item.Label, "draft")
		require.NotEmpty(t, item.Label)
	}
	require.Equal(t, "Clear Drafts...", model.sidebar.declaration.Items[2].Label)
}

func TestSidebarContentWidthControlsDockBoundary(t *testing.T) {
	t.Parallel()

	model, _, store := newStoredTestModel(t, "draft")
	name := strings.Repeat("界", 20)
	_, err := store.Create("", name, document.New(mustLayoutWithLabel(t, "wide")))
	require.NoError(t, err)
	model.updateCatalog(store.Reconcile(model.catalog))
	require.Greater(t, model.sidebar.desired, sidebarMinimumWidth)

	updateModel(t, model, tea.WindowSizeMsg{
		Width:  model.sidebar.desired + sidebarCanvasMinimum,
		Height: 20,
	})
	updateModelCommand(t, model, sidebarKey())
	require.Equal(t, sidebarDocked, model.sidebar.placement)
	updateModelCommand(t, model, tea.WindowSizeMsg{
		Width:  model.sidebar.desired + sidebarCanvasMinimum - 1,
		Height: 20,
	})
	require.Equal(t, sidebarDrawer, model.sidebar.placement)
}

func TestSidebarDeletesDraftsWithActivePreservationAndConfirmation(t *testing.T) {
	t.Parallel()

	model, _, store := newStoredTestModel(t, "active")
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	for i := range 3 {
		_, err := store.CreateDraft(document.New(mustLayoutWithLabel(t, fmt.Sprintf("draft %d", i))))
		require.NoError(t, err)
	}
	model.updateCatalog(store.Reconcile(model.catalog))
	model.switchSidebarTab(1)
	focusSidebarItem(t, model, "drafts:clear")
	model.activateSidebar()
	require.Equal(t, surfaceConfirmation, model.dialogs.ActiveID())
	model.syncWorkspace()
	require.Contains(t, model.dialogs.confirmation.View(), "Delete 3 canvases?")
	model.handleDialogResult(model.dialogs.Update(chrome.FormSubmitMsg{ID: confirmationAccept}))
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.Equal(t, "deleted 3 canvases", model.status)
	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, model.entry.ID, entries[0].ID)
}

func TestSidebarDeleteMovesNamedCanvasToDrafts(t *testing.T) {
	t.Parallel()

	t.Run("active", func(t *testing.T) {
		t.Parallel()

		model, nodeID, store := newStoredTestModel(t, "original")
		named, err := store.Name(*model.entry, "", "Canvas")
		require.NoError(t, err)
		model.setActiveEntry(named)
		model.updateCatalog(store.Reconcile(model.catalog))
		require.NoError(t, model.geo.SetNodeLabel(nodeID, "changed"))
		focusSidebarItem(t, model, "canvas:/Canvas")

		model.deleteFocusedCanvas()

		require.NotNil(t, model.entry)
		require.True(t, model.entry.Draft)
		require.Equal(t, named.ID, model.entry.ID)
		require.Equal(t, "dg - Draft", model.windowTitle)
		require.Equal(t, "moved Canvas to Drafts", model.status)
		loaded, err := store.Load(*model.entry)
		require.NoError(t, err)
		require.Equal(t, "changed", loaded.Nodes[0].Label)
		entries, err := store.List()
		require.NoError(t, err)
		require.Equal(t, []canvasstore.Entry{*model.entry}, entries)
	})

	t.Run("inactive", func(t *testing.T) {
		t.Parallel()

		model, _, store := newStoredTestModel(t, "active")
		active := *model.entry
		named, err := store.Create("", "Canvas", document.New(mustLayoutWithLabel(t, "named")))
		require.NoError(t, err)
		model.updateCatalog(store.Reconcile(model.catalog))
		focusSidebarItem(t, model, "canvas:/Canvas")

		model.deleteFocusedCanvas()

		require.Equal(t, active.ID, model.entry.ID)
		require.True(t, model.entry.Draft)
		entries, err := store.List()
		require.NoError(t, err)
		require.Len(t, entries, 2)
		require.False(t, slices.ContainsFunc(entries, func(entry canvasstore.Entry) bool {
			return !entry.Draft && entry.Name == named.Name
		}))
		require.True(t, slices.ContainsFunc(entries, func(entry canvasstore.Entry) bool {
			return entry.Draft && entry.ID == named.ID
		}))
	})
}

func TestSidebarDeleteRemovesOnlyInactiveDraft(t *testing.T) {
	t.Parallel()

	model, _, store := newStoredTestModel(t, "active")
	active := *model.entry
	other, err := store.CreateDraft(document.New(mustLayoutWithLabel(t, "other")))
	require.NoError(t, err)
	model.updateCatalog(store.Reconcile(model.catalog))
	model.switchSidebarTab(1)
	focusSidebarItem(t, model, chrome.FocusID("draft:"+active.ID.String()))
	model.deleteFocusedCanvas()
	require.Equal(t, "cannot delete the active draft", model.status)

	focusSidebarItem(t, model, chrome.FocusID("draft:"+other.ID.String()))
	model.deleteFocusedCanvas()
	entries, err := store.List()
	require.NoError(t, err)
	require.Equal(t, []canvasstore.Entry{active}, entries)
}

func BenchmarkSidebarCatalog1000(b *testing.B) {
	model, _, _ := newStoredTestModel(b, "active")
	model.catalog = make([]canvasstore.Entry, 1000)
	for i := range model.catalog {
		model.catalog[i] = canvasstore.Entry{
			Section: fmt.Sprintf("Section %02d", i%20),
			Name:    fmt.Sprintf("Canvas %04d", i),
			ID:      uuid.New(),
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		model.rebuildSidebarCatalog()
	}
}

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
		updateModel(t, model, tea.WindowSizeMsg{Width: sidebarPreferredWidth + sidebarCanvasMinimum - 1, Height: 16})
		updateModelCommand(t, model, sidebarKey())
		require.Equal(t, sidebarDrawer, model.sidebar.placement)
		updateModelCommand(t, model, tea.WindowSizeMsg{Width: sidebarPreferredWidth + sidebarCanvasMinimum, Height: 16})
		require.Equal(t, sidebarDocked, model.sidebar.placement)
	})
}

func TestDockedSidebarUsesOneBoundaryForRenderInputAndCursor(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.sidebar.declaration.Items[0].Label = strings.Repeat("wrap ", 12)
	model.sidebar.render()
	viewport := model.viewport
	updateModelCommand(t, model, sidebarKey())
	navigation, ok := model.surfacePlan(surfaceNavigation)
	require.True(t, ok)
	navigationRect := navigation.Rect
	sidebarLines := model.sidebar.pane.Lines()
	sidebarExtent := model.sidebar.viewport.Plan().Extent

	var boundaries []int
	for model.workspace.SurfaceMoving(surfaceSidebar) {
		updateModelCommand(t, model, sidebarMotionMessage(model))
		geometry := model.workspace.Geometry()
		position := model.workspace.SurfacePosition(surfaceSidebar)
		sidebar, ok := model.surfacePlan(surfaceSidebar)
		require.True(t, ok)
		require.Equal(t, position, geometry.Canvas.X)
		require.Equal(t, 100-position, geometry.Canvas.Width)
		require.Equal(t, chrome.Rect{
			X:     -sidebarPreferredWidth + position,
			Width: sidebarPreferredWidth, Height: geometry.Terminal.Height,
		}, sidebar.Content)
		require.Equal(t, chrome.Rect{
			Width: position, Height: geometry.Terminal.Height,
		}, sidebar.Rect)
		require.Equal(t, sidebarPreferredWidth, model.sidebar.pane.Plan().Bounds.Width)
		require.Equal(t, sidebarExtent, model.sidebar.viewport.Plan().Extent)
		require.Equal(t, sidebarLines, model.sidebar.pane.Lines())
		visibleLines := model.sidebar.lines(sidebar)
		for i, line := range sidebarLines {
			want := ansi.Cut(
				line,
				sidebarPreferredWidth-position,
				sidebarPreferredWidth,
			)
			require.Equal(t, ansi.Strip(want), ansi.Strip(visibleLines[i]))
		}
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
		require.Equal(t, navigationRect.Width, navigation.Rect.Width)
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

func TestDockedSidebarKeepsNavigationCanvasAnchoredAtMinimumWidth(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: compactWidthThreshold, Height: 16})
	navigation, ok := model.surfacePlan(surfaceNavigation)
	require.True(t, ok)
	wantWidth := navigation.Rect.Width
	updateModelCommand(t, model, sidebarKey())

	for model.workspace.SurfaceMoving(surfaceSidebar) {
		updateModelCommand(t, model, sidebarMotionMessage(model))
		navigation, ok := model.surfacePlan(surfaceNavigation)
		require.True(t, ok)
		require.Equal(t, wantWidth, navigation.Rect.Width)
		require.GreaterOrEqual(t, navigation.Rect.X, model.workspace.Geometry().Canvas.X)
		lines := strings.Split(ansi.Strip(model.View().Content), "\n")
		for _, line := range lines {
			require.LessOrEqual(t, ansi.StringWidth(line), compactWidthThreshold)
		}
		navigationLine := strings.Split(ansi.Strip(model.nav.View()), "\n")[0]
		require.Equal(
			t,
			navigationLine,
			ansi.Cut(lines[navigation.Rect.Y], navigation.Rect.X, navigation.Rect.Right()),
		)
	}
}

func TestSidebarRetargetsAcrossResizeAndReversesFromCurrentCell(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	updateModelCommand(t, model, sidebarKey())
	updateModelCommand(t, model, sidebarMotionMessage(model))
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	_, focused := model.sidebar.focus.Current()
	current := model.workspace.SurfacePosition(surfaceSidebar)

	updateModelCommand(t, model, tea.WindowSizeMsg{Width: 60, Height: 16})
	require.Equal(t, sidebarDrawer, model.sidebar.placement)
	require.Equal(t, current, model.workspace.SurfacePosition(surfaceSidebar))
	require.Zero(t, model.workspace.Geometry().Canvas.X)
	drawer, ok := model.surfacePlan(surfaceSidebar)
	require.True(t, ok)
	require.Equal(t, sidebarPreferredWidth, drawer.Content.Width)
	require.Equal(t, current, drawer.Rect.Width)
	_, resizedFocus := model.sidebar.focus.Current()
	require.Equal(t, focused, resizedFocus)

	updateModelCommand(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	require.Equal(t, sidebarDocked, model.sidebar.placement)
	require.Equal(t, current, model.workspace.SurfacePosition(surfaceSidebar))
	require.Equal(t, current, model.workspace.Geometry().Canvas.X)
	dock, ok := model.surfacePlan(surfaceSidebar)
	require.True(t, ok)
	require.Equal(t, sidebarPreferredWidth, dock.Content.Width)
	require.Equal(t, current, dock.Rect.Width)

	updateModelCommand(t, model, sidebarKey())
	closing := advanceSidebarUntilExtentChanges(t, model, current)
	require.Less(t, closing, current)
	updateModelCommand(t, model, sidebarKey())
	require.Equal(t, closing, model.workspace.SurfacePosition(surfaceSidebar))
	opening := advanceSidebarUntilExtentChanges(t, model, closing)
	require.Greater(t, opening, closing)
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

func TestDockedSidebarDoesNotBoundNodeDrag(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	require.NoError(t, model.geo.PlaceNode(nodeID, layout.NewPoint(2, 8)))
	staticID, err := model.geo.NewNodeAt("static", layout.NewPoint(20, 8))
	require.NoError(t, err)
	require.NoError(t, model.rebuild())

	updateModelCommand(t, model, sidebarKey())
	advanceSidebar(t, model)
	canvas := model.workspace.Geometry().Canvas
	require.Positive(t, canvas.X)

	staticBefore := model.geo.Nodes[staticID].Rect.Min
	label := model.geo.Nodes[nodeID].LabelPoint
	y := canvas.Y + int(label.Y-model.viewport.Y)
	updateModel(t, model, tea.MouseClickMsg{
		X:      canvas.X + int(label.X-model.viewport.X),
		Y:      y,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      0,
		Y:      y,
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X:      0,
		Y:      y,
		Button: tea.MouseLeft,
	})

	require.Equal(t, uint32(0), model.geo.Nodes[nodeID].Rect.Min.X)
	require.Less(
		t,
		int64(canvas.X)+
			int64(model.geo.Nodes[nodeID].Rect.Min.X)-
			int64(model.viewport.X),
		int64(0),
	)
	staticAfter := model.geo.Nodes[staticID].Rect.Min
	require.Equal(t, staticBefore.X, staticAfter.X-model.viewport.X)
	require.Equal(t, staticBefore.Y, staticAfter.Y-model.viewport.Y)
}

func TestDockedSidebarDoesNotBoundAreaSelection(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	require.NoError(t, model.geo.PlaceNode(nodeID, layout.NewPoint(2, 8)))
	require.NoError(t, model.rebuild())

	updateModelCommand(t, model, sidebarKey())
	advanceSidebar(t, model)
	canvas := model.workspace.Geometry().Canvas
	start := layout.NewPoint(15, 12)
	updateModel(t, model, tea.MouseClickMsg{
		X:      canvas.X + int(start.X-model.viewport.X),
		Y:      canvas.Y + int(start.Y-model.viewport.Y),
		Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X:      0,
		Y:      canvas.Y + int(model.geo.Nodes[nodeID].Rect.Min.Y-model.viewport.Y),
		Button: tea.MouseLeft,
	})

	require.True(t, model.highlightedPoint(model.geo.Nodes[nodeID].Rect.Min))

	updateModel(t, model, tea.MouseReleaseMsg{
		X:      0,
		Y:      canvas.Y + int(model.geo.Nodes[nodeID].Rect.Min.Y-model.viewport.Y),
		Button: tea.MouseLeft,
	})

	require.True(t, selectionContains(model, layout.HitNode, nodeID))
}

func TestDockedSidebarPushesStatusLineToCanvas(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	model.selectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	updateModelCommand(t, model, sidebarKey())
	advanceSidebar(t, model)

	workspace := model.workspace.Geometry()
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	status := lines[workspace.Footer.Y]
	sidebar := ansi.Cut(status, 0, workspace.Canvas.X)
	require.Contains(t, sidebar, "Esc canvas")
	require.Contains(t, ansi.Cut(status, workspace.Canvas.X, workspace.Terminal.Width), "selected  nodes 1")
}

func TestSidebarScrollbarSupportsPointerDrag(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	for i := range 20 {
		model.sidebar.declaration.Items = append(
			model.sidebar.declaration.Items,
			sidebarItem{
				ID:    chrome.FocusID("extra-" + strconv.Itoa(i)),
				Label: "Extra " + strconv.Itoa(i),
			},
		)
	}
	model.sidebar.render()
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 16})
	updateModelCommand(t, model, sidebarKey())
	advanceSidebar(t, model)
	surface, ok := model.surfacePlan(surfaceSidebar)
	require.True(t, ok)
	plan := model.sidebar.viewport.Plan()
	require.NotEmpty(t, plan.VerticalThumb)

	x := surface.Content.X + plan.VerticalThumb.X
	y := surface.Content.Y + plan.VerticalThumb.Y
	updateModel(t, model, tea.MouseClickMsg{
		X: x, Y: y, Button: tea.MouseLeft,
	})
	require.Equal(t, surfaceSidebar, model.workspace.CaptureID())
	updateModel(t, model, tea.MouseMotionMsg{
		X:      x,
		Y:      surface.Content.Y + plan.VerticalBar.Bottom() - 1,
		Button: tea.MouseLeft,
	})
	require.Positive(t, model.sidebar.viewport.Plan().Offset.Y)
	updateModel(t, model, tea.MouseReleaseMsg{
		X: x, Y: y, Button: tea.MouseLeft,
	})
	require.Empty(t, model.workspace.CaptureID())
}

func TestSidebarFooterUsesShortcutVocabulary(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.preferences.baseline.KeyProfile = chrome.ProfileMac
	model.bindings.SetProfile(chrome.ProfileMac)
	model.syncSidebarShortcut()
	require.Contains(t, model.sidebar.declaration.Footer, "cmd+b")
	require.NotContains(t, model.sidebar.declaration.Footer, "super+b")

	model.preferences.baseline.KeyProfile = chrome.ProfileStandard
	model.bindings.SetProfile(chrome.ProfileStandard)
	model.syncSidebarShortcut()
	require.Contains(t, model.sidebar.declaration.Footer, "ctrl+b")
}

func sidebarKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'b', Mod: tea.ModCtrl})
}

func sidebarLabels(model *Model) []string {
	labels := make([]string, len(model.sidebar.declaration.Items))
	for i, item := range model.sidebar.declaration.Items {
		labels[i] = item.Label
	}
	return labels
}

func focusSidebarItem(t testing.TB, model *Model, id chrome.FocusID) {
	t.Helper()
	model.sidebar.show()
	for range len(model.sidebar.declaration.Items) {
		_, focused := model.sidebar.focus.Current()
		if focused == id {
			return
		}
		model.sidebar.moveFocus(1)
	}
	t.Fatalf("sidebar item %q not found", id)
}

func advanceSidebar(t testing.TB, model *Model) {
	t.Helper()
	for model.workspace.SurfaceMoving(surfaceSidebar) {
		updateModelCommand(t, model, sidebarMotionMessage(model))
	}
}

func sidebarMotionMessage(model *Model) sidebarMotionMsg {
	return sidebarMotionMsg{
		generation: model.sidebar.generation,
		delta:      sidebarMotionInterval,
	}
}

func advanceSidebarUntilExtentChanges(
	t testing.TB,
	model *Model,
	previous int,
) int {
	t.Helper()
	for range 120 {
		updateModelCommand(t, model, sidebarMotionMessage(model))
		if extent := model.workspace.SurfacePosition(surfaceSidebar); extent != previous {
			return extent
		}
	}
	t.Fatal("sidebar extent did not change")
	return previous
}
