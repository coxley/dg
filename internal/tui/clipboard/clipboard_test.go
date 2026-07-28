package clipboard

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/stretchr/testify/require"
)

func TestSecondCopyOpensExportWithoutWriting(t *testing.T) {
	t.Parallel()

	model := newTestModel()
	copies := 0
	model.UseFallback(func(string) error {
		copies++
		return nil
	})
	_, first := model.Update(RequestCopy("diagram", "// "))
	_, second := model.Update(RequestCopy("diagram", "// "))

	require.NotNil(t, first)
	require.IsType(t, OpenExportMsg{}, second())
	require.Zero(t, copies)
	require.Equal(t, LineSlash, model.Style())
	require.Contains(t, model.View().Content, "Line comments")
}

func TestFallbackReportsSuccessOrFailure(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		err  error
		want tea.Msg
	}{
		{"success", nil, CopiedMsg{}},
		{"failure", errors.New("unavailable"), ErrorMsg{
			Err: errors.New("unavailable"),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			model := newTestModel()
			model.UseFallback(func(string) error { return test.err })
			_, _ = model.Update(RequestCopy("diagram", "// "))
			_, command := model.Update(UpdateMsg{
				message: debounceExpiredMsg{generation: model.generation},
			})
			result := command().(UpdateMsg)
			_, command = model.Update(result)
			require.Equal(t, test.want, command())
		})
	}
}

func TestCopyDebounceWaitsForInactivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model := newTestModel()
		var copied string
		model.UseFallback(func(text string) error {
			copied = text
			return nil
		})
		_, command := model.Update(RequestCopy("diagram", "// "))
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
	model.UseFallback(func(string) error {
		require.Fail(t, "stale copy must not write")
		return nil
	})
	_, _ = model.Update(RequestCopy("diagram", "// "))
	generation := model.generation
	model.CancelPending()

	_, command := model.Update(UpdateMsg{
		message: debounceExpiredMsg{generation: generation},
	})
	require.Nil(t, command)
}

func TestClipboardProbeSelectsTerminalOrFallback(t *testing.T) {
	t.Parallel()

	t.Run("terminal response", func(t *testing.T) {
		t.Parallel()

		model := newTestModel()
		model.pending = "diagram"
		_, command := model.Update(tea.ClipboardMsg{})

		require.Equal(t, terminal, model.mode)
		require.Empty(t, model.pending)
		require.NotNil(t, command)
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()

		model := newTestModel()
		var copied string
		model.fallback = func(text string) error {
			copied = text
			return nil
		}
		model.pending = "diagram"
		model.probe = 2

		_, command := model.Update(UpdateMsg{
			message: probeExpiredMsg{generation: 1},
		})
		require.Nil(t, command)
		_, command = model.Update(UpdateMsg{
			message: probeExpiredMsg{generation: 2},
		})
		require.Equal(t, fallback, model.mode)
		require.NotNil(t, command)
		result := command().(UpdateMsg)
		_, command = model.Update(result)
		require.IsType(t, CopiedMsg{}, command())
		require.Equal(t, "diagram", copied)
	})
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
		{"block", Block, "/*\none\ntwo\n\n*/"},
		{"markdown", Markdown, "```\none\ntwo\n\n```"},
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

func newTestModel() *Model {
	return New(huh.ThemeFunc(func(bool) *huh.Styles {
		return huh.ThemeCharm(true)
	}))
}
