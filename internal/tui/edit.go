package tui

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
	"github.com/rivo/uniseg"
)

func (m *Model) updateLabel(key tea.KeyPressMsg) {
	event := key.Key()
	if event.Mod == tea.ModCtrl {
		switch event.Code {
		case 'a':
			m.editCaret = int(m.caretLine().Start)
			m.moveCursorToCaret()
			return
		case 'e':
			m.editCaret = int(m.caretLine().End)
			m.moveCursorToCaret()
			return
		case 'w':
			start := previousWordStart(m.editBuffer, m.editCaret)
			if start != m.editCaret {
				m.replaceLabelRange(start, m.editCaret, nil)
			}
			return
		case 'u':
			start := int(m.caretLine().Start)
			if m.editCaret != start {
				m.replaceLabelRange(start, m.editCaret, nil)
			}
			return
		case tea.KeyEnter:
			m.commitLabelEdit()
			return
		}
	}
	if event.Mod.Contains(tea.ModAlt) && (event.Code == 'b' || event.Code == 'B') {
		m.editCaret = previousWordStart(m.editBuffer, m.editCaret)
		m.moveCursorToCaret()
		return
	}
	if event.Mod != 0 {
		m.insertLabelText(event.Text)
		return
	}
	switch event.Code {
	case tea.KeyEscape:
		m.commitLabelEdit()
	case tea.KeyEnter:
		m.insertLabelText("\n")
	case tea.KeyLeft:
		m.editCaret = previousGraphemeStart(m.editBuffer, m.editCaret)
		m.moveCursorToCaret()
	case tea.KeyRight:
		m.editCaret = nextGraphemeEnd(m.editBuffer, m.editCaret)
		m.moveCursorToCaret()
	case tea.KeyUp:
		m.moveCaretVertically(-1)
	case tea.KeyDown:
		m.moveCaretVertically(1)
	case tea.KeyHome:
		line := m.caretLine()
		m.editCaret = int(line.Start)
		m.moveCursorToCaret()
	case tea.KeyEnd:
		line := m.caretLine()
		m.editCaret = int(line.End)
		m.moveCursorToCaret()
	case tea.KeyBackspace:
		start := previousGraphemeStart(m.editBuffer, m.editCaret)
		if start != m.editCaret {
			m.replaceLabelRange(start, m.editCaret, nil)
		}
	case tea.KeyDelete:
		end := nextGraphemeEnd(m.editBuffer, m.editCaret)
		if end != m.editCaret {
			m.replaceLabelRange(m.editCaret, end, nil)
		}
	default:
		m.insertLabelText(event.Text)
	}
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
	m.mode = modeEditLabel
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
	m.mode = modeNavigate
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
	labelPoint := m.geo.Nodes[m.target.ID].LabelPoint
	m.refreshEditLines()
	lineID := 0
	if point.Y >= labelPoint.Y {
		lineID = min(
			int(point.Y-labelPoint.Y),
			max(len(m.editLines)-1, 0),
		)
	}
	line := m.editLines[lineID]
	m.editCaret = int(line.Start)
	if point.X > labelPoint.X {
		m.editCaret += graphemeOffsetAtWidth(
			m.editBuffer[line.Start:line.End],
			int(point.X-labelPoint.X),
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
	m.cursor = labelBounds.Min.Add(uint32(column), uint32(lineID))
	m.editCaretVisible = uint32(lineID) < labelBounds.Size.Height &&
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
		if !graphemeIsWordBoundary(text[start:offset]) {
			break
		}
		offset = start
	}
	for offset > 0 {
		start := previousGraphemeStart(text, offset)
		if graphemeIsWordBoundary(text[start:offset]) {
			break
		}
		offset = start
	}
	return offset
}

// graphemeIsWordBoundary follows Readline's filename-word behavior.
func graphemeIsWordBoundary(cluster []byte) bool {
	if len(cluster) == 0 {
		return false
	}
	for len(cluster) != 0 {
		r, size := utf8.DecodeRune(cluster)
		if !unicode.IsSpace(r) && r != '/' {
			return false
		}
		cluster = cluster[size:]
	}
	return true
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
