package main

import (
	"path/filepath"
	"testing"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/layout"
	canvasstore "github.com/coxley/dg/store"
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
	source := newCanvasStore(t)
	entry, err := source.Create("", "Imported", doc)
	require.NoError(t, err)
	path, err := source.Path(entry)
	require.NoError(t, err)

	canvases := newCanvasStore(t)
	loaded, loadedDocument, imported, err := initialCanvas([]string{path}, settings.Snapshot{}, canvases)
	require.NoError(t, err)
	require.True(t, imported.Draft)
	require.Equal(t, doc.ID, loadedDocument.ID)
	require.Equal(t, "sinks", loaded.Label(0))
	require.NoError(t, loaded.Build())
	require.FileExists(t, path)
}

func TestInitialLayoutRejectsExtraArguments(t *testing.T) {
	t.Parallel()

	_, _, _, err := initialCanvas(
		[]string{"one.json", "two.json"},
		settings.Snapshot{},
		newCanvasStore(t),
	)
	require.EqualError(t, err, "usage: dg [path]")
}

func TestInitialLayoutUsesInjectedRouterForNewDiagram(t *testing.T) {
	t.Parallel()

	router := layout.DefaultRouter()
	router.Costs.Step = 37

	geo, doc, entry, err := initialCanvas(nil, settings.Snapshot{
		Router:        router,
		ApplyToFuture: true,
	}, newCanvasStore(t))

	require.NoError(t, err)
	require.True(t, entry.Draft)
	require.NotEmpty(t, doc.ID)
	require.Equal(t, router, geo.Router())
}

func TestInitialCanvasLoadsMostRecentlyModifiedRecord(t *testing.T) {
	t.Parallel()

	canvases := newCanvasStore(t)
	first := document.New(mustLayoutWithLabel(t, "first"))
	_, err := canvases.Create("", "First", first)
	require.NoError(t, err)
	second := document.New(mustLayoutWithLabel(t, "second"))
	secondEntry, err := canvases.CreateDraft(second)
	require.NoError(t, err)
	// Saving establishes an unambiguously newer filesystem revision.
	secondEntry, err = canvases.Save(secondEntry, second)
	require.NoError(t, err)

	geo, doc, entry, err := initialCanvas(nil, settings.Snapshot{}, canvases)
	require.NoError(t, err)
	require.Equal(t, second.ID, doc.ID)
	require.Equal(t, secondEntry.ID, entry.ID)
	require.Equal(t, "second", geo.Label(0))
}

func newCanvasStore(t testing.TB) *canvasstore.Store {
	t.Helper()
	root := t.TempDir()
	store, err := canvasstore.New(
		filepath.Join(root, "canvases"),
		canvasstore.WithStateDir(filepath.Join(root, "state")),
		canvasstore.WithCacheDir(filepath.Join(root, "cache")),
	)
	require.NoError(t, err)
	return store
}

func mustLayoutWithLabel(t testing.TB, label string) *layout.Layout {
	t.Helper()
	geo, err := layout.New()
	require.NoError(t, err)
	_, err = geo.NewNode(label)
	require.NoError(t, err)
	return geo
}
