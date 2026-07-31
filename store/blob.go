package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// BlobStore persists disposable name-keyed values for history caches.
type BlobStore struct {
	store *Store
}

// History returns the Store's disposable history blob storage.
func (s *Store) History() BlobStore {
	return BlobStore{store: s}
}

// Read returns the value stored under name.
func (s BlobStore) Read(name string) ([]byte, error) {
	if err := validateBlobName(name); err != nil {
		return nil, err
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	key := "history:" + name
	if data, ok := s.store.warm.get(key); ok {
		return append([]byte(nil), data...), nil
	}
	data, err := os.ReadFile(filepath.Join(s.store.historyDir(), name)) //nolint:gosec // validateBlobName rejects paths.
	if err != nil {
		return nil, err
	}
	s.store.warm.put(key, data)
	return append([]byte(nil), data...), nil
}

// Write atomically replaces the value stored under name.
func (s BlobStore) Write(name string, data []byte) error {
	if err := validateBlobName(name); err != nil {
		return err
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	path := filepath.Join(s.store.historyDir(), name)
	if err := replaceFile(path, data); err != nil {
		return err
	}
	s.store.warm.put("history:"+name, data)
	return nil
}

func validateBlobName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return errors.New("store blob name must be one non-empty path component")
	}
	return nil
}

var _ interface {
	Read(string) ([]byte, error)
	Write(string, []byte) error
} = BlobStore{}
