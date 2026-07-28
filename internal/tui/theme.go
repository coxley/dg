package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	modalview "github.com/coxley/dg/internal/tui/modal"
	"github.com/coxley/dg/internal/tui/nav"
)

// Theme contains every terminal-facing visual style.
type Theme struct {
	Canvas            canvasview.Styles
	Nav               nav.Styles
	Modal             modalview.Styles
	HelpKey           lipgloss.Style
	HelpDescription   lipgloss.Style
	StepperTitle      lipgloss.Style
	StepperTitleFocus lipgloss.Style
	Button            lipgloss.Style
	ButtonActive      lipgloss.Style
	FormSeparator     lipgloss.Style
	FormField         lipgloss.Style
}

// DefaultTheme returns the editor's default terminal theme.
func DefaultTheme(dark bool) Theme {
	lightDark := lipgloss.LightDark(dark)
	focus := lightDark(lipgloss.Color("153"), lipgloss.Color("24"))
	text := lightDark(lipgloss.Color("16"), lipgloss.Color("231"))
	muted := lightDark(lipgloss.Color("240"), lipgloss.Color("245"))
	tab := lipgloss.NewStyle().
		Faint(true).
		Padding(0, 1)
	tabActive := lipgloss.NewStyle().
		Bold(true).
		Underline(true).
		Padding(0, 1)
	toolbar := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 1)
	modal := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1)
	noticeModal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder())
	modalBody := lipgloss.NewStyle().
		PaddingTop(1)
	return Theme{
		Canvas: canvasview.Styles{
			Selection: lipgloss.NewStyle().
				Background(focus).
				Foreground(text),
			Port: lipgloss.NewStyle().
				Foreground(lipgloss.Color("46")),
			Error: lipgloss.NewStyle().
				Foreground(lipgloss.Color("1")),
		},
		Nav: nav.Styles{
			Container: toolbar,
			Active: lipgloss.NewStyle().
				Background(focus).
				Foreground(text),
			Hover: lipgloss.NewStyle().
				Foreground(muted).
				Underline(true),
		},
		Modal: modalview.Styles{
			Container: modal,
			Notice:    noticeModal,
			Body:      modalBody,
			Tab:       tab,
			ActiveTab: tabActive,
		},
		HelpKey:           tabActive.Padding(0),
		HelpDescription:   tab.Padding(0),
		StepperTitle:      tab.Padding(0),
		StepperTitleFocus: tabActive.Padding(0),
		Button: lipgloss.NewStyle().
			Padding(0, 1),
		ButtonActive: lipgloss.NewStyle().
			Background(focus).
			Foreground(text).
			Padding(0, 1),
		FormSeparator: lipgloss.NewStyle().
			SetString("\n"),
		FormField: lipgloss.NewStyle().
			PaddingLeft(1),
	}
}

func (t Theme) helpStyles(dark bool) help.Styles {
	styles := help.DefaultStyles(dark)
	styles.FullKey = t.HelpKey
	styles.FullDesc = t.HelpDescription
	return styles
}

func (t Theme) formTheme() huh.Theme {
	return huh.ThemeFunc(func(dark bool) *huh.Styles {
		styles := huh.ThemeCharm(dark)
		styles.FieldSeparator = t.FormSeparator
		styles.Focused.Base = t.FormField
		styles.Blurred.Base = t.FormField
		styles.Focused.Title = styles.Focused.Title.Bold(true)
		return styles
	})
}
