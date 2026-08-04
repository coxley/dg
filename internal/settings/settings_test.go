package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestConfigPathUsesXDGConfigHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)

	path, err := ConfigPath()

	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "dg", "config.json"), path)
}

func TestConfigPathFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	path, err := ConfigPath()

	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "dg", "config.json"), path)
}

func TestConfigPathRejectsRelativeXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative")

	_, err := ConfigPath()

	require.EqualError(t, err, "XDG_CONFIG_HOME must be an absolute path")
}

func TestStoreLoadsMissingFile(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "config.json"))

	snapshot, err := store.Load()

	require.NoError(t, err)
	require.Equal(t, Snapshot{}, snapshot)
}

func TestStoreAtomicallyReplacesSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store := NewStore(path)
	first := Snapshot{
		Router:        layout.DefaultRouter(),
		SaveDirectory: "/first",
		CommentPrefix: "# ",
		Theme:         ThemeDark,
		Keybinds: []Keybind{{
			Scope: "global", Action: "save", Mappings: []string{"ctrl+s", "super+s"},
		}},
		DarkTint:         "dark",
		LightTint:        "light",
		OpaqueBackground: true,
	}
	require.NoError(t, store.Save(first))

	second := first
	second.SaveDirectory = "/second"
	second.Theme = ThemeLight
	require.NoError(t, store.Save(second))

	loaded, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, second, loaded)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "config.json", entries[0].Name())
}

func TestStoreIgnoresLegacyShortcutStyle(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"shortcut_style":"mac"}`), 0o600))

	snapshot, err := NewStore(path).Load()
	require.NoError(t, err)
	require.NoError(t, NewStore(path).Save(snapshot))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(data), "shortcut_style")
}

func TestStoreReportsMalformedSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

	_, err := NewStore(path).Load()

	require.ErrorContains(t, err, "decode settings")
}
