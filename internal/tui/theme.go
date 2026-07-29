package tui

import (
	"charm.land/lipgloss/v2"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/internal/tui/directorypicker"
	modalview "github.com/coxley/dg/internal/tui/modal"
	"github.com/coxley/dg/internal/tui/nav"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
)

// Theme contains every terminal-facing visual style.
type Theme struct {
	Dark            bool
	Canvas          canvasview.Styles
	Nav             nav.Styles
	Modal           modalview.Styles
	HelpKey         lipgloss.Style
	HelpDescription lipgloss.Style
	Button          lipgloss.Style
	FocusedButton   lipgloss.Style
	SidebarHeader   lipgloss.Style
	SidebarItem     lipgloss.Style
	SidebarFocused  lipgloss.Style
	SidebarFooter   lipgloss.Style
}

type sidebarStyles struct {
	Header      lipgloss.Style
	Item        lipgloss.Style
	FocusedItem lipgloss.Style
	Footer      lipgloss.Style
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

	button := lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder(), true).MarginRight(1)
	focusedButton := button.Border(lipgloss.DoubleBorder(), true)
	return Theme{
		Dark: dark,
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
		HelpKey:         tabActive.Padding(0),
		HelpDescription: tab.Padding(0),
		Button:          button,
		FocusedButton:   focusedButton,
		SidebarHeader:   tabActive.Padding(0, 1),
		SidebarItem:     tab.Padding(0, 1),
		SidebarFocused: lipgloss.NewStyle().
			Background(focus).
			Foreground(text).
			Padding(0, 1),
		SidebarFooter: tab.Foreground(muted),
	}
}

func (t Theme) formStyles() chrome.FormStyles {
	return chrome.FormStyles{
		Label:          t.HelpDescription,
		FocusedLabel:   t.HelpKey,
		Value:          lipgloss.NewStyle(),
		FocusedValue:   t.HelpKey,
		ActiveValue:    t.Nav.Active,
		Action:         t.Button,
		SelectedAction: t.FocusedButton,
		TextInput: chrome.TextInputStyles{
			Text:        lipgloss.NewStyle(),
			FocusedText: lipgloss.NewStyle().Bold(true),
			Placeholder: lipgloss.NewStyle().Faint(true),
			Cursor:      lipgloss.NewStyle().Reverse(true),
		},
	}
}

func (t Theme) preferenceStyles() preferencesview.Styles {
	return preferencesview.Styles{
		Picker: directorypicker.Styles{Dark: t.Dark},
		Form:   t.formStyles(),
	}
}

func (t Theme) sidebarStyles() sidebarStyles {
	return sidebarStyles{
		Header:      t.SidebarHeader,
		Item:        t.SidebarItem,
		FocusedItem: t.SidebarFocused,
		Footer:      t.SidebarFooter,
	}
}
