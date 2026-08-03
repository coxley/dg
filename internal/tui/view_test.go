package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

var benchmarkBackground string

func TestBackgroundRendererFillsTransparentCellsAndPreservesBackgrounds(t *testing.T) {
	t.Parallel()

	red := lipgloss.Color("#ff0000")
	blue := lipgloss.Color("#0000ff")
	renderer := backgroundRenderer{}
	painted := renderer.render(
		lipgloss.NewStyle().Background(red).Render("x"),
		2,
		1,
		lipgloss.NewStyle().Background(blue),
	)
	canvas := lipgloss.NewCanvas(2, 1)
	canvas.Compose(lipgloss.NewLayer(painted))

	require.Equal(t, rgba(red), rgba(canvas.CellAt(0, 0).Style.Bg))
	require.Equal(t, rgba(blue), rgba(canvas.CellAt(1, 0).Style.Bg))
	retained := renderer.canvas
	renderer.render("", 3, 2, lipgloss.NewStyle().Background(blue))
	require.Same(t, retained, renderer.canvas)
}

func BenchmarkBackgroundRenderer80x24(b *testing.B) {
	content := strings.Repeat(strings.Repeat("x", 80)+"\n", 23) + strings.Repeat("x", 80)
	style := lipgloss.NewStyle().Background(lipgloss.Color("#101010"))
	renderer := backgroundRenderer{}
	b.ReportAllocs()
	for b.Loop() {
		benchmarkBackground = renderer.render(content, 80, 24, style)
	}
}
