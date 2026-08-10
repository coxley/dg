package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coxley/dg/document"
	undohistory "github.com/coxley/dg/history"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestDevSessionRestoresEditorAndHistory(t *testing.T) {
	t.Parallel()

	model, nodeID, store := newStoredTestModel(t, "before")
	entry := *model.entry
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "after"))
	model.cursor = layout.NewPoint(13, 17)
	model.viewport = layout.NewPoint(5, 7)
	model.interaction.tool = toolConnect
	model.nodeStyle.Border = layout.BorderDouble
	model.edgeStyle.Stroke = layout.StrokeDashed
	model.selectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.sidebar.show()
	model.sidebar.focusActiveItem()
	model.sidebar.collapsed["RFCs"] = true
	model.helpInspector.visible = true
	model.helpInspector.requested = chrome.Rect{X: 2, Y: 3, Width: 30, Height: 10}
	model.helpInspector.positioned = true

	path := filepath.Join(t.TempDir(), "session.json.gz")
	require.NoError(t, model.writeDevSession(path))
	compressed, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte{0x1f, 0x8b}, compressed[:2])

	session, found, err := ConsumeDevSession(path)
	require.NoError(t, err)
	require.True(t, found)
	require.NoFileExists(t, path)
	require.Equal(t, entry.ID, session.EntryID)
	require.True(t, session.NeedsSave)

	geo, err := session.Document.Convert()
	require.NoError(t, err)
	history, err := undohistory.New(geo, undohistory.WithStore(store.History()))
	require.NoError(t, err)
	restored, err := history.Restore(session.Document)
	require.NoError(t, err)
	require.True(t, restored)
	model, err = New(
		geo,
		WithDocument(session.Document),
		WithHistory(history),
		WithCanvasStore(store, &entry),
		testModelSettings(),
		WithDevSession(session),
	)
	require.NoError(t, err)

	require.Equal(t, "after", model.geo.Label(nodeID))
	require.Equal(t, layout.NewPoint(13, 17), model.cursor)
	require.Equal(t, layout.NewPoint(5, 7), model.viewport)
	require.Equal(t, toolConnect, model.interaction.tool)
	require.Equal(t, layout.BorderDouble, model.nodeStyle.Border)
	require.Equal(t, layout.StrokeDashed, model.edgeStyle.Stroke)
	require.True(t, model.geo.Selection().HasNodes())
	require.True(t, model.sidebar.open)
	require.True(t, model.sidebar.focused)
	require.True(t, model.sidebar.collapsed["RFCs"])
	require.True(t, model.helpInspector.visible)
	require.Equal(t, model.helpInspector.requested, session.Help.Requested)
	require.NotEqual(t, model.saved, model.dirty)

	changed, err := model.history.Undo()
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "before", model.geo.Label(nodeID))
}

func TestDecodeDevSessionMigratesDocument(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	_, err = geo.NewNode("node")
	require.NoError(t, err)
	doc := document.New(geo)
	doc.Version = 3
	data, err := encodeDevSession(DevSession{Version: devSessionVersion, Document: doc})
	require.NoError(t, err)

	session, err := decodeDevSession(data)
	require.NoError(t, err)
	require.Equal(t, document.CurrentVersion, session.Document.Version)
}

func TestDevSessionPreservesLogicalGroupSelection(t *testing.T) {
	t.Parallel()

	model, first := newTestModel(t)
	second, err := model.geo.NewNodeAt("second", layout.NewPoint(20, 2))
	require.NoError(t, err)
	groupID, err := model.geo.NewGroup([]ir.Member{
		{ID: first, Kind: ir.MemberNode},
		{ID: second, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	require.True(t, model.geo.Selection().SelectOnly(layout.Hit{ID: groupID, Kind: layout.HitGroup}))

	session := model.captureDevSession()
	require.Empty(t, session.Selection.Nodes)
	require.Equal(t, []uint32{groupID}, session.Selection.Groups)

	model.geo.Selection().Clear()
	model.restoreDevSession(session)
	require.True(t, model.geo.Selection().DirectlyContains(layout.Hit{ID: groupID, Kind: layout.HitGroup}))
	require.False(t, model.geo.Selection().DirectlyContains(layout.Hit{ID: first, Kind: layout.HitNode}))
}

func TestDevSessionRestoresPreferencesDraft(t *testing.T) {
	t.Parallel()

	model, _, _ := newStoredTestModel(t, "node")
	baseline := model.geo.Router()
	draft := model.preferences.draft
	draft.Router.Costs.Step++
	draft.CommentPrefix = "// "
	model.openPreferences()
	model.previewPreferences(draft)
	const commentField chrome.ID = "comment"
	model.dialogs.preferences.model.Focus(commentField)

	path := filepath.Join(t.TempDir(), "session.json.gz")
	require.NoError(t, model.writeDevSession(path))
	require.Equal(t, baseline, model.geo.Router())
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())

	session, found, err := ConsumeDevSession(path)
	require.NoError(t, err)
	require.True(t, found)
	geo, err := session.Document.Convert()
	require.NoError(t, err)
	history, err := undohistory.New(geo, undohistory.WithCacheDir(t.TempDir()))
	require.NoError(t, err)
	model, err = New(
		geo,
		WithDocument(session.Document),
		WithHistory(history),
		testModelSettings(),
		WithDevSession(session),
	)
	require.NoError(t, err)

	require.Equal(t, surfacePreferences, model.dialogs.ActiveID())
	require.Equal(t, draft, model.preferences.draft)
	require.Equal(t, draft.Router, model.geo.Router())
	require.Equal(t, commentField, model.dialogs.preferences.model.FocusID())

	model.cancelPreferences()
	require.Equal(t, baseline, model.geo.Router())
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())
}

func TestConsumeDevSessionRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	data, err := encodeDevSession(DevSession{Version: devSessionVersion + 1})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "session.json.gz")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	_, found, err := ConsumeDevSession(path)
	require.ErrorContains(t, err, "unsupported development session version")
	require.False(t, found)
	require.NoFileExists(t, path)
}
