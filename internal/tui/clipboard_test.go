package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

	for _, key := range []tea.Key{
		{Code: 'c', Text: "c", Mod: tea.ModCtrl},
		{Code: 'c', Text: "c", Mod: tea.ModSuper},
	} {
		model, nodeID := newTestModel(t)
		model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
		var copied string
		model.clipboardMode = clipboardFallback
		model.clipboardFallback = func(text string) error {
			copied = text
			return nil
		}

		command := updateModelCommand(t, model, tea.KeyPressMsg(key))
		require.NotNil(t, command)
		message := command()
		command = updateModelCommand(t, model, message)

		require.Equal(t, strings.Join([]string{
			"┌──────┐",
			"│ node │",
			"└──────┘",
		}, "\n"), copied)
		require.Equal(t, modalNotice, model.modal)
		require.Equal(t, "Copied to clipboard", model.notice)
		require.NotNil(t, command)
	}
}

func TestSecondCopyOpensExportPrompt(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.clipboardMode = clipboardFallback
	model.clipboardFallback = func(string) error { return nil }
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	command := updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	}))
	updateModelCommand(t, model, command())
	updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	}))

	require.Equal(t, modalExport, model.modal)
	require.Equal(t, exportLineSlash, model.exportStyle)
	require.Contains(t, model.View().Content, "Line comments")
}

func TestExportPromptStartsWithPreferredComments(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.preferences.commentPrefix = "# "
	model.openExport("diagram")

	require.Equal(t, exportLineHash, model.exportStyle)
	options := exportOptions(model.exportStyle)
	require.Equal(t, exportLineHash, options[0].Value)
}

func TestCopySelectionReportsClipboardFailure(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.clipboardMode = clipboardFallback
	model.clipboardFallback = func(string) error { return errors.New("unavailable") }

	command := updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{
		Code: 'c',
		Mod:  tea.ModCtrl,
	}))
	updateModel(t, model, command())

	require.Equal(t, "copy selection: unavailable", model.status)
	require.False(t, model.copyArmed)
}

func TestClipboardProbeSelectsTerminalOrFallback(t *testing.T) {
	t.Parallel()

	t.Run("terminal response", func(t *testing.T) {
		t.Parallel()

		model, _ := newTestModel(t)
		model.clipboardPending = "diagram"
		command := model.handleClipboardResponse()

		require.Equal(t, clipboardTerminal, model.clipboardMode)
		require.Empty(t, model.clipboardPending)
		require.Equal(t, modalNotice, model.modal)
		require.NotNil(t, command)
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()

		model, _ := newTestModel(t)
		model.clipboardPending = "diagram"
		model.clipboardProbe = 2
		var copied string
		model.clipboardFallback = func(text string) error {
			copied = text
			return nil
		}

		require.Nil(t, model.handleClipboardTimeout(
			clipboardProbeExpiredMsg{generation: 1},
		))
		command := model.handleClipboardTimeout(
			clipboardProbeExpiredMsg{generation: 2},
		)
		require.Equal(t, clipboardFallback, model.clipboardMode)
		require.NotNil(t, command)
		require.NotNil(t, updateModelCommand(t, model, command()))
		require.Equal(t, "diagram", copied)
		require.Equal(t, modalNotice, model.modal)
	})
}

func TestFormatExport(t *testing.T) {
	t.Parallel()

	const diagram = "one   \ntwo\t\n"
	tests := []struct {
		name  string
		style exportStyle
		want  string
	}{
		{"slash", exportLineSlash, "// one\n// two\n//"},
		{"hash", exportLineHash, "# one\n# two\n#"},
		{"block", exportBlock, "/*\none\ntwo\n\n*/"},
		{"markdown", exportMarkdown, "```\none\ntwo\n\n```"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, formatExport(diagram, test.style))
		})
	}
}
