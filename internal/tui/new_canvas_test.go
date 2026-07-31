package tui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestControlNCreatesBlankDurableDraftAndFlushesPreviousCanvas(t *testing.T) {
	t.Parallel()

	model, nodeID, store := newStoredTestModel(t, "original")
	previousID := model.document.ID
	geo := model.geo
	transaction := model.history.Begin()
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "changed"))
	require.NoError(t, transaction.Commit())

	require.NotNil(t, updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl})))

	require.Same(t, geo, model.geo)
	require.NotEqual(t, previousID, model.document.ID)
	require.True(t, model.entry.Draft)
	require.Empty(t, slices.Collect(model.geo.DrawOrder()))
	require.False(t, model.history.CanUndo())
	require.Equal(t, "new draft", model.status)
	require.Equal(t, "dg - Draft", model.View().WindowTitle)
	previous := findStoreEntry(t, store, previousID)
	loaded, err := store.Load(previous)
	require.NoError(t, err)
	require.Equal(t, "changed", loaded.Nodes[nodeID].Label)
	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 2)
}

func TestNewCanvasUsesSavedFutureRouter(t *testing.T) {
	t.Parallel()

	model, _, _ := newStoredTestModel(t, "original")
	router := layout.DefaultRouter()
	router.Costs.Step = 41
	model.preferences.applyToFuture = true
	model.preferences.baseline.Router = router

	require.NotNil(t, updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl})))

	require.Equal(t, router, model.geo.Router())
}

func TestNewCanvasStopsWhenCurrentCanvasCannotFlush(t *testing.T) {
	t.Parallel()

	model, store := newNamedStoredTestModel(t, "original")
	previousID := model.document.ID
	external := document.New(mustLayoutWithLabel(t, "external"))
	external.ID = previousID
	replaceStoredDocument(t, store, *model.entry, external)
	transaction := model.history.Begin()
	require.NoError(t, model.geo.SetNodeLabel(0, "local"))
	require.NoError(t, transaction.Commit())

	updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))

	require.Equal(t, previousID, model.document.ID)
	require.Contains(t, model.statusError, "externally modified")
	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestNewCanvasUsesControlNWithoutPrimaryProjection(t *testing.T) {
	t.Parallel()

	model, _, _ := newStoredTestModel(t, "original")
	for _, profile := range []chrome.KeyProfile{chrome.ProfileMac, chrome.ProfileStandard} {
		model.bindings.SetProfile(profile)
		resolved, ok := model.bindings.ResolveKey(
			tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}),
			[]chrome.ScopeID{scopeCanvas},
			false,
		)
		require.True(t, ok)
		require.Equal(t, commandNewCanvas, resolved.Command)
		_, ok = model.bindings.ResolveKey(
			tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModSuper}),
			[]chrome.ScopeID{scopeCanvas},
			false,
		)
		require.False(t, ok)
	}
}
