package tui

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) modalView() tea.View {
	lines := m.modalLines()
	top := max((m.height-len(lines))/2, 0)
	m.viewBuffer = m.viewBuffer[:0]
	for range top {
		m.viewBuffer = appendStatusLine(m.viewBuffer, nil, m.width)
	}
	for _, line := range lines {
		left := max((m.width-displayWidth([]byte(line)))/2, 0)
		m.statusText = appendSpaces(m.statusText[:0], left)
		m.statusText = append(m.statusText, line...)
		m.viewBuffer = appendStatusLine(m.viewBuffer, m.statusText, m.width)
	}
	for len(m.viewBuffer) != 0 &&
		countLines(m.viewBuffer) < m.height {
		m.viewBuffer = appendStatusLine(m.viewBuffer, nil, m.width)
	}

	view := tea.NewView(string(m.viewBuffer))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = "dg"
	if m.modal == modalSave && len(lines) >= 3 {
		prefix := "│ Path: "
		lineWidth := displayWidth([]byte(lines[2]))
		left := max((m.width-lineWidth)/2, 0)
		x := left + len(prefix) + displayWidth(m.editBuffer[:m.editCaret])
		if x < m.width {
			cursor := &m.viewCursor[m.nextCursor]
			m.nextCursor ^= 1
			cursor.X = x
			cursor.Y = top + 2
			view.Cursor = cursor
		}
	}
	return view
}

func (m *Model) modalLines() []string {
	switch m.modal {
	case modalSave:
		path := string(m.editBuffer)
		width := max(42, displayWidth([]byte(path))+10)
		return []string{
			"┌" + repeatRune('─', width-2) + "┐",
			padModalLine("Save diagram", width),
			padModalLine("Path: "+path, width),
			padModalLine("Tab complete  Enter save  Esc cancel", width),
			"└" + repeatRune('─', width-2) + "┘",
		}
	case modalPreferences:
		return m.preferenceLines()
	default:
		return []string{
			"┌──────────────────────────────────────────────────────────┐",
			"│ Shortcuts                                                │",
			"│ ? help/preferences   Backspace delete   d duplicate      │",
			"│ r rectangle          e edit label      l line            │",
			"│ b border             a/A arrows       t/T text align     │",
			"│ Tab/Shift-Tab focus  arrows move      Ctrl-A expand      │",
			"│ [ ] layer step       { } back/front   Ctrl-S save        │",
			"│ u/Ctrl-Z undo        Ctrl-R/Ctrl-Y redo                  │",
			"│ Alt-drag duplicate   Ctrl-click add/remove selection     │",
			"│                                                          │",
			"│ p preferences                         Esc close           │",
			"└──────────────────────────────────────────────────────────┘",
		}
	}
}

func (m *Model) preferenceLines() []string {
	router := m.preferences.router
	values := [...]string{
		"Step cost: " + strconv.FormatUint(uint64(router.Costs.Step), 10),
		"Shared-step cost: " + strconv.FormatUint(uint64(router.Costs.SharedStep), 10),
		"Bend cost: " + strconv.FormatUint(uint64(router.Costs.Bend), 10),
		"Crossing cost: " + strconv.FormatUint(uint64(router.Costs.Crossing), 10),
		"Endpoint cost: " + strconv.FormatUint(uint64(router.Costs.EndpointStep), 10),
		"Reroute passes: " + strconv.FormatUint(uint64(router.ReroutePasses), 10),
		fmt.Sprintf("Apply to future diagrams? [%s]", checkbox(m.preferences.applyToFuture)),
		"Default save directory: " + m.preferences.saveDirectory,
	}
	lines := make([]string, 0, 2+len(values)+2)
	lines = append(
		lines,
		"┌──────────────────────────────────────────────────────────┐",
		"│ Preferences                                              │",
	)
	for i, value := range values {
		prefix := "  "
		if i == m.preferenceRow {
			prefix = "▶ "
		}
		lines = append(lines, padModalLine(prefix+value, 60))
	}
	lines = append(
		lines,
		"│ arrows adjust  Space toggles  Enter apply  Esc cancel    │",
		"└──────────────────────────────────────────────────────────┘",
	)
	return lines
}

func padModalLine(text string, width int) string {
	content := width - 2
	textWidth := displayWidth([]byte(text))
	if textWidth > content {
		text = string([]rune(text)[:content])
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

func countLines(text []byte) int {
	count := 0
	for _, value := range text {
		if value == '\n' {
			count++
		}
	}
	return count
}
