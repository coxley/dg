package ir

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGraphReusesDeletedIDs(t *testing.T) {
	t.Parallel()

	var graph Graph
	left := graph.NewNode("left")
	middle := graph.NewNode("middle")
	right := graph.NewNode("right")
	middlePorts := append([]uint32(nil), graph.Nodes[middle].Ports...)
	edgeA := graph.ConnectNodes(left, RightSide, LeftSide, middle)
	edgeB := graph.ConnectNodes(middle, RightSide, LeftSide, right)
	nodeCount := len(graph.Nodes)
	portCount := len(graph.Ports)
	edgeCount := len(graph.Edges)
	middlePortCapacity := cap(graph.Nodes[middle].Ports)

	require.NoError(t, graph.DeleteNode(middle))
	require.False(t, graph.NodeExists(middle))
	require.Empty(t, graph.Nodes[middle].Ports)
	require.Equal(t, middlePortCapacity, cap(graph.Nodes[middle].Ports))
	require.False(t, graph.EdgeExists(edgeA))
	require.False(t, graph.EdgeExists(edgeB))
	require.False(t, graph.AllPortsLive())
	for _, portID := range middlePorts {
		require.False(t, graph.PortExists(portID))
	}

	replacement := graph.NewNode("replacement")
	require.Equal(t, middle, replacement)
	require.Len(t, graph.Nodes, nodeCount)
	require.Len(t, graph.Ports, portCount)
	require.ElementsMatch(t, middlePorts, graph.Nodes[replacement].Ports)
	require.True(t, graph.AllPortsLive())

	replacementEdge := graph.ConnectNodes(left, RightSide, LeftSide, replacement)
	require.Contains(t, []uint32{edgeA, edgeB}, replacementEdge)
	require.Len(t, graph.Edges, edgeCount)
}

func TestGraphDeleteErrors(t *testing.T) {
	t.Parallel()

	var graph Graph
	node := graph.NewNode("node")

	require.ErrorIs(t, graph.DeleteEdge(0), ErrEdgeNotFound)
	require.NoError(t, graph.DeleteNode(node))
	require.ErrorIs(t, graph.DeleteNode(node), ErrNodeNotFound)
}
