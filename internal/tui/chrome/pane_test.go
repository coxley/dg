package chrome

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestPaneKeepsHeaderAndFooterSticky(t *testing.T) {
	t.Parallel()

	viewport := NewViewport("body")
	viewport.SetContent([]string{"zero", viewportOne, viewportTwo, viewportThree})
	pane := NewPane("pane", viewport)
	pane.SetHeader([]string{"header"})
	pane.SetFooter([]string{"footer"})
	pane.SetBounds(Rect{Width: 8, Height: 4})
	viewport.Scroll(0, 2)

	require.Equal(t, []string{
		"header  ",
		"two    │",
		"three  █",
		"footer  ",
	}, pane.Lines())
	plan := pane.Plan()
	require.Equal(t, Rect{Width: 8, Height: 1}, plan.Header)
	require.Equal(t, Rect{Y: 1, Width: 8, Height: 2}, plan.Body)
	require.Equal(t, Rect{Y: 3, Width: 8, Height: 1}, plan.Footer)
}

func TestNestedPanesRetainIndependentViewports(t *testing.T) {
	t.Parallel()

	outerViewport := NewViewport("outer-body")
	outerViewport.SetContent([]string{"outer zero", "outer one", "outer two"})
	outer := NewPane("outer", outerViewport)
	outer.SetHeader([]string{"outer"})

	innerViewport := NewViewport("inner-body")
	innerViewport.SetContent([]string{"inner zero", "inner one", "inner two"})
	inner := NewPane("inner", innerViewport)
	inner.SetHeader([]string{"inner"})
	outer.SetNested(inner)
	outer.SetBounds(Rect{Width: 12, Height: 4})
	innerViewport.Scroll(0, 1)

	require.Equal(t, 0, outerViewport.Plan().Offset.Y)
	require.Equal(t, 1, innerViewport.Plan().Offset.Y)
	require.Equal(t, []string{
		"outer       ",
		"inner       ",
		"inner one  │",
		"inner two  █",
	}, outer.Lines())
}

func TestPaneStyleFramesEverySlot(t *testing.T) {
	t.Parallel()

	viewport := NewViewport("body")
	viewport.SetContent([]string{"body"})
	pane := NewPane("pane", viewport)
	pane.SetHeader([]string{"header"})
	pane.SetFooter([]string{"footer"})
	pane.SetStyle(lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, true, false, false).
		PaddingLeft(1).
		MarginRight(1))
	pane.SetBounds(Rect{Width: 8, Height: 4})

	require.Equal(t, []string{
		" heade│ ",
		" body │ ",
		"      │ ",
		" foote│ ",
	}, stripLines(pane.Lines()))
	plan := pane.Plan()
	require.Equal(t, Rect{X: 1, Width: 5, Height: 4}, plan.Content)
	require.Equal(t, Rect{X: 1, Width: 5, Height: 1}, plan.Header)
	require.Equal(t, Rect{X: 1, Y: 1, Width: 5, Height: 2}, plan.Body)
	require.Equal(t, Rect{X: 1, Y: 3, Width: 5, Height: 1}, plan.Footer)
	require.Equal(t, Rect{X: 1, Y: 1, Width: 5, Height: 2}, viewport.Plan().Bounds)
}

func stripLines(lines []string) []string {
	for i := range lines {
		lines[i] = ansi.Strip(lines[i])
	}
	return lines
}
