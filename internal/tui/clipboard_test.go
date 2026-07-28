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
		{Code: 'c', Text: "c"},
	} {
		model, nodeID := newTestModel(t)
		model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
		var copied string
		model.clipboardWrite = func(text string) error {
			copied = text
			return nil
		}

		updateModelCommand(t, model, tea.KeyPressMsg(key))

		require.Equal(t, strings.Join([]string{
			"┌──────┐",
			"│ node │",
			"└──────┘",
		}, "\n"), copied)
		require.Equal(t, modalNotice, model.modal)
		require.Equal(t, "Copied to clipboard", model.notice)
	}
}

func TestSecondCopyOpensExportPrompt(t *testing.T) {
	t.Parallel()

	model, nodeID := newTestModel(t)
	model.geo.Selection().SelectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	model.clipboardWrite = func(string) error { return nil }
	updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	updateModelCommand(t, model, keyPress('c', "c"))
	updateModelCommand(t, model, keyPress('c', "c"))

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
	model.clipboardWrite = func(string) error { return errors.New("unavailable") }

	updateModelCommand(t, model, keyPress('c', "c"))

	require.Equal(t, "copy selection: unavailable", model.status)
	require.False(t, model.copyArmed)
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
