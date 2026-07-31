package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/coxley/dg/document"
	"github.com/google/uuid"
)

// LoadCurrentInto reads entry's current path without requiring its recorded revision.
// An error may leave dst partially decoded.
func (s *Store) LoadCurrentInto(entry Entry, dst *document.Document) (Entry, error) {
	if dst == nil {
		return Entry{}, errors.New("load current canvas into nil document")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.entryPath(entry)
	if err != nil {
		return Entry{}, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // Entry validation confines the path to Store roots.
	if errors.Is(err, fs.ErrNotExist) {
		return Entry{}, ErrEntryNotFound
	}
	if err != nil {
		return Entry{}, fmt.Errorf("read current canvas: %w", err)
	}
	if err := decodeDocumentInto(data, dst); err != nil {
		return Entry{}, fmt.Errorf("decode current canvas: %w", err)
	}
	current, err := entryFromData(path, entry.Section, entry.Name, entry.Draft, dst.ID, data)
	if err != nil {
		return Entry{}, err
	}
	s.warm.put(warmKey(path, current.Revision), data)
	return current, nil
}

// BackupAndRestore moves the current raw named file to the next backup name and
// atomically recreates the original name from doc.
func (s *Store) BackupAndRestore(entry Entry, doc document.Document) (Entry, Entry, error) {
	return s.backupAndRestore(entry, doc, writeNew)
}

func (s *Store) backupAndRestore(
	entry Entry,
	doc document.Document,
	restore func(string, []byte) error,
) (Entry, Entry, error) {
	if entry.Draft {
		return Entry{}, Entry{}, errors.New("store cannot back up a draft")
	}
	if doc.ID != entry.ID {
		return Entry{}, Entry{}, errors.New("store document identity does not match entry")
	}
	local, err := encodeDocument(doc)
	if err != nil {
		return Entry{}, Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.entryPath(entry)
	if err != nil {
		return Entry{}, Entry{}, err
	}
	external, err := os.ReadFile(source) //nolint:gosec // Entry validation confines the path to Store roots.
	if errors.Is(err, fs.ErrNotExist) {
		return Entry{}, Entry{}, ErrEntryNotFound
	}
	if err != nil {
		return Entry{}, Entry{}, fmt.Errorf("read external canvas: %w", err)
	}
	externalID := uuid.Nil
	if externalDoc, decodeErr := decodeDocument(external); decodeErr == nil {
		externalID = externalDoc.ID
	}
	backup, backupPath, err := s.linkBackup(source, entry, externalID, external)
	if err != nil {
		return Entry{}, Entry{}, err
	}
	if err := os.Remove(source); err != nil {
		removeErr := os.Remove(backupPath) //nolint:gosec // Store generated backupPath below the preferred root.
		return Entry{}, Entry{}, errors.Join(fmt.Errorf("remove external canvas: %w", err), removeErr)
	}
	s.self[backupPath] = backup.Revision
	s.self[source] = Revision{}
	if externalID != uuid.Nil {
		s.warm.put(warmKey(backupPath, backup.Revision), external)
	}
	if err := restore(source, local); err != nil {
		return backup, Entry{}, fmt.Errorf("restore local canvas: %w", err)
	}
	restored, err := entryFromData(source, entry.Section, entry.Name, false, doc.ID, local)
	if err != nil {
		return backup, Entry{}, err
	}
	s.warm.put(warmKey(source, restored.Revision), local)
	s.self[source] = restored.Revision
	return backup, restored, nil
}

func (s *Store) linkBackup(
	source string,
	entry Entry,
	id uuid.UUID,
	data []byte,
) (Entry, string, error) {
	for suffix := 0; ; suffix++ {
		name := entry.Name + ".bak"
		if suffix != 0 {
			name += strconv.Itoa(suffix)
		}
		path := s.namedPath(entry.Section, name)
		if err := os.Link(source, path); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return Entry{}, "", fmt.Errorf("create canvas backup: %w", err)
		}
		backup, err := entryFromData(path, entry.Section, name, false, id, data)
		if err != nil {
			return Entry{}, "", errors.Join(err, os.Remove(path))
		}
		return backup, path, nil
	}
}

// RestoreDeleted recreates a missing named entry from doc without replacing a
// file that reappeared at the same location.
func (s *Store) RestoreDeleted(entry Entry, doc document.Document) (Entry, error) {
	if entry.Draft {
		return Entry{}, errors.New("store cannot restore a deleted draft as named")
	}
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
	if err := writeNew(path, data); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Entry{}, ErrRevision
		}
		return Entry{}, err
	}
	restored, err := entryFromData(path, entry.Section, entry.Name, false, doc.ID, data)
	if err == nil {
		s.warm.put(warmKey(path, restored.Revision), data)
		s.self[path] = restored.Revision
	}
	return restored, err
}

// PreserveDraft writes doc as the durable draft for its identity.
func (s *Store) PreserveDraft(doc document.Document) (Entry, error) {
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
		if !errors.Is(err, fs.ErrExist) {
			return Entry{}, err
		}
		if err := replaceFile(path, data); err != nil {
			return Entry{}, err
		}
	}
	draft, err := entryFromData(path, "", "", true, doc.ID, data)
	if err == nil {
		s.warm.put(warmKey(path, draft.Revision), data)
		s.self[path] = draft.Revision
	}
	return draft, err
}
