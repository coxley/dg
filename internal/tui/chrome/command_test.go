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
	testBack        CommandID = "back"
	testHelp        CommandID = "show-help"
	testPreferences CommandID = "preferences"
)

func TestResolverProjectsScopesProfilesAndTextPrecedence(t *testing.T) {
	t.Parallel()

	resolver, err := NewResolver([]Binding{
		{Scope: testScopeField, Chords: Keys("q"), Command: "type-q", Label: "type"},
		{Scope: testScopeCanvas, Chords: Keys("q"), Command: "quit", Label: "quit"},
		{Scope: testScopeGlobal, Chords: []Chord{Primary("p")}, Command: testPreferences, Label: string(testPreferences)},
	})
	require.NoError(t, err)
	resolver.SetProfile(ProfileMac)
	resolver.SetKeyDisambiguation(true)

	message, ok := resolver.Resolve("q", []ScopeID{testScopeField, testScopeCanvas}, false)
	require.True(t, ok)
	require.Equal(t, CommandID("type-q"), message.Command)
	_, ok = resolver.Resolve("q", []ScopeID{testScopeField, testScopeCanvas}, true)
	require.False(t, ok)
	message, ok = resolver.Resolve("super+p", []ScopeID{testScopeField, testScopeGlobal}, true)
	require.True(t, ok)
	require.Equal(t, testPreferences, message.Command)

	effective := resolver.Effective([]ScopeID{testScopeField, testScopeCanvas, testScopeGlobal})
	require.Equal(t, []EffectiveBinding{
		{Scope: testScopeField, Chord: "q", Command: "type-q", Label: "type"},
		{Scope: testScopeGlobal, Chord: "super+p", Command: testPreferences, Label: string(testPreferences)},
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

func TestResolverReportsProfileProjectedCollisions(t *testing.T) {
	t.Parallel()

	_, err := NewResolver([]Binding{
		{Scope: testScopeCanvas, Chords: []Chord{Primary("s")}, Command: "primary-save"},
		{Scope: testScopeCanvas, Chords: Keys("ctrl+s"), Command: "control-save"},
	})

	require.ErrorIs(t, err, ErrBindingCollision)
	var collision CollisionError
	require.True(t, errors.As(err, &collision))
	require.Equal(t, Chord("ctrl+s"), collision.Chord)
}

func TestResolverReportsShiftAliasCollisions(t *testing.T) {
	t.Parallel()

	_, err := NewResolver([]Binding{
		{Scope: testScopeCanvas, Chords: Keys("{"), Command: testBack},
		{Scope: testScopeCanvas, Chords: Keys("shift+["), Command: "other"},
	})

	require.ErrorIs(t, err, ErrBindingCollision)
	var collision CollisionError
	require.True(t, errors.As(err, &collision))
	require.Equal(t, Chord("{"), collision.Chord)
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

	message, ok := resolver.Resolve(
		"super+c",
		[]ScopeID{testScopeGlobal},
		false,
	)
	require.True(t, ok)
	require.Equal(t, CommandID(testFormSave), message.Command)
}

func TestResolverAdvertisesAmbiguousChordsOnlyWithKeyDisambiguation(t *testing.T) {
	t.Parallel()

	resolver, err := NewResolver([]Binding{
		{
			Scope:   testScopeGlobal,
			Chords:  Keys(escapeChord, "ctrl+enter", "ctrl+y", "ctrl+shift+z"),
			Command: testBack,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []EffectiveBinding{
		{Scope: testScopeGlobal, Chord: escapeChord, Command: testBack},
		{Scope: testScopeGlobal, Chord: "ctrl+y", Command: testBack},
	}, resolver.Effective([]ScopeID{testScopeGlobal}))

	resolver.SetKeyDisambiguation(true)
	require.Equal(t, []EffectiveBinding{
		{Scope: testScopeGlobal, Chord: escapeChord, Command: testBack},
		{Scope: testScopeGlobal, Chord: "ctrl+enter", Command: testBack},
		{Scope: testScopeGlobal, Chord: "ctrl+y", Command: testBack},
		{Scope: testScopeGlobal, Chord: "ctrl+shift+z", Command: testBack},
	}, resolver.Effective([]ScopeID{testScopeGlobal}))
}

func TestResolverExecutesObservedMacPrimaryBeforeCapabilityReport(t *testing.T) {
	t.Parallel()

	resolver, err := NewResolver([]Binding{
		{
			Scope: testScopeGlobal, Chords: []Chord{Primary("s")},
			Command: CommandID(testFormSave),
		},
	})
	require.NoError(t, err)
	resolver.SetProfile(ProfileMac)
	require.Empty(t, resolver.Effective([]ScopeID{testScopeGlobal}))

	message, ok := resolver.Resolve(
		"super+s",
		[]ScopeID{testScopeGlobal},
		false,
	)
	require.True(t, ok)
	require.Equal(t, CommandID(testFormSave), message.Command)
}

func TestResolveKeyNormalizesShiftedInput(t *testing.T) {
	t.Parallel()

	resolver, err := NewResolver([]Binding{
		{Scope: testScopeCanvas, Chords: Keys("?"), Command: testHelp},
		{Scope: testScopeCanvas, Chords: Keys("{", "shift+["), Command: testBack},
		{Scope: testScopeCanvas, Chords: Keys("shift+a"), Command: "start-arrow"},
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		key  tea.Key
		want CommandID
	}{
		{
			name: "shifted question",
			key:  tea.Key{Code: '/', ShiftedCode: '?', Text: "?", Mod: tea.ModShift},
			want: testHelp,
		},
		{
			name: "shifted question without text",
			key:  tea.Key{Code: '/', Mod: tea.ModShift},
			want: testHelp,
		},
		{
			name: "shifted brace text",
			key:  tea.Key{Code: '[', ShiftedCode: '{', Text: "{", Mod: tea.ModShift},
			want: testBack,
		},
		{
			name: "shifted brace without text",
			key:  tea.Key{Code: '[', Mod: tea.ModShift},
			want: testBack,
		},
		{
			name: "shifted letter",
			key:  tea.Key{Code: 'a', ShiftedCode: 'A', Text: "A", Mod: tea.ModShift},
			want: "start-arrow",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			message, ok := resolver.ResolveKey(
				tea.KeyPressMsg(test.key),
				[]ScopeID{testScopeCanvas},
				false,
			)
			require.True(t, ok)
			require.Equal(t, test.want, message.Command)
		})
	}
}

func TestDisplayChordUsesPresentationVocabulary(t *testing.T) {
	t.Parallel()

	require.Equal(t, "shift+cmd+s", DisplayChord("shift+super+s", VocabularyMac))
	require.Equal(t, "shift+super+s", DisplayChord("shift+super+s", VocabularyStandard))
	require.Equal(t, "ctrl+s", DisplayChord("ctrl+s", VocabularyMac))
	require.Equal(t, VocabularyMac, VocabularyForProfile(ProfileMac))
	require.Equal(t, VocabularyStandard, VocabularyForProfile(ProfileStandard))
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
