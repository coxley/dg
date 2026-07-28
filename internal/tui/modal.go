package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const settingsModalWidth = 84

type modalOverlay struct {
	lines []string
	left  int
	top   int
	width int
}

func (m *Model) currentModalOverlay() modalOverlay {
	if m.modal == modalNone || m.width < 2 {
		return modalOverlay{}
	}
	width := min(settingsModalWidth, m.width)
	if m.modal == modalSave {
		width = min(68, m.width)
	}
	lines := m.modalLines(width)
	height := m.diagramHeight()
	top := max((height-len(lines))/2, 0)
	belowToolbar := toolbarTop + toolbarBoxHeight
	if belowToolbar+len(lines) <= height {
		top = max(top, belowToolbar)
	}
	return modalOverlay{
		lines: lines,
		left:  max((m.width-width)/2, 0),
		top:   top,
		width: width,
	}
}

func (o modalOverlay) line(screenY int) (string, bool) {
	row := screenY - o.top
	if row < 0 || row >= len(o.lines) {
		return "", false
	}
	return o.lines[row], true
}

func (m *Model) modalLines(width int) []string {
	switch m.modal {
	case modalSave:
		return componentModalLines(m.saveForm.View(), width)
	case modalExport:
		return componentModalLines(m.exportForm.View(), width)
	default:
		return m.settingsModalLines(width)
	}
}

func (m *Model) settingsModalLines(width int) []string {
	return componentModalLines(m.settingsTabs.View().Content, width)
}

func componentModalLines(content string, width int) []string {
	rows := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, "┌"+repeatRune('─', width-2)+"┐")
	for _, row := range rows {
		lines = append(lines, padANSIModalLine(row, width))
	}
	lines = append(lines, "└"+repeatRune('─', width-2)+"┘")
	return lines
}

func shortcutContent() string {
	rows := [][6]string{
		{"?", "Help", "Backspace", "Delete", "d", "Duplicate"},
		{"r", "Rectangle", "e", "Edit label", "l", "Line"},
		{"b", "Border", "a / A", "Arrows", "t / T", "Text align"},
		{"Tab/Shift-Tab", "Focus", "Arrows", "Move", "Ctrl-A", "Expand"},
		{"[ / ]", "Layer", "{ / }", "Back/front", "Ctrl-S", "Save"},
		{"u / Ctrl-Z", "Undo", "Ctrl-R/Ctrl-Y", "Redo", "Alt-drag", "Duplicate"},
		{"Ctrl-click", "Add/remove", "Esc", "Close", "", ""},
	}
	lines := make([]string, 0, len(rows)+1)
	for _, row := range rows {
		lines = append(lines, shortcutRow(row))
	}
	lines = append(lines, "Tab / Shift-Tab switch tabs    Esc close")
	return strings.Join(lines, "\n")
}

func shortcutRow(values [6]string) string {
	const (
		keyWidth         = 13
		descriptionWidth = 10
	)
	text := fmt.Sprintf(
		" %-*s  %-*s  %-*s  %-*s  %-*s  %-*s",
		keyWidth, values[0],
		descriptionWidth, values[1],
		keyWidth, values[2],
		descriptionWidth, values[3],
		keyWidth, values[4],
		descriptionWidth, values[5],
	)
	return text
}

func padModalLine(text string, width int) string {
	content := max(width-2, 0)
	textWidth := displayWidth([]byte(text))
	if textWidth > content {
		runes := []rune(text)
		for len(runes) != 0 && displayWidth([]byte(string(runes))) > content {
			runes = runes[:len(runes)-1]
		}
		text = string(runes)
		textWidth = displayWidth([]byte(text))
	}
	return "│" + text + repeatRune(' ', content-textWidth) + "│"
}

func padANSIModalLine(text string, width int) string {
	content := max(width-2, 0)
	text = ansi.Truncate(text, content, "")
	return "│" + text + repeatRune(' ', content-ansi.StringWidth(text)) + "│"
}

func repeatRune(value rune, count int) string {
	result := make([]rune, max(count, 0))
	for i := range result {
		result[i] = value
	}
	return string(result)
}
