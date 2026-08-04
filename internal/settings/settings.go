// Package settings owns durable editor configuration.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coxley/dg/layout"
)

const (
	configDirectory = "dg"
	configName      = "config.json"
)

// Theme selects which color scheme drives semantic tint selection.
type Theme string

const (
	// ThemeAuto follows the terminal color scheme.
	ThemeAuto Theme = "auto"
	// ThemeDark always uses the configured dark theme.
	ThemeDark Theme = "dark"
	// ThemeLight always uses the configured light theme.
	ThemeLight Theme = "light"
)

// Keybind contains the configured mappings for one action in one scope.
type Keybind struct {
	Scope    string   `json:"scope"`
	Action   string   `json:"action"`
	Mappings []string `json:"mappings"`
}

// Snapshot contains one complete settings load.
type Snapshot struct {
	Router           layout.Router `json:"router"`
	SaveDirectory    string        `json:"save_directory,omitempty"`
	CommentPrefix    string        `json:"comment_prefix,omitempty"`
	Theme            Theme         `json:"theme,omitempty"`
	Keybinds         []Keybind     `json:"keybinds,omitempty"`
	DarkTint         string        `json:"dark_tint,omitempty"`
	LightTint        string        `json:"light_tint,omitempty"`
	OpaqueBackground bool          `json:"opaque_background,omitempty"`
}

// Store loads and atomically saves one configuration file.
type Store struct {
	path string
}

// NewStore returns a store backed by path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// DefaultStore returns the XDG-backed application store.
func DefaultStore() (*Store, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return NewStore(path), nil
}

// ConfigPath returns the application configuration path.
func ConfigPath() (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		root = filepath.Join(home, ".config")
	} else if !filepath.IsAbs(root) {
		return "", errors.New("XDG_CONFIG_HOME must be an absolute path")
	}
	return filepath.Join(root, configDirectory, configName), nil
}

// Path returns the configuration file path.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Load returns the current snapshot or an empty snapshot when no file exists.
func (s *Store) Load() (Snapshot, error) {
	if s == nil || s.path == "" {
		return Snapshot{}, errors.New("settings store has no path")
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("read settings %q: %w", s.path, err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode settings %q: %w", s.path, err)
	}
	return snapshot, nil
}

// Save atomically replaces the current snapshot.
func (s *Store) Save(snapshot Snapshot) error {
	if s == nil || s.path == "" {
		return errors.New("settings store has no path")
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create settings directory %q: %w", directory, err)
	}
	file, err := os.CreateTemp(directory, "."+filepath.Base(s.path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary settings file: %w", err)
	}
	temporary := file.Name()
	defer os.Remove(temporary)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("protect temporary settings file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temporary settings file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync temporary settings file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary settings file: %w", err)
	}
	if err := os.Rename(temporary, s.path); err != nil {
		return fmt.Errorf("replace settings %q: %w", s.path, err)
	}
	return nil
}
