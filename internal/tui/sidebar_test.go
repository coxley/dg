package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		"Candidate 1",
		"Candidate 2",
		"▾ [RFCs]",
		"Proposal.bak [backup]",
	}, labels)
	wantWidth := model.sidebar.desired
	focusSidebarItem(t, model, "section:Interviews")
	model.activateSidebar()
	require.NotContains(t, sidebarLabels(model), "Candidate 1")
	require.Equal(t, wantWidth, model.sidebar.desired)

	model.switchSidebarTab(1)
	require.True(t, model.sidebar.drafts)
	require.Equal(t, wantWidth, model.sidebar.desired)
	require.Len(t, model.sidebar.declaration.Items, 3)
	for _, item := range model.sidebar.declaration.Items[:2] {
		require.NotContains(t, item.Label, "draft")
		require.NotEmpty(t, item.Label)
	}
	require.Equal(t, "[Clear Drafts]", model.sidebar.declaration.Items[2].Label)
}

func TestSidebarPrefixesFocusAndKeepsActiveCanvasVisible(t *testing.T) {
	t.Parallel()

	active := canvasstore.Entry{ID: uuid.New(), Name: "Active"}
	other := canvasstore.Entry{ID: uuid.New(), Name: "Other"}
	styles := sidebarStyles{
		Item:        lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		FocusedItem: lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		ActiveItem:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
	}
	sidebar := newSidebar(sidebarDeclaration{Items: []sidebarItem{
		canvasSidebarItem(active),
		canvasSidebarItem(other),
	}}, styles)
	sidebar.setBounds(chrome.Rect{Width: 30, Height: 6})
	sidebar.setActive(&active)
	sidebar.show()
	require.True(t, sidebar.focusTarget(canvasSidebarItem(other).ID))
	sidebar.render()

	rendered := strings.Join(sidebar.viewport.Lines(), "\n")
	require.Contains(t, rendered, styles.ActiveItem.Render("  Active"))
	require.Contains(t, rendered, styles.FocusedItem.Render("▸ Other"))

	require.True(t, sidebar.focusTarget(canvasSidebarItem(active).ID))
	sidebar.render()
	rendered = strings.Join(sidebar.viewport.Lines(), "\n")
	require.Contains(t, rendered, styles.ActiveItem.Render("▸ Active"))
}

func TestSidebarSectionsUseDisclosureMarkerAndIndentChildren(t *testing.T) {
	t.Parallel()

	child := canvasstore.Entry{ID: uuid.New(), Section: "Sub", Name: "Child"}
	sidebar := newSidebar(sidebarDeclaration{Items: []sidebarItem{
		{ID: "section:Sub", Label: "▾ [Sub]", Kind: sidebarItemSection},
		canvasSidebarItem(child),
	}}, sidebarStyles{})
	sidebar.setBounds(chrome.Rect{Width: 30, Height: 6})
	sidebar.show()
	require.True(t, sidebar.focusTarget("section:Sub"))
	sidebar.render()

	rendered := ansi.Strip(strings.Join(sidebar.viewport.Lines(), "\n"))
	require.Contains(t, rendered, "  ▾ [Sub]")
	require.NotContains(t, rendered, "▸ ▾ [Sub]")

	require.True(t, sidebar.focusTarget(canvasSidebarItem(child).ID))
	sidebar.render()
	rendered = ansi.Strip(strings.Join(sidebar.viewport.Lines(), "\n"))
	require.Contains(t, rendered, "  ▸ Child")
}

func TestSidebarClearDraftsStyleAddsSeparateRow(t *testing.T) {
	t.Parallel()

	styles := sidebarStyles{
		ClearDrafts: lipgloss.NewStyle().MarginTop(1),
	}
	sidebar := newSidebar(sidebarDeclaration{Items: []sidebarItem{
		{ID: "draft", Label: "Jul 31, 12:00", Kind: sidebarItemRecord},
		{ID: "drafts:clear", Label: "[Clear Drafts]", Kind: sidebarItemClearDrafts},
	}}, styles)
	sidebar.setBounds(chrome.Rect{Width: 30, Height: 7})

	require.Equal(t, 3, sidebar.viewport.Plan().Extent.Height)
	require.Equal(t, "", strings.TrimSpace(ansi.Strip(sidebar.viewport.Lines()[1])))
	require.Equal(t, "[Clear Drafts]", strings.TrimSpace(ansi.Strip(sidebar.viewport.Lines()[2])))
	_, ok := sidebar.itemAt(1)
	require.False(t, ok)
	index, ok := sidebar.itemAt(2)
	require.True(t, ok)
	require.Equal(t, 1, index)
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

func TestSidebarStartsOpenAndClickFocusesIt(t *testing.T) {
	t.Parallel()

	geo := mustLayoutWithLabel(t, "node")
	model, err := New(geo)
	require.NoError(t, err)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})

	require.True(t, model.sidebar.open)
	require.False(t, model.sidebar.focused)
	require.Equal(t, sidebarPreferredWidth, model.workspace.SurfacePosition(surfaceSidebar))
	require.Equal(t, sidebarPreferredWidth, model.workspace.Geometry().Canvas.X)
	scope, _ := model.sidebar.focus.Current()
	require.Equal(t, scopeCanvas, scope)

	surface, ok := model.surfacePlan(surfaceSidebar)
	require.True(t, ok)
	updateModel(t, model, tea.MouseClickMsg{
		X:      surface.Rect.X + 1,
		Y:      surface.Rect.Y,
		Button: tea.MouseLeft,
	})
	require.True(t, model.sidebar.focused)
	scope, _ = model.sidebar.focus.Current()
	require.Equal(t, scopeSidebar, scope)
}

func TestSidebarKeyboardFocusesActiveCanvas(t *testing.T) {
	t.Parallel()

	t.Run("preserve canvases tab", func(t *testing.T) {
		t.Parallel()

		model, _, store := newStoredTestModel(t, "active")
		canvas, err := store.Create("", "Architecture", document.New(mustLayoutWithLabel(t, "named")))
		require.NoError(t, err)
		model.updateCatalog(store.Reconcile(model.catalog))
		model.sidebar.openInitially()

		updateModelCommand(t, model, sidebarKey())

		require.True(t, model.sidebar.focused)
		require.False(t, model.sidebar.drafts)
		item, ok := model.sidebar.focusedItem()
		require.True(t, ok)
		require.Equal(t, canvas.ID, item.Entry.ID)
	})

	t.Run("active draft on current tab", func(t *testing.T) {
		t.Parallel()

		model, _, _ := newStoredTestModel(t, "active")
		active := *model.entry
		model.sidebar.drafts = true
		model.rebuildSidebarCatalog()
		model.sidebar.openInitially()

		updateModelCommand(t, model, sidebarKey())

		require.True(t, model.sidebar.drafts)
		item, ok := model.sidebar.focusedItem()
		require.True(t, ok)
		require.Equal(t, active.ID, item.Entry.ID)
	})

	t.Run("closed canvas section", func(t *testing.T) {
		t.Parallel()

		model, _, store := newStoredTestModel(t, "active")
		active, err := store.Name(*model.entry, "Design", "Architecture")
		require.NoError(t, err)
		model.setActiveEntry(active)
		model.updateCatalog(store.Reconcile(model.catalog))
		model.sidebar.collapsed[active.Section] = true

		updateModelCommand(t, model, sidebarKey())

		require.True(t, model.sidebar.focused)
		require.False(t, model.sidebar.drafts)
		require.False(t, model.sidebar.collapsed[active.Section])
		item, ok := model.sidebar.focusedItem()
		require.True(t, ok)
		require.Equal(t, active.ID, item.Entry.ID)
	})
}

func TestSidebarVerticalFocusTreatsTabsAsOneRow(t *testing.T) {
	t.Parallel()

	sidebar := newSidebar(sidebarDeclaration{Items: []sidebarItem{
		{ID: "first", Label: "First", Kind: sidebarItemRecord},
		{ID: "last", Label: "Last", Kind: sidebarItemRecord},
	}}, sidebarStyles{})
	sidebar.setBounds(chrome.Rect{Width: 30, Height: 6})
	sidebar.show()
	sidebar.focusTab(false)

	sidebar.moveFocus(1)
	_, focused := sidebar.focus.Current()
	require.Equal(t, chrome.FocusID("first"), focused)

	sidebar.moveFocus(-1)
	_, focused = sidebar.focus.Current()
	require.Equal(t, sidebarCanvasesTab, focused)

	sidebar.moveFocus(-1)
	_, focused = sidebar.focus.Current()
	require.Equal(t, chrome.FocusID("last"), focused)

	sidebar.moveFocus(1)
	_, focused = sidebar.focus.Current()
	require.Equal(t, sidebarCanvasesTab, focused)

	sidebar.focusTab(true)
	sidebar.moveFocus(1)
	_, focused = sidebar.focus.Current()
	require.Equal(t, chrome.FocusID("first"), focused)
}

func TestSidebarTabsShareHeaderAndActivateOnClick(t *testing.T) {
	t.Parallel()

	model, _, _ := newStoredTestModel(t, "draft")
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 12})
	model.setMotionEnabled(false)
	updateModelCommand(t, model, sidebarKey())
	require.Len(t, model.sidebar.tabs, 2)
	canvases := model.sidebar.tabs[0]
	drafts := model.sidebar.tabs[1]
	require.Equal(t, canvases.Rect.Right(), drafts.Rect.X)
	require.LessOrEqual(
		t,
		max(canvases.Rect.Width, drafts.Rect.Width)-min(canvases.Rect.Width, drafts.Rect.Width),
		1,
	)

	clickSidebarTab(t, model, drafts)
	require.True(t, model.sidebar.drafts)
	focusedDrafts, ok := model.sidebar.focusedTab()
	require.True(t, ok)
	require.True(t, focusedDrafts)

	clickSidebarTab(t, model, canvases)
	require.False(t, model.sidebar.drafts)
	focusedDrafts, ok = model.sidebar.focusedTab()
	require.True(t, ok)
	require.False(t, focusedDrafts)
}

func TestSidebarUsesDeclaredHeaderTabAndSectionStyles(t *testing.T) {
	t.Parallel()

	styles := DefaultTheme(true).Sidebar
	styles.Header = lipgloss.NewStyle().Background(lipgloss.Color("4"))
	styles.Tab = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styles.FocusedTab = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styles.HoveredTab = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	styles.ActiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styles.Section = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styles.FocusedSection = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	sidebar := newSidebar(sidebarDeclaration{
		Items: []sidebarItem{{
			ID: "section:Design", Label: "▾ [Design]", Kind: sidebarItemSection,
		}},
	}, styles)
	sidebar.setBounds(chrome.Rect{Width: 30, Height: 6})
	sidebar.show()

	rendered := strings.Join(sidebar.pane.Lines(), "\n")
	require.Contains(t, rendered, "\x1b[44m")
	require.Contains(t, rendered, "\x1b[31m")
	require.Contains(t, rendered, "\x1b[33m")
	require.Contains(t, rendered, "\x1b[35m")

	drafts := sidebar.tabs[1].Rect
	sidebar.motion(chrome.Point{X: drafts.X, Y: drafts.Y}, chrome.SurfacePlan{})
	rendered = strings.Join(sidebar.pane.Lines(), "\n")
	require.Contains(t, rendered, "\x1b[37m")

	require.True(t, sidebar.focusTarget(sidebarDraftsTab))
	sidebar.render()
	rendered = strings.Join(sidebar.pane.Lines(), "\n")
	require.Contains(t, rendered, "\x1b[32m")

	require.True(t, sidebar.focusTarget("section:Design"))
	sidebar.render()
	rendered = strings.Join(sidebar.pane.Lines(), "\n")
	require.Contains(t, rendered, "\x1b[36m")
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

func TestSidebarDragMovesCanvasBetweenSectionsAndRoot(t *testing.T) {
	t.Parallel()

	model, nodeID, store := newStoredTestModel(t, "original")
	active, err := store.Name(*model.entry, "", "Architecture")
	require.NoError(t, err)
	model.setActiveEntry(active)
	for _, location := range [][2]string{
		{"Candidates", "Applicant"},
		{"Specifications", "Draft spec"},
	} {
		_, err := store.Create(
			location[0],
			location[1],
			document.New(mustLayoutWithLabel(t, location[1])),
		)
		require.NoError(t, err)
	}
	model.updateCatalog(store.Reconcile(model.catalog))
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "changed"))
	require.NoError(t, model.rebuild())
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	model.setMotionEnabled(false)
	updateModelCommand(t, model, sidebarKey())

	dragSidebarItem(t, model, "canvas:/Architecture", "canvas:Candidates/Applicant")
	require.Equal(t, "Candidates", model.entry.Section)
	require.Equal(t, "dg - Candidates/Architecture", model.windowTitle)
	require.Equal(t, "moved Architecture to Candidates", model.status)
	loaded, err := store.Load(*model.entry)
	require.NoError(t, err)
	require.Equal(t, "changed", loaded.Nodes[0].Label)

	dragSidebarItem(t, model, "canvas:Candidates/Architecture", "section:Specifications")
	require.Equal(t, "Specifications", model.entry.Section)
	require.Equal(t, "moved Architecture to Specifications", model.status)

	dragSidebarItemToHeader(t, model, "canvas:Specifications/Architecture")
	require.Empty(t, model.entry.Section)
	require.Equal(t, "dg - Architecture", model.windowTitle)
	require.Equal(t, "moved Architecture to Canvases", model.status)
}

func TestSidebarStationaryClickOpensCanvasOnRelease(t *testing.T) {
	t.Parallel()

	model, _, store := newStoredTestModel(t, "active")
	named, err := store.Create("", "Architecture", document.New(mustLayoutWithLabel(t, "named")))
	require.NoError(t, err)
	model.updateCatalog(store.Reconcile(model.catalog))
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	model.setMotionEnabled(false)
	updateModelCommand(t, model, sidebarKey())
	model.selectSidebarTab(false)
	point := sidebarItemPoint(t, model, "canvas:/Architecture")

	updateModel(t, model, tea.MouseClickMsg{
		X: point.X, Y: point.Y, Button: tea.MouseLeft,
	})
	require.NotEqual(t, named.ID, model.entry.ID)
	require.Equal(t, surfaceSidebar, model.workspace.CaptureID())
	updateModelCommand(t, model, tea.MouseReleaseMsg{
		X: point.X, Y: point.Y, Button: tea.MouseLeft,
	})

	require.Equal(t, named.ID, model.entry.ID)
	require.Empty(t, model.workspace.CaptureID())
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
		require.Equal(t, navigationRect, navigation.Rect)

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

func TestDockedSidebarKeepsNavigationScreenAnchoredAtMinimumWidth(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: compactWidthThreshold, Height: 16})
	navigation, ok := model.surfacePlan(surfaceNavigation)
	require.True(t, ok)
	want := navigation.Rect
	updateModelCommand(t, model, sidebarKey())

	for model.workspace.SurfaceMoving(surfaceSidebar) {
		updateModelCommand(t, model, sidebarMotionMessage(model))
		navigation, ok := model.surfacePlan(surfaceNavigation)
		require.True(t, ok)
		require.Equal(t, want, navigation.Rect)
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

	require.Contains(t, ansi.Strip(strings.Join(lines[:model.sidebar.pane.Plan().Header.Height], "\n")), "Canvases")
	require.Contains(t, ansi.Strip(strings.Join(lines[model.sidebar.pane.Plan().Header.Height:len(lines)-1], "\n")), "Overview")
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
	value := model.preferences.baseline
	setPreferenceMappings(&value, scopeGlobal, commandSidebar, "super+b")
	model.preferences.baseline = value
	model.bindings.SetBindings(bindingsFromValues(value.Keybinds))
	model.syncSidebarShortcut()
	displayed := chrome.DisplayChord("super+b", chrome.VocabularyForProfile(chrome.ProfileAuto))
	require.Contains(t, model.sidebar.declaration.Footer, displayed)

	setPreferenceMappings(&value, scopeGlobal, commandSidebar, "ctrl+b")
	model.preferences.baseline = value
	model.bindings.SetBindings(bindingsFromValues(value.Keybinds))
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

func clickSidebarTab(t *testing.T, model *Model, tab sidebarTabPlan) {
	t.Helper()

	surface, ok := model.surfacePlan(surfaceSidebar)
	require.True(t, ok)
	point := chrome.Point{
		X: surface.Content.X + tab.Rect.X + tab.Rect.Width/2,
		Y: surface.Content.Y + tab.Rect.Y,
	}
	updateModelCommand(t, model, tea.MouseClickMsg{
		X: point.X, Y: point.Y, Button: tea.MouseLeft,
	})
}

func dragSidebarItem(t *testing.T, model *Model, source, target chrome.FocusID) {
	t.Helper()

	start := sidebarItemPoint(t, model, source)
	end := sidebarItemPoint(t, model, target)
	updateModel(t, model, tea.MouseClickMsg{
		X: start.X, Y: start.Y, Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X: end.X, Y: end.Y, Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X: end.X, Y: end.Y, Button: tea.MouseLeft,
	})
}

func dragSidebarItemToHeader(t *testing.T, model *Model, source chrome.FocusID) {
	t.Helper()

	start := sidebarItemPoint(t, model, source)
	surface, ok := model.surfacePlan(surfaceSidebar)
	require.True(t, ok)
	header := model.sidebar.pane.Plan().Header
	end := chrome.Point{
		X: surface.Content.X + header.X + 1,
		Y: surface.Content.Y + header.Y,
	}
	updateModel(t, model, tea.MouseClickMsg{
		X: start.X, Y: start.Y, Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseMotionMsg{
		X: end.X, Y: end.Y, Button: tea.MouseLeft,
	})
	updateModel(t, model, tea.MouseReleaseMsg{
		X: end.X, Y: end.Y, Button: tea.MouseLeft,
	})
}

func sidebarItemPoint(t *testing.T, model *Model, id chrome.FocusID) chrome.Point {
	t.Helper()

	surface, ok := model.surfacePlan(surfaceSidebar)
	require.True(t, ok)
	index := slices.IndexFunc(model.sidebar.declaration.Items, func(item sidebarItem) bool {
		return item.ID == id
	})
	require.NotEqual(t, -1, index, id)
	body := model.sidebar.pane.Plan().Body
	planIndex := slices.IndexFunc(model.sidebar.itemPlans, func(plan sidebarItemPlan) bool {
		return plan.Index == index
	})
	require.NotEqual(t, -1, planIndex, id)
	plan := model.sidebar.itemPlans[planIndex]
	return chrome.Point{
		X: surface.Content.X + body.X + 1,
		Y: surface.Content.Y + body.Y + plan.Rect.Y - model.sidebar.viewport.Plan().Offset.Y,
	}
}

func focusSidebarItem(t testing.TB, model *Model, id chrome.FocusID) {
	t.Helper()
	model.sidebar.show()
	if model.sidebar.focusTarget(id) {
		model.sidebar.render()
		return
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
