package chrome

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

const (
	testScopeField  ScopeID   = "field"
	testScopeCanvas ScopeID   = "canvas"
	testScopeGlobal ScopeID   = "global"
	testFocusLast   FocusID   = "last"
	testPreferences CommandID = "preferences"
)

func TestResolverProjectsScopesProfilesAndTextPrecedence(t *testing.T) {
	t.Parallel()

	resolver, err := NewResolver([]Binding{
		{Scope: testScopeField, Chords: Keys("q"), Command: "type-q", Label: "type"},
		{Scope: testScopeCanvas, Chords: Keys("q"), Command: "quit", Label: "quit"},
		{Scope: testScopeGlobal, Chords: []Chord{Primary(",")}, Command: testPreferences, Label: string(testPreferences)},
	})
	require.NoError(t, err)
	resolver.SetProfile(ProfileMac)
	resolver.SetSuperAvailable(true)

	message, ok := resolver.Resolve("q", []ScopeID{testScopeField, testScopeCanvas}, false)
	require.True(t, ok)
	require.Equal(t, CommandID("type-q"), message.Command)
	_, ok = resolver.Resolve("q", []ScopeID{testScopeField, testScopeCanvas}, true)
	require.False(t, ok)
	message, ok = resolver.Resolve("super+,", []ScopeID{testScopeField, testScopeGlobal}, true)
	require.True(t, ok)
	require.Equal(t, testPreferences, message.Command)

	effective := resolver.Effective([]ScopeID{testScopeField, testScopeCanvas, testScopeGlobal})
	require.Equal(t, []EffectiveBinding{
		{Scope: testScopeField, Chord: "q", Command: "type-q", Label: "type"},
		{Scope: testScopeGlobal, Chord: "super+,", Command: testPreferences, Label: string(testPreferences)},
	}, effective)
}

func TestResolverReportsSameScopeCollisions(t *testing.T) {
	t.Parallel()

	_, err := NewResolver([]Binding{
		{Scope: testScopeCanvas, Chords: Keys("cmd+c"), Command: "copy"},
		{Scope: testScopeCanvas, Chords: Keys("super+c"), Command: "other"},
	})
	require.ErrorIs(t, err, ErrBindingCollision)
	var collision CollisionError
	require.True(t, errors.As(err, &collision))
	require.Equal(t, Chord("super+c"), collision.Chord)
}

func TestResolverStandardProfileAndUnavailableSuper(t *testing.T) {
	t.Parallel()

	resolver, err := NewResolver([]Binding{
		{Scope: testScopeGlobal, Chords: []Chord{Primary("s"), "super+c"}, Command: CommandID(testFormSave)},
	})
	require.NoError(t, err)
	resolver.SetProfile(ProfileStandard)
	require.Equal(t, []EffectiveBinding{
		{Scope: testScopeGlobal, Chord: "ctrl+s", Command: "save"},
	}, resolver.Effective([]ScopeID{testScopeGlobal}))
}

func TestResolveControlIntent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		key       tea.Key
		textEntry bool
		want      ControlIntent
	}{
		{name: "left", key: tea.Key{Code: tea.KeyLeft}, want: NavigateLeft},
		{name: "h", key: tea.Key{Code: 'h', Text: "h"}, want: NavigateLeft},
		{name: "right", key: tea.Key{Code: tea.KeyRight}, want: NavigateRight},
		{name: "l", key: tea.Key{Code: 'l', Text: "l"}, want: NavigateRight},
		{name: "up", key: tea.Key{Code: tea.KeyUp}, want: FocusPrevious},
		{name: "shift tab", key: tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}, want: FocusPrevious},
		{name: "k", key: tea.Key{Code: 'k', Text: "k"}, want: FocusPrevious},
		{name: "down", key: tea.Key{Code: tea.KeyDown}, want: FocusNext},
		{name: "tab", key: tea.Key{Code: tea.KeyTab}, want: FocusNext},
		{name: "j", key: tea.Key{Code: 'j', Text: "j"}, want: FocusNext},
		{name: "enter", key: tea.Key{Code: tea.KeyEnter}, want: Activate},
		{
			name: "text h", key: tea.Key{Code: 'h', Text: "h"},
			textEntry: true, want: NoControlIntent,
		},
		{
			name: "text l", key: tea.Key{Code: 'l', Text: "l"},
			textEntry: true, want: NoControlIntent,
		},
		{
			name: "text left", key: tea.Key{Code: tea.KeyLeft},
			textEntry: true, want: NavigateLeft,
		},
		{
			name: "modified h", key: tea.Key{Code: 'h', Text: "h", Mod: tea.ModAlt},
			want: NoControlIntent,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(
				t,
				test.want,
				ResolveControlIntent(tea.KeyPressMsg(test.key), test.textEntry),
			)
		})
	}
}
