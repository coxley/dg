package layout

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestAttachmentFollowsHostRouteAndHistory(t *testing.T) {
	t.Parallel()

	geo, history, source, destination, node, edge := newAttachmentLayout(t)
	attachment, ok := geo.NodeAttachment(node)
	require.True(t, ok)
	beforeNode := geo.Nodes[node].Rect.Min
	beforeDestination := geo.Nodes[destination].Rect.Min

	transaction := history.Begin()
	require.NoError(t, geo.PlaceNode(destination, NewPoint(30, 18)))
	require.NoError(t, geo.Build())
	require.NoError(t, transaction.Commit())

	require.NotEqual(t, beforeNode, geo.Nodes[node].Rect.Min)
	point, err := attachmentPoint(geo.Edges[edge].Points, attachment.Position)
	require.NoError(t, err)
	require.Equal(
		t,
		geo.Nodes[node].Rect.Min.Add(attachment.Anchor.X, attachment.Anchor.Y),
		point,
	)
	require.Equal(t, source, uint32(0))

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, beforeDestination, geo.Nodes[destination].Rect.Min)
	require.Equal(t, beforeNode, geo.Nodes[node].Rect.Min)
	require.Equal(t, attachment, mustAttachment(t, geo, node))

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, NewPoint(30, 18), geo.Nodes[destination].Rect.Min)
	require.Equal(t, attachment, mustAttachment(t, geo, node))
}

func TestAttachmentSelectionAndEdgeDeletion(t *testing.T) {
	t.Parallel()

	geo, history, _, _, node, edge := newAttachmentLayout(t)
	edgeHit := Hit{ID: edge, Kind: HitEdge}
	nodeHit := Hit{ID: node, Kind: HitNode}

	require.True(t, geo.Selection().SelectOnly(edgeHit))
	require.True(t, geo.Selection().Contains(edgeHit))
	require.True(t, geo.Selection().Contains(nodeHit))

	require.True(t, geo.Selection().SelectOnly(edgeHit))
	require.True(t, geo.Selection().Contains(edgeHit))
	require.False(t, geo.Selection().Contains(nodeHit))

	before := geo.Nodes[node].Rect.Min
	transaction := history.Begin()
	require.NoError(t, geo.DeleteEdge(edge))
	require.NoError(t, geo.Build())
	require.NoError(t, transaction.Commit())
	_, attached := geo.NodeAttachment(node)
	require.False(t, attached)
	require.Equal(t, before, geo.Nodes[node].Rect.Min)

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, geo.EdgeExists(edge))
	require.Equal(t, before, geo.Nodes[node].Rect.Min)
	require.Equal(t, edge, mustAttachment(t, geo, node).EdgeID)
}

func TestDuplicateSelectionPreservesCompleteAttachment(t *testing.T) {
	t.Parallel()

	geo, _, source, destination, node, edge := newAttachmentLayout(t)
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

	geo, _, source, _, _, edge := newAttachmentLayout(t)
	before := geo.historyState()
	point := geo.Edges[edge].Points[0]

	err := geo.AttachNode(source, edge, point)
	require.ErrorContains(t, err, "cannot attach")
	require.Equal(t, before, geo.historyState())
	require.NoError(t, geo.Build())
}

func TestAttachmentCloneIsIndependent(t *testing.T) {
	t.Parallel()

	geo, _, _, destination, node, edge := newAttachmentLayout(t)
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

	geo, _, _, _, node, edge := newAttachmentLayout(t)
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

func TestAttachmentMutationHistoryProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		history, err := NewHistory()
		require.NoError(t, err)
		geo, err := New(WithHistory(history))
		require.NoError(t, err)
		leftTop, err := geo.NewNodeAt("left top", NewPoint(10, 10))
		require.NoError(t, err)
		rightTop, err := geo.NewNodeAt("right top", NewPoint(80, 10))
		require.NoError(t, err)
		leftBottom, err := geo.NewNodeAt("left bottom", NewPoint(10, 50))
		require.NoError(t, err)
		rightBottom, err := geo.NewNodeAt("right bottom", NewPoint(80, 50))
		require.NoError(t, err)
		first, err := geo.NewNodeAt("first", NewPoint(30, 30))
		require.NoError(t, err)
		second, err := geo.NewNodeAt("second", NewPoint(55, 30))
		require.NoError(t, err)
		top := geo.ConnectNodes(leftTop, ir.RightSide, ir.LeftSide, rightTop)
		bottom := geo.ConnectNodes(
			leftBottom,
			ir.RightSide,
			ir.LeftSide,
			rightBottom,
		)
		require.NoError(t, geo.Build())
		history.Clear()

		states := []layoutHistoryState{geo.historyState()}
		steps := rapid.IntRange(1, 12).Draw(t, "steps")
		for step := range steps {
			before := geo.historyState()
			transaction := history.Begin()
			nodeID := rapid.SampledFrom([]uint32{first, second}).
				Draw(t, fmt.Sprintf("node %d", step))
			switch rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("operation %d", step)) {
			case 0:
				edgeID := rapid.SampledFrom([]uint32{top, bottom}).
					Draw(t, fmt.Sprintf("host %d", step))
				err = geo.SetAttachment(Attachment{
					NodeID: nodeID,
					EdgeID: edgeID,
					Position: rapid.Uint16Range(2000, attachmentPositionMax-2000).
						Draw(t, fmt.Sprintf("position %d", step)),
					Anchor: NewPoint(1, 1),
				})
			case 1:
				err = geo.DetachNode(nodeID)
			case 2:
				destination := rapid.SampledFrom([]uint32{
					rightTop,
					rightBottom,
				}).Draw(t, fmt.Sprintf("destination %d", step))
				err = geo.PlaceNode(destination, NewPoint(
					rapid.Uint32Range(60, 100).
						Draw(t, fmt.Sprintf("x %d", step)),
					rapid.Uint32Range(5, 60).
						Draw(t, fmt.Sprintf("y %d", step)),
				))
				if err == nil {
					err = geo.Build()
				}
			}
			if err != nil {
				require.NoError(t, transaction.Cancel())
				require.Equal(t, before, geo.historyState())
				continue
			}
			require.NoError(t, transaction.Commit())
			after := geo.historyState()
			if !reflect.DeepEqual(before, after) {
				states = append(states, after)
			}
		}

		for i := len(states) - 2; i >= 0; i-- {
			changed, undoErr := history.Undo()
			require.NoError(t, undoErr)
			require.True(t, changed)
			require.Equal(t, states[i], geo.historyState())
		}
		for i := 1; i < len(states); i++ {
			changed, redoErr := history.Redo()
			require.NoError(t, redoErr)
			require.True(t, changed)
			require.Equal(t, states[i], geo.historyState())
		}
	})
}

func newAttachmentLayout(
	t testing.TB,
) (*Layout, *History, uint32, uint32, uint32, uint32) {
	t.Helper()

	history, err := NewHistory(WithHistoryCacheDir(t.TempDir()))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
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
	history.Clear()
	return geo, history, source, destination, node, edge
}

func mustAttachment(t testing.TB, geo *Layout, nodeID uint32) Attachment {
	t.Helper()
	attachment, ok := geo.NodeAttachment(nodeID)
	require.True(t, ok)
	return attachment
}

func TestAttachmentIDsRemainAlignedAfterReuse(t *testing.T) {
	t.Parallel()

	geo, _, _, _, node, _ := newAttachmentLayout(t)
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
