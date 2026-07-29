package directorypicker

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

const testTitle = "Directory"

func TestPickerRetainsBoundedLifecycleAndValue(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	picker := New(Config{Title: testTitle, Value: directory}, Styles{Dark: true})
	picker.SetBounds(40, 10)
	picker.Open()
	require.True(t, picker.Opened())
	require.Equal(t, directory, picker.Value())
	require.NotEmpty(t, picker.View().Content)

	picker.Close()
	require.False(t, picker.Opened())
	path := filepath.Join(directory, "diagram.json")
	picker.SetValue(path)
	require.Equal(t, path, picker.Value())
}

func TestPickerShowsOnlyVisibleDirectories(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "alpha"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "beta"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(directory, ".hidden"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(directory, "diagram.json"),
		nil,
		0o600,
	))
	picker := New(Config{Title: testTitle, Value: directory}, Styles{})
	picker.SetBounds(40, 8)
	picker.Open()

	view := ansi.Strip(picker.View().Content)

	require.Contains(t, view, "alpha")
	require.Contains(t, view, "beta")
	require.NotContains(t, view, ".hidden")
	require.NotContains(t, view, "diagram.json")
}

func TestPickerNavigatesAndSelectsDirectories(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	alpha := filepath.Join(directory, "alpha")
	child := filepath.Join(alpha, "child")
	require.NoError(t, os.MkdirAll(child, 0o700))
	picker := New(Config{Title: testTitle, Value: directory}, Styles{})
	picker.SetBounds(40, 8)
	picker.Open()

	_, command := picker.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	require.Nil(t, command)
	require.Contains(t, ansi.Strip(picker.View().Content), "child")
	_, command = picker.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.Nil(t, command)
	require.False(t, picker.Opened())
	require.Equal(t, child, picker.Value())
}

func TestPickerImplementsTeaModel(t *testing.T) {
	t.Parallel()

	var model tea.Model = New(Config{Value: t.TempDir()}, Styles{})
	require.NotNil(t, model)
}
