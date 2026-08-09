package tui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
	"github.com/rivo/uniseg"
)

func (m *Model) updateLabel(key tea.KeyPressMsg) {
	event := key.Key()
	if event.Code == tea.KeyEnter {
		switch {
		case event.Mod.Contains(tea.ModCtrl) || event.Mod.Contains(tea.ModSuper):
			m.commitLabelEdit()
		case event.Mod.Contains(tea.ModShift) || strings.ContainsRune(string(m.editBuffer), '\n'):
			m.insertLabelText("\n")
		default:
			m.commitLabelEdit()
		}
		return
	}
	if m.applyLabelEditIntent(chrome.ResolveTextEditIntent(key)) {
		return
	}
	if event.Mod != 0 {
		m.insertLabelText(event.Text)
		return
	}
	switch event.Code {
	case tea.KeyEscape:
		m.commitLabelEdit()
	case tea.KeyUp:
		m.moveCaretVertically(-1)
	case tea.KeyDown:
		m.moveCaretVertically(1)
	default:
		m.insertLabelText(event.Text)
	}
}

func (m *Model) applyLabelEditIntent(intent chrome.TextEditIntent) bool {
	line := m.caretLine()
	switch intent {
	case chrome.TextEditBackward:
		m.editCaret = previousGraphemeStart(m.editBuffer, m.editCaret)
	case chrome.TextEditForward:
		m.editCaret = nextGraphemeEnd(m.editBuffer, m.editCaret)
	case chrome.TextEditLineStart:
		m.editCaret = int(line.Start)
	case chrome.TextEditLineEnd:
		m.editCaret = int(line.End)
	case chrome.TextEditWordBackward:
		m.editCaret = previousWordStart(m.editBuffer, m.editCaret)
	case chrome.TextEditWordForward:
		m.editCaret = nextWordEnd(m.editBuffer, m.editCaret)
	case chrome.TextEditDeleteBackward:
		start := previousGraphemeStart(m.editBuffer, m.editCaret)
		if start != m.editCaret {
			m.replaceLabelRange(start, m.editCaret, nil)
		}
		return true
	case chrome.TextEditDeleteForward:
		end := nextGraphemeEnd(m.editBuffer, m.editCaret)
		if end != m.editCaret {
			m.replaceLabelRange(m.editCaret, end, nil)
		}
		return true
	case chrome.TextEditDeleteWordBackward:
		start := previousWordStart(m.editBuffer, m.editCaret)
		if start != m.editCaret {
			m.replaceLabelRange(start, m.editCaret, nil)
		}
		return true
	case chrome.TextEditDeleteToLineStart:
		if start := int(line.Start); start != m.editCaret {
			m.replaceLabelRange(start, m.editCaret, nil)
		}
		return true
	case chrome.TextEditDeleteToLineEnd:
		if end := int(line.End); end != m.editCaret {
			m.replaceLabelRange(m.editCaret, end, nil)
		}
		return true
	case chrome.TextEditNone:
		return false
	}
	m.moveCursorToCaret()
	return true
}

func (m *Model) insertLabelText(text string) {
	if text == "" {
		return
	}
	if strings.ContainsRune(text, '\r') {
		m.setError("labels do not support carriage returns")
		return
	}
	m.replaceLabelRange(m.editCaret, m.editCaret, []byte(text))
}

func (m *Model) replaceLabelRange(start, end int, replacement []byte) {
	m.editDraft = append(m.editDraft[:0], m.editBuffer[:start]...)
	m.editDraft = append(m.editDraft, replacement...)
	m.editDraft = append(m.editDraft, m.editBuffer[end:]...)
	caret := start + len(replacement)

	previous := m.geo.Label(m.target.ID)
	if err := m.geo.SetNodeLabel(m.target.ID, string(m.editDraft)); err != nil {
		m.setError(err.Error())
		return
	}
	if err := m.rebuild(); err != nil {
		restoreErr := m.geo.SetNodeLabel(m.target.ID, previous)
		if restoreErr == nil {
			restoreErr = m.rebuild()
		}
		m.setError(errors.Join(err, restoreErr).Error())
		return
	}

	m.editBuffer, m.editDraft = m.editDraft, m.editBuffer[:0]
	m.editCaret = caret
	m.refreshEditLines()
	m.moveCursorToCaret()
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) startLabelEdit(hit layout.Hit) {
	label := m.geo.Label(hit.ID)
	m.selectOnly(hit)
	m.target = hit
	m.editBuffer = append(m.editBuffer[:0], label...)
	m.editDraft = m.editDraft[:0]
	m.editCaret = len(m.editBuffer)
	m.interaction.session = interactionSession{kind: sessionLabelEdit}
	m.refreshEditLines()
	m.moveCursorToCaret()
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) commitLabelEdit() {
	target := m.target
	err := m.commitTransaction()
	m.finishLabelEdit()
	m.refreshHits()
	m.target = target
	m.selectTarget()
	if err != nil {
		m.setError(err.Error())
	} else {
		m.status = ""
	}
}

func (m *Model) finishLabelEdit() {
	m.interaction.session = interactionSession{}
	m.editBuffer = m.editBuffer[:0]
	m.editDraft = m.editDraft[:0]
	m.editLines = m.editLines[:0]
	m.editCaret = 0
	m.editCaretVisible = false
}

func (m *Model) moveCaretToPoint(point layout.Point) {
	if !m.geo.NodeExists(m.target.ID) {
		return
	}
	m.refreshEditLines()
	first := m.editLines[0]
	firstPoint, _ := m.geo.LabelLinePoint(
		m.target.ID,
		0,
		uint32(len(m.editLines)),
		uint32(displayWidth(m.editBuffer[first.Start:first.End])),
	)
	lineID := 0
	if point.Y >= firstPoint.Y {
		lineID = min(
			int(point.Y-firstPoint.Y),
			max(len(m.editLines)-1, 0),
		)
	}
	line := m.editLines[lineID]
	linePoint, _ := m.geo.LabelLinePoint(
		m.target.ID,
		uint32(lineID),
		uint32(len(m.editLines)),
		uint32(displayWidth(m.editBuffer[line.Start:line.End])),
	)
	m.editCaret = int(line.Start)
	if point.X > linePoint.X {
		m.editCaret += graphemeOffsetAtWidth(
			m.editBuffer[line.Start:line.End],
			int(point.X-linePoint.X),
		)
	}
	m.moveCursorToCaret()
}

func (m *Model) moveCursorToCaret() {
	if !m.geo.NodeExists(m.target.ID) {
		return
	}
	m.refreshEditLines()
	lineID, line := m.caretLineAt(m.editCaret)
	column := displayWidth(m.editBuffer[line.Start:min(uint32(m.editCaret), line.End)])
	labelBounds := m.geo.LabelBounds(m.target.ID)
	lineWidth := displayWidth(m.editBuffer[line.Start:line.End])
	linePoint, visible := m.geo.LabelLinePoint(
		m.target.ID,
		uint32(lineID),
		uint32(len(m.editLines)),
		uint32(lineWidth),
	)
	m.cursor = linePoint.Add(uint32(column), 0)
	m.editCaretVisible = visible &&
		uint32(column) <= labelBounds.Size.Width
	m.ensureCursorVisible()
}

func (m *Model) refreshEditLines() {
	if !m.geo.NodeExists(m.target.ID) {
		m.editLines = m.editLines[:0]
		return
	}
	wrapWidth := uint32(0)
	if _, explicit := m.geo.ExplicitNodeSize(m.target.ID); explicit {
		wrapWidth = m.geo.LabelBounds(m.target.ID).Size.Width
	}
	m.editLines = layout.AppendLabelLines(
		m.editLines[:0],
		string(m.editBuffer),
		wrapWidth,
	)
}

func (m *Model) caretLine() layout.LabelLine {
	_, line := m.caretLineAt(m.editCaret)
	return line
}

func (m *Model) caretLineAt(caret int) (int, layout.LabelLine) {
	for i, line := range m.editLines {
		last := i == len(m.editLines)-1
		atHardEnd := !last && uint32(caret) == line.End &&
			m.editLines[i+1].Start > uint32(caret)
		if uint32(caret) < line.End || atHardEnd || last {
			return i, line
		}
	}
	return 0, layout.LabelLine{}
}

func (m *Model) moveCaretVertically(delta int) {
	lineID, line := m.caretLineAt(m.editCaret)
	targetID := min(max(lineID+delta, 0), len(m.editLines)-1)
	if targetID == lineID {
		return
	}
	column := displayWidth(m.editBuffer[line.Start:min(uint32(m.editCaret), line.End)])
	target := m.editLines[targetID]
	m.editCaret = int(target.Start) + graphemeOffsetAtWidth(
		m.editBuffer[target.Start:target.End],
		column,
	)
	m.moveCursorToCaret()
}

func previousGraphemeStart(text []byte, offset int) int {
	remaining := text[:offset]
	start := 0
	position := 0
	state := -1
	for len(remaining) != 0 {
		start = position
		cluster, rest, _, nextState := uniseg.FirstGraphemeCluster(remaining, state)
		position += len(cluster)
		remaining, state = rest, nextState
	}
	return start
}

func nextGraphemeEnd(text []byte, offset int) int {
	if offset >= len(text) {
		return len(text)
	}
	cluster, _, _, _ := uniseg.FirstGraphemeCluster(text[offset:], -1)
	return offset + len(cluster)
}

func previousWordStart(text []byte, offset int) int {
	for offset > 0 {
		start := previousGraphemeStart(text, offset)
		if !chrome.IsTextWordBoundary(text[start:offset]) {
			break
		}
		offset = start
	}
	for offset > 0 {
		start := previousGraphemeStart(text, offset)
		if chrome.IsTextWordBoundary(text[start:offset]) {
			break
		}
		offset = start
	}
	return offset
}

func nextWordEnd(text []byte, offset int) int {
	for offset < len(text) {
		end := nextGraphemeEnd(text, offset)
		if !chrome.IsTextWordBoundary(text[offset:end]) {
			break
		}
		offset = end
	}
	for offset < len(text) {
		end := nextGraphemeEnd(text, offset)
		if chrome.IsTextWordBoundary(text[offset:end]) {
			break
		}
		offset = end
	}
	return offset
}

func graphemeOffsetAtWidth(text []byte, target int) int {
	remaining := text
	offset := 0
	width := 0
	state := -1
	for len(remaining) != 0 {
		cluster, rest, clusterWidth, nextState := uniseg.FirstGraphemeCluster(remaining, state)
		if width+clusterWidth > target {
			return offset
		}
		offset += len(cluster)
		width += clusterWidth
		remaining, state = rest, nextState
	}
	return offset
}

func displayWidth(text []byte) int {
	remaining := text
	width := 0
	state := -1
	for len(remaining) != 0 {
		_, rest, clusterWidth, nextState := uniseg.FirstGraphemeCluster(remaining, state)
		width += clusterWidth
		remaining, state = rest, nextState
	}
	return width
}
