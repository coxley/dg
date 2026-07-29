package chrome

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFocusRegistryRestoresStableTargets(t *testing.T) {
	t.Parallel()

	registry := NewFocusRegistry()
	registry.Register(testScopeCanvas, []FocusTarget{{ID: FocusID(testScopeCanvas), Enabled: true}})
	registry.Register("panel", []FocusTarget{
		{ID: "first", Enabled: true},
		{ID: "disabled"},
		{ID: testFocusLast, Enabled: true},
	})
	require.Equal(t, FocusID(testScopeCanvas), registry.Open(testScopeCanvas))
	require.Equal(t, FocusID("first"), registry.Open("panel"))
	require.Equal(t, testFocusLast, registry.Move(1))
	require.Equal(t, FocusID(testScopeCanvas), registry.Close(testScopeCanvas))

	registry.Register("panel", []FocusTarget{
		{ID: "inserted", Enabled: true},
		{ID: testFocusLast, Enabled: true},
	})
	require.Equal(t, testFocusLast, registry.Open("panel"))
	registry.Register("panel", []FocusTarget{{ID: "inserted", Enabled: true}})
	_, focus := registry.Current()
	require.Equal(t, FocusID("inserted"), focus)
}

func TestFocusRegistryRevealsFocusedRect(t *testing.T) {
	t.Parallel()

	viewport := NewViewport("body")
	viewport.SetContent([]string{"0", "1", "2", "3", "4"})
	viewport.SetBounds(Rect{Width: 4, Height: 2})
	registry := NewFocusRegistry()
	registry.Register("form", []FocusTarget{
		{ID: testFocusLast, Rect: Rect{Y: 4, Width: 1, Height: 1}, Enabled: true},
	})
	registry.Open("form")
	registry.Reveal(viewport)
	require.Equal(t, 3, viewport.Plan().Offset.Y)
}
