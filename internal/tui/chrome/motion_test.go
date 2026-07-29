package chrome

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScalarTransitionRetargetsWithoutLosingContinuousState(t *testing.T) {
	t.Parallel()

	transition := newScalarTransition()
	require.True(t, transition.retarget(26))
	require.False(t, transition.advance(transitionStep/2))
	require.Zero(t, transition.extent())
	require.True(t, transition.moving())
	require.True(t, transition.advance(transitionStep-transitionStep/2))

	position := transition.position
	velocity := transition.velocity
	extent := transition.extent()
	require.Positive(t, position)
	require.Positive(t, velocity)

	require.True(t, transition.retarget(30))
	require.Equal(t, position, transition.position)
	require.Equal(t, velocity, transition.velocity)
	require.Equal(t, extent, transition.extent())

	require.True(t, transition.retarget(0))
	require.Equal(t, position, transition.position)
	require.Zero(t, transition.velocity)
	require.Equal(t, extent, transition.extent())
}

func TestScalarTransitionPublishesMonotonicCellsAndExactEndpoints(t *testing.T) {
	t.Parallel()

	transition := newScalarTransition()
	require.True(t, transition.retarget(26))
	open := transitionExtents(t, transition)
	require.NotEmpty(t, open)
	require.Equal(t, 26, open[len(open)-1])
	require.IsNonDecreasing(t, open)
	require.False(t, transition.moving())
	require.Equal(t, 26.0, transition.position)
	require.Zero(t, transition.velocity)

	require.True(t, transition.retarget(0))
	closeExtents := transitionExtents(t, transition)
	require.NotEmpty(t, closeExtents)
	require.Zero(t, closeExtents[len(closeExtents)-1])
	require.IsNonIncreasing(t, closeExtents)
	require.False(t, transition.moving())
	require.Zero(t, transition.position)
	require.Zero(t, transition.velocity)
}

func TestScalarTransitionSnapUsesCurrentTarget(t *testing.T) {
	t.Parallel()

	transition := newScalarTransition()
	require.True(t, transition.retarget(18))
	transition.snap()
	require.Equal(t, 18, transition.extent())
	require.False(t, transition.moving())
	require.False(t, transition.advance(transitionStep))
}

func transitionExtents(t testing.TB, transition *scalarTransition) []int {
	t.Helper()

	var extents []int
	for range 240 {
		if transition.advance(transitionStep) {
			extents = append(extents, transition.extent())
		}
		if !transition.moving() {
			return extents
		}
	}
	t.Fatal("transition did not settle")
	return nil
}
