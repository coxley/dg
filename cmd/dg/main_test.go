package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/layout"
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
	doc := document.New(geo)
	data, err := document.Marshal(doc)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "diagram.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	loaded, loadedDocument, gotPath, err := initialLayout([]string{path}, settings.Snapshot{})
	require.NoError(t, err)
	require.Equal(t, path, gotPath)
	require.Equal(t, doc.ID, loadedDocument.ID)
	require.Equal(t, "sinks", loaded.Label(0))
	require.NoError(t, loaded.Build())
}

func TestInitialLayoutRejectsExtraArguments(t *testing.T) {
	t.Parallel()

	_, _, _, err := initialLayout(
		[]string{"one.json", "two.json"},
		settings.Snapshot{},
	)
	require.EqualError(t, err, "usage: dg [path]")
}

func TestInitialLayoutUsesInjectedRouterForNewDiagram(t *testing.T) {
	t.Parallel()

	router := layout.DefaultRouter()
	router.Costs.Step = 37

	geo, doc, path, err := initialLayout(nil, settings.Snapshot{
		Router:        router,
		ApplyToFuture: true,
	})

	require.NoError(t, err)
	require.Empty(t, path)
	require.NotEmpty(t, doc.ID)
	require.Equal(t, router, geo.Router())
}
