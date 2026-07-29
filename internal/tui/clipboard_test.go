package tui

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

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

func TestCopySelectionUsesControlCAndFallback(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	var copied string
	model.clipboard.UseFallback(func(text string) error {
		copied = text
		return nil
	})

	command := updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	}))
	require.NotNil(t, command)
	command = updateModelCommand(t, model, command())
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
	require.Equal(t, surfaceNotice, model.dialogs.ActiveID())
	require.Equal(t, "Copied to clipboard", model.dialogs.notice.text)
}

func TestSecondCopyBeforeDebounceOpensExportPrompt(t *testing.T) {
	keys := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"control", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
		{"super", tea.KeyPressMsg{Code: 'c', Text: "c", Mod: tea.ModSuper}},
	}
	for _, first := range keys {
		for _, second := range keys {
			t.Run(first.name+" then "+second.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					model, nodeID := newTestModel(t)
					updateModel(t, model, tea.WindowSizeMsg{
						Width:  80,
						Height: 24,
					})
					model.geo.Selection().SelectOnly(layout.Hit{
						ID:   nodeID,
						Kind: layout.HitNode,
					})
					model.clipboard.UseFallback(func(string) error {
						require.Fail(t, "second copy must not write")
						return nil
					})

					firstCommand := updateModelCommand(t, model, first.key)
					require.NotNil(t, firstCommand)
					messages := make(chan tea.Msg, 1)
					go func() {
						messages <- firstCommand()
					}()

					time.Sleep(
						clipboardview.DebounceDuration -
							time.Millisecond,
					)
					require.Empty(t, messages)
					updateModel(t, model, tea.MouseMotionMsg{
						X: 10,
						Y: 10,
					})
					updateModel(t, model, tea.KeyPressMsg{
						Code: tea.KeyLeftCtrl,
					})

					command := updateModelCommand(t, model, second.key)
					require.NotNil(t, command)
					updateModel(t, model, command())

					require.Equal(t, surfaceExport, model.dialogs.ActiveID())
					require.Equal(
						t,
						clipboardview.LineSlash,
						model.clipboard.Style(),
					)
					require.Contains(
						t,
						ansi.Strip(model.View().Content),
						"Line comments",
					)

					time.Sleep(time.Millisecond)
					stale := <-messages
					require.Nil(
						t,
						updateModelCommand(t, model, stale),
					)
					require.Equal(t, surfaceExport, model.dialogs.ActiveID())
				})
			})
		}
	}
}

func TestCopySelectionReportsClipboardFailure(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.clipboard.UseFallback(func(string) error {
		return errors.New("unavailable")
	})

	command := updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	}))
	for range 2 {
		require.NotNil(t, command)
		command = updateModelCommand(t, model, command())
	}
	require.NotNil(t, command)
	require.Nil(t, updateModelCommand(t, model, command()))

	require.Equal(t, "copy selection: unavailable", model.status)
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
