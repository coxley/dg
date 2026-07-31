package history

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/layout"
	"github.com/google/uuid"
)

const (
	cacheVersion      = 5
	defaultCacheDelay = 100 * time.Millisecond
)

// Store reads and atomically replaces name-keyed cache values.
type Store interface {
	Read(name string) ([]byte, error)
	Write(name string, data []byte) error
}

// WithCacheDir stores cache files directly in dir.
func WithCacheDir(dir string) Option {
	return func(h *History) {
		h.cache.dir = dir
		h.cache.customDir = true
	}
}

// WithStore replaces filesystem cache access.
func WithStore(store Store) Option {
	return func(h *History) {
		h.cache.store = store
		h.cache.customStore = true
	}
}

// WithCacheDelay changes the inactivity interval before cache writes.
func WithCacheDelay(delay time.Duration) Option {
	return func(h *History) {
		h.cache.delay = delay
	}
}

type directoryStore struct {
	dir string
}

func (s directoryStore) Read(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.dir, name)) //nolint:gosec // name is a generated digest.
}

func (s directoryStore) Write(name string, data []byte) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create history cache directory: %w", err)
	}
	file, err := os.CreateTemp(s.dir, ".history-*")
	if err != nil {
		return fmt.Errorf("create history cache temporary file: %w", err)
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		return cleanupCacheTemp(file, path, fmt.Errorf("set history cache permissions: %w", err))
	}
	if _, err := file.Write(data); err != nil {
		return cleanupCacheTemp(file, path, fmt.Errorf("write history cache: %w", err))
	}
	if err := file.Close(); err != nil {
		return cleanupCachePath(path, fmt.Errorf("close history cache: %w", err))
	}
	if err := os.Rename(path, filepath.Join(s.dir, name)); err != nil {
		return cleanupCachePath(path, fmt.Errorf("replace history cache: %w", err))
	}
	return nil
}

func cleanupCacheTemp(file *os.File, path string, cause error) error {
	return errors.Join(cause, file.Close(), removeCacheTemp(path))
}

func cleanupCachePath(path string, cause error) error {
	return errors.Join(cause, removeCacheTemp(path))
}

func removeCacheTemp(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

type cacheFile struct {
	Version     uint32          `json:"version"`
	DocumentID  uuid.UUID       `json:"document_id"`
	DocumentCRC uint32          `json:"document_crc"`
	Saved       layout.Snapshot `json:"saved"`
	Entries     []cacheEntry    `json:"entries"`
	SavedCursor int             `json:"saved_cursor"`
}

type cacheEntry struct {
	Changes []layout.Change `json:"changes"`
}

type cacheWrite struct {
	generation uint64
	store      Store
	key        string
	data       []byte
}

type cacheState struct {
	dir         string
	store       Store
	delay       time.Duration
	customDir   bool
	customStore bool
	key         string
	documentID  uuid.UUID
	documentCRC uint32
	saved       layout.Snapshot
	savedCursor int
	branchValid bool

	mu         sync.Mutex
	writeMu    sync.Mutex
	timer      *time.Timer
	generation uint64
	flushed    uint64
	pending    cacheWrite
	err        error
}

func (c *cacheState) validate() error {
	switch {
	case c.delay < 0:
		return errors.New("history cache delay must not be negative")
	case c.customDir && c.customStore:
		return errors.New("history cache directory and store are mutually exclusive")
	case c.customDir && c.dir == "":
		return errors.New("history cache directory must not be empty")
	case c.customStore && c.store == nil:
		return errors.New("history cache store must not be nil")
	default:
		return nil
	}
}

// Save marks the current document as saved and writes its history.
func (h *History) Save(doc document.Document) error {
	if h == nil || h.layout == nil {
		return errors.New("history is not attached")
	}
	h.Interrupt()
	if err := h.layout.Build(); err != nil {
		return fmt.Errorf("build history layout: %w", err)
	}
	key, guard, err := cacheIdentity(doc)
	if err != nil {
		return err
	}
	store, err := h.cacheStore()
	if err != nil {
		return err
	}
	h.cache.key = key
	h.cache.documentID = doc.ID
	h.cache.documentCRC = guard
	h.cache.savedCursor = h.cursor
	h.cache.saved = h.layout.Snapshot()
	h.cache.branchValid = true
	data, err := h.marshalCache()
	if err != nil {
		return err
	}
	h.cache.clearPending()
	generation := h.cache.currentGeneration()
	err = h.writeCache(cacheWrite{generation: generation, store: store, key: key, data: data})
	h.cache.markFlushed(generation, err)
	h.setCacheError(err)
	return err
}

// Restore attaches cached history for doc without changing its saved render.
func (h *History) Restore(doc document.Document) (bool, error) {
	if h == nil || h.layout == nil {
		return false, errors.New("history is not attached")
	}
	key, guard, err := cacheIdentity(doc)
	if err != nil {
		return false, err
	}
	store, err := h.cacheStore()
	if err != nil {
		return false, err
	}
	h.cache.clearPending()
	h.markCache(doc.ID, key, guard)
	data, err := store.Read(key)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		err = fmt.Errorf("read history cache: %w", err)
		h.setCacheError(err)
		return false, err
	}
	cache, ok := decodeCache(data)
	if !ok {
		return false, nil
	}
	if cache.DocumentID != doc.ID || cache.DocumentCRC != guard ||
		cache.SavedCursor < 0 || cache.SavedCursor > len(cache.Entries) {
		return false, nil
	}
	entries := make([]entry, len(cache.Entries))
	for i := range cache.Entries {
		entries[i].changes = cache.Entries[i].Changes
	}
	if len(entries) > h.limit {
		entries = nil
		cache.SavedCursor = 0
	}
	if err := h.layout.Restore(cache.Saved); err != nil {
		return false, fmt.Errorf("restore cached history layout: %w", err)
	}
	h.entries = entries
	h.cursor = cache.SavedCursor
	h.cache.savedCursor = cache.SavedCursor
	h.cache.saved = cache.Saved
	h.cache.key = key
	h.cache.documentID = doc.ID
	h.cache.documentCRC = guard
	h.cache.branchValid = true
	h.closeActive()
	h.setCacheError(nil)
	return true, nil
}

func (h *History) markCache(id uuid.UUID, key string, guard uint32) {
	h.cache.key = key
	h.cache.documentID = id
	h.cache.documentCRC = guard
	h.cache.savedCursor = h.cursor
	h.cache.saved = h.layout.Snapshot()
	h.cache.branchValid = true
	h.cache.markCurrentFlushed()
	h.setCacheError(nil)
}

// Dirty reports whether cache state changed after its last successful flush.
func (h *History) Dirty() bool {
	if h == nil {
		return false
	}
	h.cache.mu.Lock()
	defer h.cache.mu.Unlock()
	return h.cache.key != "" && h.cache.generation != h.cache.flushed
}

// Flush synchronously writes the latest pending cache snapshot.
func (h *History) Flush() error {
	if h == nil {
		return nil
	}
	h.cache.mu.Lock()
	if h.cache.timer != nil {
		h.cache.timer.Stop()
	}
	pending := h.cache.pending
	h.cache.pending = cacheWrite{}
	h.cache.mu.Unlock()
	if pending.data == nil {
		return h.CacheError()
	}
	err := h.writeCache(pending)
	h.cache.markFlushed(pending.generation, err)
	h.setCacheError(err)
	return err
}

// CacheError returns the most recent asynchronous cache error.
func (h *History) CacheError() error {
	if h == nil {
		return nil
	}
	h.cache.mu.Lock()
	defer h.cache.mu.Unlock()
	return h.cache.err
}

func (h *History) scheduleCache() {
	if h.cache.key == "" || !h.cache.branchValid {
		return
	}
	data, err := h.marshalCache()
	if err != nil {
		h.setCacheError(err)
		return
	}
	store, err := h.cacheStore()
	if err != nil {
		h.setCacheError(err)
		return
	}
	h.cache.mu.Lock()
	h.cache.generation++
	generation := h.cache.generation
	h.cache.pending = cacheWrite{generation: generation, store: store, key: h.cache.key, data: data}
	if h.cache.timer != nil {
		h.cache.timer.Stop()
	}
	h.cache.timer = time.AfterFunc(h.cache.delay, func() { h.flushGeneration(generation) })
	h.cache.mu.Unlock()
}

func (h *History) flushGeneration(generation uint64) {
	h.cache.mu.Lock()
	if h.cache.pending.generation != generation {
		h.cache.mu.Unlock()
		return
	}
	pending := h.cache.pending
	h.cache.pending = cacheWrite{}
	h.cache.mu.Unlock()
	err := h.writeCache(pending)
	h.cache.mu.Lock()
	if h.cache.generation == generation {
		h.cache.err = err
		if err == nil {
			h.cache.flushed = generation
		}
	}
	h.cache.mu.Unlock()
}

func (h *History) marshalCache() ([]byte, error) {
	entries, savedCursor := h.cacheEntries()
	cache := cacheFile{
		Version:     cacheVersion,
		DocumentID:  h.cache.documentID,
		DocumentCRC: h.cache.documentCRC,
		Saved:       h.cache.saved,
		Entries:     make([]cacheEntry, len(entries)),
		SavedCursor: savedCursor,
	}
	for i := range entries {
		cache.Entries[i].Changes = entries[i].changes
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return nil, fmt.Errorf("encode history cache: %w", err)
	}
	return data, nil
}

func (h *History) cacheEntries() ([]entry, int) {
	for i := range h.entries {
		if !h.entries[i].reload {
			continue
		}
		if h.cache.savedCursor <= i {
			return h.entries[:i], h.cache.savedCursor
		}
		return h.entries[i+1:], h.cache.savedCursor - i - 1
	}
	return h.entries, h.cache.savedCursor
}

func compressCache(data []byte) ([]byte, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(data); err != nil {
		return nil, errors.Join(fmt.Errorf("compress history cache: %w", err), writer.Close())
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish history cache compression: %w", err)
	}
	return compressed.Bytes(), nil
}

func decodeCache(data []byte) (cacheFile, bool) {
	var source io.Reader = bytes.NewReader(data)
	var compressed *gzip.Reader
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		var err error
		compressed, err = gzip.NewReader(source)
		if err != nil {
			return cacheFile{}, false
		}
		source = compressed
	}
	cache, ok := decodeCacheJSON(source)
	if compressed != nil && compressed.Close() != nil {
		return cacheFile{}, false
	}
	return cache, ok
}

func decodeCacheJSON(source io.Reader) (cacheFile, bool) {
	decoder := json.NewDecoder(source)
	decoder.DisallowUnknownFields()
	var cache cacheFile
	if err := decoder.Decode(&cache); err != nil ||
		cache.Version != cacheVersion ||
		cache.DocumentID == uuid.Nil ||
		cache.DocumentID.Version() != 4 {
		return cacheFile{}, false
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return cacheFile{}, false
	}
	return cache, true
}

func (h *History) cacheStore() (Store, error) {
	if err := h.cache.validate(); err != nil {
		return nil, err
	}
	if h.cache.store != nil {
		return h.cache.store, nil
	}
	dir := h.cache.dir
	if dir == "" {
		root, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("locate user cache directory: %w", err)
		}
		app := "dg"
		if runtime.GOOS == "darwin" {
			app = "org.coxley.dg"
		}
		dir = filepath.Join(root, app, "history")
	}
	h.cache.store = directoryStore{dir: dir}
	return h.cache.store, nil
}

func (h *History) writeCache(write cacheWrite) error {
	h.cache.writeMu.Lock()
	defer h.cache.writeMu.Unlock()
	if write.data == nil {
		return nil
	}
	if write.generation != h.cache.currentGeneration() {
		return nil
	}
	compressed, err := compressCache(write.data)
	if err != nil {
		return err
	}
	if write.generation != h.cache.currentGeneration() {
		return nil
	}
	if err := write.store.Write(write.key, compressed); err != nil {
		return fmt.Errorf("write history cache: %w", err)
	}
	return nil
}

func (c *cacheState) clearPending() {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.generation++
	c.pending = cacheWrite{}
	c.mu.Unlock()
}

func (c *cacheState) currentGeneration() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

func (c *cacheState) markFlushed(generation uint64, err error) {
	if err != nil {
		return
	}
	c.mu.Lock()
	if c.generation == generation {
		c.flushed = generation
	}
	c.mu.Unlock()
}

func (c *cacheState) markCurrentFlushed() {
	c.mu.Lock()
	c.flushed = c.generation
	c.mu.Unlock()
}

func (h *History) setCacheError(err error) {
	h.cache.mu.Lock()
	h.cache.err = err
	h.cache.mu.Unlock()
}

func cacheIdentity(doc document.Document) (string, uint32, error) {
	if doc.ID == uuid.Nil || doc.ID.Version() != 4 {
		return "", 0, errors.New("history document must have a UUIDv4 identity")
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return "", 0, fmt.Errorf("encode history document guard: %w", err)
	}
	return doc.ID.String(), crc32.ChecksumIEEE(data), nil
}

var _ Store = directoryStore{}
