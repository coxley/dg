package chrome

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// TextInputStyles defines geometry-stable text input states.
type TextInputStyles struct {
	Text        lipgloss.Style
	FocusedText lipgloss.Style
	Placeholder lipgloss.Style
	Cursor      lipgloss.Style
}

// TextInput retains one single-line value, caret, and visible cell window.
type TextInput struct {
	value       []string
	placeholder string
	cursor      int
	width       int
	focused     bool
	selectAll   bool
	styles      TextInputStyles
}

// NewTextInput returns a single-line text input.
func NewTextInput(value, placeholder string, styles TextInputStyles) *TextInput {
	input := &TextInput{
		placeholder: placeholder,
		styles:      styles,
	}
	input.SetValue(value)
	return input
}

// Update applies editing keys and paste.
func (i *TextInput) Update(message tea.Msg) {
	if !i.focused {
		return
	}
	switch message := message.(type) {
	case tea.PasteMsg:
		i.insert(sanitizeSingleLine(message.Content))
	case tea.KeyPressMsg:
		i.updateKey(message)
	}
}

// SetValue replaces the value and moves the caret to its end.
func (i *TextInput) SetValue(value string) {
	i.value = splitGraphemes(sanitizeSingleLine(value))
	i.cursor = len(i.value)
	i.selectAll = false
}

// Value returns the current value.
func (i *TextInput) Value() string {
	return strings.Join(i.value, "")
}

// SetWidth replaces the visible cell width.
func (i *TextInput) SetWidth(width int) {
	i.width = max(width, 0)
}

// SetStyles replaces semantic visual states.
func (i *TextInput) SetStyles(styles TextInputStyles) {
	i.styles = styles
}

// Focus enables editing.
func (i *TextInput) Focus() {
	i.focused = true
}

// Blur disables editing.
func (i *TextInput) Blur() {
	i.focused = false
}

// Click moves the caret to the nearest visible cell.
func (i *TextInput) Click(x int) {
	i.selectAll = false
	start := i.visibleStart()
	x = max(x, 0)
	cell := 0
	i.cursor = len(i.value)
	for index := start; index < len(i.value); index++ {
		width := ansi.StringWidth(i.value[index])
		if x < cell+max(width, 1) {
			i.cursor = index
			return
		}
		cell += width
	}
}

// View renders the current visible cell window.
func (i *TextInput) View() string {
	if i.width == 0 {
		return ""
	}
	if len(i.value) == 0 && !i.focused {
		return padLine(
			i.styles.Placeholder.Render(ansi.Truncate(i.placeholder, i.width, "")),
			i.width,
		)
	}
	start := i.visibleStart()
	before := strings.Join(i.value[start:i.cursor], "")
	if !i.focused {
		text := i.styles.Text.Render(ansi.Truncate(strings.Join(i.value[start:], ""), i.width, ""))
		return padLine(text, i.width)
	}
	cursor := " "
	afterIndex := i.cursor
	if afterIndex < len(i.value) {
		cursor = i.value[afterIndex]
		afterIndex++
	}
	before = i.styles.FocusedText.Render(before)
	cursor = i.styles.Cursor.Render(cursor)
	remaining := max(i.width-ansi.StringWidth(before)-ansi.StringWidth(cursor), 0)
	after := i.styles.FocusedText.Render(
		ansi.Truncate(strings.Join(i.value[afterIndex:], ""), remaining, ""),
	)
	return padLine(ansi.Truncate(before+cursor+after, i.width, ""), i.width)
}

func (i *TextInput) updateKey(message tea.KeyPressMsg) {
	switch {
	case message.Code == tea.KeyLeft:
		i.selectAll = false
		i.cursor = max(i.cursor-1, 0)
	case message.Code == tea.KeyRight:
		i.selectAll = false
		i.cursor = min(i.cursor+1, len(i.value))
	case message.Code == 'a' && message.Mod == tea.ModCtrl:
		i.selectAll = true
		i.cursor = len(i.value)
	case message.Code == tea.KeyHome:
		i.selectAll = false
		i.cursor = 0
	case message.Code == tea.KeyEnd || message.Code == 'e' && message.Mod == tea.ModCtrl:
		i.selectAll = false
		i.cursor = len(i.value)
	case message.Code == tea.KeyBackspace:
		if i.selectAll {
			i.value = i.value[:0]
			i.cursor = 0
			i.selectAll = false
		} else if i.cursor != 0 {
			i.value = append(i.value[:i.cursor-1], i.value[i.cursor:]...)
			i.cursor--
		}
	case message.Code == tea.KeyDelete:
		if i.selectAll {
			i.value = i.value[:0]
			i.cursor = 0
			i.selectAll = false
		} else if i.cursor < len(i.value) {
			i.value = append(i.value[:i.cursor], i.value[i.cursor+1:]...)
		}
	case message.Text != "" && message.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0:
		i.insert(message.Text)
	}
}

func (i *TextInput) insert(text string) {
	graphemes := splitGraphemes(sanitizeSingleLine(text))
	if len(graphemes) == 0 {
		return
	}
	if i.selectAll {
		i.value = i.value[:0]
		i.cursor = 0
		i.selectAll = false
	}
	value := make([]string, 0, len(i.value)+len(graphemes))
	value = append(value, i.value[:i.cursor]...)
	value = append(value, graphemes...)
	value = append(value, i.value[i.cursor:]...)
	i.value = value
	i.cursor += len(graphemes)
}

func (i *TextInput) visibleStart() int {
	if i.width <= 0 {
		return i.cursor
	}
	start := 0
	for start < i.cursor &&
		ansi.StringWidth(strings.Join(i.value[start:i.cursor], "")) >= i.width {
		start++
	}
	return start
}

func sanitizeSingleLine(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", "")
}

func splitGraphemes(value string) []string {
	graphemes := uniseg.NewGraphemes(value)
	var result []string
	for graphemes.Next() {
		result = append(result, graphemes.Str())
	}
	return result
}
