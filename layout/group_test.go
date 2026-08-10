package layout

import (
	"slices"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
)

func TestLayoutNestedGroupBounds(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(2, 3))
	require.NoError(t, err)
	middle, err := geo.NewNodeAt("middle", NewPoint(20, 8))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(40, 1))
	require.NoError(t, err)
	shape, err := geo.NewGroup([]ir.Member{
		{ID: left, Kind: ir.MemberNode},
		{ID: middle, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	section, err := geo.NewGroup([]ir.Member{
		{ID: shape, Kind: ir.MemberGroup},
		{ID: right, Kind: ir.MemberNode},
	})
	require.NoError(t, err)

	bounds, ok := geo.GroupBounds(section)
	require.True(t, ok)
	require.Equal(t, NewPoint(2, 1), bounds.Min)
	require.Equal(t, geo.Nodes[right].Rect.Max().X-2, bounds.Size.Width)
	require.Equal(t, max(geo.Nodes[left].Rect.Max().Y, geo.Nodes[middle].Rect.Max().Y)-1, bounds.Size.Height)
	require.Equal(t, []uint32{left, middle, right}, slices.Collect(geo.GroupNodes(section)))
}

func TestLayoutGroupChangesReplay(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNode("left")
	require.NoError(t, err)
	right, err := geo.NewNode("right")
	require.NoError(t, err)
	var changes []Change
	require.NoError(t, geo.SetChangeCallback(func(change Change) {
		changes = append(changes, change)
	}))
	groupID, err := geo.NewGroup([]ir.Member{
		{ID: left, Kind: ir.MemberNode},
		{ID: right, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	require.Len(t, changes, 1)

	require.NoError(t, geo.Replay(changes, ReplayBackward))
	require.False(t, geo.GroupExists(groupID))
	require.NoError(t, geo.Replay(changes, ReplayForward))
	require.True(t, geo.GroupExists(groupID))

	changes = changes[:0]
	_, err = geo.Ungroup(groupID)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.NoError(t, geo.Replay(changes, ReplayBackward))
	require.True(t, geo.GroupExists(groupID))
}

func TestLayoutDeleteNodeRestoresGroupOnReplay(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNode("left")
	require.NoError(t, err)
	right, err := geo.NewNode("right")
	require.NoError(t, err)
	groupID, err := geo.NewGroup([]ir.Member{
		{ID: left, Kind: ir.MemberNode},
		{ID: right, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	var changes []Change
	require.NoError(t, geo.SetChangeCallback(func(change Change) {
		changes = append(changes, change)
	}))
	require.NoError(t, geo.DeleteNode(left))
	require.False(t, geo.GroupExists(groupID))
	require.Len(t, changes, 1)

	require.NoError(t, geo.Replay(changes, ReplayBackward))
	require.True(t, geo.NodeExists(left))
	require.True(t, geo.GroupExists(groupID))
	require.NoError(t, geo.Replay(changes, ReplayForward))
	require.False(t, geo.NodeExists(left))
	require.False(t, geo.GroupExists(groupID))
}
