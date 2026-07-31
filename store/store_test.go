package store

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/layout"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const externalLabel = "external"

func newTestStore(t testing.TB, options ...Option) *Store {
	t.Helper()
	root := t.TempDir()
	options = append([]Option{
		WithStateDir(filepath.Join(root, "state")),
		WithCacheDir(filepath.Join(root, "cache")),
	}, options...)
	store, err := New(filepath.Join(root, "canvases"), options...)
	require.NoError(t, err)
	return store
}

func testDocument(t testing.TB, label string) document.Document {
	t.Helper()
	geo, err := layout.New()
	require.NoError(t, err)
	_, err = geo.NewNode(label)
	require.NoError(t, err)
	return document.New(geo)
}

func TestStoreNamedCanvasLifecycle(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "first")
	entry, err := store.Create("RFCs", "Proposal 1", doc)
	require.NoError(t, err)
	require.Equal(t, "RFCs", entry.Section)
	require.Equal(t, "Proposal 1", entry.Name)
	require.Equal(t, doc.ID, entry.ID)
	require.False(t, entry.Draft)

	loaded, err := store.Load(entry)
	require.NoError(t, err)
	require.Equal(t, "first", loaded.Nodes[0].Label)
	loaded.Nodes[0].Label = "changed"
	updated, err := store.Save(entry, loaded)
	require.NoError(t, err)
	require.NotEqual(t, entry.Revision, updated.Revision)

	moved, err := store.Move(updated, "", "Architecture")
	require.NoError(t, err)
	require.Empty(t, moved.Section)
	require.Equal(t, "Architecture", moved.Name)
	loaded, err = store.Load(moved)
	require.NoError(t, err)
	require.Equal(t, "changed", loaded.Nodes[0].Label)
	require.NoError(t, store.Delete(moved))
	_, err = store.Load(moved)
	require.ErrorIs(t, err, ErrEntryNotFound)
}

func TestStoreDemotesNamedCanvasWithoutReencoding(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "canvas")
	named, err := store.Create("RFCs", "Proposal", doc)
	require.NoError(t, err)
	source, err := store.Path(named)
	require.NoError(t, err)
	want, err := os.ReadFile(source)
	require.NoError(t, err)

	draft, err := store.Demote(named)
	require.NoError(t, err)
	require.True(t, draft.Draft)
	require.Equal(t, doc.ID, draft.ID)
	require.Empty(t, draft.Section)
	require.Empty(t, draft.Name)
	_, err = os.Stat(source)
	require.ErrorIs(t, err, fs.ErrNotExist)
	target, err := store.Path(draft)
	require.NoError(t, err)
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestStoreDemoteRejectsStaleRevisionAndDraftCollision(t *testing.T) {
	t.Parallel()

	t.Run("stale revision", func(t *testing.T) {
		store := newTestStore(t)
		named, err := store.Create("", "Canvas", testDocument(t, "named"))
		require.NoError(t, err)
		require.NoError(t, replaceFile(store.namedPath("", "Canvas"), []byte("external")))
		_, err = store.Demote(named)
		require.ErrorIs(t, err, ErrRevision)
	})

	t.Run("draft collision", func(t *testing.T) {
		store := newTestStore(t)
		doc := testDocument(t, "named")
		named, err := store.Create("", "Canvas", doc)
		require.NoError(t, err)
		_, err = store.CreateDraft(doc)
		require.NoError(t, err)
		_, err = store.Demote(named)
		require.ErrorIs(t, err, ErrEntryExists)
		_, err = store.Load(named)
		require.NoError(t, err)
	})
}

func TestStoreImportPreservesSourceAndIdentity(t *testing.T) {
	t.Parallel()

	source := newTestStore(t)
	doc := testDocument(t, "shared")
	entry, err := source.Create("", "Shared", doc)
	require.NoError(t, err)
	path, err := source.Path(entry)
	require.NoError(t, err)
	want, err := os.ReadFile(path)
	require.NoError(t, err)

	target := newTestStore(t)
	draft, imported, err := target.Import(path)
	require.NoError(t, err)
	require.True(t, draft.Draft)
	require.Equal(t, doc.ID, imported.ID)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestStoreReturnsIndependentDocuments(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	entry, err := store.Create("", "Canvas", testDocument(t, "original"))
	require.NoError(t, err)
	first, err := store.Load(entry)
	require.NoError(t, err)
	second, err := store.Load(entry)
	require.NoError(t, err)
	first.Nodes[0].Label = "mutated"
	require.Equal(t, "original", second.Nodes[0].Label)
}

func TestStoreLoadIntoReusesDocumentCapacity(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	entry, err := store.Create("", "Canvas", testDocument(t, "original"))
	require.NoError(t, err)
	var dst document.Document
	require.NoError(t, store.LoadInto(entry, &dst))
	capacity := cap(dst.Nodes)
	require.NoError(t, store.LoadInto(entry, &dst))
	require.Equal(t, capacity, cap(dst.Nodes))
	require.Equal(t, "original", dst.Nodes[0].Label)
}

func TestStoreRejectsStaleRevision(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "original")
	entry, err := store.Create("", "Canvas", doc)
	require.NoError(t, err)
	path := store.namedPath("", "Canvas")
	external := testDocument(t, externalLabel)
	external.ID = doc.ID
	data, err := encodeDocument(external)
	require.NoError(t, err)
	require.NoError(t, replaceFile(path, data))

	_, err = store.Load(entry)
	require.ErrorIs(t, err, ErrRevision)
	_, err = store.Save(entry, doc)
	require.ErrorIs(t, err, ErrRevision)
	require.ErrorIs(t, store.Delete(entry), ErrRevision)
}

func TestStoreLoadsCurrentExternalRevision(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "original")
	entry, err := store.Create("", "Canvas", doc)
	require.NoError(t, err)
	external := doc
	external.Nodes[0].Label = externalLabel
	data, err := encodeDocument(external)
	require.NoError(t, err)
	require.NoError(t, replaceFile(store.namedPath("", "Canvas"), data))

	var loaded document.Document
	current, err := store.LoadCurrentInto(entry, &loaded)
	require.NoError(t, err)
	require.NotEqual(t, entry.Revision, current.Revision)
	require.Equal(t, externalLabel, loaded.Nodes[0].Label)
}

func TestStoreBacksUpRawExternalRevisionAndRestoresLocalDocument(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	local := testDocument(t, "local")
	entry, err := store.Create("", "Canvas", local)
	require.NoError(t, err)
	_, err = store.Create("", "Canvas.bak", testDocument(t, "occupied"))
	require.NoError(t, err)
	external := testDocument(t, externalLabel)
	external.ID = local.ID
	raw, err := encodeDocument(external)
	require.NoError(t, err)
	require.NoError(t, replaceFile(store.namedPath("", "Canvas"), raw))

	backup, restored, err := store.BackupAndRestore(entry, local)
	require.NoError(t, err)
	require.Equal(t, "Canvas.bak1", backup.Name)
	require.Equal(t, local.ID, restored.ID)
	got, err := os.ReadFile(store.namedPath("", "Canvas.bak1"))
	require.NoError(t, err)
	require.Equal(t, raw, got)
	loaded, err := store.Load(restored)
	require.NoError(t, err)
	require.Equal(t, "local", loaded.Nodes[0].Label)
}

func TestStoreBacksUpMalformedExternalBytes(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	local := testDocument(t, "local")
	entry, err := store.Create("", "Canvas", local)
	require.NoError(t, err)
	path, err := store.Path(entry)
	require.NoError(t, err)
	raw := []byte("not a document")
	require.NoError(t, replaceFile(path, raw))

	backup, restored, err := store.BackupAndRestore(entry, local)
	require.NoError(t, err)
	require.Equal(t, uuid.Nil, backup.ID)
	got, err := os.ReadFile(store.namedPath("", "Canvas.bak"))
	require.NoError(t, err)
	require.Equal(t, raw, got)
	loaded, err := store.Load(restored)
	require.NoError(t, err)
	require.Equal(t, "local", loaded.Nodes[0].Label)
}

func TestStorePreservesBackupWhenRestoreFails(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	local := testDocument(t, "local")
	entry, err := store.Create("", "Canvas", local)
	require.NoError(t, err)
	path, err := store.Path(entry)
	require.NoError(t, err)
	raw := []byte("external bytes")
	require.NoError(t, replaceFile(path, raw))
	wantErr := errors.New("restore failed")

	backup, _, err := store.backupAndRestore(entry, local, func(string, []byte) error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "Canvas.bak", backup.Name)
	_, statErr := os.Stat(path)
	require.ErrorIs(t, statErr, fs.ErrNotExist)
	got, readErr := os.ReadFile(store.namedPath("", "Canvas.bak"))
	require.NoError(t, readErr)
	require.Equal(t, raw, got)
}

func TestStoreRestoresDeletedCanvasAndPreservesDraft(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "local")
	entry, err := store.Create("", "Canvas", doc)
	require.NoError(t, err)
	path, err := store.Path(entry)
	require.NoError(t, err)
	require.NoError(t, os.Remove(path))

	restored, err := store.RestoreDeleted(entry, doc)
	require.NoError(t, err)
	loaded, err := store.Load(restored)
	require.NoError(t, err)
	require.Equal(t, "local", loaded.Nodes[0].Label)
	draft, err := store.PreserveDraft(doc)
	require.NoError(t, err)
	require.True(t, draft.Draft)
	doc.Nodes[0].Label = "newer"
	draft, err = store.PreserveDraft(doc)
	require.NoError(t, err)
	loaded, err = store.Load(draft)
	require.NoError(t, err)
	require.Equal(t, "newer", loaded.Nodes[0].Label)
}

func TestStoreCreateAndNameDoNotReplaceExistingCanvas(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	named, err := store.Create("", "Canvas", testDocument(t, "named"))
	require.NoError(t, err)
	_, err = store.Create("", "Canvas", testDocument(t, "other"))
	require.ErrorIs(t, err, ErrEntryExists)
	draft, err := store.CreateDraft(testDocument(t, "draft"))
	require.NoError(t, err)
	_, err = store.Name(draft, "", "Canvas")
	require.ErrorIs(t, err, ErrEntryExists)
	loaded, err := store.Load(named)
	require.NoError(t, err)
	require.Equal(t, "named", loaded.Nodes[0].Label)
	_, err = store.Load(draft)
	require.NoError(t, err)
}

func TestStoreAllowsSameNameInDifferentSections(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	_, err := store.Create("Interviews", "Candidate", testDocument(t, "first"))
	require.NoError(t, err)
	_, err = store.Create("RFCs", "Candidate", testDocument(t, "second"))
	require.NoError(t, err)
}

func TestStoreNamesDraftAndRecoversCompletedPromotion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	preferred := filepath.Join(root, "canvases")
	state := filepath.Join(root, "state")
	cache := filepath.Join(root, "cache")
	store, err := New(preferred, WithStateDir(state), WithCacheDir(cache))
	require.NoError(t, err)
	doc := testDocument(t, "draft")
	draft, err := store.CreateDraft(doc)
	require.NoError(t, err)

	data, err := os.ReadFile(store.draftPath(doc.ID))
	require.NoError(t, err)
	require.NoError(t, store.writePromotion(promotion{DraftID: doc.ID, Name: "Named"}))
	require.NoError(t, writeNew(store.namedPath("", "Named"), data))

	reopened, err := New(preferred, WithStateDir(state), WithCacheDir(cache))
	require.NoError(t, err)
	_, err = reopened.Load(draft)
	require.ErrorIs(t, err, ErrEntryNotFound)
	entries, err := reopened.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "Named", entries[0].Name)
}

func TestStoreRecoversCompletedDemotion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	preferred := filepath.Join(root, "canvases")
	state := filepath.Join(root, "state")
	cache := filepath.Join(root, "cache")
	store, err := New(preferred, WithStateDir(state), WithCacheDir(cache))
	require.NoError(t, err)
	doc := testDocument(t, "named")
	named, err := store.Create("RFCs", "Proposal", doc)
	require.NoError(t, err)
	source, err := store.Path(named)
	require.NoError(t, err)
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, store.writeDemotion(demotion{
		ID:       named.ID,
		Section:  named.Section,
		Name:     named.Name,
		Revision: named.Revision,
	}))
	require.NoError(t, writeNew(store.draftPath(named.ID), data))

	reopened, err := New(preferred, WithStateDir(state), WithCacheDir(cache))
	require.NoError(t, err)
	_, err = os.Stat(source)
	require.ErrorIs(t, err, fs.ErrNotExist)
	entries, err := reopened.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.True(t, entries[0].Draft)
	require.Equal(t, named.ID, entries[0].ID)
}

func TestStoreListsOneLevelAndClearsDrafts(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	_, err := store.Create("", "Root", testDocument(t, "root"))
	require.NoError(t, err)
	_, err = store.Create("Section", "Child", testDocument(t, "child"))
	require.NoError(t, err)
	preserve, err := store.CreateDraft(testDocument(t, "preserve"))
	require.NoError(t, err)
	_, err = store.CreateDraft(testDocument(t, "remove"))
	require.NoError(t, err)
	deep := filepath.Join(store.preferred, "One", "Two")
	require.NoError(t, os.MkdirAll(deep, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(deep, "Ignored.dg"), []byte("invalid"), 0o600))

	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 4)
	removed, err := store.ClearDrafts(preserve.ID)
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	entries, err = store.List()
	require.NoError(t, err)
	require.Len(t, entries, 3)
}

func TestStoreValidatesLocations(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "node")
	for _, location := range [][2]string{
		{"", ""},
		{"", "."},
		{"", ".."},
		{"", "a/b"},
		{"a/b", "name"},
		{"..", "name"},
	} {
		_, err := store.Create(location[0], location[1], doc)
		require.Error(t, err)
	}
}

func TestDocumentCodecRejectsMultipleMembersAndOversize(t *testing.T) {
	t.Parallel()

	encoded, err := encodeDocument(testDocument(t, "node"))
	require.NoError(t, err)
	_, err = decodeDocument(append(append([]byte(nil), encoded...), encoded...))
	require.ErrorContains(t, err, "multiple gzip members")

	var oversized bytes.Buffer
	writer := gzip.NewWriter(&oversized)
	_, err = writer.Write(make([]byte, maxDocumentSize+1))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	_, err = decodeDocument(oversized.Bytes())
	require.ErrorContains(t, err, "exceeds")
}

func TestBlobStoreOwnsValuesAndUsesFilesystemSemantics(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	blobs := store.History()
	value := []byte("history")
	require.NoError(t, blobs.Write("key", value))
	value[0] = 'X'
	loaded, err := blobs.Read("key")
	require.NoError(t, err)
	require.Equal(t, []byte("history"), loaded)
	loaded[0] = 'X'
	again, err := blobs.Read("key")
	require.NoError(t, err)
	require.Equal(t, []byte("history"), again)
	_, err = blobs.Read("missing")
	require.ErrorIs(t, err, fs.ErrNotExist)
	_, err = blobs.Read("../escape")
	require.Error(t, err)
}

func TestWarmCacheEnforcesCountAndByteBudget(t *testing.T) {
	t.Parallel()

	cache := newWarmCache(2, 5)
	cache.put("one", []byte("12"))
	cache.put("two", []byte("34"))
	cache.put("three", []byte("5"))
	_, ok := cache.get("one")
	require.False(t, ok)
	_, ok = cache.get("two")
	require.True(t, ok)
	cache.put("large", []byte("123456"))
	_, ok = cache.get("large")
	require.False(t, ok)
}

func TestStoreRejectsInvalidDraftIdentity(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "node")
	doc.ID = uuid.Nil
	_, err := store.CreateDraft(doc)
	require.Error(t, err)
}

func TestStoreMissingEntryErrors(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "node")
	entry := Entry{Name: "Missing", ID: doc.ID}
	_, err := store.Load(entry)
	require.ErrorIs(t, err, ErrEntryNotFound)
	_, err = store.Save(entry, doc)
	require.ErrorIs(t, err, ErrEntryNotFound)
	require.ErrorIs(t, store.Delete(entry), ErrEntryNotFound)
}

func TestStorePromotionJournalWithoutTargetPreservesDraft(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	preferred := filepath.Join(root, "canvases")
	state := filepath.Join(root, "state")
	cache := filepath.Join(root, "cache")
	store, err := New(preferred, WithStateDir(state), WithCacheDir(cache))
	require.NoError(t, err)
	draft, err := store.CreateDraft(testDocument(t, "draft"))
	require.NoError(t, err)
	require.NoError(t, store.writePromotion(promotion{DraftID: draft.ID, Name: "Missing"}))

	reopened, err := New(preferred, WithStateDir(state), WithCacheDir(cache))
	require.NoError(t, err)
	_, err = reopened.Load(draft)
	require.NoError(t, err)
	_, err = os.Stat(reopened.promotionPath())
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestStoreRejectsDocumentIdentityChange(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	entry, err := store.Create("", "Canvas", testDocument(t, "one"))
	require.NoError(t, err)
	_, err = store.Save(entry, testDocument(t, "two"))
	require.Error(t, err)
}

func TestStoreMoveReportsMissingAndCollision(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	first, err := store.Create("", "First", testDocument(t, "first"))
	require.NoError(t, err)
	_, err = store.Create("", "Second", testDocument(t, "second"))
	require.NoError(t, err)
	_, err = store.Move(first, "", "Second")
	require.ErrorIs(t, err, ErrEntryExists)
	require.NoError(t, os.Remove(store.namedPath("", "First")))
	_, err = store.Move(first, "", "Third")
	require.ErrorIs(t, err, ErrEntryNotFound)
}

func TestStoreCreateCollisionIsRaceFree(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	doc := testDocument(t, "node")
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := store.Create("", "Canvas", doc)
			results <- err
		}()
	}
	errs := []error{<-results, <-results}
	require.Equal(t, 1, countErrors(errs, nil))
	require.Equal(t, 1, countErrors(errs, ErrEntryExists))
}

func TestStoreBackupAllocationIsRaceFree(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	local := testDocument(t, "local")
	entry, err := store.Create("", "Canvas", local)
	require.NoError(t, err)
	external := testDocument(t, externalLabel)
	external.ID = local.ID
	raw, err := encodeDocument(external)
	require.NoError(t, err)
	require.NoError(t, replaceFile(store.namedPath("", "Canvas"), raw))

	backupResult := make(chan error, 1)
	createResult := make(chan error, 1)
	start := make(chan struct{})
	userBackup := testDocument(t, "user backup")
	go func() {
		<-start
		_, _, err := store.BackupAndRestore(entry, local)
		backupResult <- err
	}()
	go func() {
		<-start
		_, err := store.Create("", "Canvas.bak", userBackup)
		createResult <- err
	}()
	close(start)
	require.NoError(t, <-backupResult)
	createErr := <-createResult
	require.True(t, createErr == nil || errors.Is(createErr, ErrEntryExists))

	matched := false
	for _, name := range []string{"Canvas.bak", "Canvas.bak1"} {
		data, readErr := os.ReadFile(store.namedPath("", name))
		if readErr == nil && bytes.Equal(data, raw) {
			matched = true
		}
	}
	require.True(t, matched)
}

func countErrors(errs []error, target error) int {
	count := 0
	for _, err := range errs {
		if err == nil && target == nil || target != nil && errors.Is(err, target) {
			count++
		}
	}
	return count
}
