package tui

import (
	"fmt"
	"strconv"
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
		width = min(
			max(42, displayWidth(m.editBuffer)+10),
			m.width,
		)
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
		path := string(m.editBuffer)
		return []string{
			"┌" + repeatRune('─', width-2) + "┐",
			padModalLine("Save diagram", width),
			padModalLine("Path: "+path, width),
			padModalLine("Tab complete  Enter save  Esc cancel", width),
			"└" + repeatRune('─', width-2) + "┘",
		}
	case modalPreferences:
		return m.preferenceLines(width)
	default:
		return shortcutLines(width)
	}
}

func shortcutLines(width int) []string {
	rows := [][6]string{
		{"?", "Help", "Backspace", "Delete", "d", "Duplicate"},
		{"r", "Rectangle", "e", "Edit label", "l", "Line"},
		{"b", "Border", "a / A", "Arrows", "t / T", "Text align"},
		{"Tab/Shift-Tab", "Focus", "Arrows", "Move", "Ctrl-A", "Expand"},
		{"[ / ]", "Layer", "{ / }", "Back/front", "Ctrl-S", "Save"},
		{"u / Ctrl-Z", "Undo", "Ctrl-R/Ctrl-Y", "Redo", "Alt-drag", "Duplicate"},
		{"Ctrl-click", "Add/remove", "Esc", "Close", "", ""},
	}
	lines := make([]string, 0, len(rows)+4)
	lines = append(
		lines,
		"┌"+repeatRune('─', width-2)+"┐",
		settingsTabLine(width, modalHelp),
	)
	for _, row := range rows {
		lines = append(lines, shortcutRow(width, row))
	}
	lines = append(
		lines,
		padModalLine("Tab / Shift-Tab switch tabs", width),
		"└"+repeatRune('─', width-2)+"┘",
	)
	return lines
}

func shortcutRow(width int, values [6]string) string {
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
	return padModalLine(text, width)
}

func (m *Model) preferenceLines(width int) []string {
	router := m.preferences.router
	values := [...]struct {
		title string
		value string
	}{
		{"Step cost", strconv.FormatUint(uint64(router.Costs.Step), 10)},
		{"Shared-step cost", strconv.FormatUint(uint64(router.Costs.SharedStep), 10)},
		{"Bend cost", strconv.FormatUint(uint64(router.Costs.Bend), 10)},
		{"Crossing cost", strconv.FormatUint(uint64(router.Costs.Crossing), 10)},
		{"Endpoint cost", strconv.FormatUint(uint64(router.Costs.EndpointStep), 10)},
		{"Reroute passes", strconv.FormatUint(uint64(router.ReroutePasses), 10)},
		{"Apply to future diagrams?", "[" + checkbox(m.preferences.applyToFuture) + "]"},
		{"Default save directory", m.preferences.saveDirectory},
	}
	lines := make([]string, 0, len(values)+4)
	lines = append(
		lines,
		"┌"+repeatRune('─', width-2)+"┐",
		settingsTabLine(width, modalPreferences),
	)
	for i, value := range values {
		lines = append(
			lines,
			justifiedModalLine(value.title, value.value, width, i == m.preferenceRow),
		)
	}
	lines = append(
		lines,
		padModalLine("↑/↓ select  ←/→ adjust  Space toggle  Enter apply  Esc cancel", width),
		"└"+repeatRune('─', width-2)+"┘",
	)
	return lines
}

func settingsTabLine(width int, active modal) string {
	shortcuts, preferences := "  Shortcuts  ", "  Preferences  "
	if active == modalHelp {
		shortcuts = "[ Shortcuts ]"
	} else {
		preferences = "[ Preferences ]"
	}
	return padModalLine(shortcuts+"   "+preferences, width)
}

func justifiedModalLine(title, value string, width int, selected bool) string {
	contentWidth := max(width-4, 0)
	prefix := "  "
	if selected {
		prefix = "▶ "
	}
	titleWidth := displayWidth([]byte(title))
	valueWidth := displayWidth([]byte(value))
	gap := max(contentWidth-titleWidth-valueWidth, 1)
	return padModalLine(prefix+title+repeatRune(' ', gap)+value, width)
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

func repeatRune(value rune, count int) string {
	result := make([]rune, max(count, 0))
	for i := range result {
		result[i] = value
	}
	return string(result)
}

func checkbox(checked bool) string {
	if checked {
		return "x"
	}
	return " "
}
