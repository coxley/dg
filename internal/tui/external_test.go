package tui

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/tui/chrome"
	canvasstore "github.com/coxley/dg/store"
	"github.com/stretchr/testify/require"
)

func TestExternalReloadParticipatesInUndoChain(t *testing.T) {
	t.Parallel()

	model, store := newNamedStoredTestModel(t, "original")
	external := document.New(mustLayoutWithLabel(t, "external"))
	external.ID = model.document.ID
	replaceStoredDocument(t, store, *model.entry, external)
	model.updateCatalog(store.Reconcile(model.catalog))
	model.syncWorkspace()

	require.Equal(t, surfaceConfirmation, model.dialogs.ActiveID())
	require.Contains(t, model.dialogs.confirmation.View(), "Canvas has been externally modified;")
	require.Contains(t, model.dialogs.confirmation.View(), "load it?")
	model.handleDialogResult(model.dialogs.Update(chrome.FormSubmitMsg{ID: confirmationAccept}))
	require.Equal(t, "external", model.geo.Label(0))

	transaction := model.history.Begin()
	require.NoError(t, model.geo.SetNodeLabel(0, "changed"))
	require.NoError(t, transaction.Commit())
	model.undo()
	require.Equal(t, "external", model.geo.Label(0))
	model.undo()
	require.Equal(t, "original", model.geo.Label(0))
	model.redo()
	require.Equal(t, "external", model.geo.Label(0))
	model.redo()
	require.Equal(t, "changed", model.geo.Label(0))
}

func TestExternalModificationCanKeepLocalWithRawBackup(t *testing.T) {
	t.Parallel()

	model, store := newNamedStoredTestModel(t, "local")
	external := document.New(mustLayoutWithLabel(t, "external"))
	external.ID = model.document.ID
	raw := replaceStoredDocument(t, store, *model.entry, external)
	model.updateCatalog(store.Reconcile(model.catalog))
	model.handleDialogResult(model.dialogs.Update(chrome.FormSubmitMsg{ID: confirmationCancel}))

	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.Equal(t, "local", model.geo.Label(0))
	entries, err := store.List()
	require.NoError(t, err)
	backup := requireEntry(t, entries, "", "Canvas.bak")
	path, err := store.Path(backup)
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, raw, got)
	model.updateCatalog(store.Reconcile(model.catalog))
	require.Contains(t, sidebarLabels(model), "Canvas.bak [backup]")
}

func TestExternalDeletionCanRestoreOrPreserveDraft(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		restore bool
	}{
		{name: "restore", restore: true},
		{name: "preserve draft"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			model, store := newNamedStoredTestModel(t, "local")
			path, err := store.Path(*model.entry)
			require.NoError(t, err)
			require.NoError(t, os.Remove(path))
			model.updateCatalog(store.Reconcile(model.catalog))
			model.syncWorkspace()
			require.Contains(t, model.dialogs.confirmation.View(), "Canvas was externally deleted; restore")
			require.Contains(t, model.dialogs.confirmation.View(), "it?")
			button := confirmationCancel
			if test.restore {
				button = confirmationAccept
			}
			model.handleDialogResult(model.dialogs.Update(chrome.FormSubmitMsg{ID: button}))
			require.Equal(t, surfaceNone, model.dialogs.ActiveID())
			require.Equal(t, "local", model.geo.Label(0))
			if test.restore {
				require.False(t, model.entry.Draft)
				_, err = os.Stat(path)
				require.NoError(t, err)
			} else {
				require.True(t, model.entry.Draft)
				_, err = os.Stat(path)
				require.ErrorIs(t, err, fs.ErrNotExist)
			}
		})
	}
}

func TestExternalDifferentIdentityBecomesNamedCanvasAndPreservesDraft(t *testing.T) {
	t.Parallel()

	model, store := newNamedStoredTestModel(t, "local")
	previousID := model.document.ID
	replacement := document.New(mustLayoutWithLabel(t, "replacement"))
	replaceStoredDocument(t, store, *model.entry, replacement)
	model.updateCatalog(store.Reconcile(model.catalog))

	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	require.Equal(t, replacement.ID, model.document.ID)
	require.Equal(t, replacement.ID, model.entry.ID)
	require.Equal(t, "Canvas", model.entry.Name)
	require.Equal(t, "replacement", model.geo.Label(0))
	entries, err := store.List()
	require.NoError(t, err)
	draft := requireDraft(t, entries, previousID)
	loaded, err := store.Load(draft)
	require.NoError(t, err)
	require.Equal(t, "local", loaded.Nodes[0].Label)
	require.False(t, model.history.CanUndo())
}

func TestExternalMalformedReplacementCanStillBeBackedUp(t *testing.T) {
	t.Parallel()

	model, store := newNamedStoredTestModel(t, "local")
	path, err := store.Path(*model.entry)
	require.NoError(t, err)
	raw := []byte("malformed external canvas")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	model.updateCatalog(store.Reconcile(model.catalog))
	model.syncWorkspace()

	require.Contains(t, model.dialogs.confirmation.View(), "externally modified")
	model.handleDialogResult(model.dialogs.Update(chrome.FormSubmitMsg{ID: confirmationAccept}))
	require.Equal(t, surfaceConfirmation, model.dialogs.ActiveID())
	require.Contains(t, model.statusError, "decode current canvas")
	model.handleDialogResult(model.dialogs.Update(chrome.FormSubmitMsg{ID: confirmationCancel}))
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
	backupPath := filepath.Join(filepath.Dir(path), "Canvas.bak.dg")
	got, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	require.Equal(t, raw, got)
}

func TestExternalPromptCoalescesToLatestRevision(t *testing.T) {
	t.Parallel()

	model, store := newNamedStoredTestModel(t, "original")
	first := document.New(mustLayoutWithLabel(t, "first"))
	first.ID = model.document.ID
	replaceStoredDocument(t, store, *model.entry, first)
	model.updateCatalog(store.Reconcile(model.catalog))
	firstConflict := model.external.entry.Revision

	second := document.New(mustLayoutWithLabel(t, "second"))
	second.ID = model.document.ID
	replaceStoredDocument(t, store, *model.entry, second)
	model.updateCatalog(store.Reconcile(model.catalog))
	require.NotEqual(t, firstConflict, model.external.entry.Revision)
	model.handleDialogResult(model.dialogs.Update(chrome.FormSubmitMsg{ID: confirmationAccept}))
	require.Equal(t, "second", model.geo.Label(0))
}

func newNamedStoredTestModel(t testing.TB, label string) (*Model, *canvasstore.Store) {
	t.Helper()
	model, _, store := newStoredTestModel(t, label)
	updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	named, err := store.Name(*model.entry, "", "Canvas")
	require.NoError(t, err)
	active := named
	model.entry = &active
	model.catalog, err = store.List()
	require.NoError(t, err)
	model.rebuildSidebarCatalog()
	return model, store
}

func replaceStoredDocument(
	t testing.TB,
	store *canvasstore.Store,
	entry canvasstore.Entry,
	doc document.Document,
) []byte {
	t.Helper()
	root := t.TempDir()
	external, err := canvasstore.New(
		filepath.Join(root, "canvases"),
		canvasstore.WithStateDir(filepath.Join(root, "state")),
		canvasstore.WithCacheDir(filepath.Join(root, "cache")),
	)
	require.NoError(t, err)
	source, err := external.Create("", "External", doc)
	require.NoError(t, err)
	sourcePath, err := external.Path(source)
	require.NoError(t, err)
	raw, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	target, err := store.Path(entry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, raw, 0o600)) //nolint:gosec // Store returned the test-owned target path.
	return raw
}

func requireEntry(
	t testing.TB,
	entries []canvasstore.Entry,
	section, name string,
) canvasstore.Entry {
	t.Helper()
	for _, entry := range entries {
		if !entry.Draft && entry.Section == section && entry.Name == name {
			return entry
		}
	}
	require.FailNow(t, "entry not found", "%s/%s", section, name)
	return canvasstore.Entry{}
}

func requireDraft(t testing.TB, entries []canvasstore.Entry, id [16]byte) canvasstore.Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Draft && entry.ID == id {
			return entry
		}
	}
	require.FailNow(t, "draft not found", "%s", id)
	return canvasstore.Entry{}
}
