package history

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestHistoryReplaysAttachmentRouteChange(t *testing.T) {
	t.Parallel()

	geo, history, source, destination, node, edge := newHistoryAttachmentLayout(t)
	attachment := mustAttachment(t, geo, node)
	beforeNode := geo.Nodes[node].Rect.Min
	beforeDestination := geo.Nodes[destination].Rect.Min

	transaction := history.Begin()
	require.NoError(t, geo.PlaceNode(destination, layout.NewPoint(30, 18)))
	require.NoError(t, geo.Build())
	require.NoError(t, transaction.Commit())

	require.Equal(t, beforeNode, geo.Nodes[node].Rect.Min)
	anchor := geo.Nodes[node].Rect.Min.Add(attachment.Anchor.X, attachment.Anchor.Y)
	require.True(t, geo.Edges[edge].Contains(anchor))
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
	require.Equal(t, layout.NewPoint(30, 18), geo.Nodes[destination].Rect.Min)
	require.Equal(t, beforeNode, geo.Nodes[node].Rect.Min)
	require.Equal(t, attachment, mustAttachment(t, geo, node))
}

func TestHistoryGroupsAttachmentFallbackWithRouteMutation(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	source, err := geo.NewNodeAt("source", layout.NewPoint(2, 4))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", layout.NewPoint(30, 4))
	require.NoError(t, err)
	node, err := geo.NewNodeAt("label", layout.NewPoint(2, 20))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	portA, _, err := geo.EdgePorts(edge)
	require.NoError(t, err)
	y := geo.Ports[portA].Exit.Y
	bends := []layout.PinnedBend{
		{Point: layout.NewPoint(16, y), Incoming: layout.East, Outgoing: layout.South},
		{Point: layout.NewPoint(16, y+8), Incoming: layout.South, Outgoing: layout.East},
	}
	require.NoError(t, geo.SetPinnedBends(edge, bends))
	require.NoError(t, geo.Build())
	point := bends[1].Point.Add(2, 0)
	anchor := layout.NewPoint(1, 1)
	require.NoError(t, geo.PlaceNode(node, layout.NewPoint(point.X-anchor.X, point.Y-anchor.Y)))
	require.NoError(t, geo.AttachNode(node, edge, point))
	history.Clear()
	beforeAttachment := mustAttachment(t, geo, node)
	beforeOrigin := geo.Nodes[node].Rect.Min

	transaction := history.Begin()
	require.NoError(t, geo.SetPinnedBends(edge, nil))
	require.NoError(t, geo.Build())
	require.NoError(t, transaction.Commit())
	afterAttachment := mustAttachment(t, geo, node)
	afterOrigin := geo.Nodes[node].Rect.Min
	require.NotEqual(t, beforeAttachment.Reference, afterAttachment.Reference)

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, history.CanUndo())
	require.Equal(t, beforeAttachment, mustAttachment(t, geo, node))
	require.Equal(t, beforeOrigin, geo.Nodes[node].Rect.Min)
	restoredBends, err := geo.PinnedBends(edge)
	require.NoError(t, err)
	require.Equal(t, bends, restoredBends)

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, afterAttachment, mustAttachment(t, geo, node))
	require.Equal(t, afterOrigin, geo.Nodes[node].Rect.Min)
	restoredBends, err = geo.PinnedBends(edge)
	require.NoError(t, err)
	require.Empty(t, restoredBends)
}

func TestHistoryRestoresAttachmentAfterEdgeDeletion(t *testing.T) {
	t.Parallel()

	geo, history, _, _, node, edge := newHistoryAttachmentLayout(t)
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

func TestAttachmentMutationHistoryProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		geo, err := layout.New()
		require.NoError(t, err)
		history, err := New(geo)
		require.NoError(t, err)
		leftTop, err := geo.NewNodeAt("left top", layout.NewPoint(10, 10))
		require.NoError(t, err)
		rightTop, err := geo.NewNodeAt("right top", layout.NewPoint(80, 10))
		require.NoError(t, err)
		leftBottom, err := geo.NewNodeAt("left bottom", layout.NewPoint(10, 50))
		require.NoError(t, err)
		rightBottom, err := geo.NewNodeAt("right bottom", layout.NewPoint(80, 50))
		require.NoError(t, err)
		first, err := geo.NewNodeAt("first", layout.NewPoint(30, 30))
		require.NoError(t, err)
		second, err := geo.NewNodeAt("second", layout.NewPoint(55, 30))
		require.NoError(t, err)
		top := geo.ConnectNodes(leftTop, ir.RightSide, ir.LeftSide, rightTop)
		bottom := geo.ConnectNodes(leftBottom, ir.RightSide, ir.LeftSide, rightBottom)
		require.NoError(t, geo.Build())
		history.Clear()
		digest := func() string {
			value, err := json.Marshal(geo.Snapshot())
			require.NoError(t, err)
			return string(value)
		}

		states := []string{digest()}
		steps := rapid.IntRange(1, 12).Draw(t, "steps")
		for step := range steps {
			before := digest()
			transaction := history.Begin()
			nodeID := rapid.SampledFrom([]uint32{first, second}).
				Draw(t, fmt.Sprintf("node %d", step))
			switch rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("operation %d", step)) {
			case 0:
				edgeID := rapid.SampledFrom([]uint32{top, bottom}).
					Draw(t, fmt.Sprintf("host %d", step))
				err = geo.SetAttachment(layout.Attachment{
					NodeID: nodeID,
					EdgeID: edgeID,
					Reference: layout.AttachmentReference{
						End: layout.AttachmentPortA,
					},
					Offset: rapid.Int64Range(2, 20).
						Draw(t, fmt.Sprintf("offset %d", step)),
					Anchor: layout.NewPoint(1, 1),
				})
			case 1:
				err = geo.DetachNode(nodeID)
			case 2:
				destination := rapid.SampledFrom([]uint32{rightTop, rightBottom}).
					Draw(t, fmt.Sprintf("destination %d", step))
				err = geo.PlaceNode(destination, layout.NewPoint(
					rapid.Uint32Range(60, 100).Draw(t, fmt.Sprintf("x %d", step)),
					rapid.Uint32Range(5, 60).Draw(t, fmt.Sprintf("y %d", step)),
				))
				if err == nil {
					err = geo.Build()
				}
			}
			if err != nil {
				require.NoError(t, transaction.Cancel())
				require.Equal(t, before, digest())
				continue
			}
			require.NoError(t, transaction.Commit())
			after := digest()
			if before != after {
				states = append(states, after)
			}
		}

		for i := len(states) - 2; i >= 0; i-- {
			changed, err := history.Undo()
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, states[i], digest())
		}
		for i := 1; i < len(states); i++ {
			changed, err := history.Redo()
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, states[i], digest())
		}
	})
}

func TestHistoryReplaysPinnedBends(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(22, 12))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	before := []layout.PinnedBend{
		{Point: layout.NewPoint(14, 3), Incoming: layout.East, Outgoing: layout.South},
		{Point: layout.NewPoint(14, 13), Incoming: layout.South, Outgoing: layout.East},
	}
	require.NoError(t, geo.SetPinnedBends(edgeID, before))
	require.NoError(t, geo.Build())
	history.Clear()

	after := []layout.PinnedBend{
		{Point: layout.NewPoint(16, 3), Incoming: layout.East, Outgoing: layout.South},
		{Point: layout.NewPoint(16, 13), Incoming: layout.South, Outgoing: layout.East},
	}
	transaction := history.Begin()
	require.NoError(t, geo.SetPinnedBends(edgeID, after))
	require.NoError(t, geo.Build())
	require.NoError(t, transaction.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	got, err := geo.PinnedBends(edgeID)
	require.NoError(t, err)
	require.Equal(t, before, got)
	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	got, err = geo.PinnedBends(edgeID)
	require.NoError(t, err)
	require.Equal(t, after, got)
}

func TestHistoryDoesNotCoalesceEdgePropertiesAcrossLifecycle(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(22, 12))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, geo.Build())
	history.Clear()

	firstBends := []layout.PinnedBend{
		{Point: layout.NewPoint(14, 3), Incoming: layout.East, Outgoing: layout.South},
		{Point: layout.NewPoint(14, 13), Incoming: layout.South, Outgoing: layout.East},
	}
	lastBends := []layout.PinnedBend{
		{Point: layout.NewPoint(16, 3), Incoming: layout.East, Outgoing: layout.South},
		{Point: layout.NewPoint(16, 13), Incoming: layout.South, Outgoing: layout.East},
	}
	firstStyle := layout.EdgeStyle{PortAArrow: layout.ArrowOpen}
	lastStyle := layout.EdgeStyle{PortBArrow: layout.ArrowFilled}

	transaction := history.Begin()
	require.NoError(t, geo.SetEdgeStyle(edgeID, firstStyle))
	require.NoError(t, geo.SetPinnedBends(edgeID, firstBends))
	require.NoError(t, geo.DeleteEdge(edgeID))
	require.Equal(t, edgeID, geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right))
	require.NoError(t, geo.SetEdgeStyle(edgeID, lastStyle))
	require.NoError(t, geo.SetPinnedBends(edgeID, lastBends))
	require.NoError(t, geo.Build())
	require.NoError(t, transaction.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	style, ok := geo.EdgeStyle(edgeID)
	require.True(t, ok)
	require.Equal(t, layout.EdgeStyle{}, style)
	bends, err := geo.PinnedBends(edgeID)
	require.NoError(t, err)
	require.Empty(t, bends)

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	style, ok = geo.EdgeStyle(edgeID)
	require.True(t, ok)
	require.Equal(t, lastStyle, style)
	bends, err = geo.PinnedBends(edgeID)
	require.NoError(t, err)
	require.Equal(t, lastBends, bends)
}

func TestHistoryUndoesDuplicateSelection(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 3))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(12, 3))
	require.NoError(t, err)
	geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, geo.Build())
	history.Clear()

	geo.Selection().SelectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(t, geo.Selection().Toggle(layout.Hit{ID: right, Kind: layout.HitNode}))
	transaction := history.Begin()
	require.NoError(t, geo.DuplicateSelection(4, 5))
	require.NoError(t, geo.Build())
	require.NoError(t, transaction.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, geo.Graph().Nodes, 4)
	require.False(t, geo.NodeExists(2))
	require.False(t, geo.NodeExists(3))
}

func TestHistoryReplaysRouterChange(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	before := geo.Router()
	after := before
	after.Costs.Crossing++
	geo.SetRouter(after)
	require.Equal(t, after, geo.Router())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, before, geo.Router())
}

func TestHistoryReplaysTranslation(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	a, err := geo.NewNodeAt("a", layout.NewPoint(2, 3))
	require.NoError(t, err)
	b, err := geo.NewNodeAt("b", layout.NewPoint(14, 3))
	require.NoError(t, err)
	geo.ConnectNodes(a, ir.RightSide, ir.LeftSide, b)
	require.NoError(t, geo.Build())
	history.Clear()

	transaction := history.Begin()
	require.NoError(t, geo.Translate(5, 7))
	require.NoError(t, transaction.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, layout.NewPoint(2, 3), geo.Nodes[a].Rect.Min)
	require.Equal(t, layout.NewPoint(14, 3), geo.Nodes[b].Rect.Min)
	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, layout.NewPoint(7, 10), geo.Nodes[a].Rect.Min)
	require.Equal(t, layout.NewPoint(19, 10), geo.Nodes[b].Rect.Min)
}

func TestHistoryReplaysLayerChanges(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	back, err := geo.NewNode("back")
	require.NoError(t, err)
	front, err := geo.NewNode("front")
	require.NoError(t, err)
	history.Clear()

	backHit := layout.Hit{ID: back, Kind: layout.HitNode}
	require.NoError(t, geo.BringToFront(backHit))
	require.Equal(t, backHit, slices.Collect(geo.DrawOrder())[1])

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []layout.Hit{
		{ID: back, Kind: layout.HitNode},
		{ID: front, Kind: layout.HitNode},
	}, slices.Collect(geo.DrawOrder()))

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, backHit, slices.Collect(geo.DrawOrder())[1])
}

func TestHistoryReplaysInterleavedLayerTransaction(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	first, err := geo.NewNode("first")
	require.NoError(t, err)
	second, err := geo.NewNode("second")
	require.NoError(t, err)
	third, err := geo.NewNode("third")
	require.NoError(t, err)
	history.Clear()

	firstHit := layout.Hit{ID: first, Kind: layout.HitNode}
	secondHit := layout.Hit{ID: second, Kind: layout.HitNode}
	initial := []layout.Hit{firstHit, secondHit, {ID: third, Kind: layout.HitNode}}
	transaction := history.Begin()
	require.NoError(t, geo.BringToFront(firstHit))
	require.NoError(t, geo.BringToFront(secondHit))
	require.NoError(t, geo.SendToBack(firstHit))
	require.NoError(t, transaction.Commit())
	final := slices.Collect(geo.DrawOrder())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, initial, slices.Collect(geo.DrawOrder()))
	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, final, slices.Collect(geo.DrawOrder()))
}

func TestHistoryRestoresDeletedNodeLayerOrder(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	middle, err := geo.NewNodeAt("middle", layout.NewPoint(20, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(40, 2))
	require.NoError(t, err)
	firstEdge := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, middle)
	geo.ConnectNodes(middle, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, geo.SendToBack(layout.Hit{ID: firstEdge, Kind: layout.HitEdge}))
	before := slices.Collect(geo.DrawOrder())
	history.Clear()

	require.NoError(t, geo.DeleteNode(middle))
	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, before, slices.Collect(geo.DrawOrder()))
}

func TestStylesRoundTripThroughHistory(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	left, err := geo.NewNodeAt("left", layout.NewPoint(1, 1))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(20, 1))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, geo.Build())
	history.Clear()

	transaction := history.Begin()
	nodeStyle := layout.NodeStyle{Border: layout.BorderRounded}
	edgeStyle := layout.EdgeStyle{PortAArrow: layout.ArrowOpen, PortBArrow: layout.ArrowFilled}
	require.NoError(t, geo.SetNodeStyle(left, nodeStyle))
	require.NoError(t, geo.SetEdgeStyle(edgeID, edgeStyle))
	require.NoError(t, transaction.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, layout.NodeStyle{}, mustNodeStyle(t, geo, left))
	require.Equal(t, layout.EdgeStyle{}, mustEdgeStyle(t, geo, edgeID))

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, nodeStyle, mustNodeStyle(t, geo, left))
	require.Equal(t, edgeStyle, mustEdgeStyle(t, geo, edgeID))
}

func TestDeleteNodeHistoryRestoresStyles(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	left, err := geo.NewNodeAt("left", layout.NewPoint(1, 1))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(20, 1))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	nodeStyle := layout.NodeStyle{Border: layout.BorderNone}
	edgeStyle := layout.EdgeStyle{PortBArrow: layout.ArrowOpen}
	require.NoError(t, geo.SetNodeStyle(left, nodeStyle))
	require.NoError(t, geo.SetEdgeStyle(edgeID, edgeStyle))
	require.NoError(t, geo.Build())
	history.Clear()

	require.NoError(t, geo.DeleteNode(left))
	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, nodeStyle, mustNodeStyle(t, geo, left))
	require.Equal(t, edgeStyle, mustEdgeStyle(t, geo, edgeID))
}

func newHistoryAttachmentLayout(
	t testing.TB,
) (*layout.Layout, *History, uint32, uint32, uint32, uint32) {
	t.Helper()

	geo, history := newHistoryLayout(t)
	source, err := geo.NewNodeAt("source", layout.NewPoint(2, 4))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", layout.NewPoint(30, 4))
	require.NoError(t, err)
	node, err := geo.NewNodeAt("tag", layout.NewPoint(12, 15))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	point := midpointOnRoute(t, geo.Edges[edge].Points)
	anchor := layout.NewPoint(2, 1)
	require.Greater(t, point.X, anchor.X)
	require.Greater(t, point.Y, anchor.Y)
	require.NoError(t, geo.PlaceNode(node, layout.NewPoint(point.X-anchor.X, point.Y-anchor.Y)))
	require.NoError(t, geo.AttachNode(node, edge, point))
	history.Clear()
	return geo, history, source, destination, node, edge
}
