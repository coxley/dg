package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestWatchDevReloadReportsMarkerChange(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	markerPath := filepath.Join(root, "reload")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	events, err := watchDevReload(ctx, markerPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(markerPath, []byte("1"), 0o600))

	select {
	case event := <-events:
		require.NoError(t, event.err)
		require.False(t, event.closed)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for development reload marker")
	}
}

func TestUpdateDevReloadWritesSessionAndQuits(t *testing.T) {
	t.Parallel()

	model, _, _ := newStoredTestModel(t, "node")
	model.devReload.sessionPath = filepath.Join(t.TempDir(), "session.json.gz")

	command, handled := model.updateDevReload(devReloadMsg{})
	require.True(t, handled)
	require.True(t, model.devReload.requested)
	require.IsType(t, tea.QuitMsg{}, command())

	session, found, err := ConsumeDevSession(model.devReload.sessionPath)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "node", session.Document.Nodes[0].Label)
}

func TestUpdateDevReloadKeepsRunningWhenHandoffFails(t *testing.T) {
	t.Parallel()

	model, _, _ := newStoredTestModel(t, "node")
	model.devReload.sessionPath = filepath.Join(t.TempDir(), "missing", "session.json.gz")

	command, handled := model.updateDevReload(devReloadMsg{})
	require.True(t, handled)
	require.Nil(t, command)
	require.False(t, model.devReload.requested)
	require.Contains(t, model.statusError, "write development session")
}
