package history

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

type mapHistoryStore struct {
	mu     sync.Mutex
	files  fstest.MapFS
	writes int
	err    error
}

type blockingHistoryStore struct {
	*mapHistoryStore

	mu         sync.Mutex
	calls      int
	blockWrite int
	started    chan struct{}
	release    chan struct{}
}

func newBlockingHistoryStore(blockWrite int) *blockingHistoryStore {
	return &blockingHistoryStore{
		mapHistoryStore: newMapHistoryStore(),
		blockWrite:      blockWrite,
		started:         make(chan struct{}),
		release:         make(chan struct{}),
	}
}

func (s *blockingHistoryStore) Write(name string, data []byte) error {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == s.blockWrite {
		close(s.started)
		<-s.release
	}
	return s.mapHistoryStore.Write(name, data)
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

func newHistoryLayout(t testing.TB, options ...Option) (*layout.Layout, *History) {
	t.Helper()
	geo, err := layout.New()
	require.NoError(t, err)
	history, err := New(geo, options...)
	require.NoError(t, err)
	return geo, history
}

func TestHistoryCacheDebouncesCommittedInteractions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newMapHistoryStore()
		geo, history := newHistoryLayout(
			t,
			WithStore(store),
			WithCacheDelay(100*time.Millisecond),
		)
		nodeID, err := geo.NewNode("one")
		require.NoError(t, err)
		require.NoError(t, history.Save("diagram.json"))
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

func TestHistoryCacheNewGenerationOverwritesBlockedWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newBlockingHistoryStore(2)
		geo, history := newHistoryLayout(
			t,
			WithStore(store),
			WithCacheDelay(100*time.Millisecond),
		)
		nodeID, err := geo.NewNode("one")
		require.NoError(t, err)
		require.NoError(t, history.Save("diagram.json"))
		require.NoError(t, geo.SetNodeLabel(nodeID, "two"))
		time.Sleep(100 * time.Millisecond)
		<-store.started

		require.NoError(t, geo.SetNodeLabel(nodeID, "three"))
		time.Sleep(100 * time.Millisecond)
		close(store.release)
		synctest.Wait()
		require.Equal(t, 3, store.writeCount())

		restored, restoredHistory := newHistoryLayout(t, WithStore(store))
		restoredID, err := restored.NewNode("one")
		require.NoError(t, err)
		ok, err := restoredHistory.Restore("diagram.json")
		require.NoError(t, err)
		require.True(t, ok)
		for range 2 {
			changed, redoErr := restoredHistory.Redo()
			require.NoError(t, redoErr)
			require.True(t, changed)
		}
		require.Equal(t, "three", restored.Label(restoredID))
	})
}

func TestHistoryResetDetachesPreviousCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newMapHistoryStore()
		geo, history := newHistoryLayout(
			t,
			WithStore(store),
			WithCacheDelay(100*time.Millisecond),
		)
		_, err := geo.NewNode("saved")
		require.NoError(t, err)
		require.NoError(t, history.Save("original.dg"))

		replacement, err := layout.New()
		require.NoError(t, err)
		_, err = replacement.NewNode("replacement")
		require.NoError(t, err)
		require.NoError(t, history.Reset(func() error {
			return geo.Restore(replacement.Snapshot())
		}))
		require.NoError(t, geo.SetNodeLabel(0, "changed"))
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		require.Equal(t, 1, store.writeCount())
	})
}

func TestHistoryRestoreMissingCacheStartsPersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newMapHistoryStore()
		geo, history := newHistoryLayout(
			t,
			WithStore(store),
			WithCacheDelay(100*time.Millisecond),
		)
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
	geo, history := newHistoryLayout(t, WithStore(store))
	_, err := geo.NewNode("saved")
	require.NoError(t, err)
	require.NoError(t, history.Save("diagram.json"))

	_, key, err := cacheKey("diagram.json")
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

	compressedCache, ok := decodeCache(compressed)
	require.True(t, ok)
	require.NotEmpty(t, compressedCache.Entries)
	require.False(t, bytes.HasPrefix(plain, []byte{0x1f, 0x8b}))
}

func TestHistoryRestoreKeepsSavedRenderAndRecoversRedoTail(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	geo, history := newHistoryLayout(t, WithStore(store))
	nodeID, err := geo.NewNodeAt("saved", layout.NewPoint(4, 5))
	require.NoError(t, err)
	require.NoError(t, geo.Build())
	require.NoError(t, history.Save("diagram.json"))

	explicit := layout.Size{Width: 10, Height: 5}
	require.NoError(t, geo.SetNodeSize(nodeID, explicit))
	require.NoError(t, geo.SetNodeLabel(nodeID, "unsaved"))
	require.NoError(t, geo.PlaceNode(nodeID, layout.NewPoint(20, 30)))
	require.NoError(t, history.Flush())

	restored, restoredHistory := newHistoryLayout(t, WithStore(store))
	restoredID, err := restored.NewNodeAt("saved", layout.NewPoint(4, 5))
	require.NoError(t, err)
	require.NoError(t, restored.Build())

	ok, err := restoredHistory.Restore("diagram.json")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "saved", restored.Label(restoredID))
	require.Equal(t, layout.NewPoint(4, 5), restored.Nodes[restoredID].Rect.Min)

	changed, err := restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, explicit, explicitNodeSize(t, restored, restoredID))
	changed, err = restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "unsaved", restored.Label(restoredID))
	changed, err = restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, layout.NewPoint(20, 30), restored.Nodes[restoredID].Rect.Min)
}

func TestHistoryRestoreDiscardsCacheLargerThanConfiguredLimit(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	geo, history := newHistoryLayout(t, WithStore(store))
	nodeID, err := geo.NewNode("zero")
	require.NoError(t, err)
	require.NoError(t, geo.SetNodeLabel(nodeID, "one"))
	require.NoError(t, geo.SetNodeLabel(nodeID, "two"))
	require.NoError(t, history.Save("diagram.json"))
	require.NoError(t, geo.SetNodeLabel(nodeID, "three"))
	require.NoError(t, history.Flush())

	restored, restoredHistory := newHistoryLayout(
		t,
		WithStore(store),
		WithLimit(2),
	)
	restoredID, err := restored.NewNode("two")
	require.NoError(t, err)

	ok, err := restoredHistory.Restore("diagram.json")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "two", restored.Label(restoredID))
	require.False(t, restoredHistory.CanUndo())
	require.False(t, restoredHistory.CanRedo())
}

func TestHistoryRestoreRecoversAttachmentRedoTail(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	geo, history := newHistoryLayout(t, WithStore(store))
	geo, node := historyAttachmentLayout(t, geo, history)
	saved := mustAttachment(t, geo, node)
	require.NoError(t, history.Save("diagram.json"))

	require.NoError(t, geo.DetachNode(node))
	require.NoError(t, history.Flush())

	restored, restoredHistory := newHistoryLayout(t, WithStore(store))
	restored, restoredNode := historyAttachmentLayout(t, restored, restoredHistory)
	ok, err := restoredHistory.Restore("diagram.json")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, saved, mustAttachment(t, restored, restoredNode))

	changed, err := restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	_, attached := restored.NodeAttachment(restoredNode)
	require.False(t, attached)
}

func historyAttachmentLayout(
	t testing.TB,
	geo *layout.Layout,
	history *History,
) (*layout.Layout, uint32) {
	t.Helper()

	source, err := geo.NewNodeAt("source", layout.NewPoint(10, 10))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", layout.NewPoint(60, 10))
	require.NoError(t, err)
	node, err := geo.NewNodeAt("tag", layout.NewPoint(30, 20))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	point := midpointOnRoute(t, geo.Edges[edge].Points)
	require.NoError(t, geo.PlaceNode(node, layout.NewPoint(point.X-1, point.Y-1)))
	require.NoError(t, geo.AttachNode(node, edge, point))
	history.Clear()
	return geo, node
}

func mustAttachment(t testing.TB, geo *layout.Layout, nodeID uint32) layout.Attachment {
	t.Helper()
	attachment, ok := geo.NodeAttachment(nodeID)
	require.True(t, ok)
	return attachment
}

func TestHistoryRestoreRecoversLayerRedoTail(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	geo, history := newHistoryLayout(t, WithStore(store))
	back, err := geo.NewNodeAt("back", layout.NewPoint(2, 2))
	require.NoError(t, err)
	front, err := geo.NewNodeAt("front", layout.NewPoint(20, 2))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(
		back,
		ir.RightSide,
		ir.LeftSide,
		front,
	)
	edgeHit := layout.Hit{ID: edgeID, Kind: layout.HitEdge}
	backHit := layout.Hit{ID: back, Kind: layout.HitNode}
	require.NoError(t, geo.SendToBack(edgeHit))
	savedOrder := slices.Collect(geo.DrawOrder())
	require.NoError(t, history.Save("diagram.json"))

	require.NoError(t, geo.BringToFront(backHit))
	unsavedOrder := slices.Collect(geo.DrawOrder())
	require.NoError(t, history.Flush())

	restored, restoredHistory := newHistoryLayout(t, WithStore(store))
	restoredBack, err := restored.NewNodeAt("back", layout.NewPoint(2, 2))
	require.NoError(t, err)
	restoredFront, err := restored.NewNodeAt("front", layout.NewPoint(20, 2))
	require.NoError(t, err)
	restoredEdge := restored.ConnectNodes(
		restoredBack,
		ir.RightSide,
		ir.LeftSide,
		restoredFront,
	)
	require.NoError(t, restored.SendToBack(layout.Hit{
		ID:   restoredEdge,
		Kind: layout.HitEdge,
	}))

	ok, err := restoredHistory.Restore("diagram.json")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, savedOrder, slices.Collect(restored.DrawOrder()))
	changed, err := restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, unsavedOrder, slices.Collect(restored.DrawOrder()))
}

func TestHistoryRestoreRecoversStyleAndBendRedoTail(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	geo, history := newHistoryLayout(t, WithStore(store))
	left, err := geo.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	right, err := geo.NewNodeAt("right", layout.NewPoint(20, 2))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(left, ir.RightSide, ir.LeftSide, right)
	require.NoError(t, history.Save("diagram.json"))

	nodeStyle := layout.NodeStyle{Border: layout.BorderRounded}
	edgeStyle := layout.EdgeStyle{
		PortAArrow: layout.ArrowOpen,
		PortBArrow: layout.ArrowFilled,
	}
	bends := []layout.PinnedBend{{
		Point:    layout.NewPoint(12, 3),
		Incoming: layout.East,
		Outgoing: layout.South,
	}}
	require.NoError(t, geo.SetNodeStyle(left, nodeStyle))
	require.NoError(t, geo.SetEdgeStyle(edgeID, edgeStyle))
	require.NoError(t, geo.SetPinnedBends(edgeID, bends))
	require.NoError(t, history.Flush())

	restored, restoredHistory := newHistoryLayout(t, WithStore(store))
	restoredLeft, err := restored.NewNodeAt("left", layout.NewPoint(2, 2))
	require.NoError(t, err)
	restoredRight, err := restored.NewNodeAt("right", layout.NewPoint(20, 2))
	require.NoError(t, err)
	restoredEdge := restored.ConnectNodes(
		restoredLeft,
		ir.RightSide,
		ir.LeftSide,
		restoredRight,
	)

	ok, err := restoredHistory.Restore("diagram.json")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, layout.NodeStyle{}, mustNodeStyle(t, restored, restoredLeft))
	require.Equal(t, layout.EdgeStyle{}, mustEdgeStyle(t, restored, restoredEdge))

	changed, err := restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, nodeStyle, mustNodeStyle(t, restored, restoredLeft))
	changed, err = restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, edgeStyle, mustEdgeStyle(t, restored, restoredEdge))
	changed, err = restoredHistory.Redo()
	require.NoError(t, err)
	require.True(t, changed)
	restoredBends, err := restored.PinnedBends(restoredEdge)
	require.NoError(t, err)
	require.Equal(t, bends, restoredBends)
}

func TestHistoryRestoreRecoversExactDeletedSlots(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	geo, history := newHistoryLayout(t, WithStore(store))
	first, err := geo.NewNodeAt("first", layout.NewPoint(1, 1))
	require.NoError(t, err)
	deleted, err := geo.NewNodeAt("deleted", layout.NewPoint(10, 1))
	require.NoError(t, err)
	last, err := geo.NewNodeAt("last", layout.NewPoint(20, 1))
	require.NoError(t, err)
	require.NoError(t, geo.DeleteNode(deleted))
	require.NoError(t, history.Save("diagram.json"))

	restored, restoredHistory := newHistoryLayout(t, WithStore(store))
	_, err = restored.NewNodeAt("first", layout.NewPoint(1, 1))
	require.NoError(t, err)
	_, err = restored.NewNodeAt("last", layout.NewPoint(20, 1))
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

func mustNodeStyle(t testing.TB, geo *layout.Layout, nodeID uint32) layout.NodeStyle {
	t.Helper()
	style, ok := geo.NodeStyle(nodeID)
	require.True(t, ok)
	return style
}

func mustEdgeStyle(t testing.TB, geo *layout.Layout, edgeID uint32) layout.EdgeStyle {
	t.Helper()
	style, ok := geo.EdgeStyle(edgeID)
	require.True(t, ok)
	return style
}

func TestHistoryCacheRejectsDifferentSavedDocument(t *testing.T) {
	t.Parallel()

	store := newMapHistoryStore()
	geo, history := newHistoryLayout(t, WithStore(store))
	_, err := geo.NewNode("saved")
	require.NoError(t, err)
	require.NoError(t, history.Save("diagram.json"))

	other, otherHistory := newHistoryLayout(t, WithStore(store))
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
	geo, history := newHistoryLayout(t, WithStore(store))
	nodeID, err := geo.NewNode("before")
	require.NoError(t, err)

	require.ErrorIs(t, history.Save("diagram.json"), cacheErr)
	require.NoError(t, geo.SetNodeLabel(nodeID, "after"))
	require.Equal(t, "after", geo.Label(nodeID))
	require.ErrorIs(t, history.CacheError(), cacheErr)
}

func TestHistoryCacheDirectoryStore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	geo, history := newHistoryLayout(t, WithCacheDir(dir))
	_, err := geo.NewNode("node")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "diagram.json")
	require.NoError(t, history.Save(path))

	_, key, err := cacheKey(path)
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(dir, key))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestHistoryOptionsValidateCacheConfiguration(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	_, err = New(geo, WithCacheDelay(-1))
	require.Error(t, err)
	geo, err = layout.New()
	require.NoError(t, err)
	_, err = New(geo, WithCacheDir(""))
	require.Error(t, err)
	geo, err = layout.New()
	require.NoError(t, err)
	_, err = New(geo, WithStore(nil))
	require.Error(t, err)
	geo, err = layout.New()
	require.NoError(t, err)
	_, err = New(
		geo,
		WithCacheDir(t.TempDir()),
		WithStore(newMapHistoryStore()),
	)
	require.Error(t, err)
	_, _, err = cacheKey("")
	require.Error(t, err)
}

func TestHistoryCacheRejectsInvalidLayerOrder(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	_, err = geo.NewNode("node")
	require.NoError(t, err)
	encodedSnapshot, err := json.Marshal(geo.Snapshot())
	require.NoError(t, err)
	var saved map[string]any
	require.NoError(t, json.Unmarshal(encodedSnapshot, &saved))
	saved["order"] = []any{map[string]any{
		"ID":   float64(1),
		"Kind": float64(layout.HitNode),
	}}
	invalid := map[string]any{
		"version":      cacheVersion,
		"saved_digest": "invalid",
		"saved":        saved,
		"entries":      []any{},
		"saved_cursor": 0,
	}
	plain, err := json.Marshal(invalid)
	require.NoError(t, err)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err = writer.Write(plain)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	_, ok := decodeCache(compressed.Bytes())
	require.False(t, ok)
}

func midpointOnRoute(t testing.TB, points []layout.Point) layout.Point {
	t.Helper()
	require.GreaterOrEqual(t, len(points), 2)
	length := uint32(0)
	for i := 1; i < len(points); i++ {
		length += absDiff(points[i-1].X, points[i].X) + absDiff(points[i-1].Y, points[i].Y)
	}
	require.Greater(t, length, uint32(2))
	distance := length / 2
	for i := 1; i < len(points); i++ {
		from, to := points[i-1], points[i]
		segment := absDiff(from.X, to.X) + absDiff(from.Y, to.Y)
		if distance <= segment {
			switch {
			case from.X < to.X:
				return layout.NewPoint(from.X+distance, from.Y)
			case from.X > to.X:
				return layout.NewPoint(from.X-distance, from.Y)
			case from.Y < to.Y:
				return layout.NewPoint(from.X, from.Y+distance)
			default:
				return layout.NewPoint(from.X, from.Y-distance)
			}
		}
		distance -= segment
	}
	t.Fatalf("route %v has no midpoint", points)
	return layout.Point{}
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
