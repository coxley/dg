package chrome

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCellTransitionOpensEaseOutAndClosesEaseIn(t *testing.T) {
	t.Parallel()

	var transition cellTransition
	require.True(t, transition.retarget(12, false))
	open := transitionPositions(&transition)
	require.Equal(t, 12, open[len(open)-1])
	require.Greater(t, open[0], 0)
	require.GreaterOrEqual(t, open[1]-open[0], open[len(open)-1]-open[len(open)-2])

	require.True(t, transition.retarget(0, false))
	closePositions := transitionPositions(&transition)
	require.Zero(t, closePositions[len(closePositions)-1])
	require.GreaterOrEqual(
		t,
		closePositions[0]-closePositions[1],
		12-closePositions[0],
	)
}

func TestCellTransitionRetargetsAndReversesWithoutJumping(t *testing.T) {
	t.Parallel()

	var transition cellTransition
	transition.retarget(20, false)
	require.True(t, transition.advance())
	require.True(t, transition.advance())
	current := transition.position

	require.True(t, transition.retarget(0, false))
	require.Equal(t, current, transition.position)
	require.True(t, transition.advance())
	require.Less(t, transition.position, current)

	current = transition.position
	require.True(t, transition.retarget(14, false))
	require.Equal(t, current, transition.position)
	require.True(t, transition.advance())
	require.Greater(t, transition.position, current)
}

func TestCellTransitionDisabledMotionSnapsToTarget(t *testing.T) {
	t.Parallel()

	var transition cellTransition
	require.False(t, transition.retarget(18, true))
	require.Equal(t, 18, transition.position)
	require.False(t, transition.advance())
}

func transitionPositions(transition *cellTransition) []int {
	var positions []int
	for transition.advance() {
		positions = append(positions, transition.position)
	}
	return positions
}
