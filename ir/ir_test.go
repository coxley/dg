package ir

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSideJSONUsesNames(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(RightSide)
	require.NoError(t, err)
	require.JSONEq(t, `"right"`, string(data))

	var side Side
	require.NoError(t, json.Unmarshal([]byte(`"left"`), &side))
	require.Equal(t, LeftSide, side)
	require.Error(t, json.Unmarshal([]byte(`"diagonal"`), &side))
}

func TestSideMarshalTextDoesNotAllocate(t *testing.T) {
	var text []byte
	allocations := testing.AllocsPerRun(100, func() {
		var err error
		text, err = RightSide.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocations)
	require.Equal(t, "right", string(text))
}

func TestEdgeRelationships(t *testing.T) {
	t.Parallel()

	edge := Edge{PortA: 2, PortB: 7}

	require.True(t, edge.HasPort(2))
	require.False(t, edge.HasPort(3))
	require.True(t, edge.Connects(2, 7))
	require.True(t, edge.Connects(7, 2))
	require.False(t, edge.Connects(2, 2))
	require.True(t, edge.SharesPort(Edge{PortA: 7, PortB: 9}))
	require.False(t, edge.SharesPort(Edge{PortA: 3, PortB: 9}))
}

func TestComponentsRebuildAfterDeletion(t *testing.T) {
	t.Parallel()

	var graph Graph
	left := graph.NewNode("left")
	right := graph.NewNode("right")
	edgeID := graph.ConnectNodes(left, RightSide, LeftSide, right)

	var components Components
	components.Build(&graph)
	leftComponent, ok := components.ID(left)
	require.True(t, ok)
	rightComponent, ok := components.ID(right)
	require.True(t, ok)
	require.Equal(t, leftComponent, rightComponent)

	require.NoError(t, graph.DeleteEdge(edgeID))
	components.Build(&graph)
	leftComponent, ok = components.ID(left)
	require.True(t, ok)
	rightComponent, ok = components.ID(right)
	require.True(t, ok)
	require.NotEqual(t, leftComponent, rightComponent)
}

func TestGraphValidate(t *testing.T) {
	t.Parallel()

	var valid Graph
	left := valid.NewNode("left")
	right := valid.NewNode("right")
	edgeID := valid.ConnectNodes(left, RightSide, LeftSide, right)
	require.NoError(t, valid.Validate())
	require.True(t, valid.EdgeIncidentTo(edgeID, left))
	require.True(t, valid.EdgeIncidentTo(edgeID, right))
	require.False(t, valid.EdgeIncidentTo(edgeID, math.MaxUint32))

	invalid := valid.Clone()
	invalid.Edges[edgeID].PortB = math.MaxUint32
	require.EqualError(t, invalid.Validate(), "edge 0 references an unknown port")
}

func TestNewNodeCreatesInteriorPorts(t *testing.T) {
	t.Parallel()

	var graph Graph
	nodeID := graph.NewNode("node")
	require.Len(t, graph.Nodes[nodeID].Ports, 12)

	wantOffsets := []float32{.5, .25, .75}
	for _, side := range []Side{Top, RightSide, Bottom, LeftSide} {
		portIDs := graph.PortsOnSide(nodeID, side)
		require.Len(t, portIDs, len(wantOffsets))
		for i, portID := range portIDs {
			require.Equal(t, wantOffsets[i], graph.Ports[portID].Offset)
		}
	}
}

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
