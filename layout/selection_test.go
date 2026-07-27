package layout

import (
	"fmt"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestSelectionComponentProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		nodeCount := rapid.IntRange(1, 10).Draw(t, "node count")
		pairCount := nodeCount * (nodeCount - 1) / 2
		connections := rapid.SliceOfN(rapid.Bool(), pairCount, pairCount).
			Draw(t, "connections")
		seeds := rapid.SliceOfN(rapid.Bool(), nodeCount, nodeCount).
			Draw(t, "seeds")
		seeds[rapid.IntRange(0, nodeCount-1).Draw(t, "required seed")] = true

		geo, err := New()
		require.NoError(t, err)
		for nodeID := range nodeCount {
			_, err := geo.NewNodeAt(
				fmt.Sprintf("node%d", nodeID),
				NewPoint(uint32(nodeID*20), 1),
			)
			require.NoError(t, err)
		}

		adjacent := make([]bool, nodeCount*nodeCount)
		connectionID := 0
		for nodeA := range nodeCount {
			for nodeB := nodeA + 1; nodeB < nodeCount; nodeB++ {
				if connections[connectionID] {
					geo.ConnectNodes(
						uint32(nodeA),
						ir.RightSide,
						ir.LeftSide,
						uint32(nodeB),
					)
					adjacent[nodeA*nodeCount+nodeB] = true
					adjacent[nodeB*nodeCount+nodeA] = true
				}
				connectionID++
			}
		}

		selection := geo.Selection()
		for nodeID, selected := range seeds {
			if selected {
				selection.Toggle(Hit{ID: uint32(nodeID), Kind: HitNode})
			}
		}
		selection.Expand()

		want := append([]bool(nil), seeds...)
		for changed := true; changed; {
			changed = false
			for nodeA, selected := range want {
				if !selected {
					continue
				}
				for nodeB := range nodeCount {
					if adjacent[nodeA*nodeCount+nodeB] && !want[nodeB] {
						want[nodeB] = true
						changed = true
					}
				}
			}
		}
		for nodeID, selected := range want {
			require.Equal(
				t,
				selected,
				selection.Contains(Hit{ID: uint32(nodeID), Kind: HitNode}),
			)
		}

		selection.Expand()
		nodes, edges := selection.Counts()
		require.Equal(t, nodeCount, nodes)
		require.Equal(t, len(geo.graph.Edges), edges)
	})
}

func TestSelectionAreaAndReuse(t *testing.T) {
	t.Parallel()

	geo, err := New()
	require.NoError(t, err)
	left, err := geo.NewNodeAt("left", NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", NewPoint(20, 2))
	require.NoError(t, err)

	selection := geo.Selection()
	selection.SelectArea(NewPoint(0, 0), NewPoint(10, 8))
	require.True(t, selection.Contains(Hit{ID: left, Kind: HitNode}))
	require.False(t, selection.Contains(Hit{ID: right, Kind: HitNode}))

	require.NoError(t, geo.DeleteNode(left))
	reused, err := geo.NewNodeAt("reused", NewPoint(2, 2))
	require.NoError(t, err)
	require.Equal(t, left, reused)
	require.False(t, selection.Contains(Hit{ID: reused, Kind: HitNode}))
}
