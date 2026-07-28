package chrome

import (
	"errors"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestMeasureTextUsesDisplayCells(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Metrics
	}{
		{
			name: "wide glyph",
			text: "A界B",
			want: Metrics{
				Min:       Size{Width: 2, Height: 1},
				Preferred: Size{Width: 4, Height: 1},
			},
		},
		{
			name: "combining mark",
			text: "e\u0301",
			want: Metrics{
				Min:       Size{Width: 1, Height: 1},
				Preferred: Size{Width: 1, Height: 1},
			},
		},
		{
			name: "styled multiline with empty line",
			text: "\x1b[31mred\x1b[0m\n\nx",
			want: Metrics{
				Min:       Size{Width: 1, Height: 3},
				Preferred: Size{Width: 3, Height: 3},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := Measure(Text("text", test.text), Constraints{
				Max: Size{Width: 80, Height: 24},
			})
			require.Equal(t, test.want, got)
		})
	}
}

func TestMeasureTextWrapsAtAssignedWidth(t *testing.T) {
	t.Parallel()

	got := Measure(Text("text", "one two"), Constraints{
		Max: Size{Width: 4, Height: 10},
	})
	require.Equal(t, Size{Width: 4, Height: 2}, got.Preferred)
}

func TestFitTextPreservesDisplayCells(t *testing.T) {
	t.Parallel()

	styled := "\x1b[31mA界B\x1b[0m"
	wrapped := FitText(styled, 2, WrapText)
	require.Len(t, wrapped, 3)
	for _, line := range wrapped {
		require.LessOrEqual(t, ansi.StringWidth(line), 2)
	}
	clipped := FitText(styled, 3, ClipText)
	require.Len(t, clipped, 1)
	require.Equal(t, 3, ansi.StringWidth(clipped[0]))
	require.Equal(t, "A界", ansi.Strip(clipped[0]))
	require.Equal(t, []string{"", ""}, FitText("\n", 4, ClipText))
}

func TestArrangeRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	_, err := Arrange(
		Row("root", Text("item", "one"), Text("item", "two")),
		Rect{Width: 20, Height: 1},
		1,
	)
	require.ErrorIs(t, err, ErrDuplicateID)
	var duplicate DuplicateIDError
	require.True(t, errors.As(err, &duplicate))
	require.Equal(t, ID("item"), duplicate.ID)
}

func TestRowDistributesRemaindersInStableOrder(t *testing.T) {
	t.Parallel()

	first := Text("first", "a")
	first.Grow = 1
	second := Text("second", "b")
	second.Grow = 1
	plan, err := Arrange(Row("root", first, second), Rect{Width: 5, Height: 1}, 1)
	require.NoError(t, err)

	firstRect, ok := plan.Rect("first")
	require.True(t, ok)
	secondRect, ok := plan.Rect("second")
	require.True(t, ok)
	require.Equal(t, 3, firstRect.Width)
	require.Equal(t, 2, secondRect.Width)
}

func TestLinearLayoutPolicies(t *testing.T) {
	t.Parallel()

	t.Run("shrink and gap", func(t *testing.T) {
		first := Text("first", "aaaa")
		first.Shrink = 1
		second := Text("second", "bbbb")
		second.Shrink = 1
		root := Row("root", first, second)
		root.Gap = 1
		plan, err := Arrange(root, Rect{Width: 7, Height: 1}, 1)
		require.NoError(t, err)
		firstRect, ok := plan.Rect("first")
		require.True(t, ok)
		secondRect, ok := plan.Rect("second")
		require.True(t, ok)
		require.Equal(t, Rect{Width: 3, Height: 1}, firstRect)
		require.Equal(t, Rect{X: 4, Width: 3, Height: 1}, secondRect)
	})

	t.Run("spacer", func(t *testing.T) {
		plan, err := Arrange(
			Row("root", Text("left", "L"), Spacer("space"), Text("right", "R")),
			Rect{Width: 10, Height: 1},
			1,
		)
		require.NoError(t, err)
		spacer, ok := plan.Rect("space")
		require.True(t, ok)
		require.Equal(t, 8, spacer.Width)
		right, ok := plan.Rect("right")
		require.True(t, ok)
		require.Equal(t, 9, right.X)
	})

	t.Run("main and cross alignment", func(t *testing.T) {
		root := Row("root", Text("item", "x"))
		root.Justify = JustifyCenter
		root.Align = AlignEnd
		plan, err := Arrange(root, Rect{Width: 5, Height: 3}, 1)
		require.NoError(t, err)
		item, ok := plan.Rect("item")
		require.True(t, ok)
		require.Equal(t, Rect{X: 2, Y: 2, Width: 1, Height: 1}, item)
	})
}

func TestBoxUsesStyleGeometryForChild(t *testing.T) {
	t.Parallel()

	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	plan, err := Arrange(
		Box("box", style, Text("content", "x")),
		Rect{X: 4, Y: 3, Width: 7, Height: 5},
		1,
	)
	require.NoError(t, err)
	child, ok := plan.Rect("content")
	require.True(t, ok)
	require.Equal(t, Rect{X: 7, Y: 5, Width: 1, Height: 1}, child)
}

func TestArrangementsStayInsideParents(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		width := rapid.IntRange(2, 160).Draw(t, "width")
		height := rapid.IntRange(1, 40).Draw(t, "height")
		count := rapid.IntRange(1, 8).Draw(t, "children")
		children := make([]Node, count)
		for i := range children {
			child := Text(ID(rune('a'+i)), "content")
			child.Grow = 1
			child.Shrink = 1
			children[i] = child
		}
		root := Row("root", children...)
		root.Gap = rapid.IntRange(0, 2).Draw(t, "gap")
		root.Align = AlignStretch

		plan, err := Arrange(root, Rect{Width: width, Height: height}, 1)
		require.NoError(t, err)
		for _, entry := range plan.Entries() {
			require.GreaterOrEqual(t, entry.Rect.X, plan.Bounds.X)
			require.GreaterOrEqual(t, entry.Rect.Y, plan.Bounds.Y)
			require.LessOrEqual(t, entry.Rect.Right(), plan.Bounds.Right())
			require.LessOrEqual(t, entry.Rect.Bottom(), plan.Bounds.Bottom())
		}
	})
}

func BenchmarkMeasure(b *testing.B) {
	root := Box(
		"box",
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1),
		Row("row", Text("a", " Cursor "), Text("b", " Rectangle "), Text("c", " Line ")),
	)
	constraints := Constraints{Max: Size{Width: 80, Height: 24}}
	b.ReportAllocs()
	for b.Loop() {
		benchmarkMetrics = Measure(root, constraints)
	}
}

func BenchmarkArrange(b *testing.B) {
	root := Box(
		"box",
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1),
		Row("row", Text("a", " Cursor "), Text("b", " Rectangle "), Text("c", " Line ")),
	)
	b.ReportAllocs()
	for b.Loop() {
		var err error
		benchmarkPlan, err = Arrange(root, Rect{Width: 29, Height: 5}, 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

var (
	benchmarkMetrics Metrics
	benchmarkPlan    Plan
)
