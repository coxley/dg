// Package store persists named canvases, durable drafts, and disposable blobs.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coxley/dg/document"
	"github.com/google/uuid"
)

const (
	defaultWarmCount = 5
	defaultWarmBytes = 16 << 20
)

var (
	ErrEntryNotFound = errors.New("store entry not found")
	ErrEntryExists   = errors.New("store entry already exists")
	ErrRevision      = errors.New("store entry externally modified")
)

// Revision identifies the raw file version observed with an Entry.
type Revision struct {
	Modified int64
	Size     int64
	CRC32    uint32
}

// Entry identifies one named canvas or draft.
type Entry struct {
	Section  string
	Name     string
	ID       uuid.UUID
	Modified time.Time
	Revision Revision
	Draft    bool
}

// Option configures a Store.
type Option func(*config)

type config struct {
	stateDir  string
	cacheDir  string
	warmCount int
	warmBytes int
}

// WithStateDir stores durable drafts and promotion recovery data in dir.
func WithStateDir(dir string) Option {
	return func(config *config) { config.stateDir = dir }
}

// WithCacheDir stores disposable blobs in dir.
func WithCacheDir(dir string) Option {
	return func(config *config) { config.cacheDir = dir }
}

// WithWarmLimits bounds retained compressed values by count and bytes.
func WithWarmLimits(count, bytes int) Option {
	return func(config *config) {
		config.warmCount = count
		config.warmBytes = bytes
	}
}

// Store manages canvas records below one preferred directory.
type Store struct {
	mu sync.Mutex

	preferred string
	stateDir  string
	cacheDir  string
	warm      warmCache
}

// New opens a Store rooted at preferred and recovers interrupted draft promotion.
func New(preferred string, options ...Option) (*Store, error) {
	if preferred == "" {
		return nil, errors.New("store preferred directory must not be empty")
	}
	stateDir, err := defaultStateDir()
	if err != nil {
		return nil, err
	}
	cacheDir, err := defaultCacheDir()
	if err != nil {
		return nil, err
	}
	config := config{
		stateDir:  stateDir,
		cacheDir:  cacheDir,
		warmCount: defaultWarmCount,
		warmBytes: defaultWarmBytes,
	}
	for _, option := range options {
		option(&config)
	}
	if config.stateDir == "" || config.cacheDir == "" {
		return nil, errors.New("store state and cache directories must not be empty")
	}
	if config.warmCount < 0 || config.warmBytes < 0 {
		return nil, errors.New("store warm limits must not be negative")
	}
	preferred, err = filepath.Abs(preferred)
	if err != nil {
		return nil, fmt.Errorf("resolve preferred directory: %w", err)
	}
	store := &Store{
		preferred: filepath.Clean(preferred),
		stateDir:  filepath.Clean(config.stateDir),
		cacheDir:  filepath.Clean(config.cacheDir),
		warm:      newWarmCache(config.warmCount, config.warmBytes),
	}
	if err := os.MkdirAll(store.preferred, 0o700); err != nil {
		return nil, fmt.Errorf("create preferred directory: %w", err)
	}
	if err := os.MkdirAll(store.draftsDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create drafts directory: %w", err)
	}
	if err := store.recoverPromotion(); err != nil {
		return nil, err
	}
	return store, nil
}

// Create writes a new named canvas without replacing an existing name.
func (s *Store) Create(section, name string, doc document.Document) (Entry, error) {
	if err := validateLocation(section, name); err != nil {
		return Entry{}, err
	}
	data, err := encodeDocument(doc)
	if err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.namedPath(section, name)
	if err := writeNew(path, data); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Entry{}, ErrEntryExists
		}
		return Entry{}, err
	}
	entry, err := entryFromFile(path, section, name, false, doc.ID)
	if err == nil {
		s.warm.put(warmKey(path, entry.Revision), data)
	}
	return entry, err
}

// CreateDraft writes a new durable draft identified by the document UUID.
func (s *Store) CreateDraft(doc document.Document) (Entry, error) {
	if doc.ID == uuid.Nil || doc.ID.Version() != 4 {
		return Entry{}, errors.New("store draft must have a UUIDv4 identity")
	}
	data, err := encodeDocument(doc)
	if err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.draftPath(doc.ID)
	if err := writeNew(path, data); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Entry{}, ErrEntryExists
		}
		return Entry{}, err
	}
	entry, err := entryFromFile(path, "", "", true, doc.ID)
	if err == nil {
		s.warm.put(warmKey(path, entry.Revision), data)
	}
	return entry, err
}

// Load reads and validates entry, returning independently owned document data.
func (s *Store) Load(entry Entry) (document.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.entryPath(entry)
	if err != nil {
		return document.Document{}, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return document.Document{}, ErrEntryNotFound
	}
	if err != nil {
		return document.Document{}, fmt.Errorf("inspect canvas: %w", err)
	}
	if info.ModTime().UnixNano() != entry.Revision.Modified || info.Size() != entry.Revision.Size {
		return document.Document{}, ErrRevision
	}
	data, ok := s.warm.get(warmKey(path, entry.Revision))
	if !ok {
		data, err = os.ReadFile(path) //nolint:gosec // Entry validation confines the path to Store roots.
		if errors.Is(err, fs.ErrNotExist) {
			return document.Document{}, ErrEntryNotFound
		}
		if err != nil {
			return document.Document{}, fmt.Errorf("read canvas: %w", err)
		}
		revision, revisionErr := revisionFor(path, data)
		if revisionErr != nil {
			return document.Document{}, revisionErr
		}
		if revision != entry.Revision {
			return document.Document{}, ErrRevision
		}
		s.warm.put(warmKey(path, entry.Revision), data)
	}
	doc, err := decodeDocument(data)
	if err != nil {
		return document.Document{}, fmt.Errorf("decode canvas: %w", err)
	}
	if doc.ID != entry.ID {
		return document.Document{}, ErrRevision
	}
	return doc, nil
}

// Save atomically replaces entry when its observed revision still matches.
func (s *Store) Save(entry Entry, doc document.Document) (Entry, error) {
	if doc.ID != entry.ID {
		return Entry{}, errors.New("store document identity does not match entry")
	}
	data, err := encodeDocument(doc)
	if err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.entryPath(entry)
	if err != nil {
		return Entry{}, err
	}
	if err := checkRevision(path, entry.Revision); err != nil {
		return Entry{}, err
	}
	if err := replaceFile(path, data); err != nil {
		return Entry{}, err
	}
	updated, err := entryFromFile(path, entry.Section, entry.Name, entry.Draft, doc.ID)
	if err == nil {
		s.warm.put(warmKey(path, updated.Revision), data)
	}
	return updated, err
}

// Move changes a named canvas section or name without changing its document.
func (s *Store) Move(entry Entry, section, name string) (Entry, error) {
	if entry.Draft {
		return Entry{}, errors.New("store cannot move a draft; name it instead")
	}
	if err := validateLocation(section, name); err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.entryPath(entry)
	if err != nil {
		return Entry{}, err
	}
	if err := checkRevision(source, entry.Revision); err != nil {
		return Entry{}, err
	}
	target := s.namedPath(section, name)
	if _, err := os.Stat(target); err == nil {
		return Entry{}, ErrEntryExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Entry{}, fmt.Errorf("inspect canvas destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return Entry{}, fmt.Errorf("create canvas section: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		return Entry{}, fmt.Errorf("move canvas: %w", err)
	}
	s.warm.remove(warmKey(source, entry.Revision))
	return entryFromFile(target, section, name, false, entry.ID)
}

// Name promotes a draft to a named canvas, preserving its identity.
func (s *Store) Name(entry Entry, section, name string) (Entry, error) {
	if !entry.Draft {
		return Entry{}, errors.New("store can only name a draft")
	}
	if err := validateLocation(section, name); err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.entryPath(entry)
	if err != nil {
		return Entry{}, err
	}
	if err := checkRevision(source, entry.Revision); err != nil {
		return Entry{}, err
	}
	data, err := os.ReadFile(source) //nolint:gosec // Entry validation confines the path to the drafts directory.
	if err != nil {
		return Entry{}, fmt.Errorf("read draft for naming: %w", err)
	}
	target := s.namedPath(section, name)
	journal := promotion{DraftID: entry.ID, Section: section, Name: name}
	if err := s.writePromotion(journal); err != nil {
		return Entry{}, err
	}
	if err := writeNew(target, data); err != nil {
		_ = os.Remove(s.promotionPath())
		if errors.Is(err, fs.ErrExist) {
			return Entry{}, ErrEntryExists
		}
		return Entry{}, err
	}
	if err := os.Remove(source); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Entry{}, fmt.Errorf("remove named draft: %w", err)
	}
	if err := os.Remove(s.promotionPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Entry{}, fmt.Errorf("finish draft promotion: %w", err)
	}
	s.warm.remove(warmKey(source, entry.Revision))
	updated, err := entryFromFile(target, section, name, false, entry.ID)
	if err == nil {
		s.warm.put(warmKey(target, updated.Revision), data)
	}
	return updated, err
}

// Delete removes entry when its observed revision still matches.
func (s *Store) Delete(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.entryPath(entry)
	if err != nil {
		return err
	}
	if err := checkRevision(path, entry.Revision); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrEntryNotFound
		}
		return fmt.Errorf("delete canvas: %w", err)
	}
	s.warm.remove(warmKey(path, entry.Revision))
	return nil
}

// ClearDrafts deletes every draft except preserve and returns the number removed.
func (s *Store) ClearDrafts(preserve uuid.UUID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	names, err := fs.Glob(os.DirFS(s.draftsDir()), "*.dg")
	if err != nil {
		return 0, fmt.Errorf("list drafts: %w", err)
	}
	removed := 0
	for _, name := range names {
		id, parseErr := uuid.Parse(strings.TrimSuffix(name, ".dg"))
		if parseErr != nil || id == preserve {
			continue
		}
		if err := os.Remove(filepath.Join(s.draftsDir(), name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return removed, fmt.Errorf("delete draft: %w", err)
		}
		removed++
	}
	return removed, nil
}

// List scans named canvases and drafts and returns valid records.
func (s *Store) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var entries []Entry
	patterns := []string{"*.dg", "*/*.dg"}
	root := os.DirFS(s.preferred)
	for _, pattern := range patterns {
		names, err := fs.Glob(root, pattern)
		if err != nil {
			return nil, fmt.Errorf("scan canvases: %w", err)
		}
		for _, relative := range names {
			section, file := filepath.Split(relative)
			section = strings.TrimSuffix(section, string(filepath.Separator))
			entry, err := s.inspectNamed(section, strings.TrimSuffix(file, ".dg"))
			if err == nil {
				entries = append(entries, entry)
			}
		}
	}
	drafts, err := fs.Glob(os.DirFS(s.draftsDir()), "*.dg")
	if err != nil {
		return nil, fmt.Errorf("scan drafts: %w", err)
	}
	for _, file := range drafts {
		id, err := uuid.Parse(strings.TrimSuffix(file, ".dg"))
		if err != nil {
			continue
		}
		entry, err := s.inspectDraft(id)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if a.Draft != b.Draft {
			if a.Draft {
				return 1
			}
			return -1
		}
		if a.Section != b.Section {
			return strings.Compare(a.Section, b.Section)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return entries, nil
}

func (s *Store) inspectNamed(section, name string) (Entry, error) {
	path := s.namedPath(section, name)
	data, err := os.ReadFile(path) //nolint:gosec // Glob results remain below the preferred directory.
	if err != nil {
		return Entry{}, err
	}
	doc, err := decodeDocument(data)
	if err != nil {
		return Entry{}, err
	}
	return entryFromData(path, section, name, false, doc.ID, data)
}

func (s *Store) inspectDraft(id uuid.UUID) (Entry, error) {
	path := s.draftPath(id)
	data, err := os.ReadFile(path) //nolint:gosec // UUID-derived paths remain below the drafts directory.
	if err != nil {
		return Entry{}, err
	}
	doc, err := decodeDocument(data)
	if err != nil || doc.ID != id {
		return Entry{}, errors.New("invalid draft document")
	}
	return entryFromData(path, "", "", true, id, data)
}

func (s *Store) entryPath(entry Entry) (string, error) {
	if entry.ID == uuid.Nil || entry.ID.Version() != 4 {
		return "", errors.New("store entry must have a UUIDv4 identity")
	}
	if entry.Draft {
		return s.draftPath(entry.ID), nil
	}
	if err := validateLocation(entry.Section, entry.Name); err != nil {
		return "", err
	}
	return s.namedPath(entry.Section, entry.Name), nil
}

func validateLocation(section, name string) error {
	if err := validateComponent(name, "name"); err != nil {
		return err
	}
	if section != "" {
		return validateComponent(section, "section")
	}
	return nil
}

func validateComponent(value, kind string) error {
	if value == "" || value == "." || value == ".." ||
		filepath.IsAbs(value) || filepath.Base(value) != value ||
		strings.ContainsAny(value, `/\`) {
		return fmt.Errorf("store %s must be one non-empty path component", kind)
	}
	return nil
}

func (s *Store) namedPath(section, name string) string {
	if section == "" {
		return filepath.Join(s.preferred, name+".dg")
	}
	return filepath.Join(s.preferred, section, name+".dg")
}

func (s *Store) draftsDir() string { return filepath.Join(s.stateDir, "drafts") }
func (s *Store) draftPath(id uuid.UUID) string {
	return filepath.Join(s.draftsDir(), id.String()+".dg")
}

func (s *Store) promotionPath() string { return filepath.Join(s.stateDir, "promotion.json") }
func (s *Store) historyDir() string    { return filepath.Join(s.cacheDir, "history") }
func warmKey(path string, revision Revision) string {
	return fmt.Sprintf("%s:%d:%d:%d", path, revision.Modified, revision.Size, revision.CRC32)
}

func entryFromFile(path, section, name string, draft bool, id uuid.UUID) (Entry, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Callers provide validated Store-owned paths.
	if err != nil {
		return Entry{}, fmt.Errorf("read stored canvas: %w", err)
	}
	return entryFromData(path, section, name, draft, id, data)
}

func entryFromData(path, section, name string, draft bool, id uuid.UUID, data []byte) (Entry, error) {
	revision, err := revisionFor(path, data)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Section:  section,
		Name:     name,
		ID:       id,
		Modified: time.Unix(0, revision.Modified),
		Revision: revision,
		Draft:    draft,
	}, nil
}

func revisionFor(path string, data []byte) (Revision, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Revision{}, ErrEntryNotFound
		}
		return Revision{}, fmt.Errorf("inspect stored canvas: %w", err)
	}
	return Revision{Modified: info.ModTime().UnixNano(), Size: info.Size(), CRC32: crc32.ChecksumIEEE(data)}, nil
}

func checkRevision(path string, want Revision) error {
	data, err := os.ReadFile(path) //nolint:gosec // Callers provide validated Store-owned paths.
	if errors.Is(err, fs.ErrNotExist) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("read stored canvas revision: %w", err)
	}
	got, err := revisionFor(path, data)
	if err != nil {
		return err
	}
	if got != want {
		return ErrRevision
	}
	return nil
}

func writeNew(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create canvas directory: %w", err)
	}
	temp, err := writeTemp(filepath.Dir(path), data)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if err := os.Link(temp, path); err != nil {
		return fmt.Errorf("create canvas: %w", err)
	}
	return nil
}

func replaceFile(path string, data []byte) error {
	temp, err := writeTemp(filepath.Dir(path), data)
	if err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("replace canvas: %w", err)
	}
	return nil
}

func writeTemp(dir string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create storage directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".dg-*")
	if err != nil {
		return "", fmt.Errorf("create temporary file: %w", err)
	}
	path := file.Name()
	cleanup := func(cause error) (string, error) {
		return "", errors.Join(cause, file.Close(), os.Remove(path))
	}
	if err := file.Chmod(0o600); err != nil {
		return cleanup(fmt.Errorf("set storage permissions: %w", err))
	}
	if _, err := file.Write(data); err != nil {
		return cleanup(fmt.Errorf("write temporary file: %w", err))
	}
	if err := file.Close(); err != nil {
		return "", errors.Join(fmt.Errorf("close temporary file: %w", err), os.Remove(path))
	}
	return path, nil
}

type promotion struct {
	DraftID uuid.UUID `json:"draft_id"`
	Section string    `json:"section"`
	Name    string    `json:"name"`
}

func (s *Store) writePromotion(value promotion) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode draft promotion: %w", err)
	}
	if err := replaceFile(s.promotionPath(), data); err != nil {
		return fmt.Errorf("write draft promotion: %w", err)
	}
	return nil
}

func (s *Store) recoverPromotion() error {
	data, err := os.ReadFile(s.promotionPath()) //nolint:gosec // Store owns the promotion path.
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read draft promotion: %w", err)
	}
	var value promotion
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode draft promotion: %w", err)
	}
	if err := validateLocation(value.Section, value.Name); err != nil {
		return fmt.Errorf("validate draft promotion: %w", err)
	}
	target := s.namedPath(value.Section, value.Name)
	if targetDoc, readErr := readDocumentFile(target); readErr == nil && targetDoc.ID == value.DraftID {
		if err := os.Remove(s.draftPath(value.DraftID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("recover promoted draft: %w", err)
		}
	}
	if err := os.Remove(s.promotionPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("finish draft promotion recovery: %w", err)
	}
	return nil
}

func readDocumentFile(path string) (document.Document, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Caller controls the validated Store path.
	if err != nil {
		return document.Document{}, err
	}
	return decodeDocument(data)
}

func defaultStateDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user state directory: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(root, "org.coxley.dg", "state"), nil
	}
	return filepath.Join(root, "dg", "state"), nil
}

func defaultCacheDir() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache directory: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(root, "org.coxley.dg"), nil
	}
	return filepath.Join(root, "dg"), nil
}
