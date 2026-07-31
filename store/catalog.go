package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
)

const catalogDebounce = 100 * time.Millisecond

// ChangeKind identifies a catalog difference.
type ChangeKind uint8

const (
	ChangeAdded ChangeKind = iota + 1
	ChangeModified
	ChangeDeleted
)

// CatalogChange describes one reconciled record difference.
type CatalogChange struct {
	Kind     ChangeKind
	Entry    Entry
	Previous Entry
	External bool
}

// CatalogEvent reports a reconciled catalog, watcher error, or closure.
type CatalogEvent struct {
	Entries []Entry
	Changes []CatalogChange
	Err     error
	Closed  bool
}

// Reconcile scans current records and compares them with previous.
func (s *Store) Reconcile(previous []Entry) CatalogEvent {
	entries, errs := s.scanCatalog()
	return CatalogEvent{
		Entries: entries,
		Changes: s.catalogChanges(previous, entries),
		Err:     errors.Join(errs...),
	}
}

// Watch reconciles the catalog after filesystem invalidations until ctx ends.
func (s *Store) Watch(ctx context.Context) (<-chan CatalogEvent, error) {
	if ctx == nil {
		return nil, errors.New("store watch context must not be nil")
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create store watcher: %w", err)
	}
	if err := watcher.Add(s.preferred); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("watch preferred directory: %w", err)
	}
	if err := watcher.Add(s.draftsDir()); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("watch drafts directory: %w", err)
	}
	watched := map[string]bool{s.preferred: true, s.draftsDir(): true}
	if err := s.syncSectionWatches(watcher, watched); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	entries, scanErrs := s.scanCatalog()
	events := make(chan CatalogEvent, 4)
	go s.watchLoop(ctx, watcher, watched, entries, scanErrs, events)
	return events, nil
}

func (s *Store) watchLoop(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	watched map[string]bool,
	entries []Entry,
	scanErrs []error,
	events chan<- CatalogEvent,
) {
	defer close(events)
	defer watcher.Close()
	s.sendCatalogEvent(ctx, events, CatalogEvent{Entries: entries, Err: errors.Join(scanErrs...)})
	var timer *time.Timer
	var timerC <-chan time.Time
	schedule := func() {
		if timer == nil {
			timer = time.NewTimer(catalogDebounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(catalogDebounce)
		}
		timerC = timer.C
	}
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			s.sendClosed(events)
			return
		case _, ok := <-watcher.Events:
			if !ok {
				s.sendClosed(events)
				return
			}
			schedule()
		case err, ok := <-watcher.Errors:
			if !ok {
				s.sendClosed(events)
				return
			}
			s.sendCatalogEvent(ctx, events, CatalogEvent{Entries: slices.Clone(entries), Err: err})
			schedule()
		case <-timerC:
			timerC = nil
			watchErr := s.syncSectionWatches(watcher, watched)
			event := s.Reconcile(entries)
			entries = event.Entries
			event.Entries = slices.Clone(event.Entries)
			event.Err = errors.Join(event.Err, watchErr)
			s.sendCatalogEvent(ctx, events, event)
		}
	}
}

func (s *Store) sendCatalogEvent(ctx context.Context, events chan<- CatalogEvent, event CatalogEvent) {
	select {
	case events <- event:
	case <-ctx.Done():
	}
}

func (s *Store) sendClosed(events chan<- CatalogEvent) {
	select {
	case events <- CatalogEvent{Closed: true}:
	default:
	}
}

func (s *Store) syncSectionWatches(watcher *fsnotify.Watcher, watched map[string]bool) error {
	directories, err := os.ReadDir(s.preferred)
	if err != nil {
		return fmt.Errorf("list canvas sections: %w", err)
	}
	want := map[string]bool{s.preferred: true, s.draftsDir(): true}
	var errs []error
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		path := filepath.Join(s.preferred, directory.Name())
		want[path] = true
		if !watched[path] {
			if err := watcher.Add(path); err != nil {
				errs = append(errs, fmt.Errorf("watch canvas section %q: %w", directory.Name(), err))
				continue
			}
			watched[path] = true
		}
	}
	for path := range watched {
		if want[path] {
			continue
		}
		if err := watcher.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("stop watching canvas section: %w", err))
		}
		delete(watched, path)
	}
	return errors.Join(errs...)
}

func (s *Store) scanCatalog() ([]Entry, []error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var entries []Entry
	var errs []error
	root := os.DirFS(s.preferred)
	for _, pattern := range []string{"*.dg", "*/*.dg"} {
		names, err := fs.Glob(root, pattern)
		if err != nil {
			errs = append(errs, fmt.Errorf("scan canvases with %q: %w", pattern, err))
			continue
		}
		for _, relative := range names {
			section, file := filepath.Split(relative)
			section = strings.TrimSuffix(section, string(filepath.Separator))
			entry, err := s.inspectNamed(section, strings.TrimSuffix(file, ".dg"))
			if err != nil {
				errs = append(errs, fmt.Errorf("inspect canvas %q: %w", relative, err))
				continue
			}
			entries = append(entries, entry)
		}
	}
	drafts, err := fs.Glob(os.DirFS(s.draftsDir()), "*.dg")
	if err != nil {
		errs = append(errs, fmt.Errorf("scan drafts: %w", err))
	}
	for _, file := range drafts {
		id, err := parseDraftName(file)
		if err != nil {
			continue
		}
		entry, err := s.inspectDraft(id)
		if err != nil {
			errs = append(errs, fmt.Errorf("inspect draft %q: %w", file, err))
			continue
		}
		entries = append(entries, entry)
	}
	sortEntries(entries)
	return entries, errs
}

func (s *Store) catalogChanges(previous, next []Entry) []CatalogChange {
	old := make(map[string]Entry, len(previous))
	for _, entry := range previous {
		old[entryKey(entry)] = entry
	}
	var changes []CatalogChange
	for _, entry := range next {
		key := entryKey(entry)
		prior, exists := old[key]
		delete(old, key)
		if exists && prior.Revision == entry.Revision {
			continue
		}
		kind := ChangeAdded
		if exists {
			kind = ChangeModified
		}
		changes = append(changes, CatalogChange{
			Kind:     kind,
			Entry:    entry,
			Previous: prior,
			External: s.consumeSelf(entry, entry.Revision),
		})
	}
	for _, entry := range old {
		changes = append(changes, CatalogChange{
			Kind:     ChangeDeleted,
			Previous: entry,
			External: s.consumeSelf(entry, Revision{}),
		})
	}
	slices.SortFunc(changes, func(a, b CatalogChange) int {
		return strings.Compare(entryKey(changeEntry(a)), entryKey(changeEntry(b)))
	})
	return changes
}

func (s *Store) consumeSelf(entry Entry, revision Revision) bool {
	path, err := s.entryPath(entry)
	if err != nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warm.removePrefix(path + ":")
	want, ok := s.self[path]
	if ok && want == revision {
		delete(s.self, path)
		return false
	}
	return true
}

func changeEntry(change CatalogChange) Entry {
	if change.Kind == ChangeDeleted {
		return change.Previous
	}
	return change.Entry
}

func entryKey(entry Entry) string {
	if entry.Draft {
		return "draft:" + entry.ID.String()
	}
	return "named:" + entry.Section + "/" + entry.Name
}

func parseDraftName(file string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSuffix(file, ".dg"))
}

func sortEntries(entries []Entry) {
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
}
