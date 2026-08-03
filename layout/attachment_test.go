package layout

import (
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestAttachmentSelection(t *testing.T) {
	t.Parallel()

	geo, _, _, node, edge := newAttachmentLayout(t)
	edgeHit := Hit{ID: edge, Kind: HitEdge}
	nodeHit := Hit{ID: node, Kind: HitNode}

	require.True(t, geo.Selection().SelectOnly(edgeHit))
	require.True(t, geo.Selection().Contains(edgeHit))
	require.True(t, geo.Selection().Contains(nodeHit))

	require.True(t, geo.Selection().SelectOnly(edgeHit))
	require.True(t, geo.Selection().Contains(edgeHit))
	require.False(t, geo.Selection().Contains(nodeHit))
}

func TestAttachmentAtDoesNotMutate(t *testing.T) {
	t.Parallel()

	geo, _, _, node, edge := newAttachmentLayout(t)
	before := mustAttachment(t, geo, node)
	point := geo.Nodes[node].Rect.Min.Add(before.Anchor.X, before.Anchor.Y)
	attachment, err := geo.AttachmentAt(node, edge, point)
	require.NoError(t, err)
	require.Equal(t, before, attachment)
	require.Equal(t, before, mustAttachment(t, geo, node))
}

func TestRouteAttachmentHostsDoesNotRetainRelationships(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", NewPoint(2, 4))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", NewPoint(30, 4))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	portA, _, err := geo.EdgePorts(edge)
	require.NoError(t, err)
	crossing := NewPoint(16, geo.Ports[portA].Exit.Y)
	hosted, err := geo.NewNodeAt("hosted", NewPoint(15, crossing.Y-1))
	require.NoError(t, err)
	require.NoError(t, geo.Build())
	require.False(t, geo.Edges[edge].Contains(crossing))

	require.NoError(t, geo.RouteAttachmentHosts(AttachmentHost{NodeID: hosted, EdgeID: edge}))
	require.True(t, geo.Edges[edge].Contains(crossing))
	_, attached := geo.NodeAttachment(hosted)
	require.False(t, attached)
}

func TestDuplicateSelectionPreservesCompleteAttachment(t *testing.T) {
	t.Parallel()

	geo, source, destination, node, edge := newAttachmentLayout(t)
	geo.Selection().SelectOnly(Hit{ID: edge, Kind: HitEdge})
	geo.Selection().Toggle(Hit{ID: source, Kind: HitNode})
	geo.Selection().Toggle(Hit{ID: destination, Kind: HitNode})

	require.NoError(t, geo.DuplicateSelection(40, 0))
	require.NoError(t, geo.Build())

	var attached []Attachment
	for nodeID := range geo.Selection().Nodes() {
		if attachment, ok := geo.NodeAttachment(nodeID); ok {
			attached = append(attached, attachment)
		}
	}
	require.Len(t, attached, 1)
	require.True(t, geo.Selection().Contains(Hit{
		ID:   attached[0].EdgeID,
		Kind: HitEdge,
	}))

	geo.Selection().SelectOnly(Hit{ID: node, Kind: HitNode})
	require.NoError(t, geo.DuplicateSelection(0, 20))
	require.NoError(t, geo.Build())
	duplicate, ok := geo.Selection().FirstNode()
	require.True(t, ok)
	_, ok = geo.NodeAttachment(duplicate)
	require.False(t, ok)
}

func TestAttachmentRejectsIncidentHostAtomically(t *testing.T) {
	t.Parallel()

	geo, source, _, _, edge := newAttachmentLayout(t)
	before := geo.historyState()
	point := geo.Edges[edge].Points[0]

	err := geo.AttachNode(source, edge, point)
	require.ErrorContains(t, err, "cannot attach")
	require.Equal(t, before, geo.historyState())
	require.NoError(t, geo.Build())
}

func TestAttachmentCloneIsIndependent(t *testing.T) {
	t.Parallel()

	geo, _, destination, node, edge := newAttachmentLayout(t)
	beforeNode := geo.Nodes[node].Rect.Min
	beforeRoute := slices.Clone(geo.Edges[edge].Points)
	beforeAttachment := mustAttachment(t, geo, node)

	cloned, err := geo.Clone()
	require.NoError(t, err)
	require.NoError(t, cloned.PlaceNode(destination, NewPoint(45, 18)))
	require.NoError(t, cloned.Build())
	require.Equal(t, beforeNode, cloned.Nodes[node].Rect.Min)

	require.Equal(t, beforeNode, geo.Nodes[node].Rect.Min)
	require.Equal(t, beforeRoute, geo.Edges[edge].Points)
	require.Equal(t, beforeAttachment, mustAttachment(t, geo, node))
}

func TestAttachmentFollowsMovedBend(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", NewPoint(2, 4))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", NewPoint(30, 14))
	require.NoError(t, err)
	node, err := geo.NewNodeAt("label", NewPoint(2, 25))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	portA, portB, err := geo.EdgePorts(edge)
	require.NoError(t, err)
	a, b := geo.Ports[portA], geo.Ports[portB]
	bends := []PinnedBend{
		{Point: NewPoint(18, a.Exit.Y), Incoming: East, Outgoing: South},
		{Point: NewPoint(18, b.Exit.Y), Incoming: South, Outgoing: East},
	}
	require.NoError(t, geo.SetPinnedBends(edge, bends))
	require.NoError(t, geo.Build())
	point := bends[1].Point.Add(2, 0)
	anchor := NewPoint(1, 1)
	reference, _, ok := attachmentLocation(geo.Edges[edge].Points, point)
	require.True(t, ok, "route=%v point=%v", geo.Edges[edge].Points, point)
	require.True(t, reference.Valid(), "route=%v reference=%+v", geo.Edges[edge].Points, reference)
	require.NoError(t, geo.PlaceNode(node, NewPoint(point.X-anchor.X, point.Y-anchor.Y)))
	require.NoError(t, geo.AttachNode(node, edge, point))
	before := mustAttachment(t, geo, node)
	require.NotZero(t, before.Reference.Bend)

	for index := range bends {
		bends[index].Point.X += 4
	}
	require.NoError(t, geo.SetPinnedBends(edge, bends))
	require.NoError(t, geo.Build())

	wantPoint := bends[1].Point.Add(2, 0)
	require.Equal(t, NewPoint(wantPoint.X-anchor.X, wantPoint.Y-anchor.Y), geo.Nodes[node].Rect.Min)
	require.Equal(t, before, mustAttachment(t, geo, node))
}

func TestAttachmentReanchorsOrProjectsAfterBendDisappears(t *testing.T) {
	t.Parallel()

	oldRoute := []Point{
		NewPoint(0, 0),
		NewPoint(5, 0),
		NewPoint(5, 5),
		NewPoint(12, 5),
	}
	reference, offset, ok := attachmentLocation(oldRoute, NewPoint(7, 5))
	require.True(t, ok)
	attachment := Attachment{Reference: reference, Offset: offset}

	withBends := []Point{
		NewPoint(0, 0),
		NewPoint(6, 0),
		NewPoint(6, 8),
		NewPoint(2, 8),
	}
	reanchored, point, err := resolveAttachment(
		oldRoute,
		withBends,
		attachment,
		NewPoint(7, 5),
	)
	require.NoError(t, err)
	require.Equal(t, NewPoint(4, 8), point)
	require.Equal(t, AttachmentPortB, reanchored.Reference.End)
	require.Equal(t, uint32(1), reanchored.Reference.Bend)
	require.Equal(t, South, reanchored.Reference.Incoming)
	require.Equal(t, West, reanchored.Reference.Outgoing)

	straight := []Point{NewPoint(0, 0), NewPoint(12, 0)}
	projected, point, err := resolveAttachment(
		oldRoute,
		straight,
		attachment,
		NewPoint(7, 5),
	)
	require.NoError(t, err)
	require.Equal(t, NewPoint(7, 0), point)
	require.Equal(t, AttachmentReference{End: AttachmentPortB}, projected.Reference)
	require.Equal(t, int64(-5), projected.Offset)
}

func TestAttachmentFallbackReplaysWithRouteMutation(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", NewPoint(2, 4))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", NewPoint(30, 4))
	require.NoError(t, err)
	node, err := geo.NewNodeAt("label", NewPoint(2, 20))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	portA, _, err := geo.EdgePorts(edge)
	require.NoError(t, err)
	y := geo.Ports[portA].Exit.Y
	bends := []PinnedBend{
		{Point: NewPoint(16, y), Incoming: East, Outgoing: South},
		{Point: NewPoint(16, y+8), Incoming: South, Outgoing: East},
	}
	require.NoError(t, geo.SetPinnedBends(edge, bends))
	require.NoError(t, geo.Build())
	point := bends[1].Point.Add(2, 0)
	anchor := NewPoint(1, 1)
	require.NoError(t, geo.PlaceNode(node, NewPoint(point.X-anchor.X, point.Y-anchor.Y)))
	require.NoError(t, geo.AttachNode(node, edge, point))
	beforeAttachment := mustAttachment(t, geo, node)
	beforeOrigin := geo.Nodes[node].Rect.Min

	var changes []Change
	require.NoError(t, geo.SetChangeCallback(func(change Change) {
		changes = append(changes, change)
	}))
	require.NoError(t, geo.SetPinnedBends(edge, nil))
	require.NoError(t, geo.Build())
	require.Len(t, changes, 2)
	afterAttachment := mustAttachment(t, geo, node)
	afterOrigin := geo.Nodes[node].Rect.Min
	require.NotEqual(t, beforeAttachment.Reference, afterAttachment.Reference)
	require.NotEqual(t, beforeOrigin, afterOrigin)

	require.NoError(t, geo.Replay(changes, ReplayBackward))
	require.Equal(t, beforeAttachment, mustAttachment(t, geo, node))
	require.Equal(t, beforeOrigin, geo.Nodes[node].Rect.Min)
	require.NoError(t, geo.Replay(changes, ReplayForward))
	require.Equal(t, afterAttachment, mustAttachment(t, geo, node))
	require.Equal(t, afterOrigin, geo.Nodes[node].Rect.Min)
}

func TestAttachmentZeroValueIsDetached(t *testing.T) {
	t.Parallel()

	geo, _, _, node, edge := newAttachmentLayout(t)
	require.NoError(t, geo.DetachNode(node))
	require.Equal(t, Attachment{}, geo.attachments[node])

	start := geo.Edges[edge].Points[0]
	require.NoError(t, geo.PlaceNode(node, start))
	require.False(t, geo.CanAttachNodeAt(node, edge, start))
	err := geo.AttachNode(node, edge, start)
	require.ErrorContains(t, err, "overlap an edge endpoint")
	_, attached := geo.NodeAttachment(node)
	require.False(t, attached)
}

func newAttachmentLayout(
	t testing.TB,
) (*Layout, uint32, uint32, uint32, uint32) {
	t.Helper()

	geo, err := New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", NewPoint(2, 4))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", NewPoint(30, 4))
	require.NoError(t, err)
	node, err := geo.NewNodeAt("tag", NewPoint(12, 15))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())

	points := geo.Edges[edge].Points
	point, ok := routePointAtDistance(points, pathLength(points)/2)
	require.True(t, ok)
	anchor := NewPoint(2, 1)
	require.Greater(t, point.X, anchor.X)
	require.Greater(t, point.Y, anchor.Y)
	require.NoError(t, geo.PlaceNode(
		node,
		NewPoint(point.X-anchor.X, point.Y-anchor.Y),
	))
	require.NoError(t, geo.AttachNode(node, edge, point))
	return geo, source, destination, node, edge
}

func mustAttachment(t testing.TB, geo *Layout, nodeID uint32) Attachment {
	t.Helper()
	attachment, ok := geo.NodeAttachment(nodeID)
	require.True(t, ok)
	return attachment
}

func TestAttachmentIDsRemainAlignedAfterReuse(t *testing.T) {
	t.Parallel()

	geo, _, _, node, _ := newAttachmentLayout(t)
	require.NoError(t, geo.DeleteNode(node))
	replacement, err := geo.NewNode("replacement")
	require.NoError(t, err)
	require.Equal(t, node, replacement)
	_, attached := geo.NodeAttachment(replacement)
	require.False(t, attached)
	require.False(t, slices.ContainsFunc(
		geo.attachments,
		func(attachment Attachment) bool {
			return attachment != (Attachment{})
		},
	))
}
