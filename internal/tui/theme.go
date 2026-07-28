package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// Theme contains every terminal-facing visual style.
type Theme struct {
	Selection     lipgloss.Style
	Port          lipgloss.Style
	Error         lipgloss.Style
	Toolbar       lipgloss.Style
	ToolbarActive lipgloss.Style
	ToolbarHover  lipgloss.Style
	Modal         lipgloss.Style
	Tab           lipgloss.Style
	TabActive     lipgloss.Style
	Button        lipgloss.Style
	ButtonActive  lipgloss.Style
}

// DefaultTheme returns the editor's default terminal theme.
func DefaultTheme(dark bool) Theme {
	lightDark := lipgloss.LightDark(dark)
	focus := lightDark(lipgloss.Color("153"), lipgloss.Color("24"))
	text := lightDark(lipgloss.Color("16"), lipgloss.Color("231"))
	muted := lightDark(lipgloss.Color("240"), lipgloss.Color("245"))
	return Theme{
		Selection: lipgloss.NewStyle().
			Background(focus).
			Foreground(text),
		Port: lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")),
		Toolbar: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 1),
		ToolbarActive: lipgloss.NewStyle().
			Background(focus).
			Foreground(text),
		ToolbarHover: lipgloss.NewStyle().
			Foreground(muted).
			Underline(true),
		Modal: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()),
		Tab: lipgloss.NewStyle().
			Faint(true).
			Padding(0, 1),
		TabActive: lipgloss.NewStyle().
			Bold(true).
			Underline(true).
			Padding(0, 1),
		Button: lipgloss.NewStyle().
			Padding(0, 1),
		ButtonActive: lipgloss.NewStyle().
			Background(focus).
			Foreground(text).
			Padding(0, 1),
	}
}

func (t Theme) helpStyles(dark bool) help.Styles {
	styles := help.DefaultStyles(dark)
	styles.FullKey = t.TabActive.Padding(0)
	styles.FullDesc = t.Tab.Padding(0)
	return styles
}

func (t Theme) formTheme() huh.Theme {
	return huh.ThemeFunc(func(dark bool) *huh.Styles {
		styles := huh.ThemeCharm(dark)
		styles.FieldSeparator = lipgloss.NewStyle().SetString("\n")
		styles.Focused.Base = lipgloss.NewStyle().PaddingLeft(1)
		styles.Blurred.Base = lipgloss.NewStyle().PaddingLeft(1)
		styles.Focused.Title = styles.Focused.Title.Bold(true)
		return styles
	})
}
