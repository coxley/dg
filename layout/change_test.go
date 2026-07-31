package layout_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestChangeJSONUsesNestedStates(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	var changes []layout.Change
	require.NoError(t, geo.SetChangeCallback(func(change layout.Change) {
		changes = append(changes, change)
	}))
	nodeID, err := geo.NewNode("before")
	require.NoError(t, err)
	changes = changes[:0]
	require.NoError(t, geo.SetNodeLabel(nodeID, "after"))
	require.Len(t, changes, 1)

	data, err := json.Marshal(changes[0])
	require.NoError(t, err)
	var encoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &encoded))
	require.Contains(t, encoded, "before")
	require.Contains(t, encoded, "after")
	require.NotContains(t, encoded, "before_label")

	var before map[string]any
	require.NoError(t, json.Unmarshal(encoded["before"], &before))
	require.Equal(t, "before", before["label"])

	var decoded layout.Change
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NoError(t, geo.Replay([]layout.Change{decoded}, layout.ReplayBackward))
	require.Equal(t, "before", geo.Label(nodeID))
}

func TestReplayRejectsInvalidDirectionWithoutChangingLayout(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	var changes []layout.Change
	require.NoError(t, geo.SetChangeCallback(func(change layout.Change) {
		changes = append(changes, change)
	}))
	nodeID, err := geo.NewNode("node")
	require.NoError(t, err)
	require.Len(t, changes, 1)

	err = geo.Replay(changes, layout.ReplayDirection(255))
	require.ErrorContains(t, err, "invalid layout replay direction")
	require.True(t, geo.NodeExists(nodeID))
	require.Equal(t, "node", geo.Label(nodeID))
}

func TestReplayRejectsSparseCachedNodeID(t *testing.T) {
	t.Parallel()

	source, err := layout.New()
	require.NoError(t, err)
	var changes []layout.Change
	require.NoError(t, source.SetChangeCallback(func(change layout.Change) {
		changes = append(changes, change)
	}))
	_, err = source.NewNode("node")
	require.NoError(t, err)
	require.Len(t, changes, 1)

	data, err := json.Marshal(changes[0])
	require.NoError(t, err)
	var encoded map[string]any
	require.NoError(t, json.Unmarshal(data, &encoded))
	encoded["id"] = float64(math.MaxUint32)
	after := encoded["after"].(map[string]any)
	node := after["node"].(map[string]any)
	node["id"] = float64(math.MaxUint32)
	data, err = json.Marshal(encoded)
	require.NoError(t, err)
	var change layout.Change
	require.NoError(t, json.Unmarshal(data, &change))

	target, err := layout.New()
	require.NoError(t, err)
	err = target.Replay([]layout.Change{change}, layout.ReplayForward)
	require.ErrorContains(t, err, "occupied or sparse slot")
	require.Empty(t, target.Nodes)
}

func TestRestorePreservesChangeCallback(t *testing.T) {
	t.Parallel()

	replacement, err := layout.New()
	require.NoError(t, err)
	nodeID, err := replacement.NewNode("replacement")
	require.NoError(t, err)

	geo, err := layout.New()
	require.NoError(t, err)
	var changes []layout.Change
	require.NoError(t, geo.SetChangeCallback(func(change layout.Change) {
		changes = append(changes, change)
	}))
	require.NoError(t, geo.Restore(replacement.Snapshot()))
	require.Empty(t, changes)

	require.NoError(t, geo.SetNodeLabel(nodeID, "changed"))
	require.Len(t, changes, 1)
}

func TestRestoreClearsSelection(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	nodeID, err := geo.NewNode("selected")
	require.NoError(t, err)
	require.True(t, geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode}))

	replacement, err := layout.New()
	require.NoError(t, err)
	_, err = replacement.NewNode("replacement")
	require.NoError(t, err)
	require.NoError(t, geo.Restore(replacement.Snapshot()))

	require.True(t, geo.Selection().Empty())
}

func TestFailedReplayRestoresSelection(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	left, err := geo.NewNode("left")
	require.NoError(t, err)
	right, err := geo.NewNode("right")
	require.NoError(t, err)
	var changes []layout.Change
	require.NoError(t, geo.SetChangeCallback(func(change layout.Change) {
		changes = append(changes, change)
	}))
	require.NoError(t, geo.PlaceNode(left, layout.NewPoint(4, 5)))
	require.Len(t, changes, 1)
	require.NoError(t, geo.DeleteNode(left))
	require.True(t, geo.Selection().SelectOnly(layout.Hit{ID: right, Kind: layout.HitNode}))

	err = geo.Replay(changes[:1], layout.ReplayBackward)
	require.Error(t, err)
	require.True(t, geo.Selection().Contains(layout.Hit{ID: right, Kind: layout.HitNode}))
	nodes, edges := geo.Selection().Counts()
	require.Equal(t, 1, nodes)
	require.Zero(t, edges)
}
