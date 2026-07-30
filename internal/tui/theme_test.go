package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
	tint "github.com/lrstanley/bubbletint/v2"
	"github.com/stretchr/testify/require"
)

func TestThemeUsesRegisteredTintForTerminalBackground(t *testing.T) {
	t.Parallel()

	dark := themeForTints(true, "dracula_plus", defaultLightTint.ID)
	light := themeForTints(false, defaultDarkTint.ID, "github")

	require.Equal(t, "dracula_plus", dark.TintID)
	require.Equal(t, "github", light.TintID)
	require.True(t, dark.Dark)
	require.False(t, light.Dark)
}

func TestThemeFallsBackToRegisteredDefaultTint(t *testing.T) {
	t.Parallel()

	dark := themeForTints(true, "missing-dark", "missing-light")
	light := themeForTints(false, "missing-dark", "missing-light")

	require.Equal(t, defaultDarkTint.ID, dark.TintID)
	require.Equal(t, defaultLightTint.ID, light.TintID)
}

func TestThemeTintOptionsComeFromIndependentDefaultLists(t *testing.T) {
	t.Parallel()

	dark := tintOptions(darkTints)
	light := tintOptions(lightTints)

	require.Len(t, dark, len(tint.DefaultDarkTints()))
	require.Len(t, light, len(tint.DefaultLightTints()))
	require.NotEqual(t, len(dark), len(light))
	require.Contains(t, dark, preferencesview.TintOption{
		ID: defaultDarkTint.ID, Label: defaultDarkTint.DisplayName,
	})
	require.Contains(t, light, preferencesview.TintOption{
		ID: defaultLightTint.ID, Label: defaultLightTint.DisplayName,
	})
}

func TestHelpRendersEveryRootOwnedStyle(t *testing.T) {
	t.Parallel()

	styles := helpStyles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()),
		ActiveContainer: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()),
		Header:      lipgloss.NewStyle().Bold(true),
		Key:         lipgloss.NewStyle().Underline(true),
		Description: lipgloss.NewStyle().Italic(true),
		Footer:      lipgloss.NewStyle().Faint(true),
	}
	help := newHelpInspector(styles)
	help.visible = true
	help.setPlan(
		chrome.Rect{Width: 40, Height: 8},
		"canvas",
		[]chrome.EffectiveBinding{{
			Chord: "r",
			Label: "rectangle",
		}},
		chrome.VocabularyStandard,
	)

	rendered := strings.Join(help.lines(), "\n")
	require.Contains(t, rendered, styles.Header.Render("HELP · canvas"))
	require.Contains(t, rendered, styles.Key.Render("r             "))
	require.Contains(t, rendered, styles.Description.Render("rectangle"))
	require.Contains(t, rendered, styles.Footer.Render("? hide · wheel scroll"))
	require.True(t, strings.HasPrefix(ansi.Strip(rendered), "┌"))

	help.dragging = true
	rendered = strings.Join(help.lines(), "\n")
	require.True(t, strings.HasPrefix(ansi.Strip(rendered), "╔"))
}
