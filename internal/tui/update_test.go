package tui

import (
	"runtime/debug"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

const (
	testCurrentVersion = "v0.0.4"
	testLatestVersion  = "v0.0.5"
)

func TestInstalledVersionBypassesLocalBuilds(t *testing.T) {
	t.Parallel()

	_, ok := installedVersion(&debug.BuildInfo{
		Main: debug.Module{Path: modulePath, Version: "(devel)"},
	}, true)
	require.False(t, ok)
	_, ok = installedVersion(&debug.BuildInfo{
		Main: debug.Module{Path: "example.com/other", Version: "v1.2.3"},
	}, true)
	require.False(t, ok)

	version, ok := installedVersion(&debug.BuildInfo{
		Main: debug.Module{Path: modulePath, Version: "v1.2.3"},
	}, true)
	require.True(t, ok)
	require.Equal(t, "v1.2.3", version)
}

func TestNewerVersionComparesReleaseTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		current, latest string
		want            bool
	}{
		{name: "patch", current: testCurrentVersion, latest: testLatestVersion, want: true},
		{name: "same", current: testLatestVersion, latest: testLatestVersion},
		{name: "older latest", current: "v0.0.6", latest: testLatestVersion},
		{name: "stable replaces prerelease", current: "v1.0.0-rc.1", latest: "v1.0.0", want: true},
		{name: "newer prerelease", current: "v1.0.0-rc.1", latest: "v1.0.0-rc.2", want: true},
		{name: "malformed", current: "devel", latest: "v1.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, newerVersion(test.current, test.latest))
		})
	}
}

func TestUpdateBannerFocusesAndDismisses(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 20})
	updateModel(t, model, updateAvailableMsg{current: testCurrentVersion, latest: testLatestVersion})
	surface, ok := model.surfacePlan(surfaceUpdate)
	require.True(t, ok)
	require.Equal(t, model.width, surface.Rect.Right())
	require.Contains(t, model.View().Content, "Update Available")

	updateModel(t, model, tea.MouseClickMsg{
		X: surface.Rect.X, Y: surface.Rect.Y, Button: tea.MouseLeft,
	})
	require.True(t, model.updateNotice.focused)
	updateModel(t, model, keyPress(tea.KeyBackspace, ""))
	require.False(t, model.updateNotice.visible())
}

func TestFocusedUpdateBannerStartsInstall(t *testing.T) {
	t.Parallel()

	var update updateState
	update.show(updateAvailableMsg{current: testCurrentVersion, latest: testLatestVersion})
	update.focus()

	command := update.install()

	require.NotNil(t, command)
	require.True(t, update.installing)
}
