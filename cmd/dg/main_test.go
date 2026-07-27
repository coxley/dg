package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExampleLayoutBuilds(t *testing.T) {
	t.Parallel()

	geo, err := exampleLayout()
	require.NoError(t, err)
	require.NoError(t, geo.Build())
}
