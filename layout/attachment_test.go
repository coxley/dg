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
	require.NotEqual(t, beforeNode, cloned.Nodes[node].Rect.Min)

	require.Equal(t, beforeNode, geo.Nodes[node].Rect.Min)
	require.Equal(t, beforeRoute, geo.Edges[edge].Points)
	require.Equal(t, beforeAttachment, mustAttachment(t, geo, node))
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

	point, err := attachmentPoint(geo.Edges[edge].Points, attachmentPositionMax/2)
	require.NoError(t, err)
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
