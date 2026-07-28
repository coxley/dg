package preferences

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/internal/tui/numinput"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestModelUpdatesRouterCost(t *testing.T) {
	t.Parallel()

	router := layout.DefaultRouter()
	model := New(Value{Router: router}, 64, 10, testStyles())
	next, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))

	require.Same(t, model, next)
	require.NotNil(t, command)
	require.Equal(t, router.Costs.Step+1, model.Value().Router.Costs.Step)
	require.Equal(t, 1, model.FieldFlash(0))
}

func TestModelReachesCompletion(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 10, testStyles())
	for range 9 {
		_, _ = model.Update(huh.NextField())
	}
	require.Contains(t, model.View().Content, "Save")
	require.Contains(t, model.View().Content, "Cancel")
}

func TestModelExpandsFromViewportToNaturalHeight(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 5, testStyles())
	require.NotContains(t, model.View().Content, "Save")

	model.SetHeight(100)
	require.Contains(t, model.View().Content, "Save")
	require.Contains(t, model.View().Content, "Cancel")
}

func TestModelImplementsTeaModel(t *testing.T) {
	t.Parallel()

	var model tea.Model = New(
		Value{Router: layout.DefaultRouter()},
		64,
		10,
		testStyles(),
	)
	require.NotEmpty(t, model.View().Content)
}

func TestSelectArrowsNavigateConsistently(t *testing.T) {
	t.Parallel()

	keys := keyMap()
	up := tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	down := tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	left := tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft})
	right := tea.KeyPressMsg(tea.Key{Code: tea.KeyRight})

	require.True(t, key.Matches(up, keys.Select.Prev))
	require.False(t, key.Matches(up, keys.Select.Up))
	require.True(t, key.Matches(down, keys.Select.Next))
	require.False(t, key.Matches(down, keys.Select.Down))
	require.True(t, key.Matches(left, keys.Select.Up))
	require.True(t, key.Matches(right, keys.Select.Down))
}

func testStyles() Styles {
	return Styles{
		Form: huh.ThemeFunc(func(bool) *huh.Styles {
			return huh.ThemeCharm(true)
		}),
		NumInput: numinput.Styles{
			Title:        lipgloss.NewStyle(),
			FocusedTitle: lipgloss.NewStyle().Bold(true),
			Button:       lipgloss.NewStyle(),
			ActiveButton: lipgloss.NewStyle().Bold(true),
		},
	}
}
