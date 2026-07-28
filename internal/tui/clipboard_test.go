package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	require.Equal(t, modalNotice, model.modal)
	require.Equal(t, "Copied to clipboard", model.notice)
}

func TestSecondCopyOpensExportPromptForControlAndSuper(t *testing.T) {
	t.Parallel()

	keys := map[string]tea.KeyPressMsg{
		"control": {Code: 'c', Mod: tea.ModCtrl},
		"super":   {Code: 'c', Text: "c", Mod: tea.ModSuper},
	}
	for firstName, first := range keys {
		for secondName, second := range keys {
			t.Run(firstName+" then "+secondName, func(t *testing.T) {
				t.Parallel()

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

				require.NotNil(t, updateModelCommand(t, model, first))
				command := updateModelCommand(t, model, second)
				require.NotNil(t, command)
				updateModel(t, model, command())

				require.Equal(t, modalExport, model.modal)
				require.Equal(
					t,
					clipboardview.LineSlash,
					model.clipboard.Style(),
				)
				require.Contains(t, model.View().Content, "Line comments")
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
