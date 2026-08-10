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
	require.Error(t, geo.SetNodeStyle(nodeID, NodeStyle{
		Padding: PaddingLevel(255),
	}))
	require.Error(t, geo.SetEdgeStyle(edgeID, EdgeStyle{
		PortAArrow: ArrowStyle(255),
	}))
	require.Error(t, geo.SetEdgeStyle(edgeID, EdgeStyle{
		Stroke: StrokeStyle(255),
	}))
}

func TestArrowStyleNext(t *testing.T) {
	t.Parallel()

	styles := []ArrowStyle{
		ArrowNone,
		ArrowFilled,
		ArrowOpen,
		ArrowCircle,
		ArrowCircleBullet,
		ArrowNone,
	}
	for i := 1; i < len(styles); i++ {
		require.Equal(t, styles[i], styles[i-1].Next())
	}
}

func TestSetNodeStyleAppliesPadding(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("node", NewPoint(4, 5))
	require.NoError(t, err)
	require.Equal(t, Size{Width: 8, Height: 3}, geo.Nodes[nodeID].Rect.Size)
	require.Equal(t, NewPoint(6, 6), geo.Nodes[nodeID].LabelPoint)

	style, ok := geo.NodeStyle(nodeID)
	require.True(t, ok)
	style.Padding = PaddingNone
	require.NoError(t, geo.SetNodeStyle(nodeID, style))
	require.Equal(t, Size{Width: 6, Height: 3}, geo.Nodes[nodeID].Rect.Size)
	require.Equal(t, NewPoint(5, 6), geo.Nodes[nodeID].LabelPoint)

	style.Padding = PaddingExtra
	require.NoError(t, geo.SetNodeStyle(nodeID, style))
	require.Equal(t, Size{Width: 10, Height: 5}, geo.Nodes[nodeID].Rect.Size)
	require.Equal(t, NewPoint(7, 7), geo.Nodes[nodeID].LabelPoint)
}

func TestSetNodeStyleRejectsPaddingLargerThanExplicitNode(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	nodeID, err := geo.NewNode("node")
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeSize(nodeID, Size{Width: 6, Height: 3}))

	style, _ := geo.NodeStyle(nodeID)
	style.Padding = PaddingExtra
	require.ErrorContains(t, geo.SetNodeStyle(nodeID, style), "smaller than minimum")
	got, _ := geo.NodeStyle(nodeID)
	require.Equal(t, PaddingDefault, got.Padding)
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
