package chrome

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestResolveTextEditIntent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  tea.Key
		want TextEditIntent
	}{
		{name: "left arrow", key: tea.Key{Code: tea.KeyLeft}, want: TextEditBackward},
		{name: "control-b", key: tea.Key{Code: 'b', Mod: tea.ModCtrl}, want: TextEditBackward},
		{name: "right arrow", key: tea.Key{Code: tea.KeyRight}, want: TextEditForward},
		{name: "control-f", key: tea.Key{Code: 'f', Mod: tea.ModCtrl}, want: TextEditForward},
		{name: "control-a", key: tea.Key{Code: 'a', Mod: tea.ModCtrl}, want: TextEditLineStart},
		{name: "control-e", key: tea.Key{Code: 'e', Mod: tea.ModCtrl}, want: TextEditLineEnd},
		{name: "alt-b", key: tea.Key{Code: 'b', Mod: tea.ModAlt}, want: TextEditWordBackward},
		{name: "alt-f", key: tea.Key{Code: 'f', Mod: tea.ModAlt}, want: TextEditWordForward},
		{name: "control-h", key: tea.Key{Code: 'h', Mod: tea.ModCtrl}, want: TextEditDeleteBackward},
		{name: "control-d", key: tea.Key{Code: 'd', Mod: tea.ModCtrl}, want: TextEditDeleteForward},
		{name: "control-w", key: tea.Key{Code: 'w', Mod: tea.ModCtrl}, want: TextEditDeleteWordBackward},
		{name: "alt-backspace", key: tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}, want: TextEditDeleteWordBackward},
		{name: "control-u", key: tea.Key{Code: 'u', Mod: tea.ModCtrl}, want: TextEditDeleteToLineStart},
		{name: "control-k", key: tea.Key{Code: 'k', Mod: tea.ModCtrl}, want: TextEditDeleteToLineEnd},
		{name: "unhandled", key: tea.Key{Code: 'x', Mod: tea.ModCtrl}, want: TextEditNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, ResolveTextEditIntent(tea.KeyPressMsg(test.key)))
		})
	}
}
