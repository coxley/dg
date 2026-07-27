package layout

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

type mapHistoryStore struct {
	mu     sync.Mutex
	files  fstest.MapFS
	writes int
	err    error
}

func newMapHistoryStore() *mapHistoryStore {
	return &mapHistoryStore{files: make(fstest.MapFS)}
}

func (s *mapHistoryStore) Read(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.files.ReadFile(name)
}

func (s *mapHistoryStore) Write(name string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.files[name] = &fstest.MapFile{
		Data: append([]byte(nil), data...),
		Mode: 0o600,
	}
	s.writes++
	return nil
}

func (s *mapHistoryStore) writeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

func TestHistoryCacheDebouncesCommittedInteractions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newMapHistoryStore()
		history, err := NewHistory(
			WithHistoryStore(store),
			WithHistoryCacheDelay(100*time.Millisecond),
		)
		require.NoError(t, err)
		geo, err := New(WithHistory(history))
		require.NoError(t, err)
		nodeID, err := geo.NewNode("one")
		require.NoError(t, err)
		require.NoError(t, history.Store("diagram.json"))
		require.Equal(t, 1, store.writeCount())

		require.NoError(t, geo.SetNodeLabel(nodeID, "two"))
		time.Sleep(99 * time.Millisecond)
		require.Equal(t, 1, store.writeCount())
		require.NoError(t, geo.SetNodeLabel(nodeID, "three"))
		time.Sleep(99 * time.Millisecond)
		require.Equal(t, 1, store.writeCount())
		time.Sleep(time.Millisecond)
		synctest.Wait()
		require.Equal(t, 2, store.writeCount())
	})
}

func TestHistoryRestoreMissingCacheStartsPersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newMapHistoryStore()
		history, err := NewHistory(
			WithHistoryStore(store),
			WithHistoryCacheDelay(100*time.Millisecond),
		)
		require.NoError(t, err)
		geo, err := New(WithHistory(history))
		require.NoError(t, err)
		nodeID, err := geo.NewNode("saved")
		require.NoError(t, err)
		history.Clear()

		ok, err := history.Restore("diagram.json")
		require.NoError(t, err)
		require.False(t, ok)
		require.NoError(t, geo.SetNodeLabel(nodeID, "unsaved"))
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		require.Equal(t, 1, store.writeCount())
	})
}

func TestHistoryCacheUsesGzip(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	history, err := NewHistory(WithHistoryStore(store))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	_, err = geo.NewNode("saved")
	require.NoError(t, err)
	require.NoError(t, history.Store("diagram.json"))

	_, key, err := historyCacheKey("diagram.json")
	require.NoError(t, err)
	compressed, err := store.Read(key)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(compressed, []byte{0x1f, 0x8b}))
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	plain, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Less(t, len(compressed), len(plain))

	compressedCache, ok := decodeHistoryCache(compressed)
	require.True(t, ok)
	plainCache, ok := decodeHistoryCache(plain)
	require.True(t, ok)
	require.Equal(t, compressedCache, plainCache)
}

func TestHistoryRestoreKeepsSavedRenderAndRecoversRedoTail(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	history, err := NewHistory(WithHistoryStore(store))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	nodeID, err := geo.NewNodeAt("saved", NewPoint(4, 5))
	require.NoError(t, err)
	require.NoError(t, geo.Build())
	require.NoError(t, history.Store("diagram.json"))

	require.NoError(t, geo.SetNodeLabel(nodeID, "unsaved"))
	require.NoError(t, geo.PlaceNode(nodeID, NewPoint(20, 30)))
	require.NoError(t, history.Flush())

	restoredHistory, err := NewHistory(WithHistoryStore(store))
	require.NoError(t, err)
	restored, err := New(WithHistory(restoredHistory))
	require.NoError(t, err)
	restoredID, err := restored.NewNodeAt("saved", NewPoint(4, 5))
	require.NoError(t, err)
	require.NoError(t, restored.Build())

	ok, err := restoredHistory.Restore("diagram.json")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "saved", restored.Label(restoredID))
	require.Equal(t, NewPoint(4, 5), restored.Nodes[restoredID].Rect.Min)

	changed, err := restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "unsaved", restored.Label(restoredID))
	changed, err = restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, NewPoint(20, 30), restored.Nodes[restoredID].Rect.Min)
}

func TestHistoryRestoreRecoversExactDeletedSlots(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	history, err := NewHistory(WithHistoryStore(store))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	first, err := geo.NewNodeAt("first", NewPoint(1, 1))
	require.NoError(t, err)
	deleted, err := geo.NewNodeAt("deleted", NewPoint(10, 1))
	require.NoError(t, err)
	last, err := geo.NewNodeAt("last", NewPoint(20, 1))
	require.NoError(t, err)
	require.NoError(t, geo.DeleteNode(deleted))
	require.NoError(t, history.Store("diagram.json"))

	restoredHistory, err := NewHistory(WithHistoryStore(store))
	require.NoError(t, err)
	restored, err := New(WithHistory(restoredHistory))
	require.NoError(t, err)
	_, err = restored.NewNodeAt("first", NewPoint(1, 1))
	require.NoError(t, err)
	_, err = restored.NewNodeAt("last", NewPoint(20, 1))
	require.NoError(t, err)
	restoredHistory.Clear()

	ok, err := restoredHistory.Restore("diagram.json")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, restored.NodeExists(first))
	require.False(t, restored.NodeExists(deleted))
	require.True(t, restored.NodeExists(last))
	require.Equal(t, "last", restored.Label(last))

	changed, err := restoredHistory.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, restored.NodeExists(deleted))
	require.Equal(t, "deleted", restored.Label(deleted))
}

func TestHistoryCacheRejectsDifferentSavedDocument(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	history, err := NewHistory(WithHistoryStore(store))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	_, err = geo.NewNode("saved")
	require.NoError(t, err)
	require.NoError(t, history.Store("diagram.json"))

	otherHistory, err := NewHistory(WithHistoryStore(store))
	require.NoError(t, err)
	other, err := New(WithHistory(otherHistory))
	require.NoError(t, err)
	_, err = other.NewNode("different")
	require.NoError(t, err)
	ok, err := otherHistory.Restore("diagram.json")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, "different", other.Label(0))
}

func TestHistoryCacheFailureDoesNotBlockEditing(t *testing.T) {
	t.Parallel()

	cacheErr := errors.New("cache unavailable")
	store := newMapHistoryStore()
	store.err = cacheErr
	history, err := NewHistory(WithHistoryStore(store))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	nodeID, err := geo.NewNode("before")
	require.NoError(t, err)

	require.ErrorIs(t, history.Store("diagram.json"), cacheErr)
	require.NoError(t, geo.SetNodeLabel(nodeID, "after"))
	require.Equal(t, "after", geo.Label(nodeID))
	require.ErrorIs(t, history.CacheError(), cacheErr)
}

func TestHistoryCacheDirectoryStore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	history, err := NewHistory(WithHistoryCacheDir(dir))
	require.NoError(t, err)
	geo, err := New(WithHistory(history))
	require.NoError(t, err)
	_, err = geo.NewNode("node")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "diagram.json")
	require.NoError(t, history.Store(path))

	_, key, err := historyCacheKey(path)
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(dir, key))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestHistoryOptionsValidateCacheConfiguration(t *testing.T) {
	t.Parallel()

	_, err := NewHistory(WithHistoryCacheDelay(-1))
	require.Error(t, err)
	_, err = NewHistory(WithHistoryCacheDir(""))
	require.Error(t, err)
	_, err = NewHistory(WithHistoryStore(nil))
	require.Error(t, err)
	_, err = NewHistory(
		WithHistoryCacheDir(t.TempDir()),
		WithHistoryStore(newMapHistoryStore()),
	)
	require.Error(t, err)
	_, _, err = historyCacheKey("")
	require.Error(t, err)
}
