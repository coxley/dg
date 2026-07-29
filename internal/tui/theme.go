package tui

import (
	"charm.land/lipgloss/v2"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/internal/tui/directorypicker"
	modalview "github.com/coxley/dg/internal/tui/modal"
	"github.com/coxley/dg/internal/tui/nav"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
	tint "github.com/lrstanley/bubbletint/v2"
)

const (
	defaultDarkTint  = "builtin_dark"
	defaultLightTint = "builtin_light"
)

var (
	darkTints  = tint.NewRegistry(tint.TintBuiltinDark, tint.DefaultDarkTints()...)
	lightTints = tint.NewRegistry(tint.TintBuiltinLight, tint.DefaultLightTints()...)
)

// Theme contains every terminal-facing visual style.
type Theme struct {
	Dark            bool
	TintID          string
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
	return themeForTints(dark, defaultDarkTint, defaultLightTint)
}

func themeForTints(dark bool, darkID, lightID string) Theme {
	selected := registeredTint(dark, darkID, lightID)
	focus := firstTintColor(selected.SelectionBg, selected.BrightBlue, selected.Blue)
	text := firstTintColor(selected.Fg, selected.White, selected.BrightWhite)
	muted := firstTintColor(selected.BrightBlack, selected.Black, selected.Fg)
	port := firstTintColor(selected.BrightGreen, selected.Green, selected.Fg)
	alert := firstTintColor(selected.BrightRed, selected.Red, selected.Fg)
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

	button := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), true).
		MarginRight(1)
	focusedButton := button.Border(lipgloss.DoubleBorder(), true)
	return Theme{
		Dark:   dark,
		TintID: selected.ID,
		Canvas: canvasview.Styles{
			Selection: lipgloss.NewStyle().
				Background(focus).
				Foreground(text),
			Port: lipgloss.NewStyle().
				Foreground(port),
			Error: lipgloss.NewStyle().
				Foreground(alert),
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

func registeredTint(dark bool, darkID, lightID string) *tint.Tint {
	registry, id := lightTints, lightID
	if dark {
		registry, id = darkTints, darkID
	}
	if selected, ok := registry.GetTint(id); ok {
		return selected
	}
	if dark {
		selected, _ := registry.GetTint(defaultDarkTint)
		return selected
	}
	selected, _ := registry.GetTint(defaultLightTint)
	return selected
}

func firstTintColor(colors ...*tint.Color) *tint.Color {
	for _, color := range colors {
		if color != nil {
			return color
		}
	}
	return &tint.Color{A: 0xff}
}

func tintOptions(registry *tint.Registry) []preferencesview.TintOption {
	tints := registry.Tints()
	options := make([]preferencesview.TintOption, len(tints))
	for i, registered := range tints {
		options[i] = preferencesview.TintOption{
			ID:    registered.ID,
			Label: registered.DisplayName,
		}
	}
	return options
}

func normalizeTintIDs(darkID, lightID string) (string, string) {
	return registeredTint(true, darkID, lightID).ID,
		registeredTint(false, darkID, lightID).ID
}

func (t Theme) formStyles() chrome.FormStyles {
	return chrome.FormStyles{
		Label:        t.HelpDescription,
		FocusedLabel: t.HelpKey,
		Value:        lipgloss.NewStyle(),
		FocusedValue: t.HelpKey,
		ActiveValue:  t.Nav.Active,
		Buttons: chrome.ButtonListStyles{
			Button:        t.Button,
			FocusedButton: t.FocusedButton,
		},
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
