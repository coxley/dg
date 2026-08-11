package tui

import (
	"cmp"

	"charm.land/lipgloss/v2"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/internal/tui/directorypicker"
	modalview "github.com/coxley/dg/internal/tui/modal"
	"github.com/coxley/dg/internal/tui/nav"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
	tint "github.com/lrstanley/bubbletint/v2"
)

var (
	defaultDarkTint  = tint.TintDuotoneDark
	defaultLightTint = tint.TintBuiltinLight
	darkTints        = tint.NewRegistry(tint.TintBuiltinDark, tint.DefaultDarkTints()...)
	lightTints       = tint.NewRegistry(tint.TintBuiltinLight, tint.DefaultLightTints()...)
)

// Theme contains every terminal-facing visual style.
type Theme struct {
	Dark          bool
	TintID        string
	Background    lipgloss.Style
	CandidateEdge lipgloss.Style
	Canvas        canvasview.Styles
	Navigation    nav.Styles
	Modal         modalview.Styles
	Confirmation  modalview.ConfirmationStyles
	Help          helpStyles
	Sidebar       sidebarStyles
	Status        statusStyles
	Update        updateStyles
	Preferences   preferencesview.Styles
	Save          saveStyles
	ExportForm    chrome.FormStyles
}

type helpStyles struct {
	Container       lipgloss.Style
	ActiveContainer lipgloss.Style
	Header          lipgloss.Style
	Key             lipgloss.Style
	Description     lipgloss.Style
	Footer          lipgloss.Style
	Scrollbar       chrome.ScrollbarStyles
}

type sidebarStyles struct {
	Container        lipgloss.Style
	FocusedContainer lipgloss.Style
	Header           lipgloss.Style
	Tab              lipgloss.Style
	FocusedTab       lipgloss.Style
	HoveredTab       lipgloss.Style
	ActiveTab        lipgloss.Style
	Item             lipgloss.Style
	FocusedItem      lipgloss.Style
	ActiveItem       lipgloss.Style
	Section          lipgloss.Style
	FocusedSection   lipgloss.Style
	Divider          lipgloss.Style
	ClearDrafts      lipgloss.Style
	Footer           lipgloss.Style
	Scrollbar        chrome.ScrollbarStyles
}

type statusStyles struct {
	Normal lipgloss.Style
	Error  lipgloss.Style
}

type saveStyles struct {
	Form   chrome.FormStyles
	Picker directorypicker.Styles
}

// DefaultTheme returns the editor's default terminal theme.
func DefaultTheme(dark bool) Theme {
	if dark {
		return convertTint(defaultDarkTint)
	}
	return convertTint(defaultLightTint)
}

func themeForTints(dark bool, darkID, lightID string) Theme {
	return convertTint(registeredTint(dark, darkID, lightID))
}

func convertTint(theme *tint.Tint) Theme {
	background := theme.Bg
	if background == nil {
		if theme.Dark {
			background = theme.Bg
		} else {
			background = theme.Bg
		}
	}
	focus := cmp.Or(theme.SelectionBg, theme.BrightBlue, theme.Blue)
	text := cmp.Or(theme.Fg, theme.White, theme.BrightWhite)
	muted := cmp.Or(theme.BrightWhite, theme.Black, theme.Fg)
	port := cmp.Or(theme.BrightGreen, theme.Green, theme.Fg)
	alert := cmp.Or(theme.BrightRed, theme.Red, theme.Fg)
	candidate := cmp.Or(theme.BrightYellow, theme.Yellow, theme.Fg)
	plain := lipgloss.NewStyle()
	tab := lipgloss.NewStyle().
		Faint(true).
		Padding(0, 1)
	tabActive := lipgloss.NewStyle().
		Bold(true).
		Underline(true).
		Padding(0, 1)
	toolbar := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)
	commonBox := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), true)
	modal := commonBox
	noticeModal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Background(focus).
		Foreground(theme.Fg).
		BorderBackground(focus).
		BorderForeground(focus)
	modalBody := lipgloss.NewStyle().
		PaddingTop(1)

	hoverNav := lipgloss.NewStyle().Foreground(text)
	activeNav := hoverNav.Background(focus)

	button := commonBox.MarginRight(1)
	focusedButton := button.Border(lipgloss.DoubleBorder(), true)
	sidebar := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, true, false, false).
		MarginRight(1)
	helpKey := tabActive.Padding(0)
	helpDescription := tab.Padding(0)
	focusedControl := helpKey.Underline(false)
	activeControl := lipgloss.NewStyle().Foreground(theme.Cursor)
	scrollbarTrack := lipgloss.NewStyle().Foreground(muted).Faint(true)
	scrollbarThumb := lipgloss.NewStyle().Foreground(text)
	scrollbar := chrome.ScrollbarStyles{
		Track:        scrollbarTrack,
		Thumb:        scrollbarThumb,
		HoveredTrack: scrollbarTrack,
		HoveredThumb: scrollbarThumb,
		FocusedTrack: scrollbarTrack,
		FocusedThumb: scrollbarThumb,
		ActiveTrack:  scrollbarTrack,
		ActiveThumb:  scrollbarThumb,
	}
	form := chrome.FormStyles{
		Label:          helpDescription,
		HoveredLabel:   helpDescription,
		FocusedLabel:   helpKey.Underline(false),
		AttentionLabel: activeControl,
		Value:          plain,
		HoveredValue:   plain,
		FocusedValue:   focusedControl,
		AttentionValue: activeControl,
		Number: chrome.NumberFieldStyles{
			Value:            plain,
			HoveredValue:     plain,
			FocusedValue:     focusedControl,
			FocusedDecrement: focusedControl,
			ActiveDecrement:  activeControl,
			FocusedIncrement: focusedControl,
			ActiveIncrement:  activeControl,
		},
		Buttons: chrome.ButtonListStyles{
			Button:        button,
			HoveredButton: button,
			FocusedButton: focusedButton,
		},
		TextInput: chrome.TextInputStyles{
			Text:               plain,
			HoveredText:        plain,
			FocusedText:        plain.Bold(true),
			SelectedText:       activeControl,
			Placeholder:        plain.Faint(true),
			HoveredPlaceholder: plain.Faint(true),
			Cursor:             plain.Reverse(true),
		},
	}
	picker := directorypicker.Styles{
		Container:    plain,
		Title:        helpKey,
		Item:         plain.Foreground(port),
		HoveredItem:  plain.Foreground(port),
		SelectedItem: activeControl.Bold(true),
		Empty:        plain.Foreground(muted),
		Error:        plain.Foreground(alert),
	}

	sidebarTabHeader := plain.
		MarginBottom(1).
		BorderForeground(focus).
		BorderBackground(focus).
		Border(lipgloss.NormalBorder(), false, false, true, false)

	return Theme{
		Dark:   theme.Dark,
		TintID: theme.ID,
		Background: lipgloss.NewStyle().
			Background(background).
			Foreground(text),
		CandidateEdge: lipgloss.NewStyle().Foreground(candidate).Bold(true),
		Canvas: canvasview.Styles{
			Selection: lipgloss.NewStyle().
				Background(focus).
				Foreground(text),
			Port: lipgloss.NewStyle().
				Foreground(port),
		},
		Navigation: nav.Styles{
			Container: toolbar,
			Item:      plain,
			Active:    activeNav,
			Hover:     hoverNav,
		},
		Modal: modalview.Styles{
			Container:       modal,
			ActiveContainer: modal,
			Notice:          noticeModal,
			NoticeText:      plain,
			Body:            modalBody,
			Tabs:            sidebarTabHeader,
			Tab:             plain,
			HoveredTab:      hoverNav,
			ActiveTab:       activeNav,
		},
		Confirmation: modalview.ConfirmationStyles{
			Title:   helpKey,
			Message: plain,
			Actions: form,
		},
		Help: helpStyles{
			Container:       commonBox.Border(lipgloss.HiddenBorder(), false),
			ActiveContainer: commonBox.Border(lipgloss.HiddenBorder()),
			Header:          tabActive.Padding(0),
			Key:             helpKey.Underline(false).Faint(true),
			Description:     helpDescription,
			Footer:          tab.Foreground(muted).Padding(0),
			Scrollbar:       scrollbar,
		},
		Sidebar: sidebarStyles{
			Container:        sidebar,
			FocusedContainer: sidebar,
			Header:           sidebarTabHeader,
			Tab:              plain,
			ActiveTab:        activeNav,
			HoveredTab:       hoverNav,
			Item:             tab.Padding(0, 1),
			FocusedItem: activeControl.
				Padding(0, 1),
			ActiveItem: activeControl.Padding(0, 1),
			Section:    tab.Padding(1, 0, 0, 0),
			FocusedSection: activeControl.
				Padding(1, 0, 0, 0),
			Divider:     tab.MarginTop(1),
			ClearDrafts: tab,
			Footer:      tab.Foreground(muted),
			Scrollbar:   scrollbar,
		},
		Status: statusStyles{
			Normal: plain,
			Error:  plain.Foreground(alert),
		},
		Update: updateStyles{
			Normal: plain.Foreground(candidate).
				Border(lipgloss.RoundedBorder()).Padding(0, 1),
			Focused: plain.Foreground(candidate).Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(candidate).Padding(0, 1),
		},
		Preferences: preferencesview.Styles{
			Form:   form,
			Picker: picker,
			Scope:  helpKey.Underline(false),
			Action: plain,
			Mapping: preferencesview.MappingPillStyles{
				Normal: plain.Border(lipgloss.RoundedBorder()).
					BorderForeground(theme.BrightBlack).Padding(0, 1),
				Hovered: plain.Border(lipgloss.RoundedBorder()).
					BorderForeground(text).Padding(0, 1),
				Focused: plain.Border(lipgloss.RoundedBorder()).
					Foreground(text).BorderForeground(text).Bold(true).Padding(0, 1),
				Active: activeControl.Border(lipgloss.RoundedBorder()).
					BorderForeground(focus).Padding(0, 1),
				Empty: plain.Border(lipgloss.RoundedBorder()).
					BorderForeground(theme.BrightBlack).Padding(0, 1),
				EmptyHovered: plain.Faint(true).Border(lipgloss.RoundedBorder()).
					BorderForeground(text).Padding(0, 1),
				Conflict: plain.Foreground(alert).Border(lipgloss.RoundedBorder()).
					BorderForeground(alert).Padding(0, 1),
				ConflictHovered: plain.Foreground(alert).Border(lipgloss.RoundedBorder()).
					BorderForeground(candidate).Padding(0, 1),
				ConflictFocused: plain.Foreground(alert).Border(lipgloss.RoundedBorder()).
					BorderForeground(alert).Bold(true).Padding(0, 1),
			},
		},
		Save: saveStyles{
			Form:   form,
			Picker: picker,
		},
		ExportForm: form,
	}
}

func registeredTint(dark bool, darkID, lightID string) *tint.Tint {
	var id string
	var fallback *tint.Tint
	var reg *tint.Registry
	if dark {
		id = darkID
		fallback = defaultDarkTint
		reg = darkTints
	} else {
		id = lightID
		fallback = defaultLightTint
		reg = lightTints
	}
	if tint, ok := reg.GetTint(id); ok {
		return tint
	}
	return fallback
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
