package chrome

import (
	"errors"
	"testing"

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
		{Scope: testScopeGlobal, Chords: []Chord{Primary("s"), "super+c"}, Command: "save"},
	})
	require.NoError(t, err)
	resolver.SetProfile(ProfileStandard)
	require.Equal(t, []EffectiveBinding{
		{Scope: testScopeGlobal, Chord: "ctrl+s", Command: "save"},
	}, resolver.Effective([]ScopeID{testScopeGlobal}))
}
