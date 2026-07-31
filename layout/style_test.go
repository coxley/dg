package layout

import (
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestStyleValidation(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	nodeID, err := geo.NewNode("node")
	require.NoError(t, err)
	otherID, err := geo.NewNodeAt("other", NewPoint(20, 0))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(nodeID, ir.RightSide, ir.LeftSide, otherID)

	require.Error(t, geo.SetNodeStyle(nodeID, NodeStyle{
		Border: BorderStyle(255),
	}))
	require.Error(t, geo.SetNodeStyle(nodeID, NodeStyle{
		Stroke: StrokeStyle(255),
	}))
	require.Error(t, geo.SetEdgeStyle(edgeID, EdgeStyle{
		PortAArrow: ArrowStyle(255),
	}))
	require.Error(t, geo.SetEdgeStyle(edgeID, EdgeStyle{
		Stroke: StrokeStyle(255),
	}))
}

func TestLabelLinePointAppliesNodeAlignment(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	nodeID, err := geo.NewNode("label")
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeSize(nodeID, Size{Width: 12, Height: 7}))
	require.NoError(t, geo.SetNodeStyle(nodeID, NodeStyle{
		Horizontal: AlignCenter,
		Vertical:   AlignMiddle,
	}))

	bounds := geo.LabelBounds(nodeID)
	point, visible := geo.LabelLinePoint(nodeID, 0, 1, 3)
	require.True(t, visible)
	require.Equal(t, bounds.Min.Add(2, 2), point)

	_, visible = geo.LabelLinePoint(nodeID, 1, 1, 3)
	require.False(t, visible)
	_, visible = geo.LabelLinePoint(nodeID+1, 0, 1, 3)
	require.False(t, visible)
}
