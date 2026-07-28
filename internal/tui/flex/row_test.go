package flex

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestRowGrowsAndAlignsItems(t *testing.T) {
	t.Parallel()

	row := Row(
		16,
		Item{Content: "Title"},
		Item{Content: "value", Grow: 1, Align: lipgloss.Right},
	)

	require.Equal(t, "Title      value", row)
}

func TestRowShrinksEligibleItems(t *testing.T) {
	t.Parallel()

	row := Row(
		10,
		Item{Content: "Long title", Shrink: 1},
		Item{Content: "value"},
	)

	require.Equal(t, "Long value", row)
}

func TestRowDistributesRemainderFromLeftToRight(t *testing.T) {
	t.Parallel()

	row := Row(
		5,
		Item{Content: "a", Grow: 1, Align: lipgloss.Right},
		Item{Content: "b", Grow: 1, Align: lipgloss.Right},
	)

	require.Equal(t, "  a b", row)
}

func TestRowMeasuresStyledContentByDisplayWidth(t *testing.T) {
	t.Parallel()

	left := lipgloss.NewStyle().Bold(true).Render("名")
	right := lipgloss.NewStyle().Underline(true).Render("value")
	row := Row(
		12,
		Item{Content: left, Shrink: 1},
		Item{Content: right, Grow: 1, Align: lipgloss.Right},
	)

	require.Equal(t, 12, ansi.StringWidth(row))
	require.Equal(t, "名     value", ansi.Strip(row))
}

func TestRowFitsArbitraryWidths(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		width := rapid.IntRange(1, 128).Draw(t, "width")
		content := rapid.SliceOfN(
			rapid.StringMatching(`[a-z]{0,16}`),
			1,
			8,
		).Draw(t, "content")
		items := make([]Item, len(content))
		for i := range content {
			items[i] = Item{
				Content: content[i],
				Grow:    1,
				Shrink:  1,
				Align:   lipgloss.Position(float64(i%3) / 2),
			}
		}

		row := Row(width, items...)

		require.NotContains(t, row, "\n")
		require.Equal(t, width, ansi.StringWidth(row))
	})
}
