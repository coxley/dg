package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/internal/tui"
	"github.com/coxley/dg/layout"
	canvasstore "github.com/coxley/dg/store"
	"github.com/stretchr/testify/require"
)

func TestInitialLayoutReadsDocument(t *testing.T) {
	t.Parallel()

	geo := mustLayoutWithLabel(t, "imported")
	doc := document.New(geo)
	source := newCanvasStore(t)
	entry, err := source.Create("", "Imported", doc)
	require.NoError(t, err)
	path, err := source.Path(entry)
	require.NoError(t, err)

	canvases := newCanvasStore(t)
	loaded, loadedDocument, imported, err := initialCanvas([]string{path}, settings.Snapshot{}, canvases)
	require.NoError(t, err)
	require.NotNil(t, imported)
	require.True(t, imported.Draft)
	require.Equal(t, doc.ID, loadedDocument.ID)
	require.Equal(t, "imported", loaded.Label(0))
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
	require.EqualError(t, err, "usage: dg [path] | dg dev [path]")
}

func TestInitialEditorCanvasRestoresDevelopmentEntry(t *testing.T) {
	t.Parallel()

	canvases := newCanvasStore(t)
	doc := document.New(mustLayoutWithLabel(t, "saved"))
	entry, err := canvases.Create("", "Saved", doc)
	require.NoError(t, err)
	doc.Nodes[0].Label = "in memory"
	session := tui.DevSession{Document: doc, EntryID: entry.ID}

	geo, restored, active, err := initialEditorCanvas(
		nil,
		settings.Snapshot{},
		canvases,
		&session,
	)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, entry.ID, active.ID)
	require.Equal(t, doc, restored)
	require.Equal(t, "in memory", geo.Label(0))
}

func TestFindModuleRootWalksParents(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/test\n"),
		0o600,
	))
	nested := filepath.Join(root, "one", "two")
	require.NoError(t, os.MkdirAll(nested, 0o700))

	got, err := findModuleRoot(nested)
	require.NoError(t, err)
	require.Equal(t, root, got)
}

func TestBuildDevBinaryKeepsLastSuccessfulBuild(t *testing.T) {
	root := t.TempDir()
	binaryPath := filepath.Join(t.TempDir(), "dg")
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/devtest\n\ngo 1.26.5\n"),
		0o600,
	))
	mainPath := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(
		mainPath,
		[]byte("package main\nfunc main() {}\n"),
		0o600,
	))

	output, err := buildDevBinary(t.Context(), root, binaryPath)
	require.NoError(t, err, string(output))
	require.NoError(t, exec.Command(binaryPath).Run())
	require.NoError(t, os.WriteFile(
		mainPath,
		[]byte("package main\nfunc main( {\n"),
		0o600,
	))

	output, err = buildDevBinary(t.Context(), root, binaryPath)
	require.Error(t, err)
	require.NotEmpty(t, output)
	require.NoError(t, exec.Command(binaryPath).Run())
}

func TestWatchDevSourcesDetectsNestedGoFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "internal", "feature")
	require.NoError(t, os.MkdirAll(nested, 0o700))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	events, err := watchDevSources(ctx, root)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, "feature.go"),
		[]byte("package feature\n"),
		0o600,
	))

	select {
	case event := <-events:
		require.NoError(t, event.err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for development source event")
	}
}

func TestRunDevRejectsExtraArguments(t *testing.T) {
	t.Parallel()

	require.EqualError(t, runDev([]string{"one", "two"}), "usage: dg dev [path]")
}

func TestInitialCanvasStartsTransientEmptyWithInjectedRouter(t *testing.T) {
	t.Parallel()

	router := layout.DefaultRouter()
	router.Costs.Step = 37
	canvases := newCanvasStore(t)

	geo, doc, entry, err := initialCanvas(nil, settings.Snapshot{
		Router:        router,
		ApplyToFuture: true,
	}, canvases)

	require.NoError(t, err)
	require.Nil(t, entry)
	require.NotEmpty(t, doc.ID)
	require.Empty(t, geo.Nodes)
	require.Empty(t, geo.Edges)
	require.Equal(t, router, geo.Router())
	entries, err := canvases.List()
	require.NoError(t, err)
	require.Empty(t, entries)
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
	require.NotNil(t, entry)
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
