package chrome

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func TestMenuUsesOnePlanForRenderAndInput(t *testing.T) {
	t.Parallel()

	menu := NewMenu("menu", lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1), []MenuItem{
		{ID: "cursor", Label: " Cursor "},
		{ID: "rectangle", Label: " Rectangle "},
	})
	menu.SetViewport(60, 1)
	before, err := menu.Plan()
	require.NoError(t, err)
	rect, ok := menu.ItemRect("rectangle")
	require.True(t, ok)
	item, ok := menu.ItemAt(Point{X: rect.X, Y: rect.Y})
	require.True(t, ok)
	require.Equal(t, ID("rectangle"), item.ID)
	require.Len(t, menu.Lines(func(item MenuItem) string {
		return item.Label
	}), before.Bounds.Height)

	menu.SetViewport(40, 2)
	after, err := menu.Plan()
	require.NoError(t, err)
	require.Greater(t, after.Version, before.Version)
	require.NotEqual(t, before.Bounds, after.Bounds)
	rect, ok = menu.ItemRect("rectangle")
	require.True(t, ok)
	item, ok = menu.ItemAt(Point{X: rect.X, Y: rect.Y})
	require.True(t, ok)
	require.Equal(t, ID("rectangle"), item.ID)
}

func TestMenuHidesWhenMinimumCannotFit(t *testing.T) {
	t.Parallel()

	menu := NewMenu("menu", lipgloss.NewStyle(), []MenuItem{
		{ID: "wide", Label: "界"},
	})
	menu.SetViewport(1, 1)
	require.Equal(t, Rect{}, menu.Bounds())
	require.False(t, menu.Contains(Point{}))
	require.Empty(t, menu.Lines(func(item MenuItem) string {
		return item.Label
	}))
}

func BenchmarkMenuRender(b *testing.B) {
	menu := NewMenu("menu", lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1), []MenuItem{
		{ID: "cursor", Label: " Cursor "},
		{ID: "rectangle", Label: " Rectangle "},
		{ID: "line", Label: " Line "},
	})
	menu.SetViewport(80, 1)
	b.ReportAllocs()
	for b.Loop() {
		benchmarkLines = menu.Lines(func(item MenuItem) string {
			return item.Label
		})
	}
}

var benchmarkLines []string
