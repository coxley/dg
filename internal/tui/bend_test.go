package tui

import (
	"testing"

	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestBendDragLocksToDominantAxis(t *testing.T) {
	t.Parallel()

	start := layout.NewPoint(10, 10)
	var vertical bendSession
	require.Equal(
		t,
		layout.NewPoint(11, 10),
		vertical.constrainPoint(start, layout.NewPoint(11, 10)),
	)
	require.Equal(
		t,
		layout.NewPoint(10, 13),
		vertical.constrainPoint(start, layout.NewPoint(11, 13)),
	)
	require.Equal(t, bendAxisVertical, vertical.axis)
	require.Equal(
		t,
		layout.NewPoint(10, 16),
		vertical.constrainPoint(start, layout.NewPoint(20, 16)),
	)

	var horizontal bendSession
	require.Equal(
		t,
		layout.NewPoint(14, 10),
		horizontal.constrainPoint(start, layout.NewPoint(14, 12)),
	)
	require.Equal(t, bendAxisHorizontal, horizontal.axis)
	require.Equal(
		t,
		layout.NewPoint(8, 10),
		horizontal.constrainPoint(start, layout.NewPoint(8, 2)),
	)
}
