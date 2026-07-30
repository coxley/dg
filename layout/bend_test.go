package layout

import (
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestPinnedBendsConstrainRouteInOrder(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", NewPoint(2, 2))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", NewPoint(22, 12))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())

	portA, portB, err := geo.EdgePorts(edgeID)
	require.NoError(t, err)
	a, b := geo.Ports[portA], geo.Ports[portB]
	x := (a.Exit.X + b.Exit.X) / 2
	bends := []PinnedBend{
		{
			Point:    NewPoint(x, a.Exit.Y),
			Incoming: East,
			Outgoing: South,
		},
		{
			Point:    NewPoint(x, b.Exit.Y),
			Incoming: South,
			Outgoing: East,
		},
	}
	require.NoError(t, geo.SetPinnedBends(edgeID, bends))
	require.NoError(t, geo.Build())

	points := geo.Edges[edgeID].Points
	first := requirePointIndex(t, points, bends[0].Point)
	second := requirePointIndex(t, points, bends[1].Point)
	require.Less(t, first, second)
	require.Equal(t, bends[0].Point.Y, points[first-1].Y)
	require.Less(t, points[first-1].X, bends[0].Point.X)
	require.Equal(t, bends[0].Point.X, points[first+1].X)
	require.Greater(t, points[first+1].Y, bends[0].Point.Y)
	require.Equal(t, bends[1].Point.X, points[second-1].X)
	require.Less(t, points[second-1].Y, bends[1].Point.Y)
	require.Equal(t, bends[1].Point.Y, points[second+1].Y)
	require.Greater(t, points[second+1].X, bends[1].Point.X)

	got, err := geo.PinnedBends(edgeID)
	require.NoError(t, err)
	require.Equal(t, bends, got)
	got[0].Point = Point{}
	require.Equal(t, bends[0], geo.edgeBends[edgeID][0])
}

func TestSetPinnedBendsRejectsInvalidTurn(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(20, 2))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)

	err = geo.SetPinnedBends(edgeID, []PinnedBend{{
		Point:    NewPoint(10, 3),
		Incoming: East,
		Outgoing: West,
	}})
	require.EqualError(t, err, "invalid pinned bend 0")
}

func TestPinnedBendsFollowHistoryCloneDuplicateAndTranslation(t *testing.T) {
	t.Parallel()

	history, err := NewHistory()
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(22, 12))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	bends := []PinnedBend{
		{Point: NewPoint(14, 3), Incoming: East, Outgoing: South},
		{Point: NewPoint(14, 13), Incoming: South, Outgoing: East},
	}
	require.NoError(t, geo.SetPinnedBends(edgeID, bends))
	require.NoError(t, geo.Build())
	history.Clear()

	replacement := []PinnedBend{
		{Point: NewPoint(16, 3), Incoming: East, Outgoing: South},
		{Point: NewPoint(16, 13), Incoming: South, Outgoing: East},
	}
	transaction := history.Begin()
	require.NoError(t, geo.SetPinnedBends(edgeID, replacement))
	require.NoError(t, geo.Build())
	require.NoError(t, transaction.Commit())
	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	got, err := geo.PinnedBends(edgeID)
	require.NoError(t, err)
	require.Equal(t, bends, got)
	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	got, err = geo.PinnedBends(edgeID)
	require.NoError(t, err)
	require.Equal(t, replacement, got)

	cloned, err := geo.Clone()
	require.NoError(t, err)
	clonedBends, err := cloned.PinnedBends(edgeID)
	require.NoError(t, err)
	require.Equal(t, replacement, clonedBends)

	geo.Selection().SelectOnly(Hit{ID: left, Kind: HitNode})
	require.True(t, geo.Selection().Toggle(Hit{ID: right, Kind: HitNode}))
	require.NoError(t, geo.DuplicateSelection(30, 5))
	var duplicateEdge uint32
	duplicateEdges := 0
	for selectedEdge := range geo.Selection().Edges() {
		duplicateEdge = selectedEdge
		duplicateEdges++
	}
	require.Equal(t, 1, duplicateEdges)
	duplicateBends, err := geo.PinnedBends(duplicateEdge)
	require.NoError(t, err)
	for i, bend := range replacement {
		require.Equal(t, bend.Point.Add(30, 5), duplicateBends[i].Point)
		require.Equal(t, bend.Incoming, duplicateBends[i].Incoming)
		require.Equal(t, bend.Outgoing, duplicateBends[i].Outgoing)
	}

	require.NoError(t, geo.Translate(4, 6))
	translated, err := geo.PinnedBends(edgeID)
	require.NoError(t, err)
	for i, bend := range replacement {
		require.Equal(t, bend.Point.Add(4, 6), translated[i].Point)
	}
}

func requirePointIndex(t *testing.T, points []Point, point Point) int {
	t.Helper()
	for i, candidate := range points {
		if candidate == point {
			require.NotZero(t, i)
			require.Less(t, i, len(points)-1)
			return i
		}
	}
	t.Fatalf("route %v does not contain %+v", points, point)
	return 0
}
