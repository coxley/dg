package preferences

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestKeyboardSubmitsSaveByDefault(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
	for range 8 {
		_, _ = model.Update(huh.NextField())
	}
	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	action, completed := model.Completed()
	require.True(t, completed)
	require.Equal(t, ActionSave, action)
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

func TestScrollMovesFocusWithoutActivatingFields(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 5, testStyles())
	for range 20 {
		_, _ = model.Update(ScrollMsg{Delta: 1})
	}

	_, completed := model.Completed()
	require.False(t, completed)
	require.False(t, model.DirectoryOpen())
	require.Contains(t, model.View().Content, "Save as Defaults")
}

func TestDirectoryBrowserOpensOnlyOnExplicitActivation(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
	require.False(t, model.DirectoryOpen())
	require.Contains(t, model.View().Content, "Preferred comments")

	for range 6 {
		_, _ = model.Update(huh.NextField())
	}
	require.False(t, model.DirectoryOpen())

	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	require.True(t, model.DirectoryOpen())
	require.NotContains(t, model.View().Content, "Preferred comments")

	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	require.False(t, model.DirectoryOpen())
	require.Contains(t, model.View().Content, "Preferred comments")
}

func TestVimKeysNavigateAndEditForm(t *testing.T) {
	t.Parallel()

	router := layout.DefaultRouter()
	model := New(Value{Router: router}, 64, 20, testStyles())
	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	require.Equal(t, router.Costs.Step+1, model.Value().Router.Costs.Step)

	_, command := model.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	require.NotNil(t, command)
	_, _ = model.Update(command())
	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	require.Equal(t, router.Costs.SharedStep-1, model.Value().Router.Costs.SharedStep)

	_, command = model.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	require.NotNil(t, command)
	_, _ = model.Update(command())
	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	require.Equal(t, router.Costs.Step, model.Value().Router.Costs.Step)
}

func TestActionClickSubmitsSelectedAction(t *testing.T) {
	t.Parallel()

	model := New(Value{Router: layout.DefaultRouter()}, 64, 20, testStyles())
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	for y, line := range lines {
		x := strings.Index(line, "Save as Defaults")
		if x < 0 {
			continue
		}
		_, _ = model.Update(ClickMsg{X: x + 1, Y: y})
		action, completed := model.Completed()
		require.True(t, completed)
		require.Equal(t, ActionSaveDefaults, action)
		return
	}
	require.Fail(t, "action row not rendered")
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
		Action:         lipgloss.NewStyle().Padding(0, 1),
		SelectedAction: lipgloss.NewStyle().Bold(true).Padding(0, 1),
	}
}
