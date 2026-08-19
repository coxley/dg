package history

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestHistoryReloadParticipatesInUndoChain(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	nodeID, err := geo.NewNode("original")
	require.NoError(t, err)
	history.Clear()

	require.NoError(t, history.Reload(func() error {
		return geo.SetNodeLabel(nodeID, "modified")
	}))
	require.NoError(t, geo.SetNodeLabel(nodeID, "modified+edit"))

	for _, want := range []string{"modified", "original"} {
		changed, undoErr := history.Undo()
		require.NoError(t, undoErr)
		require.True(t, changed)
		require.Equal(t, want, geo.Label(nodeID))
	}
	for _, want := range []string{"modified", "modified+edit"} {
		changed, redoErr := history.Redo()
		require.NoError(t, redoErr)
		require.True(t, changed)
		require.Equal(t, want, geo.Label(nodeID))
	}
}

func TestHistoryReloadKeepsOnlyLatestBoundary(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	nodeID, err := geo.NewNode("original")
	require.NoError(t, err)
	history.Clear()
	require.NoError(t, history.Reload(func() error {
		return geo.SetNodeLabel(nodeID, "first reload")
	}))
	require.NoError(t, geo.SetNodeLabel(nodeID, "between"))
	require.NoError(t, history.Reload(func() error {
		return geo.SetNodeLabel(nodeID, "second reload")
	}))

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "between", geo.Label(nodeID))
	changed, err = history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "first reload", geo.Label(nodeID))
	changed, err = history.Undo()
	require.NoError(t, err)
	require.False(t, changed)
}

func TestHistoryReloadRollsBackFailure(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	nodeID, err := geo.NewNode("original")
	require.NoError(t, err)
	history.Clear()
	wantErr := errors.New("replace failed")
	err = history.Reload(func() error {
		require.NoError(t, geo.SetNodeLabel(nodeID, "partial"))
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "original", geo.Label(nodeID))
	require.False(t, history.CanUndo())
}

func TestHistoryCommitsInteractionAsOneStep(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo)
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("before", layout.NewPoint(2, 3))
	require.NoError(t, err)

	transaction := history.Begin()
	require.NoError(t, geo.SetNodeLabel(nodeID, "during"))
	require.NoError(t, geo.SetNodeLabel(nodeID, "after"))
	require.NoError(t, geo.PlaceNode(nodeID, layout.NewPoint(20, 30)))
	require.NoError(t, transaction.Commit())
	require.NoError(t, geo.Build())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "before", geo.Label(nodeID))
	require.Equal(t, layout.NewPoint(2, 3), geo.Nodes[nodeID].Rect.Min)

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "after", geo.Label(nodeID))
	require.Equal(t, layout.NewPoint(20, 30), geo.Nodes[nodeID].Rect.Min)
}

func TestHistoryCancelRestoresDeletedNodeAndEdges(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo)
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(20, 2))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, geo.Build())

	transaction := history.Begin()
	require.NoError(t, geo.DeleteNode(left))
	require.NoError(t, transaction.Cancel())

	require.True(t, geo.NodeExists(left))
	require.True(t, geo.EdgeExists(edgeID))
	require.Equal(t, "left", geo.Label(left))
	require.Equal(t, layout.NewPoint(2, 2), geo.Nodes[left].Rect.Min)
	require.NotEmpty(t, geo.Edges[edgeID].Points)
}

func TestHistoryInterruptRejectsStaleTransaction(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo)
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("node", layout.NewPoint(2, 2))
	require.NoError(t, err)

	stale := history.Begin()
	require.NoError(t, geo.PlaceNode(nodeID, layout.NewPoint(5, 5)))
	history.Interrupt()
	current := history.Begin()

	require.ErrorIs(t, stale.Commit(), ErrTransactionClosed)
	require.ErrorIs(t, stale.Cancel(), ErrTransactionClosed)
	require.NoError(t, geo.PlaceNode(nodeID, layout.NewPoint(8, 8)))
	require.NoError(t, current.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, layout.NewPoint(5, 5), geo.Nodes[nodeID].Rect.Min)
	changed, err = history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, layout.NewPoint(2, 2), geo.Nodes[nodeID].Rect.Min)
}

func TestHistoryCrossesUnbuildableIntermediateState(t *testing.T) {
	t.Parallel()

	geo, history := newHistoryLayout(t)
	source, err := geo.NewNodeAt("source", layout.NewPoint(2, 2))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", layout.NewPoint(24, 2))
	require.NoError(t, err)
	blocker, err := geo.NewNodeAt("blocker", layout.NewPoint(12, 12))
	require.NoError(t, err)
	port, ok := centerPort(geo, source, ir.RightSide)
	require.True(t, ok)
	geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	history.Clear()

	initial := geo.Nodes[blocker].Rect.Min
	bad := geo.Ports[port].Exit
	good := layout.NewPoint(16, 12)
	require.NoError(t, geo.PlaceNode(blocker, bad))
	require.ErrorIs(t, geo.Build(), layout.ErrNoRoute)
	require.NoError(t, geo.PlaceNode(blocker, good))
	require.NoError(t, geo.Build())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, initial, geo.Nodes[blocker].Rect.Min)
	require.False(t, history.CanUndo())
	require.True(t, history.CanRedo())

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, good, geo.Nodes[blocker].Rect.Min)
	require.True(t, history.CanUndo())
	require.False(t, history.CanRedo())
}

func TestHistoryLimitDropsOldestInteraction(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo, WithLimit(2))
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

func TestHistoryRejectsSecondCallbackAttachment(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	_, err = New(geo)
	require.NoError(t, err)
	_, err = New(geo)
	require.ErrorIs(t, err, ErrAttached)
	require.ErrorIs(
		t,
		geo.SetChangeCallback(func(layout.Change) {}),
		layout.ErrChangeCallbackAttached,
	)
}

func TestHistoryResetReplacesLayoutAndClearsHistory(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo)
	require.NoError(t, err)
	nodeID, err := geo.NewNode("before")
	require.NoError(t, err)
	history.Clear()

	transaction := history.Begin()
	require.NoError(t, geo.SetNodeLabel(nodeID, "active"))
	replacement, err := layout.New()
	require.NoError(t, err)
	replacementID, err := replacement.NewNodeAt("replacement", layout.NewPoint(8, 9))
	require.NoError(t, err)

	require.NoError(t, history.Reset(func() error {
		return geo.Restore(replacement.Snapshot())
	}))
	require.Same(t, geo, history.Layout())
	require.ErrorIs(t, transaction.Commit(), ErrTransactionClosed)
	require.Equal(t, "replacement", geo.Label(replacementID))
	require.Equal(t, layout.NewPoint(8, 9), geo.Nodes[replacementID].Rect.Min)
	require.False(t, history.CanUndo())
	require.False(t, history.CanRedo())

	require.NoError(t, geo.SetNodeLabel(replacementID, "after reset"))
	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "replacement", geo.Label(replacementID))
}

func TestHistoryResetFailureRestoresLayoutAndActiveTransaction(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo)
	require.NoError(t, err)
	nodeID, err := geo.NewNode("before")
	require.NoError(t, err)
	history.Clear()

	transaction := history.Begin()
	require.NoError(t, geo.SetNodeLabel(nodeID, "active"))
	resetErr := errors.New("replace failed")
	err = history.Reset(func() error {
		require.NoError(t, geo.SetNodeLabel(nodeID, "replacement"))
		return resetErr
	})
	require.ErrorIs(t, err, resetErr)
	require.Equal(t, "active", geo.Label(nodeID))
	require.NoError(t, transaction.Commit())

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "before", geo.Label(nodeID))
	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "active", geo.Label(nodeID))

	next := history.Begin()
	require.NoError(t, geo.SetNodeLabel(nodeID, "after failure"))
	require.NoError(t, next.Commit())
	changed, err = history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "active", geo.Label(nodeID))
}

func TestHistoryReplaysEdgeLifecycle(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo)
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", layout.NewPoint(1, 1))
	require.NoError(t, err)
	middle, err := geo.NewNodeAt("middle", layout.NewPoint(20, 1))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(40, 1))
	require.NoError(t, err)
	leftPort, ok := centerPort(geo, left, ir.RightSide)
	require.True(t, ok)
	middlePort, ok := centerPort(geo, middle, ir.LeftSide)
	require.True(t, ok)
	rightPort, ok := centerPort(geo, right, ir.LeftSide)
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

func TestHistoryReplaysExplicitAndAutoNodeSize(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo)
	require.NoError(t, err)
	nodeID, err := geo.NewNode("one two three")
	require.NoError(t, err)
	history.Clear()

	explicit := layout.Size{Width: 8, Height: 4}
	require.NoError(t, geo.SetNodeSize(nodeID, explicit))
	require.NoError(t, geo.AutoSizeNode(nodeID))

	changed, err := history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, explicit, explicitNodeSize(t, geo, nodeID))

	changed, err = history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	_, ok := geo.ExplicitNodeSize(nodeID)
	require.False(t, ok)

	changed, err = history.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, explicit, explicitNodeSize(t, geo, nodeID))
}

func TestHistoryTransactionProperties(t *testing.T) {
	t.Parallel()

	type state struct {
		label  string
		origin layout.Point
		size   layout.Size
		order  []layout.Hit
	}

	rapid.Check(t, func(t *rapid.T) {
		geo, err := layout.New()
		require.NoError(t, err)
		history, err := New(geo)
		require.NoError(t, err)
		nodeID, err := geo.NewNodeAt("initial", layout.NewPoint(1, 1))
		require.NoError(t, err)
		_, err = geo.NewNodeAt("other", layout.NewPoint(20, 1))
		require.NoError(t, err)
		history.Clear()

		states := []state{{
			label:  "initial",
			origin: layout.NewPoint(1, 1),
			order:  slices.Collect(geo.DrawOrder()),
		}}
		transactionCount := rapid.IntRange(1, 30).Draw(t, "transaction count")
		for transactionID := range transactionCount {
			before := states[len(states)-1]
			transaction := history.Begin()
			operationCount := rapid.IntRange(1, 10).
				Draw(t, fmt.Sprintf("operation count %d", transactionID))
			for operationID := range operationCount {
				switch rapid.IntRange(0, 3).
					Draw(t, fmt.Sprintf("operation %d %d", transactionID, operationID)) {
				case 0:
					label := rapid.StringMatching(`[a-z\n]{0,8}`).
						Draw(t, fmt.Sprintf("label %d %d", transactionID, operationID))
					require.NoError(t, geo.SetNodeLabel(nodeID, label))
				case 1:
					point := layout.NewPoint(
						rapid.Uint32Range(0, 30).
							Draw(t, fmt.Sprintf("x %d %d", transactionID, operationID)),
						rapid.Uint32Range(0, 30).
							Draw(t, fmt.Sprintf("y %d %d", transactionID, operationID)),
					)
					require.NoError(t, geo.PlaceNode(nodeID, point))
				case 2:
					if rapid.Bool().Draw(
						t,
						fmt.Sprintf("auto size %d %d", transactionID, operationID),
					) {
						require.NoError(t, geo.AutoSizeNode(nodeID))
					} else {
						require.NoError(t, geo.SetNodeSize(nodeID, layout.Size{
							Width: rapid.Uint32Range(4, 20).
								Draw(t, fmt.Sprintf("width %d %d", transactionID, operationID)),
							Height: rapid.Uint32Range(2, 10).
								Draw(t, fmt.Sprintf("height %d %d", transactionID, operationID)),
						}))
					}
				case 3:
					hit := layout.Hit{ID: nodeID, Kind: layout.HitNode}
					if rapid.Bool().Draw(
						t,
						fmt.Sprintf("front %d %d", transactionID, operationID),
					) {
						require.NoError(t, geo.BringToFront(hit))
					} else {
						require.NoError(t, geo.SendToBack(hit))
					}
				}
			}
			size, _ := geo.ExplicitNodeSize(nodeID)
			after := state{
				label:  geo.Label(nodeID),
				origin: geo.Nodes[nodeID].Rect.Min,
				size:   size,
				order:  slices.Collect(geo.DrawOrder()),
			}

			if rapid.Bool().Draw(t, fmt.Sprintf("cancel %d", transactionID)) {
				require.NoError(t, transaction.Cancel())
				require.Equal(t, before.label, geo.Label(nodeID))
				require.Equal(t, before.origin, geo.Nodes[nodeID].Rect.Min)
				size, _ := geo.ExplicitNodeSize(nodeID)
				require.Equal(t, before.size, size)
				require.Equal(t, before.order, slices.Collect(geo.DrawOrder()))
				continue
			}
			if rapid.Bool().Draw(t, fmt.Sprintf("interrupt %d", transactionID)) {
				history.Interrupt()
				require.ErrorIs(t, transaction.Commit(), ErrTransactionClosed)
			} else {
				require.NoError(t, transaction.Commit())
			}
			if after.label != before.label ||
				after.origin != before.origin ||
				after.size != before.size ||
				!slices.Equal(after.order, before.order) {
				states = append(states, after)
			}
		}

		for i := len(states) - 2; i >= 0; i-- {
			changed, err := history.Undo()
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, states[i].label, geo.Label(nodeID))
			require.Equal(t, states[i].origin, geo.Nodes[nodeID].Rect.Min)
			size, _ := geo.ExplicitNodeSize(nodeID)
			require.Equal(t, states[i].size, size)
			require.Equal(t, states[i].order, slices.Collect(geo.DrawOrder()))
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
			size, _ := geo.ExplicitNodeSize(nodeID)
			require.Equal(t, states[i].size, size)
			require.Equal(t, states[i].order, slices.Collect(geo.DrawOrder()))
		}
		changed, err = history.Redo()
		require.NoError(t, err)
		require.False(t, changed)
	})
}

func centerPort(geo *layout.Layout, nodeID uint32, side ir.Side) (uint32, bool) {
	graph := geo.Graph()
	for portID := range geo.NodePorts(nodeID) {
		if graph.Ports[portID].Side == side && geo.PortUsable(portID) {
			return portID, true
		}
	}
	return 0, false
}

func explicitNodeSize(t testing.TB, geo *layout.Layout, nodeID uint32) layout.Size {
	t.Helper()
	size, ok := geo.ExplicitNodeSize(nodeID)
	require.True(t, ok)
	return size
}
