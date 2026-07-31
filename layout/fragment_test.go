package layout

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestFragmentRoundTripPreservesContainedLayout(t *testing.T) {
	t.Parallel()

	source, attachedSource, attachedDestination, attachedNode, edgeID := newAttachmentLayout(t)
	require.NoError(t, source.SetNodeStyle(attachedSource, NodeStyle{
		Border:     BorderRounded,
		Horizontal: AlignCenter,
		Vertical:   AlignMiddle,
	}))
	require.NoError(t, source.SetEdgeStyle(edgeID, EdgeStyle{PortBArrow: ArrowFilled}))
	source.Selection().SelectOnly(Hit{ID: edgeID, Kind: HitEdge})
	require.True(t, source.Selection().Toggle(Hit{ID: attachedSource, Kind: HitNode}))
	require.True(t, source.Selection().Toggle(Hit{ID: attachedDestination, Kind: HitNode}))

	fragment, err := source.CopySelection()
	require.NoError(t, err)
	encoded, err := json.Marshal(fragment)
	require.NoError(t, err)
	var decoded Fragment
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	destination, err := New()
	require.NoError(t, err)
	require.NoError(t, destination.Paste(decoded, NewPoint(7, 9)))
	require.NoError(t, destination.Build())

	nodes, edges := destination.Selection().Counts()
	require.Equal(t, 3, nodes)
	require.Equal(t, 1, edges)
	labels := make(map[string]uint32, nodes)
	for nodeID := range destination.Selection().Nodes() {
		labels[destination.Label(nodeID)] = nodeID
	}
	require.Contains(t, labels, source.Label(attachedSource))
	require.Contains(t, labels, source.Label(attachedDestination))
	require.Contains(t, labels, source.Label(attachedNode))

	style, ok := destination.NodeStyle(labels[source.Label(attachedSource)])
	require.True(t, ok)
	require.Equal(t, NodeStyle{
		Border:     BorderRounded,
		Horizontal: AlignCenter,
		Vertical:   AlignMiddle,
	}, style)
	pastedEdge := onlySelectedEdge(t, destination)
	edgeStyle, ok := destination.EdgeStyle(pastedEdge)
	require.True(t, ok)
	require.Equal(t, EdgeStyle{PortBArrow: ArrowFilled}, edgeStyle)
	attachment, ok := destination.NodeAttachment(labels[source.Label(attachedNode)])
	require.True(t, ok)
	require.Equal(t, pastedEdge, attachment.EdgeID)
}

func TestFragmentOwnsCopiedValues(t *testing.T) {
	t.Parallel()

	source, err := New()
	require.NoError(t, err)
	nodeID, err := source.NewNodeAt("original", NewPoint(3, 4))
	require.NoError(t, err)
	require.True(t, source.Selection().SelectOnly(Hit{ID: nodeID, Kind: HitNode}))
	fragment, err := source.CopySelection()
	require.NoError(t, err)
	require.NoError(t, source.SetNodeLabel(nodeID, "changed"))

	destination, err := New()
	require.NoError(t, err)
	require.NoError(t, destination.Paste(fragment, NewPoint(10, 11)))
	pasted, ok := destination.Selection().FirstNode()
	require.True(t, ok)
	require.Equal(t, "original", destination.Label(pasted))
	require.Equal(t, NewPoint(10, 11), destination.Nodes[pasted].Rect.Min)
}

func TestFragmentExcludesExternalEdges(t *testing.T) {
	t.Parallel()

	source, err := New()
	require.NoError(t, err)
	left, err := source.NewNodeAt("left", NewPoint(2, 2))
	require.NoError(t, err)
	middle, err := source.NewNodeAt("middle", NewPoint(14, 2))
	require.NoError(t, err)
	right, err := source.NewNodeAt("right", NewPoint(26, 2))
	require.NoError(t, err)
	source.ConnectNodes(left, ir.RightSide, ir.LeftSide, middle)
	source.ConnectNodes(middle, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, source.Build())
	source.Selection().SelectOnly(Hit{ID: left, Kind: HitNode})
	require.True(t, source.Selection().Toggle(Hit{ID: middle, Kind: HitNode}))
	fragment, err := source.CopySelection()
	require.NoError(t, err)

	destination, err := New()
	require.NoError(t, err)
	require.NoError(t, destination.Paste(fragment, Point{}))
	nodes, edges := destination.Selection().Counts()
	require.Equal(t, 2, nodes)
	require.Equal(t, 1, edges)
}

func TestPasteRejectsOverflowBeforeMutation(t *testing.T) {
	t.Parallel()

	source, err := New()
	require.NoError(t, err)
	nodeID, err := source.NewNodeAt("node", NewPoint(3, 4))
	require.NoError(t, err)
	require.True(t, source.Selection().SelectOnly(Hit{ID: nodeID, Kind: HitNode}))
	fragment, err := source.CopySelection()
	require.NoError(t, err)

	destination, err := New()
	require.NoError(t, err)
	before := destination.historyState()
	err = destination.Paste(fragment, NewPoint(math.MaxUint32, math.MaxUint32))
	require.ErrorContains(t, err, "outside coordinate space")
	require.Equal(t, before, destination.historyState())
}

func TestFragmentJSONRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	var fragment Fragment
	err := json.Unmarshal([]byte(`{"version":1,"unknown":true}`), &fragment)
	require.ErrorContains(t, err, "unknown field")
}

func onlySelectedEdge(t *testing.T, geo *Layout) uint32 {
	t.Helper()
	var edgeID uint32
	count := 0
	for selected := range geo.Selection().Edges() {
		edgeID = selected
		count++
	}
	require.Equal(t, 1, count)
	return edgeID
}
