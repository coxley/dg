package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
	"github.com/lrstanley/bubbletint/v2"
	"github.com/stretchr/testify/require"
)

func TestThemeUsesRegisteredTintForTerminalBackground(t *testing.T) {
	t.Parallel()

	dark := themeForTints(true, "dracula_plus", defaultLightTint)
	light := themeForTints(false, defaultDarkTint, "github")

	require.Equal(t, "dracula_plus", dark.TintID)
	require.Equal(t, "github", light.TintID)
	require.True(t, dark.Dark)
	require.False(t, light.Dark)
}

func TestThemeFallsBackToRegisteredDefaultTint(t *testing.T) {
	t.Parallel()

	dark := themeForTints(true, "missing-dark", "missing-light")
	light := themeForTints(false, "missing-dark", "missing-light")

	require.Equal(t, defaultDarkTint, dark.TintID)
	require.Equal(t, defaultLightTint, light.TintID)
}

func TestThemeTintOptionsComeFromIndependentDefaultLists(t *testing.T) {
	t.Parallel()

	dark := tintOptions(darkTints)
	light := tintOptions(lightTints)

	require.Len(t, dark, len(tint.DefaultDarkTints()))
	require.Len(t, light, len(tint.DefaultLightTints()))
	require.NotEqual(t, len(dark), len(light))
	require.Contains(t, dark, preferencesview.TintOption{
		ID: defaultDarkTint, Label: "Builtin Dark",
	})
	require.Contains(t, light, preferencesview.TintOption{
		ID: defaultLightTint, Label: "Builtin Light",
	})
}

func TestThemeLeavesChromeBackgroundUnpainted(t *testing.T) {
	t.Parallel()

	theme := themeForTints(true, defaultDarkTint, defaultLightTint)

	require.IsType(t, lipgloss.NoColor{}, theme.Nav.Container.GetBackground())
	require.IsType(t, lipgloss.NoColor{}, theme.Modal.Container.GetBackground())
	require.IsType(t, lipgloss.NoColor{}, theme.HelpDescription.GetBackground())
}
