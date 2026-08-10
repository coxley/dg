package tui

import (
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestArrangeFormFitsLongestFocusedSelector(t *testing.T) {
	t.Parallel()

	theme := DefaultTheme(true)
	form := newArrangeForm(theme.Navigation.Container, theme.Preferences.Form)
	form.Update(keyPress(tea.KeyDown, ""))
	form.Update(keyPress(tea.KeyDown, ""))
	form.Update(keyPress(tea.KeyRight, ""))

	line := strings.Split(ansi.Strip(form.form.View().Content), "\n")[2]
	require.Equal(t, arrangeFormWidth, ansi.StringWidth(line))
	require.True(t, strings.HasSuffix(line, "❮ Horizontal ❯ "), line)
	spacingLine := strings.Split(ansi.Strip(form.form.View().Content), "\n")[3]
	require.Equal(t, strings.Index(line, "❮"), strings.Index(spacingLine, "  Even"))
}

func TestArrangeSpacingScaleReplacesRoundedEvenValue(t *testing.T) {
	t.Parallel()

	positive := arrangeSpacingScale{active: true, approximate: 4}
	require.Equal(t, "3", positive.numberText(3))
	require.Equal(t, "Even (~4)", positive.numberText(4))
	require.Equal(t, "5", positive.numberText(5))
	require.Equal(t, arrangeSpacing{cells: 3, fixed: true}, positive.spacing(3))
	require.Equal(t, arrangeSpacing{}, positive.spacing(4))

	negative := arrangeSpacingScale{active: true, approximate: -4}
	require.Equal(t, "Even (~-4)", negative.numberText(0))
	require.Equal(t, "0", negative.numberText(1))
	require.Equal(t, uint64(1), negative.number(arrangeSpacing{fixed: true}))
	require.Equal(t, arrangeSpacing{fixed: true}, negative.spacing(1))
}

func TestArrangeEvenSpacingRoundsToNearestCell(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		items  []arrangeItem
		wanted int64
	}{
		{
			name:   "round 4.25 down",
			items:  horizontalArrangeItems(1, 0, 2, 4, 6, 17),
			wanted: 4,
		},
		{
			name:   "round 4.75 up",
			items:  horizontalArrangeItems(1, 0, 2, 4, 6, 19),
			wanted: 5,
		},
		{
			name:   "round negative overlap",
			items:  horizontalArrangeItems(5, 0, 1, 2),
			wanted: -3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.wanted, arrangeEvenSpacing(tt.items, true))
		})
	}
}

func TestArrangeDistributionPinsExtremesAndSharesIntegerRemainder(t *testing.T) {
	t.Parallel()

	items := []arrangeItem{
		{hit: layout.Hit{ID: 0}, bounds: layout.Rect{Min: layout.NewPoint(0, 0), Size: layout.Size{Width: 2, Height: 1}}},
		{hit: layout.Hit{ID: 1}, bounds: layout.Rect{Min: layout.NewPoint(5, 0), Size: layout.Size{Width: 2, Height: 1}}},
		{hit: layout.Hit{ID: 2}, bounds: layout.Rect{Min: layout.NewPoint(13, 0), Size: layout.Size{Width: 2, Height: 1}}},
	}
	deltas, err := arrangeDeltas(items, arrangeSettings{
		horizontal: arrangeAlignLeft,
		distribute: arrangeDistributeHorizontal,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), deltas[0][0])
	require.Equal(t, int64(1), deltas[1][0])
	require.Equal(t, int64(0), deltas[2][0])
}

func TestArrangeDistributionRequiresThreeItems(t *testing.T) {
	t.Parallel()

	items := []arrangeItem{
		{bounds: layout.Rect{Min: layout.NewPoint(0, 0), Size: layout.Size{Width: 2, Height: 1}}},
		{bounds: layout.Rect{Min: layout.NewPoint(5, 0), Size: layout.Size{Width: 2, Height: 1}}},
	}
	_, err := arrangeDeltas(items, arrangeSettings{distribute: arrangeDistributeHorizontal})
	require.EqualError(t, err, "distribution requires at least three selected items")
}

func TestArrangeFixedDistributionKeepsFirstItemAndUsesExactGaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		distribute chrome.ID
		spacing    uint64
		items      []arrangeItem
		wanted     [][2]int64
	}{
		{
			name:       "zero shares horizontal borders",
			distribute: arrangeDistributeHorizontal,
			items: []arrangeItem{
				{bounds: layout.Rect{Min: layout.NewPoint(10, 4), Size: layout.Size{Width: 3, Height: 1}}},
				{bounds: layout.Rect{Min: layout.NewPoint(20, 4), Size: layout.Size{Width: 2, Height: 1}}},
				{bounds: layout.Rect{Min: layout.NewPoint(30, 4), Size: layout.Size{Width: 4, Height: 1}}},
			},
			wanted: [][2]int64{{0, 0}, {-8, 0}, {-17, 0}},
		},
		{
			name:       "horizontal",
			distribute: arrangeDistributeHorizontal,
			spacing:    2,
			items: []arrangeItem{
				{bounds: layout.Rect{Min: layout.NewPoint(10, 4), Size: layout.Size{Width: 3, Height: 1}}},
				{bounds: layout.Rect{Min: layout.NewPoint(20, 4), Size: layout.Size{Width: 2, Height: 1}}},
				{bounds: layout.Rect{Min: layout.NewPoint(30, 4), Size: layout.Size{Width: 4, Height: 1}}},
			},
			wanted: [][2]int64{{0, 0}, {-6, 0}, {-13, 0}},
		},
		{
			name:       "vertical",
			distribute: arrangeDistributeVertical,
			spacing:    2,
			items: []arrangeItem{
				{bounds: layout.Rect{Min: layout.NewPoint(4, 10), Size: layout.Size{Width: 1, Height: 3}}},
				{bounds: layout.Rect{Min: layout.NewPoint(4, 20), Size: layout.Size{Width: 1, Height: 2}}},
				{bounds: layout.Rect{Min: layout.NewPoint(4, 30), Size: layout.Size{Width: 1, Height: 4}}},
			},
			wanted: [][2]int64{{0, 0}, {0, -6}, {0, -13}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deltas, err := arrangeDeltas(tt.items, arrangeSettings{
				distribute: tt.distribute,
				spacing:    arrangeSpacing{cells: tt.spacing, fixed: true},
			})
			require.NoError(t, err)
			require.Equal(t, tt.wanted, deltas)
		})
	}
}

func TestArrangePreviewMovesSelectedGroupRigidlyAndCommitsOnEnter(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	left, err := geo.NewNodeAt("group-a", layout.NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("group-b", layout.NewPoint(12, 4))
	require.NoError(t, err)
	outside, err := geo.NewNodeAt("outside", layout.NewPoint(30, 15))
	require.NoError(t, err)
	groupID, err := geo.NewGroup([]ir.Member{
		{ID: left, Kind: ir.MemberNode},
		{ID: right, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	model, err := New(geo, testModelSettings())
	require.NoError(t, err)
	model.geo.Selection().SelectOnly(layout.Hit{ID: groupID, Kind: layout.HitGroup})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: outside, Kind: layout.HitNode}))
	beforeLeft := model.geo.Nodes[left].Rect.Min
	beforeRight := model.geo.Nodes[right].Rect.Min

	model.toggleArrange()
	updateModel(t, model, keyPress(tea.KeyDown, ""))
	for range 3 {
		updateArrangeField(t, model, keyPress(tea.KeyRight, ""))
	}
	require.True(t, model.arrangeOpen)
	require.Equal(
		t,
		int64(beforeRight.Y)-int64(beforeLeft.Y),
		int64(model.geo.Nodes[right].Rect.Min.Y)-int64(model.geo.Nodes[left].Rect.Min.Y),
	)
	require.Equal(t, model.geo.Nodes[outside].Rect.Max().Y, model.geo.Nodes[right].Rect.Max().Y)

	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.False(t, model.arrangeOpen)
	model.undo()
	require.Equal(t, beforeLeft, model.geo.Nodes[left].Rect.Min)
	require.Equal(t, beforeRight, model.geo.Nodes[right].Rect.Min)
}

func TestArrangeExactSpacingPreviewsFromFirstItem(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	first, err := geo.NewNodeAt("first", layout.NewPoint(2, 2))
	require.NoError(t, err)
	second, err := geo.NewNodeAt("second", layout.NewPoint(20, 2))
	require.NoError(t, err)
	third, err := geo.NewNodeAt("third", layout.NewPoint(40, 2))
	require.NoError(t, err)
	model, err := New(geo, testModelSettings())
	require.NoError(t, err)
	model.geo.Selection().SelectOnly(layout.Hit{ID: first, Kind: layout.HitNode})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: second, Kind: layout.HitNode}))
	require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: third, Kind: layout.HitNode}))
	firstOrigin := model.geo.Nodes[first].Rect.Min

	model.toggleArrange()
	updateModel(t, model, keyPress(tea.KeyDown, ""))
	updateModel(t, model, keyPress(tea.KeyDown, ""))
	updateArrangeField(t, model, keyPress(tea.KeyRight, ""))
	require.Positive(t, model.arrange.spacing.approximate)
	updateModel(t, model, keyPress(tea.KeyDown, ""))
	updateArrangeField(t, model, keyPress(tea.KeyRight, ""))

	spacing := model.arrange.Settings().spacing
	require.True(t, spacing.fixed)
	require.Equal(t, uint64(model.arrange.spacing.approximate)+1, spacing.cells)
	require.Equal(t, firstOrigin, model.geo.Nodes[first].Rect.Min)
	require.Equal(
		t,
		int64(spacing.cells),
		int64(model.geo.Nodes[second].Rect.Min.X)-int64(model.geo.Nodes[first].Rect.Max().X-1),
	)
	require.Equal(
		t,
		int64(spacing.cells),
		int64(model.geo.Nodes[third].Rect.Min.X)-int64(model.geo.Nodes[second].Rect.Max().X-1),
	)
}

func TestArrangeFloatingFormPreviewsAndOutsideClickCommits(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: right, Kind: layout.HitNode}))
	beforeLeft := model.geo.Nodes[left].Rect.Min
	beforeRight := model.geo.Nodes[right].Rect.Min
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'l', Mod: tea.ModShift}))
	require.True(t, model.arrangeOpen)
	require.Contains(t, model.arrange.form.View().Content, "Align (h)")
	require.Contains(t, model.arrange.form.View().Content, "Align (v)")
	require.Contains(t, model.arrange.form.View().Content, "Distribute")
	require.Contains(t, model.arrange.form.View().Content, "Spacing")
	require.NotContains(t, model.arrange.form.View().Content, ":")
	require.Contains(t, model.arrange.form.View().Content, "—")
	fields := model.arrange.form.Plan().Fields
	require.Equal(t, fields[0].ValueX, fields[1].ValueX)
	require.Equal(t, fields[0].ValueX, fields[2].ValueX)
	require.Equal(t, fields[0].ValueX, fields[3].ValueX)
	panel, ok := model.surfacePlan(surfaceArrange)
	require.True(t, ok)
	toolbar, ok := model.surfacePlan(surfaceNavigation)
	require.True(t, ok)
	require.True(
		t,
		panel.Rect.Right() <= toolbar.Rect.X || panel.Rect.X >= toolbar.Rect.Right() ||
			panel.Rect.Bottom() <= toolbar.Rect.Y || panel.Rect.Y >= toolbar.Rect.Bottom(),
		"arrange panel %v overlaps toolbar %v",
		panel.Rect,
		toolbar.Rect,
	)

	updateArrangeField(t, model, keyPress(tea.KeyRight, ""))
	require.Equal(t, model.geo.Nodes[left].Rect.Min.X, model.geo.Nodes[right].Rect.Min.X)
	require.True(t, model.arrangeOpen)

	updateModel(t, model, tea.MouseClickMsg{X: 79, Y: 14, Button: tea.MouseLeft})
	require.False(t, model.arrangeOpen)
	require.Equal(t, model.geo.Nodes[left].Rect.Min.X, model.geo.Nodes[right].Rect.Min.X)
	require.NotEqual(t, beforeRight, model.geo.Nodes[right].Rect.Min)
	require.True(t, model.geo.Selection().Empty())
	require.True(t, model.history.CanUndo())
	model.undo()
	require.Equal(t, beforeLeft, model.geo.Nodes[left].Rect.Min)
	require.Equal(t, beforeRight, model.geo.Nodes[right].Rect.Min)
}

func TestArrangeEnterClosesFormAndRetainsPreview(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: right, Kind: layout.HitNode}))
	beforeRight := model.geo.Nodes[right].Rect.Min

	model.toggleArrange()
	updateArrangeField(t, model, keyPress(tea.KeyRight, ""))
	updateModel(t, model, keyPress(tea.KeyEnter, ""))
	require.False(t, model.arrangeOpen)
	require.Equal(t, model.geo.Nodes[left].Rect.Min.X, model.geo.Nodes[right].Rect.Min.X)
	require.True(t, model.history.CanUndo())

	model.undo()
	require.Equal(t, beforeRight, model.geo.Nodes[right].Rect.Min)
}

func TestArrangeFocusLossRestoresPreview(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: right, Kind: layout.HitNode}))
	beforeRight := model.geo.Nodes[right].Rect.Min

	model.toggleArrange()
	updateArrangeField(t, model, keyPress(tea.KeyRight, ""))
	require.NotEqual(t, beforeRight, model.geo.Nodes[right].Rect.Min)
	updateModel(t, model, tea.BlurMsg{})

	require.False(t, model.arrangeOpen)
	require.Equal(t, beforeRight, model.geo.Nodes[right].Rect.Min)
	require.False(t, model.history.CanUndo())
}

func TestArrangeShortcutCommitsPreview(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: right, Kind: layout.HitNode}))
	beforeRight := model.geo.Nodes[right].Rect.Min

	model.toggleArrange()
	updateArrangeField(t, model, keyPress(tea.KeyRight, ""))
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'l', Mod: tea.ModShift}))

	require.False(t, model.arrangeOpen)
	require.Equal(t, model.geo.Nodes[left].Rect.Min.X, model.geo.Nodes[right].Rect.Min.X)
	model.undo()
	require.Equal(t, beforeRight, model.geo.Nodes[right].Rect.Min)
}

func TestPlaceArrangeFormAvoidsNavigation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		form       chrome.Rect
		navigation chrome.Rect
		workspace  chrome.Rect
		want       chrome.Rect
	}{
		{
			name:       "place on right",
			form:       chrome.Rect{Width: 30, Height: 5},
			navigation: chrome.Rect{X: 35, Y: 1, Width: 29, Height: 3},
			workspace:  chrome.Rect{Width: 100, Height: 15},
			want:       chrome.Rect{X: 65, Y: 1, Width: 30, Height: 5},
		},
		{
			name:       "flip to left",
			form:       chrome.Rect{Width: 30, Height: 5},
			navigation: chrome.Rect{X: 40, Y: 1, Width: 29, Height: 3},
			workspace:  chrome.Rect{Width: 80, Height: 15},
			want:       chrome.Rect{X: 9, Y: 1, Width: 30, Height: 5},
		},
		{
			name:       "below when neither side fits",
			form:       chrome.Rect{Width: 30, Height: 5},
			navigation: chrome.Rect{X: 25, Y: 1, Width: 29, Height: 3},
			workspace:  chrome.Rect{Width: 80, Height: 15},
			want:       chrome.Rect{X: 50, Y: 5, Width: 30, Height: 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, placeArrangeForm(tt.form, tt.navigation, tt.workspace))
		})
	}
}

func TestArrangeSelectorHighlightExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model, left, right := newTwoNodeModel(t)
		model.geo.Selection().SelectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
		require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: right, Kind: layout.HitNode}))
		model.toggleArrange()

		command := updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
		require.NotNil(t, command)
		require.Equal(t, 1, model.arrange.form.Flash(arrangeHorizontalField))

		updateModel(t, model, command())
		require.Zero(t, model.arrange.form.Flash(arrangeHorizontalField))
	})
}

func TestArrangeSpacingWithoutDistributionHighlightsDependency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model, left, right := newTwoNodeModel(t)
		model.geo.Selection().SelectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
		require.True(t, model.geo.Selection().Toggle(layout.Hit{ID: right, Kind: layout.HitNode}))
		model.toggleArrange()
		for range 3 {
			updateModel(t, model, keyPress(tea.KeyDown, ""))
		}
		require.Equal(t, arrangeSpacingField, model.arrange.form.FocusID())

		command := updateModelCommand(t, model, keyPress(tea.KeyRight, ""))
		require.NotNil(t, command)
		require.Equal(t, arrangeSpacingField, model.arrange.form.FocusID())
		require.True(t, model.arrange.form.Highlighted(arrangeDistributeField))
		require.Empty(t, model.arrange.Settings().distribute)
		require.Empty(t, model.statusError)

		updateModel(t, model, command())
		require.False(t, model.arrange.form.Highlighted(arrangeDistributeField))
		require.Equal(t, arrangeSpacingField, model.arrange.form.FocusID())
	})
}

func horizontalArrangeItems(width uint32, positions ...uint32) []arrangeItem {
	items := make([]arrangeItem, len(positions))
	for i, position := range positions {
		items[i] = arrangeItem{
			hit: layout.Hit{ID: uint32(i)},
			bounds: layout.Rect{
				Min:  layout.NewPoint(position, 0),
				Size: layout.Size{Width: width, Height: 1},
			},
		}
	}
	return items
}

func updateArrangeField(t testing.TB, model *Model, message tea.KeyPressMsg) {
	t.Helper()

	require.NotNil(t, updateModelCommand(t, model, message))
}
