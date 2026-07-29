package directorypicker

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestPickerRetainsBoundedLifecycleAndValue(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	picker := New(Config{
		Title: "Directory", Value: directory, ShowHidden: true,
	}, Styles{Dark: true})
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

func TestPickerImplementsTeaModel(t *testing.T) {
	t.Parallel()

	var model tea.Model = New(Config{Value: t.TempDir()}, Styles{})
	require.NotNil(t, model)
}
