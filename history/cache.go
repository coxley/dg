package history

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/coxley/dg/layout"
)

const (
	cacheVersion      = 4
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
	SavedDigest string          `json:"saved_digest"`
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
	path        string
	key         string
	saved       layout.Snapshot
	savedCursor int
	branchValid bool

	mu         sync.Mutex
	writeMu    sync.Mutex
	timer      *time.Timer
	generation uint64
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

// Save marks the current layout as saved at path and writes its history.
func (h *History) Save(path string) error {
	if h == nil || h.layout == nil {
		return errors.New("history is not attached")
	}
	h.Interrupt()
	if err := h.layout.Build(); err != nil {
		return fmt.Errorf("build history layout: %w", err)
	}
	normalized, key, err := cacheKey(path)
	if err != nil {
		return err
	}
	store, err := h.cacheStore()
	if err != nil {
		return err
	}
	h.cache.path = normalized
	h.cache.key = key
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
	h.setCacheError(err)
	return err
}

// Restore attaches cached history for path without changing its saved render.
func (h *History) Restore(path string) (bool, error) {
	if h == nil || h.layout == nil {
		return false, errors.New("history is not attached")
	}
	normalized, key, err := cacheKey(path)
	if err != nil {
		return false, err
	}
	store, err := h.cacheStore()
	if err != nil {
		return false, err
	}
	h.cache.clearPending()
	h.markCache(normalized, key)
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
	digest, err := h.layout.Snapshot().Digest()
	if err != nil {
		return false, fmt.Errorf("digest current history layout: %w", err)
	}
	savedDigest, savedErr := cache.Saved.Digest()
	if savedErr != nil || cache.SavedDigest != digest || savedDigest != cache.SavedDigest ||
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
	h.cache.path = normalized
	h.cache.key = key
	h.cache.branchValid = true
	h.closeActive()
	h.setCacheError(nil)
	return true, nil
}

func (h *History) markCache(path, key string) {
	h.cache.path = path
	h.cache.key = key
	h.cache.savedCursor = h.cursor
	h.cache.saved = h.layout.Snapshot()
	h.cache.branchValid = true
	h.setCacheError(nil)
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
	if h.cache.path == "" || !h.cache.branchValid {
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
	}
	h.cache.mu.Unlock()
}

func (h *History) marshalCache() ([]byte, error) {
	digest, err := h.cache.saved.Digest()
	if err != nil {
		return nil, fmt.Errorf("digest saved history layout: %w", err)
	}
	cache := cacheFile{Version: cacheVersion, SavedDigest: digest, Saved: h.cache.saved, Entries: make([]cacheEntry, len(h.entries)), SavedCursor: h.cache.savedCursor}
	for i := range h.entries {
		cache.Entries[i].Changes = h.entries[i].changes
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return nil, fmt.Errorf("encode history cache: %w", err)
	}
	return data, nil
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
	if err := decoder.Decode(&cache); err != nil || cache.Version != cacheVersion {
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

func (h *History) setCacheError(err error) {
	h.cache.mu.Lock()
	h.cache.err = err
	h.cache.mu.Unlock()
}

func cacheKey(path string) (normalized, key string, err error) {
	if path == "" {
		return "", "", errors.New("history path must not be empty")
	}
	normalized, err = filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve history path: %w", err)
	}
	normalized = filepath.Clean(normalized)
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}
	sum := sha256.Sum256([]byte(normalized))
	return normalized, hex.EncodeToString(sum[:]), nil
}

var _ Store = directoryStore{}
