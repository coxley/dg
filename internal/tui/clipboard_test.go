package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	clipboardview "github.com/coxley/dg/internal/tui/clipboard"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestSelectionTextRendersOnlySelectedObjects(t *testing.T) {
	t.Parallel()

	model, left, right := newTwoNodeModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: left, Kind: layout.HitNode})
	require.True(t, model.geo.Selection().Toggle(layout.Hit{
		ID:   right,
		Kind: layout.HitNode,
	}))

	text, err := model.selectionText()
	require.NoError(t, err)
	require.Equal(t, strings.Join([]string{
		"┌──────┐          ┌───────┐",
		"│ left │          │ right │",
		"└──────┘          └───────┘",
	}, "\n"), text)
}

func TestCopySelectionWritesOnControlRelease(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	var copied string
	var payload []byte
	model.clipboard.UseNative(func(text string, data []byte) error {
		copied = text
		payload = append([]byte(nil), data...)
		return nil
	})
	updateModel(t, model, tea.KeyboardEnhancementsMsg{
		Flags: ansi.KittyReportEventTypes,
	})

	command := updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	}))
	require.Nil(t, command)
	require.Empty(t, copied)
	require.Nil(t, updateModelCommand(t, model, tea.KeyReleaseMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	})))
	command = updateModelCommand(t, model, tea.KeyReleaseMsg(tea.Key{
		Code: tea.KeyLeftCtrl,
		Mod:  tea.ModCtrl,
	}))
	require.NotNil(t, command)
	command = updateModelCommand(t, model, command())
	require.NotNil(t, command)
	command = updateModelCommand(t, model, command())
	require.NotNil(t, command)

	require.Equal(t, strings.Join([]string{
		"┌──────┐",
		"│ node │",
		"└──────┘",
	}, "\n"), copied)
	require.NotEmpty(t, payload)
	require.Equal(t, surfaceNotice, model.dialogs.ActiveID())
	require.Equal(t, "Copied to clipboard", model.dialogs.notice.text)
}

func TestSecondControlCBeforeReleaseOpensExportPrompt(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	updateModel(t, model, tea.KeyboardEnhancementsMsg{
		Flags: ansi.KittyReportEventTypes,
	})
	model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.clipboard.UseNative(func(string, []byte) error {
		require.Fail(t, "export gesture must not write")
		return nil
	})
	copyKey := tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	require.Nil(t, updateModelCommand(t, model, copyKey))
	require.Nil(t, updateModelCommand(t, model, tea.KeyReleaseMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	})))
	require.Nil(t, updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code:     'c',
		Mod:      tea.ModCtrl,
		IsRepeat: true,
	})))
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())

	command := updateModelCommand(t, model, copyKey)
	require.NotNil(t, command)
	updateModel(t, model, command())

	require.Equal(t, surfaceExport, model.dialogs.ActiveID())
	require.Equal(t, clipboardview.LineSlash, model.clipboard.Style())
	require.Contains(t, ansi.Strip(model.View().Content), "Line comments")
	require.Nil(t, updateModelCommand(t, model, tea.KeyReleaseMsg(tea.Key{
		Code: tea.KeyLeftCtrl,
		Mod:  tea.ModCtrl,
	})))
}

func TestCopyGestureCancelsOnBlur(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	updateModel(t, model, tea.KeyboardEnhancementsMsg{
		Flags: ansi.KittyReportEventTypes,
	})
	model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.clipboard.UseNative(func(string, []byte) error {
		require.Fail(t, "canceled copy must not write")
		return nil
	})
	require.Nil(t, updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	})))
	updateModel(t, model, tea.BlurMsg{})

	require.Nil(t, updateModelCommand(t, model, tea.KeyReleaseMsg(tea.Key{
		Code: tea.KeyLeftCtrl,
		Mod:  tea.ModCtrl,
	})))
}

func TestCopySelectionReportsClipboardFailure(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	updateModel(t, model, clipboardview.ErrorMsg{Err: errors.New("unavailable")})

	require.Equal(t, "copy selection: unavailable", model.status)
}

func TestClipboardFragmentPastesIntoSameAndOtherCanvas(t *testing.T) {
	t.Parallel()

	source, nodeID := newTestModel(t)
	source.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	payload, err := source.copySelectionPayload()
	require.NoError(t, err)
	original := source.geo.Nodes[nodeID].Rect

	updateModel(t, source, clipboardview.PasteMsg{Data: payload})
	duplicate, ok := source.geo.Selection().FirstNode()
	require.True(t, ok)
	require.Equal(t, original.Max().X+2, source.geo.Nodes[duplicate].Rect.Min.X)
	require.Equal(t, original.Min.Y, source.geo.Nodes[duplicate].Rect.Min.Y)
	firstDuplicate := source.geo.Nodes[duplicate].Rect
	updateModel(t, source, clipboardview.PasteMsg{Data: payload})
	duplicate, ok = source.geo.Selection().FirstNode()
	require.True(t, ok)
	require.Equal(t, firstDuplicate.Max().X+2, source.geo.Nodes[duplicate].Rect.Min.X)
	require.Equal(t, firstDuplicate.Min.Y, source.geo.Nodes[duplicate].Rect.Min.Y)

	destination, _ := newTestModel(t)
	destination.cursor = layout.NewPoint(30, 12)
	updateModel(t, destination, clipboardview.PasteMsg{Data: payload})
	pasted, ok := destination.geo.Selection().FirstNode()
	require.True(t, ok)
	require.Equal(t, layout.NewPoint(30, 12), destination.geo.Nodes[pasted].Rect.Min)
	require.Equal(t, "node", destination.geo.Label(pasted))
	firstPaste := destination.geo.Nodes[pasted].Rect
	updateModel(t, destination, clipboardview.PasteMsg{Data: payload})
	pasted, ok = destination.geo.Selection().FirstNode()
	require.True(t, ok)
	require.Equal(t, firstPaste.Max().X+2, destination.geo.Nodes[pasted].Rect.Min.X)
	require.Equal(t, firstPaste.Min.Y, destination.geo.Nodes[pasted].Rect.Min.Y)
}

func TestStaleExportCloseDoesNotDismissCopiedNotice(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.dialogs.OpenExport()
	command, handled := model.updatePresentation(clipboardview.CopiedMsg{})
	require.True(t, handled)
	require.NotNil(t, command)
	require.Equal(t, surfaceNotice, model.dialogs.ActiveID())

	command, handled = model.updatePresentation(clipboardview.CloseExportMsg{})

	require.True(t, handled)
	require.Nil(t, command)
	require.Equal(t, surfaceNotice, model.dialogs.ActiveID())
	require.Equal(t, "Copied to clipboard", model.dialogs.notice.text)
}
