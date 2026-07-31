package layout

import (
	"errors"
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestDrawOrderFollowsCreationAndReordering(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	back, err := geo.NewNodeAt("back", NewPoint(1, 1))
	require.NoError(t, err)
	front, err := geo.NewNodeAt("front", NewPoint(20, 1))
	require.NoError(t, err)
	edge := geo.ConnectNodes(back, ir.RightSide, ir.LeftSide, front)

	backHit := Hit{ID: back, Kind: HitNode}
	frontHit := Hit{ID: front, Kind: HitNode}
	edgeHit := Hit{ID: edge, Kind: HitEdge}
	require.Equal(
		t,
		[]Hit{backHit, frontHit, edgeHit},
		slices.Collect(geo.DrawOrder()),
	)

	require.NoError(t, geo.SendToBack(edgeHit))
	require.Equal(
		t,
		[]Hit{edgeHit, backHit, frontHit},
		slices.Collect(geo.DrawOrder()),
	)
	require.NoError(t, geo.BringForward(edgeHit))
	require.NoError(t, geo.BringToFront(backHit))
	require.Equal(
		t,
		[]Hit{edgeHit, frontHit, backHit},
		slices.Collect(geo.DrawOrder()),
	)
	require.NoError(t, geo.SendBackward(backHit))
	require.Equal(
		t,
		[]Hit{edgeHit, backHit, frontHit},
		slices.Collect(geo.DrawOrder()),
	)

	portID := geo.graph.Nodes[back].Ports[0]
	require.ErrorIs(
		t,
		geo.BringToFront(Hit{ID: portID, Kind: HitPort}),
		ErrLayerObject,
	)
	require.ErrorIs(
		t,
		geo.BringToFront(Hit{ID: 100, Kind: HitNode}),
		ErrLayerObject,
	)
}

func TestDrawOrderReusesDeletedIDsAtFront(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	first, err := geo.NewNode("first")
	require.NoError(t, err)
	deleted, err := geo.NewNode("deleted")
	require.NoError(t, err)
	require.NoError(t, geo.DeleteNode(deleted))

	replacement, err := geo.NewNode("replacement")
	require.NoError(t, err)
	require.Equal(t, deleted, replacement)
	require.Equal(t, []Hit{
		{ID: first, Kind: HitNode},
		{ID: replacement, Kind: HitNode},
	}, slices.Collect(geo.DrawOrder()))
}

func TestDrawOrderOptionValidatesCompletePermutation(t *testing.T) {
	t.Parallel()

	var graph ir.Graph
	nodeID := graph.NewNode("node")
	_, err := New(
		WithGraph(graph),
		WithDrawOrder([]Hit{
			{ID: nodeID, Kind: HitNode},
			{ID: nodeID, Kind: HitNode},
		}),
	)
	require.ErrorContains(t, err, "contains 2 objects, want 1")

	_, err = New(
		WithGraph(graph),
		WithDrawOrder([]Hit{{ID: 0, Kind: HitPort}}),
	)
	require.True(t, errors.Is(err, ErrLayerObject))
}

func TestHitsRespectVisibleLayer(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	back, err := geo.NewNodeAt("back", NewPoint(2, 2))
	require.NoError(t, err)
	front, err := geo.NewNodeAt("front", NewPoint(2, 2))
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeSize(back, Size{Width: 9, Height: 3}))
	require.NoError(t, geo.SetNodeSize(front, Size{Width: 9, Height: 3}))
	require.NoError(t, geo.Build())

	point := NewPoint(4, 3)
	require.Equal(
		t,
		[]Hit{{ID: front, Kind: HitNode}},
		slices.Collect(geo.Hits(point)),
	)
	require.NoError(t, geo.SendToBack(Hit{ID: front, Kind: HitNode}))
	require.Equal(
		t,
		[]Hit{{ID: back, Kind: HitNode}},
		slices.Collect(geo.Hits(point)),
	)
}
