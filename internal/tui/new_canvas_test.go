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

func TestControlNCreatesTransientBlankAndFlushesPreviousCanvas(t *testing.T) {
	t.Parallel()

	model, nodeID, store := newStoredTestModel(t, "original")
	previousID := model.document.ID
	geo := model.geo
	transaction := model.history.Begin()
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "changed"))
	require.NoError(t, transaction.Commit())

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))

	require.Same(t, geo, model.geo)
	require.NotEqual(t, previousID, model.document.ID)
	require.Nil(t, model.entry)
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
	require.Len(t, entries, 1)
}

func TestNewCanvasUsesSavedDefaultRouter(t *testing.T) {
	t.Parallel()

	model, _, _ := newStoredTestModel(t, "original")
	router := layout.DefaultRouter()
	router.Costs.Step = 41
	model.preferences.defaultRouter = router

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))

	require.Equal(t, router, model.geo.Router())
}

func TestTransientCanvasMaterializesAfterContentAndPersistsEmptyAgain(t *testing.T) {
	t.Parallel()

	model, _, store := newStoredTestModel(t, "original")
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))
	transientID := model.document.ID

	transaction := model.history.Begin()
	nodeID, err := model.geo.NewNode("content")
	require.NoError(t, err)
	require.NoError(t, transaction.Commit())
	updateModel(t, model, autosaveMsg(model.dirty))
	require.NotNil(t, model.entry)
	require.True(t, model.entry.Draft)
	require.Equal(t, transientID, model.entry.ID)
	loaded, err := store.Load(*model.entry)
	require.NoError(t, err)
	require.Equal(t, "content", loaded.Nodes[0].Label)

	transaction = model.history.Begin()
	require.NoError(t, model.geo.DeleteNode(nodeID))
	require.NoError(t, model.geo.Build())
	require.NoError(t, transaction.Commit())
	updateModel(t, model, autosaveMsg(model.dirty))
	loaded, err = store.Load(*model.entry)
	require.NoError(t, err)
	require.Empty(t, loaded.Nodes)
}

func TestRepeatedControlNDoesNotAccumulateDrafts(t *testing.T) {
	t.Parallel()

	model, _, store := newStoredTestModel(t, "original")
	for range 100 {
		updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))
	}

	require.Nil(t, model.entry)
	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestNamingTransientCanvasMaterializesNamedRecord(t *testing.T) {
	t.Parallel()

	model, _, store := newStoredTestModel(t, "original")
	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))
	model.saveFromDialog(saveDocumentMsg{Name: "Empty"})

	require.NotNil(t, model.entry)
	require.False(t, model.entry.Draft)
	require.Equal(t, "Empty", model.entry.Name)
	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 2)
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

func BenchmarkModelNewCanvasTransient(b *testing.B) {
	model, _, store := newStoredTestModel(b, "original")
	message := tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		updated, command := model.Update(message)
		if updated != model || command != nil {
			b.Fatal("new transient canvas returned an unexpected update")
		}
	}
	b.StopTimer()
	entries, err := store.List()
	require.NoError(b, err)
	require.Len(b, entries, 1)
}
