package chrome

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

const (
	viewportOne   = "one"
	viewportTwo   = "two"
	viewportThree = "three"
	viewportLast  = "last"
)

func TestViewportScrollbarPolicyMatrix(t *testing.T) {
	t.Parallel()

	policies := []ScrollbarPolicy{
		ScrollbarNever,
		ScrollbarAutomatic,
		ScrollbarAlways,
	}
	for _, horizontal := range policies {
		for _, vertical := range policies {
			viewport := NewViewport("body")
			viewport.SetContent([]string{
				"0123456789",
				viewportOne,
				viewportTwo,
				viewportThree,
				"four",
			})
			viewport.SetOverflow(ScrollText)
			viewport.SetScrollbars(horizontal, vertical)
			viewport.SetBounds(Rect{Width: 5, Height: 3})
			plan := viewport.Plan()

			wantHorizontal := horizontal != ScrollbarNever
			wantVertical := vertical != ScrollbarNever
			require.Equal(t, wantHorizontal, plan.HorizontalBar.Height != 0)
			require.Equal(t, wantVertical, plan.VerticalBar.Width != 0)
			require.Len(t, viewport.Lines(), 3)
			for _, line := range viewport.Lines() {
				require.Equal(t, 5, ansi.StringWidth(line))
			}
		}
	}
}

func TestViewportConvergesAfterScrollbarReservationChangesWrapping(t *testing.T) {
	t.Parallel()

	viewport := NewViewport("body")
	viewport.SetContent([]string{"abcde"})
	viewport.SetOverflow(WrapText)
	viewport.SetScrollbars(ScrollbarNever, ScrollbarAlways)
	viewport.SetBounds(Rect{Width: 5, Height: 2})
	plan := viewport.Plan()

	require.Equal(t, Rect{Width: 4, Height: 2}, plan.Content)
	require.Equal(t, Size{Width: 4, Height: 2}, plan.Extent)
	require.NotEmpty(t, plan.VerticalBar)
	require.Len(t, viewport.Lines(), 2)
}

func TestViewportRevealAndPointerTranslation(t *testing.T) {
	t.Parallel()

	viewport := NewViewport("body")
	viewport.SetContent([]string{
		"0123456789",
		viewportOne,
		viewportTwo,
		viewportThree,
		"four",
	})
	viewport.SetOverflow(ScrollText)
	viewport.SetBounds(Rect{X: 10, Y: 4, Width: 6, Height: 4})
	viewport.Reveal(Rect{X: 8, Y: 4, Width: 2, Height: 1})
	plan := viewport.Plan()
	require.Equal(t, Point{X: 5, Y: 2}, plan.Offset)

	point, ok := viewport.ContentPoint(Point{
		X: plan.Content.X + 1,
		Y: plan.Content.Y + 1,
	})
	require.True(t, ok)
	require.Equal(t, Point{X: 6, Y: 3}, point)
	_, ok = viewport.ContentPoint(Point{
		X: plan.VerticalBar.X,
		Y: plan.VerticalBar.Y,
	})
	require.False(t, ok)
}

func TestViewportHandlesConstrainedBounds(t *testing.T) {
	t.Parallel()

	viewport := NewViewport("body")
	viewport.SetContent([]string{"content"})
	viewport.SetScrollbars(ScrollbarAlways, ScrollbarAlways)
	viewport.SetBounds(Rect{Width: 1, Height: 1})
	plan := viewport.Plan()
	require.Equal(t, Rect{Width: 0, Height: 0}, plan.Content)
	require.Len(t, viewport.Lines(), 1)
	require.Equal(t, 1, ansi.StringWidth(viewport.Lines()[0]))
}

func BenchmarkViewportArrange(b *testing.B) {
	viewport := NewViewport("body")
	viewport.SetContent([]string{
		viewportOne + " " + viewportTwo + " " + viewportThree + " four five",
		"01234567890123456789",
		viewportLast,
	})
	viewport.SetOverflow(WrapText)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		viewport.SetBounds(Rect{Width: 20 + i%2, Height: 5})
		benchmarkViewportPlan = viewport.Plan()
	}
}

var benchmarkViewportPlan ViewportPlan
