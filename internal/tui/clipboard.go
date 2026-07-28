package tui

import (
	"errors"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/coxley/dg/layout"
	"github.com/rivo/uniseg"
	"golang.design/x/clipboard"
)

const clipboardProbeTimeout = 100 * time.Millisecond

type clipboardMode uint8

const (
	clipboardUnknown clipboardMode = iota
	clipboardTerminal
	clipboardFallback
)

type clipboardProbeExpiredMsg struct {
	generation uint64
}

type clipboardFallbackMsg struct {
	err error
}

var (
	clipboardInitOnce sync.Once
	errClipboardInit  error
)

func writeFallbackClipboard(text string) error {
	clipboardInitOnce.Do(func() {
		errClipboardInit = clipboard.Init()
	})
	if errClipboardInit != nil {
		return errClipboardInit
	}
	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}

type exportStyle uint8

const (
	exportLineSlash exportStyle = iota
	exportLineHash
	exportBlock
	exportMarkdown
)

func (m *Model) copySelection() tea.Cmd {
	if !m.hasSelection() {
		m.setError("select something to copy")
		m.copyArmed = false
		return nil
	}
	text, err := m.selectionText()
	if err != nil {
		m.setError(err.Error())
		m.copyArmed = false
		return nil
	}
	if m.copyArmed {
		m.openExport(text)
		m.copyArmed = false
		return nil
	}
	m.copyArmed = true
	m.status = ""
	return m.writeClipboard(text)
}

func (m *Model) openExport(text string) {
	m.exportText = text
	m.exportStyle = exportStyleForPrefix(m.preferences.commentPrefix)
	m.exportForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[exportStyle]().
				Title("Copy selection as").
				Options(exportOptions(m.exportStyle)...).
				Value(&m.exportStyle),
		),
	).
		WithWidth(46).
		WithHeight(7).
		WithShowHelp(true).
		WithTheme(m.theme.formTheme())
	_ = m.exportForm.Init()
	m.openModal(modalExport)
	m.status = ""
}

func normalizeCommentPrefix(prefix string) string {
	switch prefix {
	case "# ", "/* */":
		return prefix
	default:
		return "// "
	}
}

func exportStyleForPrefix(prefix string) exportStyle {
	switch normalizeCommentPrefix(prefix) {
	case "# ":
		return exportLineHash
	case "/* */":
		return exportBlock
	default:
		return exportLineSlash
	}
}

func exportOptions(preferred exportStyle) []huh.Option[exportStyle] {
	labels := [...]string{
		"Line comments  //",
		"Line comments  #",
		"Block comment  /* ... */",
	}
	options := make([]huh.Option[exportStyle], 0, len(labels)+1)
	options = append(options, huh.NewOption(labels[preferred], preferred))
	for style, label := range labels {
		value := exportStyle(style)
		if value != preferred {
			options = append(options, huh.NewOption(label, value))
		}
	}
	return append(options, huh.NewOption("Markdown code block", exportMarkdown))
}

func (m *Model) updateExportForm(message tea.Msg) tea.Cmd {
	form, command := m.exportForm.Update(message)
	m.exportForm = form.(*huh.Form)
	if m.exportForm.State != huh.StateCompleted {
		return componentCommand(exportComponent, command)
	}
	text := formatExport(m.exportText, m.exportStyle)
	m.modal = modalNone
	m.exportText = ""
	m.status = ""
	return m.writeClipboard(text)
}

func (m *Model) writeClipboard(text string) tea.Cmd {
	switch m.clipboardMode {
	case clipboardTerminal:
		return tea.Batch(
			tea.SetClipboard(text),
			m.showNotice("Copied to clipboard", modalNone),
		)
	case clipboardFallback:
		return m.writeFallbackClipboard(text)
	case clipboardUnknown:
		m.clipboardPending = text
		m.clipboardProbe++
		generation := m.clipboardProbe
		return tea.Batch(
			func() tea.Msg { return tea.ReadClipboard() },
			tea.Tick(clipboardProbeTimeout, func(time.Time) tea.Msg {
				return clipboardProbeExpiredMsg{generation: generation}
			}),
		)
	default:
		return nil
	}
}

func (m *Model) handleClipboardResponse() tea.Cmd {
	if m.clipboardMode != clipboardUnknown || m.clipboardPending == "" {
		return nil
	}
	text := m.clipboardPending
	m.clipboardPending = ""
	m.clipboardMode = clipboardTerminal
	return tea.Batch(
		tea.SetClipboard(text),
		m.showNotice("Copied to clipboard", modalNone),
	)
}

func (m *Model) handleClipboardTimeout(message clipboardProbeExpiredMsg) tea.Cmd {
	if m.clipboardMode != clipboardUnknown ||
		m.clipboardPending == "" ||
		message.generation != m.clipboardProbe {
		return nil
	}
	text := m.clipboardPending
	m.clipboardPending = ""
	m.clipboardMode = clipboardFallback
	return m.writeFallbackClipboard(text)
}

func (m *Model) writeFallbackClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		return clipboardFallbackMsg{err: m.clipboardFallback(text)}
	}
}

func (m *Model) handleClipboardFallback(message clipboardFallbackMsg) tea.Cmd {
	if message.err != nil {
		m.setError("copy selection: " + message.err.Error())
		m.copyArmed = false
		return nil
	}
	return m.showNotice("Copied to clipboard", modalNone)
}

func formatExport(text string, style exportStyle) string {
	text = trimTrailingWhitespace(text)
	switch style {
	case exportLineSlash:
		return prefixLines(text, "// ")
	case exportLineHash:
		return prefixLines(text, "# ")
	case exportBlock:
		return "/*\n" + text + "\n*/"
	case exportMarkdown:
		return "```\n" + text + "\n```"
	default:
		return text
	}
}

func prefixLines(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(prefix+lines[i], unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

func trimTrailingWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) selectionText() (string, error) {
	bounds, ok := m.selectionBounds()
	if !ok {
		return "", errors.New("empty selection")
	}
	lines := make([]string, 0, bounds.Size.Height)
	limit := bounds.Max()
	for y := bounds.Min.Y; y < limit.Y; y++ {
		lines = append(lines, m.selectionRow(bounds.Min.X, limit.X, y))
	}
	return trimTrailingWhitespace(strings.Join(lines, "\n")), nil
}

func (m *Model) selectionBounds() (layout.Rect, bool) {
	var (
		minPoint layout.Point
		maxPoint layout.Point
		found    bool
	)
	include := func(point layout.Point) {
		limit := point.Add(1, 1)
		if !found {
			minPoint, maxPoint, found = point, limit, true
			return
		}
		minPoint.X = min(minPoint.X, point.X)
		minPoint.Y = min(minPoint.Y, point.Y)
		maxPoint.X = max(maxPoint.X, limit.X)
		maxPoint.Y = max(maxPoint.Y, limit.Y)
	}
	for nodeID := range m.geo.Selection().Nodes() {
		rect := m.geo.Nodes[nodeID].Rect
		include(rect.Min)
		limit := rect.Max()
		include(layout.NewPoint(limit.X-1, limit.Y-1))
	}
	for edgeID := range m.geo.Selection().Edges() {
		for _, point := range m.geo.Edges[edgeID].Points {
			if point.X == math.MaxUint32 || point.Y == math.MaxUint32 {
				continue
			}
			include(point)
		}
	}
	if !found {
		return layout.Rect{}, false
	}
	return layout.Rect{
		Min: minPoint,
		Size: layout.Size{
			Width:  maxPoint.X - minPoint.X,
			Height: maxPoint.Y - minPoint.Y,
		},
	}, true
}

func (m *Model) selectionRow(start, end, y uint32) string {
	if y < m.frame.Bounds.Min.Y || y >= m.frame.Bounds.Max().Y {
		return ""
	}
	rowID := int(y - m.frame.Bounds.Min.Y)
	if rowID >= len(m.frameRows) {
		return ""
	}
	span := m.frameRows[rowID]
	row := m.frame.Text[span.start:span.end]
	documentX := m.frame.Bounds.Min.X
	outputX := start
	state := -1
	var line strings.Builder
	for len(row) != 0 && outputX < end {
		cluster, rest, width, nextState := uniseg.FirstGraphemeCluster(row, state)
		row, state = rest, nextState
		if width == 0 {
			continue
		}
		clusterStart := documentX
		clusterEnd := documentX + uint32(width)
		documentX = clusterEnd
		if clusterEnd <= start {
			continue
		}
		if clusterStart >= end {
			break
		}
		visibleStart := max(clusterStart, start)
		visibleEnd := min(clusterEnd, end)
		for outputX < visibleStart {
			line.WriteByte(' ')
			outputX++
		}
		if visibleStart == clusterStart &&
			visibleEnd == clusterEnd &&
			m.selectionContains(layout.NewPoint(clusterStart, y)) {
			line.Write(cluster)
		} else {
			for range visibleEnd - visibleStart {
				line.WriteByte(' ')
			}
		}
		outputX = visibleEnd
	}
	return strings.TrimRightFunc(line.String(), unicode.IsSpace)
}

func (m *Model) selectionContains(point layout.Point) bool {
	for nodeID := range m.geo.Selection().Nodes() {
		if m.geo.Nodes[nodeID].Rect.Contains(point) {
			return true
		}
	}
	for edgeID := range m.geo.Selection().Edges() {
		if m.geo.Edges[edgeID].Contains(point) {
			return true
		}
	}
	return false
}
