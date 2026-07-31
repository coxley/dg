package clipboard

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/stretchr/testify/require"
)

func TestSecondCopyOpensExportWithoutWriting(t *testing.T) {
	t.Parallel()

	model := newTestModel()
	copies := 0
	model.UseNative(func(string, []byte) error {
		copies++
		return nil
	})
	_, first := model.Update(RequestCopy("diagram", "// ", []byte("fragment")))
	_, second := model.Update(RequestCopy("diagram", "// ", []byte("fragment")))

	require.NotNil(t, first)
	require.IsType(t, OpenExportMsg{}, second())
	require.Zero(t, copies)
	require.Equal(t, LineSlash, model.Style())
	require.Contains(t, model.View().Content, "Line comments")
}

func TestNativeWriteReportsSuccess(t *testing.T) {
	t.Parallel()

	model := newTestModel()
	model.UseNative(func(string, []byte) error { return nil })
	_, _ = model.Update(RequestCopy("diagram", "// ", []byte("fragment")))
	_, command := model.Update(UpdateMsg{
		message: debounceExpiredMsg{generation: model.generation},
	})
	result := command().(UpdateMsg)
	_, command = model.Update(result)
	require.IsType(t, CopiedMsg{}, command())
}

func TestCopyDebounceWaitsForInactivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model := newTestModel()
		var copied string
		model.UseNative(func(text string, payload []byte) error {
			copied = text
			require.Equal(t, []byte("fragment"), payload)
			return nil
		})
		_, command := model.Update(RequestCopy("diagram", "// ", []byte("fragment")))
		messages := make(chan tea.Msg, 1)
		go func() {
			messages <- command()
		}()

		time.Sleep(DebounceDuration - time.Millisecond)
		require.Empty(t, messages)
		time.Sleep(time.Millisecond)
		message := <-messages
		require.IsType(t, UpdateMsg{}, message)
		_, command = model.Update(message)
		require.NotNil(t, command)
		result := command()
		_, command = model.Update(result)
		require.IsType(t, CopiedMsg{}, command())
		require.Equal(t, "diagram", copied)
	})
}

func TestCopyDebounceIgnoresStaleGeneration(t *testing.T) {
	t.Parallel()

	model := newTestModel()
	model.UseNative(func(string, []byte) error {
		require.Fail(t, "stale copy must not write")
		return nil
	})
	_, _ = model.Update(RequestCopy("diagram", "// ", nil))
	generation := model.generation
	model.CancelPending()

	_, command := model.Update(UpdateMsg{
		message: debounceExpiredMsg{generation: generation},
	})
	require.Nil(t, command)
}

func TestNativeFailureSelectsTerminalOrReportsFailure(t *testing.T) {
	t.Parallel()

	t.Run("terminal response", func(t *testing.T) {
		t.Parallel()

		model := newTestModel()
		model.mode = probingTerminal
		model.pending = requestCopyMsg{text: "diagram"}
		_, command := model.Update(tea.ClipboardMsg{})

		require.Equal(t, terminal, model.mode)
		require.Empty(t, model.pending)
		require.NotNil(t, command)
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()

		model := newTestModel()
		model.mode = probingTerminal
		model.pending = requestCopyMsg{text: "diagram"}
		model.nativeErr = errors.New("unavailable")
		model.probe = 2

		_, command := model.Update(UpdateMsg{
			message: probeExpiredMsg{generation: 1},
		})
		require.Nil(t, command)
		_, command = model.Update(UpdateMsg{
			message: probeExpiredMsg{generation: 2},
		})
		require.Equal(t, unknown, model.mode)
		require.NotNil(t, command)
		require.Equal(t, ErrorMsg{Err: errors.New("unavailable")}, command())
	})
}

func TestStructuredPasteRequiresMatchingText(t *testing.T) {
	t.Parallel()

	encoded := encodePayload("diagram", []byte("fragment"))
	model := newTestModel()
	model.UseNativeReader(func() []byte { return encoded })

	message := model.ReadPaste("diagram")().(PasteMsg)
	require.Equal(t, []byte("fragment"), message.Data)
	message = model.ReadPaste("other")().(PasteMsg)
	require.Nil(t, message.Data)
}

func TestFormat(t *testing.T) {
	t.Parallel()

	const diagram = "one   \ntwo\t\n"
	tests := []struct {
		name  string
		style Style
		want  string
	}{
		{"slash", LineSlash, "// one\n// two\n//"},
		{"hash", LineHash, "# one\n# two\n#"},
		{styleBlockValue, Block, "/*\none\ntwo\n\n*/"},
		{styleMarkdownValue, Markdown, "```\none\ntwo\n\n```"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, Format(diagram, test.style))
		})
	}
}

func TestModelImplementsTeaModel(t *testing.T) {
	t.Parallel()

	var model tea.Model = newTestModel()
	require.Empty(t, model.View().Content)
}

func TestExportUsesSemanticFormTraversalAndAccessibleAction(t *testing.T) {
	t.Parallel()

	model := newTestModel()
	model.UseNative(func(string, []byte) error { return nil })
	var copiedText string
	var copiedPayload []byte
	model.UseNative(func(text string, payload []byte) error {
		copiedText = text
		copiedPayload = append([]byte(nil), payload...)
		return nil
	})
	model.openExport("diagram", "// ", []byte("fragment"))
	require.Contains(t, model.AccessibleLines(), "action: Copy")

	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	require.Equal(t, LineHash, model.Style())
	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	_, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.NotNil(t, command)
	message := command().(UpdateMsg)
	_, command = model.Update(message)
	require.NotNil(t, command)
	require.Nil(t, model.form)
	for _, child := range command().(tea.BatchMsg) {
		result := child()
		if result == nil {
			continue
		}
		if update, ok := result.(UpdateMsg); ok {
			_, followup := model.Update(update)
			if followup != nil {
				_ = followup()
			}
		}
	}
	require.Equal(t, "# diagram", copiedText)
	require.Equal(t, []byte("fragment"), copiedPayload)
}

func newTestModel() *Model {
	return New(chrome.FormStyles{
		Label:        lipgloss.NewStyle(),
		HoveredLabel: lipgloss.NewStyle(),
		FocusedLabel: lipgloss.NewStyle().Bold(true),
		Value:        lipgloss.NewStyle(),
		HoveredValue: lipgloss.NewStyle(),
		FocusedValue: lipgloss.NewStyle().Bold(true),
		Number: chrome.NumberFieldStyles{
			Value:            lipgloss.NewStyle(),
			HoveredValue:     lipgloss.NewStyle(),
			FocusedValue:     lipgloss.NewStyle().Bold(true),
			FocusedDecrement: lipgloss.NewStyle().Bold(true),
			ActiveDecrement:  lipgloss.NewStyle().Reverse(true),
			FocusedIncrement: lipgloss.NewStyle().Bold(true),
			ActiveIncrement:  lipgloss.NewStyle().Reverse(true),
		},
		Buttons: chrome.ButtonListStyles{
			Button:        lipgloss.NewStyle().Padding(0, 1),
			HoveredButton: lipgloss.NewStyle().Padding(0, 1),
			FocusedButton: lipgloss.NewStyle().Reverse(true).Padding(0, 1),
		},
	})
}
