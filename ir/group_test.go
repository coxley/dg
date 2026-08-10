package ir

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGraphNestedGroups(t *testing.T) {
	t.Parallel()

	var graph Graph
	left := graph.NewNode("left")
	middle := graph.NewNode("middle")
	right := graph.NewNode("right")
	shape, err := graph.NewGroup([]Member{
		{ID: left, Kind: MemberNode},
		{ID: middle, Kind: MemberNode},
	})
	require.NoError(t, err)
	section, err := graph.NewGroup([]Member{
		{ID: shape, Kind: MemberGroup},
		{ID: right, Kind: MemberNode},
	})
	require.NoError(t, err)
	require.NoError(t, graph.Validate())

	parent, ok := graph.Parent(Member{ID: left, Kind: MemberNode})
	require.True(t, ok)
	require.Equal(t, shape, parent)
	parent, ok = graph.Parent(Member{ID: shape, Kind: MemberGroup})
	require.True(t, ok)
	require.Equal(t, section, parent)
	require.Equal(t, []uint32{left, middle, right}, slices.Collect(graph.DescendantNodes(section)))
	require.Equal(t, Member{ID: section, Kind: MemberGroup}, graph.Outermost(Member{ID: left, Kind: MemberNode}))
	child, ok := graph.ChildContaining(section, Member{ID: left, Kind: MemberNode})
	require.True(t, ok)
	require.Equal(t, Member{ID: shape, Kind: MemberGroup}, child)
	child, ok = graph.ChildContaining(shape, Member{ID: middle, Kind: MemberNode})
	require.True(t, ok)
	require.Equal(t, Member{ID: middle, Kind: MemberNode}, child)
}

func TestGraphGroupRequiresSiblings(t *testing.T) {
	t.Parallel()

	var graph Graph
	left := graph.NewNode("left")
	middle := graph.NewNode("middle")
	right := graph.NewNode("right")
	groupID, err := graph.NewGroup([]Member{
		{ID: left, Kind: MemberNode},
		{ID: middle, Kind: MemberNode},
	})
	require.NoError(t, err)

	_, err = graph.NewGroup([]Member{
		{ID: left, Kind: MemberNode},
		{ID: right, Kind: MemberNode},
	})
	require.ErrorIs(t, err, ErrMembersNotSiblings)
	_, err = graph.NewGroup([]Member{
		{ID: groupID, Kind: MemberGroup},
		{ID: left, Kind: MemberNode},
	})
	require.ErrorIs(t, err, ErrMembersNotSiblings)
	_, err = graph.NewGroup([]Member{
		{ID: right, Kind: MemberNode},
		{ID: right, Kind: MemberNode},
	})
	require.ErrorIs(t, err, ErrDuplicateMember)
	_, err = graph.NewGroup([]Member{{ID: right, Kind: MemberNode}})
	require.ErrorIs(t, err, ErrGroupTooSmall)
}

func TestGraphUngroupOneLevelAndReuseID(t *testing.T) {
	t.Parallel()

	var graph Graph
	left := graph.NewNode("left")
	middle := graph.NewNode("middle")
	right := graph.NewNode("right")
	shape, err := graph.NewGroup([]Member{
		{ID: left, Kind: MemberNode},
		{ID: middle, Kind: MemberNode},
	})
	require.NoError(t, err)
	section, err := graph.NewGroup([]Member{
		{ID: shape, Kind: MemberGroup},
		{ID: right, Kind: MemberNode},
	})
	require.NoError(t, err)

	members, err := graph.Ungroup(shape)
	require.NoError(t, err)
	require.Equal(t, []Member{
		{ID: left, Kind: MemberNode},
		{ID: middle, Kind: MemberNode},
	}, members)
	require.False(t, graph.GroupExists(shape))
	require.Equal(t, []Member{
		{ID: left, Kind: MemberNode},
		{ID: middle, Kind: MemberNode},
		{ID: right, Kind: MemberNode},
	}, graph.Groups[section].Members)

	replacement, err := graph.NewGroup([]Member{
		{ID: left, Kind: MemberNode},
		{ID: middle, Kind: MemberNode},
	})
	require.NoError(t, err)
	require.Equal(t, shape, replacement)
	require.Equal(t, []Member{
		{ID: replacement, Kind: MemberGroup},
		{ID: right, Kind: MemberNode},
	}, graph.Groups[section].Members)
}

func TestGraphDeleteNodeDissolvesSingletonGroups(t *testing.T) {
	t.Parallel()

	var graph Graph
	left := graph.NewNode("left")
	middle := graph.NewNode("middle")
	right := graph.NewNode("right")
	shape, err := graph.NewGroup([]Member{
		{ID: left, Kind: MemberNode},
		{ID: middle, Kind: MemberNode},
	})
	require.NoError(t, err)
	section, err := graph.NewGroup([]Member{
		{ID: shape, Kind: MemberGroup},
		{ID: right, Kind: MemberNode},
	})
	require.NoError(t, err)

	require.NoError(t, graph.DeleteNode(left))
	require.False(t, graph.GroupExists(shape))
	require.True(t, graph.GroupExists(section))
	require.Equal(t, []Member{
		{ID: middle, Kind: MemberNode},
		{ID: right, Kind: MemberNode},
	}, graph.Groups[section].Members)

	require.NoError(t, graph.DeleteNode(middle))
	require.False(t, graph.GroupExists(section))
	_, grouped := graph.Parent(Member{ID: right, Kind: MemberNode})
	require.False(t, grouped)
	require.NoError(t, graph.Validate())
}

func TestGraphClonePreservesIndependentGroups(t *testing.T) {
	t.Parallel()

	var graph Graph
	left := graph.NewNode("left")
	right := graph.NewNode("right")
	groupID, err := graph.NewGroup([]Member{
		{ID: left, Kind: MemberNode},
		{ID: right, Kind: MemberNode},
	})
	require.NoError(t, err)

	clone := graph.Clone()
	_, err = clone.Ungroup(groupID)
	require.NoError(t, err)
	require.True(t, graph.GroupExists(groupID))
	require.False(t, clone.GroupExists(groupID))
}

func TestGraphValidateGroups(t *testing.T) {
	t.Parallel()

	var graph Graph
	left := graph.NewNode("left")
	right := graph.NewNode("right")
	groupID, err := graph.NewGroup([]Member{
		{ID: left, Kind: MemberNode},
		{ID: right, Kind: MemberNode},
	})
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*Graph)
		want   string
	}{
		{
			name: "singleton",
			mutate: func(candidate *Graph) {
				candidate.Groups[groupID].Members = candidate.Groups[groupID].Members[:1]
			},
			want: "group 0 has fewer than two members",
		},
		{
			name: "unknown member",
			mutate: func(candidate *Graph) {
				candidate.Groups[groupID].Members[0].ID = 99
			},
			want: "group 0 references unknown node 99",
		},
		{
			name: "multiple parents",
			mutate: func(candidate *Graph) {
				candidate.Groups = append(candidate.Groups, Group{Members: slices.Clone(candidate.Groups[groupID].Members)})
			},
			want: "node 0 belongs to multiple groups",
		},
		{
			name: "cycle",
			mutate: func(candidate *Graph) {
				third := candidate.NewNode("third")
				candidate.Groups = append(candidate.Groups, Group{Members: []Member{
					{ID: groupID, Kind: MemberGroup},
					{ID: third, Kind: MemberNode},
				}})
				candidate.Groups[groupID].Members[0] = Member{ID: 1, Kind: MemberGroup}
			},
			want: "group 0 creates a membership cycle",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			candidate := graph.Clone()
			test.mutate(&candidate)
			require.EqualError(t, candidate.Validate(), test.want)
		})
	}
}
