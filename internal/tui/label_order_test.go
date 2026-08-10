package tui

import (
	"testing"

	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestOrderLabelTargetsUsesOverlappingVisualRows(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	topFoo := newLabelOrderNode(t, geo, "foo", layout.NewPoint(0, 3), layout.Size{Width: 5, Height: 3})
	topBar := newLabelOrderNode(t, geo, "bar", layout.NewPoint(10, 0), layout.Size{Width: 5, Height: 9})
	topBaz := newLabelOrderNode(t, geo, "baz", layout.NewPoint(20, 3), layout.Size{Width: 5, Height: 5})
	topQux := newLabelOrderNode(t, geo, "qux", layout.NewPoint(30, 0), layout.Size{Width: 5, Height: 11})
	bottomFoo := newLabelOrderNode(t, geo, "foo", layout.NewPoint(0, 20), layout.Size{Width: 5, Height: 3})
	bottomBar := newLabelOrderNode(t, geo, "bar", layout.NewPoint(10, 17), layout.Size{Width: 5, Height: 9})
	bottomBaz := newLabelOrderNode(t, geo, "baz", layout.NewPoint(20, 20), layout.Size{Width: 5, Height: 5})
	bottomQux := newLabelOrderNode(t, geo, "qux", layout.NewPoint(30, 17), layout.Size{Width: 5, Height: 11})

	targets := []uint32{
		bottomQux, topBaz, bottomFoo, topBar,
		bottomBaz, topQux, bottomBar, topFoo,
	}
	orderLabelTargets(geo, targets)

	require.Equal(t, []uint32{
		topFoo, topBar, topBaz, topQux,
		bottomFoo, bottomBar, bottomBaz, bottomQux,
	}, targets)
}

func TestOrderLabelTargetsUsesLabelToPlaceRowSpanningNode(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	top := newLabelOrderNode(t, geo, "top", layout.NewPoint(20, 0), layout.Size{Width: 6, Height: 3})
	bottom := newLabelOrderNode(t, geo, "bottom", layout.NewPoint(10, 10), layout.Size{Width: 8, Height: 3})
	spanning := newLabelOrderNode(t, geo, "x", layout.NewPoint(0, 0), layout.Size{Width: 5, Height: 13})
	style, ok := geo.NodeStyle(spanning)
	require.True(t, ok)
	style.Vertical = layout.AlignBottom
	require.NoError(t, geo.SetNodeStyle(spanning, style))

	targets := []uint32{spanning, bottom, top}
	orderLabelTargets(geo, targets)

	require.Equal(t, []uint32{top, spanning, bottom}, targets)
}

func newLabelOrderNode(
	t testing.TB,
	geo *layout.Layout,
	label string,
	point layout.Point,
	size layout.Size,
) uint32 {
	t.Helper()

	nodeID, err := geo.NewNodeAt(label, point)
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeSize(nodeID, size))
	return nodeID
}
