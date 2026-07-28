package render

import (
	"testing"

	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestLabelAlignment(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("x", layout.Point{})
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeSize(nodeID, layout.Size{Width: 7, Height: 5}))
	require.NoError(t, geo.SetNodeStyle(nodeID, layout.NodeStyle{
		Horizontal: layout.AlignCenter,
		Vertical:   layout.AlignMiddle,
	}))
	require.NoError(t, geo.Build())

	text, err := Unicode(geo)
	require.NoError(t, err)
	require.Equal(t, "┌─────┐\n│     │\n│  x  │\n│     │\n└─────┘\n", text)
}
