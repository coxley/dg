package layout

import (
	"errors"
	"fmt"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestHistoryCommitsInteractionAsOneStep(t *testing.T) {
	t.Parallel()

	history, err := NewHistory()
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("before", NewPoint(2, 3))
	require.NoError(t, err)

	transaction := history.Begin()
	require.NoError(t, geo.SetNodeLabel(nodeID, "during"))
	require.NoError(t, geo.SetNodeLabel(nodeID, "after"))
	require.NoError(t, geo.PlaceNode(nodeID, NewPoint(20, 30)))
	require.NoError(t, transaction.Commit())
	require.NoError(t, geo.Build())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "before", geo.Label(nodeID))
	require.Equal(t, NewPoint(2, 3), geo.Nodes[nodeID].Rect.Min)

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "after", geo.Label(nodeID))
	require.Equal(t, NewPoint(20, 30), geo.Nodes[nodeID].Rect.Min)
}

func TestHistoryCancelRestoresDeletedNodeAndEdges(t *testing.T) {
	t.Parallel()

	history, err := NewHistory()
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(20, 2))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, geo.Build())

	transaction := history.Begin()
	require.NoError(t, geo.DeleteNode(left))
	require.NoError(t, transaction.Cancel())

	require.True(t, geo.NodeExists(left))
	require.True(t, geo.EdgeExists(edgeID))
	require.Equal(t, "left", geo.Label(left))
	require.Equal(t, NewPoint(2, 2), geo.Nodes[left].Rect.Min)
	require.NotEmpty(t, geo.Edges[edgeID].Points)
}

func TestHistoryInterruptRejectsStaleTransaction(t *testing.T) {
	t.Parallel()

	history, err := NewHistory()
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("node", NewPoint(2, 2))
	require.NoError(t, err)

	stale := history.Begin()
	require.NoError(t, geo.PlaceNode(nodeID, NewPoint(5, 5)))
	history.Interrupt()
	current := history.Begin()

	require.ErrorIs(t, stale.Commit(), ErrTransactionClosed)
	require.ErrorIs(t, stale.Cancel(), ErrTransactionClosed)
	require.NoError(t, geo.PlaceNode(nodeID, NewPoint(8, 8)))
	require.NoError(t, current.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, NewPoint(5, 5), geo.Nodes[nodeID].Rect.Min)
	changed, err = history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, NewPoint(2, 2), geo.Nodes[nodeID].Rect.Min)
}

func TestHistoryLimitDropsOldestInteraction(t *testing.T) {
	t.Parallel()

	history, err := NewHistory(WithHistoryLimit(2))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	nodeID, err := geo.NewNode("one")
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeLabel(nodeID, "two"))
	require.NoError(t, geo.SetNodeLabel(nodeID, "three"))

	for range 2 {
		changed, err := history.Undo()
		require.NoError(t, err)
		require.True(t, changed)
	}
	changed, err := history.Undo()
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, "one", geo.Label(nodeID))
}

func TestHistoryCannotAttachToMultipleLayouts(t *testing.T) {
	t.Parallel()

	history, err := NewHistory()
	require.NoError(t, err)
	_, err = New(WithHistory(history))
	require.NoError(t, err)
	_, err = New(WithHistory(history))
	require.True(t, errors.Is(err, ErrHistoryAttached))
}

func TestHistoryReplaysEdgeLifecycle(t *testing.T) {
	t.Parallel()

	history, err := NewHistory()
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(1, 1))
	require.NoError(t, err)
	middle, err := geo.NewNodeAt("middle", NewPoint(20, 1))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(40, 1))
	require.NoError(t, err)
	leftPort, ok := geo.graph.PickCenterPort(left, ir.RightSide)
	require.True(t, ok)
	middlePort, ok := geo.graph.PickCenterPort(middle, ir.LeftSide)
	require.True(t, ok)
	rightPort, ok := geo.graph.PickCenterPort(right, ir.LeftSide)
	require.True(t, ok)
	history.Clear()

	edgeID, err := geo.ConnectPorts(leftPort, middlePort)
	require.NoError(t, err)
	require.NoError(t, geo.ReconnectEdge(edgeID, middlePort, rightPort))
	require.NoError(t, geo.DeleteEdge(edgeID))

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	portA, portB, err := geo.EdgePorts(edgeID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint32{leftPort, rightPort}, []uint32{portA, portB})

	changed, err = history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	portA, portB, err = geo.EdgePorts(edgeID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint32{leftPort, middlePort}, []uint32{portA, portB})

	changed, err = history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, geo.EdgeExists(edgeID))

	for range 3 {
		changed, err = history.Redo()
		require.NoError(t, err)
		require.True(t, changed)
	}
	require.False(t, geo.EdgeExists(edgeID))
}

func TestHistoryTransactionProperties(t *testing.T) {
	t.Parallel()

	type state struct {
		label  string
		origin Point
	}

	rapid.Check(t, func(t *rapid.T) {
		history, err := NewHistory()
		require.NoError(t, err)
		geo, err := New(WithHistory(history))
		require.NoError(t, err)
		nodeID, err := geo.NewNodeAt("initial", NewPoint(1, 1))
		require.NoError(t, err)
		history.Clear()

		states := []state{{label: "initial", origin: NewPoint(1, 1)}}
		transactionCount := rapid.IntRange(1, 30).Draw(t, "transaction count")
		for transactionID := range transactionCount {
			before := states[len(states)-1]
			transaction := history.Begin()
			operationCount := rapid.IntRange(1, 10).
				Draw(t, fmt.Sprintf("operation count %d", transactionID))
			for operationID := range operationCount {
				if rapid.Bool().Draw(t, fmt.Sprintf("label operation %d %d", transactionID, operationID)) {
					label := rapid.StringMatching(`[a-z]{0,8}`).
						Draw(t, fmt.Sprintf("label %d %d", transactionID, operationID))
					require.NoError(t, geo.SetNodeLabel(nodeID, label))
				} else {
					point := NewPoint(
						rapid.Uint32Range(0, 30).
							Draw(t, fmt.Sprintf("x %d %d", transactionID, operationID)),
						rapid.Uint32Range(0, 30).
							Draw(t, fmt.Sprintf("y %d %d", transactionID, operationID)),
					)
					require.NoError(t, geo.PlaceNode(nodeID, point))
				}
			}
			after := state{
				label:  geo.Label(nodeID),
				origin: geo.Nodes[nodeID].Rect.Min,
			}

			if rapid.Bool().Draw(t, fmt.Sprintf("cancel %d", transactionID)) {
				require.NoError(t, transaction.Cancel())
				require.Equal(t, before.label, geo.Label(nodeID))
				require.Equal(t, before.origin, geo.Nodes[nodeID].Rect.Min)
				continue
			}
			if rapid.Bool().Draw(t, fmt.Sprintf("interrupt %d", transactionID)) {
				history.Interrupt()
				require.ErrorIs(t, transaction.Commit(), ErrTransactionClosed)
			} else {
				require.NoError(t, transaction.Commit())
			}
			if after != before {
				states = append(states, after)
			}
		}

		for i := len(states) - 2; i >= 0; i-- {
			changed, err := history.Undo()
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, states[i].label, geo.Label(nodeID))
			require.Equal(t, states[i].origin, geo.Nodes[nodeID].Rect.Min)
		}
		changed, err := history.Undo()
		require.NoError(t, err)
		require.False(t, changed)

		for i := 1; i < len(states); i++ {
			changed, err := history.Redo()
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, states[i].label, geo.Label(nodeID))
			require.Equal(t, states[i].origin, geo.Nodes[nodeID].Rect.Min)
		}
		changed, err = history.Redo()
		require.NoError(t, err)
		require.False(t, changed)
	})
}
