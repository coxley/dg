package tui

import (
	"testing"

	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/stretchr/testify/require"
)

const escapeChord = "esc"

func TestApplicationPrimaryBindingsFollowShortcutStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile chrome.KeyProfile
		prefix  string
	}{
		{name: "standard", profile: chrome.ProfileStandard, prefix: "ctrl+"},
		{name: "mac", profile: chrome.ProfileMac, prefix: "super+"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolver, err := chrome.NewResolver(applicationBindings)
			require.NoError(t, err)
			resolver.SetProfile(test.profile)
			resolver.SetSuperAvailable(true)

			requirePrimaryBinding(t, resolver, scopeGlobal, commandSave, test.prefix+"s")
			requirePrimaryBinding(t, resolver, scopeGlobal, commandPreferences, test.prefix+"p")
			requirePrimaryBinding(t, resolver, scopeGlobal, commandSidebar, test.prefix+"b")
			requirePrimaryBinding(t, resolver, scopeCanvas, commandExpand, test.prefix+"a")
		})
	}
}

func TestApplicationBindingsCoverEditorCommands(t *testing.T) {
	t.Parallel()

	resolver, err := chrome.NewResolver(applicationBindings)
	require.NoError(t, err)
	resolver.SetProfile(chrome.ProfileStandard)
	resolver.SetSuperAvailable(true)

	tests := []struct {
		scope   chrome.ScopeID
		chord   string
		command chrome.CommandID
	}{
		{scopeCanvas, "up", commandMoveUp},
		{scopeCanvas, "right", commandMoveRight},
		{scopeCanvas, "down", commandMoveDown},
		{scopeCanvas, "left", commandMoveLeft},
		{scopeCanvas, "tab", commandFocusNext},
		{scopeCanvas, "shift+tab", commandFocusPrevious},
		{scopeCanvas, "ctrl+tab", commandCycleHitNext},
		{scopeCanvas, "ctrl+shift+tab", commandCycleHitPrevious},
		{scopeCanvas, "enter", commandActivate},
		{scopeCanvas, "m", commandMove},
		{scopeCanvas, "e", commandEditLabel},
		{scopeCanvas, "n", commandNewNode},
		{scopeCanvas, "r", commandRectangle},
		{scopeCanvas, "l", commandLine},
		{scopeCanvas, "b", commandBorder},
		{scopeCanvas, "-", commandDashed},
		{scopeCanvas, "a", commandArrowEnd},
		{scopeCanvas, "shift+a", commandArrowStart},
		{scopeCanvas, "t", commandTextHorizontal},
		{scopeCanvas, "shift+t", commandTextVertical},
		{scopeCanvas, "d", commandDuplicate},
		{scopeCanvas, "backspace", commandDelete},
		{scopeCanvas, "[", commandLayerBackward},
		{scopeCanvas, "]", commandLayerForward},
		{scopeCanvas, "{", commandLayerBack},
		{scopeCanvas, "}", commandLayerFront},
		{scopeCanvas, "u", commandUndo},
		{scopeCanvas, "ctrl+r", commandRedo},
		{scopeCanvas, "ctrl+a", commandExpand},
		{scopeCanvas, "ctrl+c", commandCopy},
		{scopeCanvas, escapeChord, commandCancel},
		{scopeCanvas, "q", commandQuit},
		{scopeSidebar, escapeChord, commandBack},
		{scopeSidebar, "tab", commandSidebarNext},
		{scopeSidebar, "shift+tab", commandSidebarPrevious},
		{scopePreferences, "q", commandBack},
		{scopeDirectory, "q", commandBack},
		{scopeModal, escapeChord, commandBack},
		{scopeGlobal, "?", commandHelp},
		{scopeGlobal, "ctrl+p", commandPreferences},
		{scopeGlobal, "ctrl+s", commandSave},
		{scopeGlobal, "ctrl+b", commandSidebar},
	}
	for _, test := range tests {
		message, ok := resolver.Resolve(
			test.chord,
			[]chrome.ScopeID{test.scope},
			false,
		)
		require.True(t, ok, "%s %s", test.scope, test.chord)
		require.Equal(t, test.command, message.Command, "%s %s", test.scope, test.chord)
	}
}

func TestLabelTextEntryExcludesCanvasCommands(t *testing.T) {
	t.Parallel()

	resolver, err := chrome.NewResolver(applicationBindings)
	require.NoError(t, err)
	resolver.SetProfile(chrome.ProfileStandard)

	_, ok := resolver.Resolve("e", labelBindingScopes[:], true)
	require.False(t, ok)
	message, ok := resolver.Resolve("ctrl+p", labelBindingScopes[:], true)
	require.True(t, ok)
	require.Equal(t, commandPreferences, message.Command)
}

func requirePrimaryBinding(
	t *testing.T,
	resolver *chrome.Resolver,
	scope chrome.ScopeID,
	command chrome.CommandID,
	chord string,
) {
	t.Helper()

	resolved, ok := resolver.Resolve(chord, []chrome.ScopeID{scope}, false)
	require.True(t, ok)
	require.Equal(t, command, resolved.Command)
}
