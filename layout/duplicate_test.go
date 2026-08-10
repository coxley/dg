package layout

import (
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestDuplicateSelectionCopiesContainedGraph(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(2, 3))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(12, 3))
	require.NoError(t, err)
	external, err := geo.NewNodeAt("external", NewPoint(22, 3))
	require.NoError(t, err)
	internalEdge := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	geo.ConnectNodes(right, ir.RightSide, ir.LeftSide, external)
	require.NoError(t, geo.SetNodeSize(left, Size{Width: 8, Height: 4}))
	require.NoError(t, geo.SetNodeStyle(left, NodeStyle{
		Border:     BorderRounded,
		Horizontal: AlignCenter,
		Vertical:   AlignMiddle,
	}))
	require.NoError(t, geo.SetEdgeStyle(internalEdge, EdgeStyle{
		PortBArrow: ArrowFilled,
	}))
	require.NoError(t, geo.Build())
	beforeRoute := append([]Point(nil), geo.Edges[internalEdge].Points...)
	geo.Selection().SelectOnly(Hit{ID: left, Kind: HitNode})
	require.True(t, geo.Selection().Toggle(Hit{ID: right, Kind: HitNode}))
	require.NoError(t, geo.DuplicateSelection(4, 5))
	for edgeID := range geo.Selection().Edges() {
		require.Len(t, geo.Edges[edgeID].Points, len(beforeRoute))
		for i, point := range beforeRoute {
			require.Equal(t, point.Add(4, 5), geo.Edges[edgeID].Points[i])
		}
	}
	require.NoError(t, geo.Build())

	nodes, edges := geo.Selection().Counts()
	require.Equal(t, 2, nodes)
	require.Equal(t, 1, edges)
	var copiedLeft uint32
	for nodeID := range geo.Selection().Nodes() {
		if geo.Label(nodeID) == "left" {
			copiedLeft = nodeID
		}
	}
	require.Equal(t, NewPoint(6, 8), geo.Nodes[copiedLeft].Rect.Min)
	require.Equal(t, Size{Width: 8, Height: 4}, geo.Nodes[copiedLeft].Rect.Size)
	style, ok := geo.NodeStyle(copiedLeft)
	require.True(t, ok)
	require.Equal(t, NodeStyle{
		Border:     BorderRounded,
		Horizontal: AlignCenter,
		Vertical:   AlignMiddle,
	}, style)
}

func TestDuplicateSelectionReusesTombstones(t *testing.T) {
	t.Parallel()

	geo := newStressBenchmarkLayout(t)
	for nodeID := len(geo.Nodes) - 1; nodeID >= benchmarkClusterNodes; nodeID-- {
		require.NoError(t, geo.DeleteNode(uint32(nodeID)))
	}
	require.True(t, geo.Selection().SelectOnly(Hit{ID: 0, Kind: HitNode}))
	require.True(t, geo.Selection().Toggle(Hit{ID: 1, Kind: HitNode}))
	require.True(t, geo.Selection().Toggle(Hit{ID: 2, Kind: HitNode}))

	require.NoError(t, geo.DuplicateSelection(30, 0))
	require.NoError(t, geo.Build())
	nodes, edges := geo.Selection().Counts()
	require.Equal(t, benchmarkClusterNodes, nodes)
	require.Equal(t, 2, edges)
}

func TestDuplicateSelectionPreservesSelectedGroupHierarchy(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNode("left")
	require.NoError(t, err)
	middle, err := geo.NewNode("middle")
	require.NoError(t, err)
	right, err := geo.NewNode("right")
	require.NoError(t, err)
	inner, err := geo.NewGroup([]ir.Member{
		{ID: left, Kind: ir.MemberNode},
		{ID: middle, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	outer, err := geo.NewGroup([]ir.Member{
		{ID: inner, Kind: ir.MemberGroup},
		{ID: right, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	require.True(t, geo.Selection().SelectOnly(Hit{ID: outer, Kind: HitGroup}))
	require.NoError(t, geo.DuplicateSelection(20, 0))

	_, groups, _ := geo.Selection().LogicalCounts()
	require.Equal(t, 1, groups)
	var duplicate uint32
	for groupID := range geo.Selection().Groups() {
		duplicate = groupID
	}
	members, ok := geo.GroupMembers(duplicate)
	require.True(t, ok)
	require.Equal(t, ir.MemberGroup, members[0].Kind)
	require.NotEqual(t, inner, members[0].ID)
	require.Equal(t, 3, len(slices.Collect(geo.GroupNodes(duplicate))))
}

func TestDuplicateDrilledNodesOmitsParentGroup(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNode("left")
	require.NoError(t, err)
	right, err := geo.NewNode("right")
	require.NoError(t, err)
	_, err = geo.NewGroup([]ir.Member{
		{ID: left, Kind: ir.MemberNode},
		{ID: right, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	require.True(t, geo.Selection().SelectOnly(Hit{ID: left, Kind: HitNode}))
	require.True(t, geo.Selection().Toggle(Hit{ID: right, Kind: HitNode}))
	require.NoError(t, geo.DuplicateSelection(20, 0))

	_, groups, _ := geo.Selection().LogicalCounts()
	require.Zero(t, groups)
}

func TestCloneIsIndependentAndPreservesSelection(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("node", NewPoint(3, 4))
	require.NoError(t, err)
	geo.Selection().SelectOnly(Hit{ID: nodeID, Kind: HitNode})
	require.NoError(t, geo.Build())

	cloned, err := geo.Clone()
	require.NoError(t, err)
	require.True(t, cloned.Selection().Contains(Hit{ID: nodeID, Kind: HitNode}))
	require.NoError(t, cloned.PlaceNode(nodeID, NewPoint(9, 10)))
	require.NoError(t, cloned.Build())

	require.Equal(t, NewPoint(3, 4), geo.Nodes[nodeID].Rect.Min)
	require.Equal(t, NewPoint(9, 10), cloned.Nodes[nodeID].Rect.Min)
}

func TestBuildSelectionPreservesUnrelatedRoutes(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	a, err := geo.NewNodeAt("a", NewPoint(2, 2))
	require.NoError(t, err)
	b, err := geo.NewNodeAt("b", NewPoint(14, 2))
	require.NoError(t, err)
	c, err := geo.NewNodeAt("c", NewPoint(2, 12))
	require.NoError(t, err)
	d, err := geo.NewNodeAt("d", NewPoint(14, 12))
	require.NoError(t, err)
	affected := geo.ConnectNodes(a, ir.RightSide, ir.LeftSide, b)
	unrelated := geo.ConnectNodes(c, ir.RightSide, ir.LeftSide, d)
	require.NoError(t, geo.Build())
	beforeAffected := append([]Point(nil), geo.Edges[affected].Points...)
	beforeUnrelated := append([]Point(nil), geo.Edges[unrelated].Points...)

	geo.Selection().SelectOnly(Hit{ID: a, Kind: HitNode})
	require.NoError(t, geo.PlaceNode(a, NewPoint(2, 6)))
	require.NoError(t, geo.BuildSelection())

	require.NotEqual(t, beforeAffected, geo.Edges[affected].Points)
	require.Equal(t, beforeUnrelated, geo.Edges[unrelated].Points)
}

func TestMoveSelectionPreservesValidInternalRoutes(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	a, err := geo.NewNodeAt("a", NewPoint(2, 2))
	require.NoError(t, err)
	b, err := geo.NewNodeAt("b", NewPoint(14, 2))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(a, ir.RightSide, ir.LeftSide, b)
	require.NoError(t, geo.Build())
	before := append([]Point(nil), geo.Edges[edgeID].Points...)
	geo.Selection().SelectOnly(Hit{ID: a, Kind: HitNode})
	require.True(t, geo.Selection().Toggle(Hit{ID: b, Kind: HitNode}))

	require.NoError(t, geo.MoveSelection(5, 6))
	require.NoError(t, geo.BuildSelection())

	for i, point := range before {
		require.Equal(t, point.Add(5, 6), geo.Edges[edgeID].Points[i])
	}
}

func TestTranslateMovesAllGeometry(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	a, err := geo.NewNodeAt("a", NewPoint(2, 3))
	require.NoError(t, err)
	b, err := geo.NewNodeAt("b", NewPoint(14, 3))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(a, ir.RightSide, ir.LeftSide, b)
	require.NoError(t, geo.Build())
	beforeRoute := append([]Point(nil), geo.Edges[edgeID].Points...)
	require.NoError(t, geo.Translate(5, 7))

	require.Equal(t, NewPoint(7, 10), geo.Nodes[a].Rect.Min)
	require.Equal(t, NewPoint(19, 10), geo.Nodes[b].Rect.Min)
	for i, point := range beforeRoute {
		require.Equal(t, point.Add(5, 7), geo.Edges[edgeID].Points[i])
	}
}

func TestSelectionMovesRigidly(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	a, err := geo.NewNodeAt("a", NewPoint(2, 2))
	require.NoError(t, err)
	b, err := geo.NewNodeAt("b", NewPoint(14, 2))
	require.NoError(t, err)
	c, err := geo.NewNodeAt("c", NewPoint(26, 2))
	require.NoError(t, err)
	geo.ConnectNodes(a, ir.RightSide, ir.LeftSide, b)
	geo.Selection().SelectOnly(Hit{ID: a, Kind: HitNode})
	require.True(t, geo.Selection().Toggle(Hit{ID: b, Kind: HitNode}))
	require.True(t, geo.SelectionMovesRigidly())

	geo.ConnectNodes(b, ir.RightSide, ir.LeftSide, c)
	require.False(t, geo.SelectionMovesRigidly())
}
