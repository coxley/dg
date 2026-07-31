package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreWatchReconcilesOwnAndExternalChanges(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	events, err := store.Watch(ctx)
	require.NoError(t, err)
	initial := receiveCatalogEvent(t, events, func(event CatalogEvent) bool {
		return !event.Closed
	})
	require.Empty(t, initial.Entries)
	require.NoError(t, initial.Err)

	doc := testDocument(t, "initial")
	_, err = store.Create("", "Canvas", doc)
	require.NoError(t, err)
	created := receiveCatalogEvent(t, events, func(event CatalogEvent) bool {
		return hasCatalogChange(event, ChangeAdded, "Canvas")
	})
	require.Len(t, created.Changes, 1)
	require.False(t, created.Changes[0].External)

	doc.Nodes[0].Label = externalLabel
	data, err := encodeDocument(doc)
	require.NoError(t, err)
	require.NoError(t, replaceFile(store.namedPath("", "Canvas"), data))
	modified := receiveCatalogEvent(t, events, func(event CatalogEvent) bool {
		return hasCatalogChange(event, ChangeModified, "Canvas")
	})
	require.True(t, modified.Changes[0].External)
	entry := modified.Changes[0].Entry

	require.NoError(t, store.Delete(entry))
	deleted := receiveCatalogEvent(t, events, func(event CatalogEvent) bool {
		return hasCatalogChange(event, ChangeDeleted, "Canvas")
	})
	require.False(t, deleted.Changes[0].External)

	cancel()
	closed := receiveCatalogEvent(t, events, func(event CatalogEvent) bool { return event.Closed })
	require.True(t, closed.Closed)
	_, ok := <-events
	require.False(t, ok)
}

func TestStoreSelfWriteRemainsInternalAcrossManualAndWatchedReconciliation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, err := store.Watch(ctx)
	require.NoError(t, err)
	initial := receiveCatalogEvent(t, events, func(event CatalogEvent) bool {
		return !event.Closed
	})

	_, err = store.Create("", "Canvas", testDocument(t, "node"))
	require.NoError(t, err)
	manual := store.Reconcile(initial.Entries)
	require.Len(t, manual.Changes, 1)
	require.False(t, manual.Changes[0].External)
	watched := receiveCatalogEvent(t, events, func(event CatalogEvent) bool {
		return hasCatalogChange(event, ChangeAdded, "Canvas")
	})
	require.Len(t, watched.Changes, 1)
	require.False(t, watched.Changes[0].External)
}

func TestStoreWatchAddsNewSectionAndCoalescesBurst(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, err := store.Watch(ctx)
	require.NoError(t, err)
	receiveCatalogEvent(t, events, func(event CatalogEvent) bool { return !event.Closed })

	for i := range 10 {
		_, err := store.Create("Section", string(rune('A'+i)), testDocument(t, "node"))
		require.NoError(t, err)
	}
	event := receiveCatalogEvent(t, events, func(event CatalogEvent) bool {
		return len(event.Entries) == 10
	})
	require.Len(t, event.Changes, 10)
	for _, change := range event.Changes {
		require.False(t, change.External)
	}

	entry := event.Entries[0]
	doc, err := store.Load(entry)
	require.NoError(t, err)
	doc.Nodes[0].Label = externalLabel
	data, err := encodeDocument(doc)
	require.NoError(t, err)
	require.NoError(t, replaceFile(store.namedPath(entry.Section, entry.Name), data))
	modified := receiveCatalogEvent(t, events, func(event CatalogEvent) bool {
		return hasCatalogChange(event, ChangeModified, entry.Name)
	})
	require.True(t, modified.Changes[0].External)
}

func TestStoreWatchReportsCorruptRecord(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	entry, err := store.Create("", "Canvas", testDocument(t, "node"))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, err := store.Watch(ctx)
	require.NoError(t, err)
	receiveCatalogEvent(t, events, func(event CatalogEvent) bool { return len(event.Entries) == 1 })
	require.NoError(t, os.WriteFile(store.namedPath("", "Canvas"), []byte("corrupt"), 0o600))
	event := receiveCatalogEvent(t, events, func(event CatalogEvent) bool { return event.Err != nil })
	require.ErrorContains(t, event.Err, "Canvas.dg")
	require.True(t, hasCatalogChange(event, ChangeDeleted, entry.Name))
}

func TestStoreReconcileFindsChangesWithoutWatcherEvent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	entry, err := store.Create("", "Canvas", testDocument(t, "node"))
	require.NoError(t, err)
	before, err := store.List()
	require.NoError(t, err)
	require.NoError(t, os.Remove(store.namedPath("", "Canvas")))
	event := store.Reconcile(before)
	require.NoError(t, event.Err)
	require.True(t, hasCatalogChange(event, ChangeDeleted, entry.Name))
	require.True(t, event.Changes[0].External)
}

func receiveCatalogEvent(
	t testing.TB,
	events <-chan CatalogEvent,
	match func(CatalogEvent) bool,
) CatalogEvent {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-events:
			require.True(t, ok, "catalog watcher closed before matching event")
			if match(event) {
				return event
			}
		case <-timer.C:
			t.Fatal("timed out waiting for catalog event")
		}
	}
}

func hasCatalogChange(event CatalogEvent, kind ChangeKind, name string) bool {
	for _, change := range event.Changes {
		entry := changeEntry(change)
		if change.Kind == kind && entry.Name == name {
			return true
		}
	}
	return false
}
