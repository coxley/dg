package layout

import (
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestStylesRoundTripThroughHistory(t *testing.T) {
	t.Parallel()

	history, err := NewHistory(WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(1, 1))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(20, 1))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, geo.Build())
	history.Clear()

	transaction := history.Begin()
	nodeStyle := NodeStyle{Border: BorderRounded}
	edgeStyle := EdgeStyle{
		PortAArrow: ArrowOpen,
		PortBArrow: ArrowFilled,
	}
	require.NoError(t, geo.SetNodeStyle(left, nodeStyle))
	require.NoError(t, geo.SetEdgeStyle(edgeID, edgeStyle))
	require.NoError(t, transaction.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	gotNode, ok := geo.NodeStyle(left)
	require.True(t, ok)
	require.Equal(t, NodeStyle{}, gotNode)
	gotEdge, ok := geo.EdgeStyle(edgeID)
	require.True(t, ok)
	require.Equal(t, EdgeStyle{}, gotEdge)

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	gotNode, ok = geo.NodeStyle(left)
	require.True(t, ok)
	require.Equal(t, nodeStyle, gotNode)
	gotEdge, ok = geo.EdgeStyle(edgeID)
	require.True(t, ok)
	require.Equal(t, edgeStyle, gotEdge)
}

func TestDeleteNodeHistoryRestoresStyles(t *testing.T) {
	t.Parallel()

	history, err := NewHistory(WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(1, 1))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(20, 1))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	nodeStyle := NodeStyle{Border: BorderNone}
	edgeStyle := EdgeStyle{PortBArrow: ArrowOpen}
	require.NoError(t, geo.SetNodeStyle(left, nodeStyle))
	require.NoError(t, geo.SetEdgeStyle(edgeID, edgeStyle))
	require.NoError(t, geo.Build())
	history.Clear()

	require.NoError(t, geo.DeleteNode(left))
	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	gotNode, ok := geo.NodeStyle(left)
	require.True(t, ok)
	require.Equal(t, nodeStyle, gotNode)
	gotEdge, ok := geo.EdgeStyle(edgeID)
	require.True(t, ok)
	require.Equal(t, edgeStyle, gotEdge)
}

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
