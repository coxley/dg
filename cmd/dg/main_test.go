package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coxley/dg/document"
	"github.com/stretchr/testify/require"
)

func TestExampleLayoutBuilds(t *testing.T) {
	t.Parallel()

	geo, err := exampleLayout()
	require.NoError(t, err)
	require.NoError(t, geo.Build())
}

func TestInitialLayoutReadsDocument(t *testing.T) {
	t.Parallel()

	geo, err := exampleLayout()
	require.NoError(t, err)
	data, err := document.Marshal(geo)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "diagram.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	loaded, gotPath, err := initialLayout([]string{path})
	require.NoError(t, err)
	require.Equal(t, path, gotPath)
	require.Equal(t, "sinks", loaded.Label(0))
	require.NoError(t, loaded.Build())
}

func TestInitialLayoutRejectsExtraArguments(t *testing.T) {
	t.Parallel()

	_, _, err := initialLayout([]string{"one.json", "two.json"})
	require.EqualError(t, err, "usage: dg [path]")
}
