package chrome

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

const (
	viewportOne   = "one"
	viewportTwo   = "two"
	viewportThree = "three"
	viewportFour  = "four"
	viewportFive  = "five"
	viewportLast  = "last"
	viewportWide  = "01234567890123456789"
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
				viewportFour,
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
		viewportFour,
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

func TestViewportScrollbarsSupportClickAndDrag(t *testing.T) {
	t.Parallel()

	viewport := NewViewport("body")
	viewport.SetContent([]string{
		viewportWide,
		viewportOne, viewportTwo, viewportThree, viewportFour,
		viewportFive, "six", "seven", "eight", "nine",
	})
	viewport.SetOverflow(ScrollText)
	viewport.SetBounds(Rect{X: 3, Y: 4, Width: 8, Height: 5})
	plan := viewport.Plan()

	require.True(t, viewport.BeginScrollbarDrag(Point{
		X: plan.VerticalBar.X,
		Y: plan.VerticalBar.Bottom() - 1,
	}))
	require.True(t, viewport.ScrollbarDragging())
	require.Equal(t, plan.Extent.Height-plan.Content.Height, viewport.Plan().Offset.Y)
	viewport.EndScrollbarDrag()

	viewport.Scroll(-100, -100)
	plan = viewport.Plan()
	require.True(t, viewport.BeginScrollbarDrag(Point{
		X: plan.HorizontalThumb.X,
		Y: plan.HorizontalThumb.Y,
	}))
	require.True(t, viewport.DragScrollbar(Point{
		X: plan.HorizontalBar.Right() - 1,
		Y: plan.HorizontalBar.Y,
	}))
	require.Equal(t, plan.Extent.Width-plan.Content.Width, viewport.Plan().Offset.X)
	viewport.EndScrollbarDrag()
	require.False(t, viewport.ScrollbarDragging())
}

func TestViewportRendersThemedScrollbarCells(t *testing.T) {
	t.Parallel()

	styles := ScrollbarStyles{
		Track: lipgloss.NewStyle().Foreground(lipgloss.Color("#123456")),
		Thumb: lipgloss.NewStyle().Foreground(lipgloss.Color("#abcdef")),
	}
	viewport := NewViewport("body")
	viewport.SetContent([]string{
		viewportWide,
		viewportOne, viewportTwo, viewportThree, viewportFour, viewportFive,
	})
	viewport.SetOverflow(ScrollText)
	viewport.SetScrollbarStyles(styles)
	viewport.SetBounds(Rect{Width: 8, Height: 4})

	rendered := strings.Join(viewport.Lines(), "\n")
	require.Contains(t, rendered, styles.Track.Render("│"))
	require.Contains(t, rendered, styles.Track.Render("─"))
	require.Contains(t, rendered, styles.Thumb.Render("█"))
}

func TestViewportRendersScrollbarInteractionStates(t *testing.T) {
	t.Parallel()

	styles := ScrollbarStyles{
		Track:        lipgloss.NewStyle(),
		Thumb:        lipgloss.NewStyle(),
		HoveredTrack: lipgloss.NewStyle().Foreground(lipgloss.Color("#111111")),
		HoveredThumb: lipgloss.NewStyle().Foreground(lipgloss.Color("#222222")),
		FocusedTrack: lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")),
		FocusedThumb: lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")),
		ActiveTrack:  lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")),
		ActiveThumb:  lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")),
	}
	viewport := NewViewport("body")
	viewport.SetContent([]string{
		viewportOne, viewportTwo, viewportThree, viewportFour, viewportFive,
	})
	viewport.SetScrollbarStyles(styles)
	viewport.SetBounds(Rect{Width: 8, Height: 3})

	viewport.SetFocused(true)
	rendered := strings.Join(viewport.Lines(), "\n")
	require.Contains(t, rendered, styles.FocusedTrack.Render("│"))
	require.Contains(t, rendered, styles.FocusedThumb.Render("█"))

	thumb := viewport.Plan().VerticalThumb
	point := Point{X: thumb.X, Y: thumb.Y}
	require.True(t, viewport.HoverScrollbar(point))
	rendered = strings.Join(viewport.Lines(), "\n")
	require.Contains(t, rendered, styles.HoveredTrack.Render("│"))
	require.Contains(t, rendered, styles.HoveredThumb.Render("█"))

	require.True(t, viewport.BeginScrollbarDrag(point))
	rendered = strings.Join(viewport.Lines(), "\n")
	require.Contains(t, rendered, styles.ActiveTrack.Render("│"))
	require.Contains(t, rendered, styles.ActiveThumb.Render("█"))
}

func BenchmarkViewportArrange(b *testing.B) {
	viewport := NewViewport("body")
	viewport.SetContent([]string{
		viewportOne + " " + viewportTwo + " " + viewportThree + " four five",
		viewportWide,
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
