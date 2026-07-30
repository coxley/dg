package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/stretchr/testify/require"
)

const (
	escapeChord = "esc"
	tabChord    = "tab"
)

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
			resolver.SetKeyDisambiguation(true)

			requirePrimaryBinding(t, resolver, scopeGlobal, commandSave, test.prefix+"s")
			requirePrimaryBinding(t, resolver, scopeGlobal, commandPreferences, test.prefix+"p")
			requirePrimaryBinding(t, resolver, scopeGlobal, commandSidebar, test.prefix+"b")
			requirePrimaryBinding(t, resolver, scopeCanvas, commandExpand, test.prefix+"a")
		})
	}
}

func TestApplicationBindingsResolveDvorakLogicalKeys(t *testing.T) {
	t.Parallel()

	profiles := []struct {
		name    string
		profile chrome.KeyProfile
	}{
		{name: "standard", profile: chrome.ProfileStandard},
		{name: "mac", profile: chrome.ProfileMac},
	}
	scopes := make([]chrome.ScopeID, 0)
	seenScopes := make(map[chrome.ScopeID]bool)
	for _, binding := range applicationBindings {
		if seenScopes[binding.Scope] {
			continue
		}
		seenScopes[binding.Scope] = true
		scopes = append(scopes, binding.Scope)
	}

	for _, profile := range profiles {
		resolver, err := chrome.NewResolver(applicationBindings)
		require.NoError(t, err)
		resolver.SetProfile(profile.profile)
		resolver.SetKeyDisambiguation(true)

		for _, scope := range scopes {
			for _, binding := range resolver.Effective([]chrome.ScopeID{scope}) {
				t.Run(
					fmt.Sprintf("%s/%s/%s", profile.name, scope, binding.Chord),
					func(t *testing.T) {
						message, ok := resolver.ResolveKey(
							dvorakKeyPress(t, binding.Chord),
							[]chrome.ScopeID{scope},
							false,
						)
						require.True(t, ok)
						require.Equal(t, binding.Command, message.Command)
					},
				)
			}
		}
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

func dvorakKeyPress(t testing.TB, chord chrome.Chord) tea.KeyPressMsg {
	t.Helper()

	modifiers, name := testChordParts(t, chord)
	if runes := []rune(name); len(runes) != 1 {
		return tea.KeyPressMsg(tea.Key{
			Code: tea.KeyExtended,
			Text: name,
			Mod:  modifiers,
		})
	}
	code, output := logicalKey(t, name, &modifiers)
	baseCode := dvorakBaseCode(code)
	key := tea.Key{
		Code:     code,
		BaseCode: baseCode,
		Mod:      modifiers,
	}
	if modifiers&(tea.ModCtrl|tea.ModAlt|tea.ModMeta|tea.ModHyper|tea.ModSuper) == 0 {
		key.Text = output
	}
	if modifiers.Contains(tea.ModShift) {
		key.ShiftedCode = qwertyShiftedCode(baseCode)
		key.Text = string(key.ShiftedCode)
	}
	return tea.KeyPressMsg(key)
}

func testChordParts(t testing.TB, chord chrome.Chord) (tea.KeyMod, string) {
	t.Helper()

	parts := strings.Split(string(chord), "+")
	var modifiers tea.KeyMod
	for _, modifier := range parts[:len(parts)-1] {
		switch modifier {
		case "alt":
			modifiers |= tea.ModAlt
		case "ctrl":
			modifiers |= tea.ModCtrl
		case "hyper":
			modifiers |= tea.ModHyper
		case "meta":
			modifiers |= tea.ModMeta
		case "shift":
			modifiers |= tea.ModShift
		case "super":
			modifiers |= tea.ModSuper
		default:
			require.FailNow(t, "unsupported key modifier", modifier)
		}
	}
	return modifiers, parts[len(parts)-1]
}

func logicalKey(t testing.TB, name string, modifiers *tea.KeyMod) (rune, string) {
	t.Helper()

	switch name {
	case "?":
		*modifiers |= tea.ModShift
		return '/', name
	case "{":
		*modifiers |= tea.ModShift
		return '[', name
	case "}":
		*modifiers |= tea.ModShift
		return ']', name
	}
	runes := []rune(name)
	require.Len(t, runes, 1, "unsupported key %q", name)
	return runes[0], name
}

func dvorakBaseCode(logical rune) rune {
	const (
		qwerty = "`1234567890-=qwertyuiop[]\\asdfghjkl;'zxcvbnm,./"
		dvorak = "`1234567890[]',.pyfgcrl/=\\aoeuidhtns-;qjkxbmwvz"
	)
	qwertyRunes := []rune(qwerty)
	index := strings.IndexRune(dvorak, unicode.ToLower(logical))
	if index == -1 {
		if logical == 'x' {
			return 'q'
		}
		return 'x'
	}
	return qwertyRunes[index]
}

func qwertyShiftedCode(code rune) rune {
	if unicode.IsLetter(code) {
		return unicode.ToUpper(code)
	}
	shifted := map[rune]rune{
		'`':  '~',
		'1':  '!',
		'2':  '@',
		'3':  '#',
		'4':  '$',
		'5':  '%',
		'6':  '^',
		'7':  '&',
		'8':  '*',
		'9':  '(',
		'0':  ')',
		'-':  '_',
		'=':  '+',
		'[':  '{',
		']':  '}',
		'\\': '|',
		';':  ':',
		'\'': '"',
		',':  '<',
		'.':  '>',
		'/':  '?',
	}
	if result, ok := shifted[code]; ok {
		return result
	}
	return 'X'
}
