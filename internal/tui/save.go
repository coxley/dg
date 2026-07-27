package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/document"
)

func (m *Model) requestSave() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	if m.path == "" {
		m.mode = modeSavePath
		m.editBuffer = m.editBuffer[:0]
		m.editDraft = m.editDraft[:0]
		m.editCaret = 0
		m.status = ""
		m.saveHint = ""
		return
	}
	m.save(m.path)
}

func (m *Model) updateSavePath(message tea.KeyPressMsg) {
	key := message.Key()
	if key.Mod == tea.ModCtrl {
		switch key.Code {
		case 'a':
			m.editCaret = 0
			return
		case 'e':
			m.editCaret = len(m.editBuffer)
			return
		case 'u':
			if m.editCaret != 0 {
				m.replaceSavePathRange(0, m.editCaret, nil)
			}
			return
		case 'w':
			start := previousWordStart(m.editBuffer, m.editCaret)
			if start != m.editCaret {
				m.replaceSavePathRange(start, m.editCaret, nil)
			}
			return
		case 's':
			m.commitSavePath()
			return
		}
	}
	if key.Mod.Contains(tea.ModAlt) && (key.Code == 'b' || key.Code == 'B') {
		m.editCaret = previousWordStart(m.editBuffer, m.editCaret)
		return
	}
	if key.Mod != 0 {
		m.insertSavePathText(key.Text)
		return
	}
	switch key.Code {
	case tea.KeyEscape:
		m.finishSavePath()
		m.status = ""
	case tea.KeyEnter:
		m.commitSavePath()
	case tea.KeyTab:
		m.completeSavePath()
	case tea.KeyLeft:
		m.editCaret = previousGraphemeStart(m.editBuffer, m.editCaret)
	case tea.KeyRight:
		m.editCaret = nextGraphemeEnd(m.editBuffer, m.editCaret)
	case tea.KeyHome:
		m.editCaret = 0
	case tea.KeyEnd:
		m.editCaret = len(m.editBuffer)
	case tea.KeyBackspace:
		start := previousGraphemeStart(m.editBuffer, m.editCaret)
		if start != m.editCaret {
			m.replaceSavePathRange(start, m.editCaret, nil)
		}
	case tea.KeyDelete:
		end := nextGraphemeEnd(m.editBuffer, m.editCaret)
		if end != m.editCaret {
			m.replaceSavePathRange(m.editCaret, end, nil)
		}
	default:
		m.insertSavePathText(key.Text)
	}
}

func (m *Model) insertSavePathText(text string) {
	if text == "" {
		return
	}
	if strings.ContainsAny(text, "\r\n") {
		m.status = "save path must fit on one line"
		return
	}
	m.replaceSavePathRange(m.editCaret, m.editCaret, []byte(text))
}

func (m *Model) replaceSavePathRange(start, end int, replacement []byte) {
	m.editDraft = append(m.editDraft[:0], m.editBuffer[:start]...)
	m.editDraft = append(m.editDraft, replacement...)
	m.editDraft = append(m.editDraft, m.editBuffer[end:]...)
	m.editBuffer, m.editDraft = m.editDraft, m.editBuffer[:0]
	m.editCaret = start + len(replacement)
	m.status = ""
	m.saveHint = ""
}

func (m *Model) commitSavePath() {
	if len(m.editBuffer) == 0 {
		m.status = "enter a save path"
		return
	}
	path := string(m.editBuffer)
	if !m.save(path) {
		return
	}
	m.finishSavePath()
}

func (m *Model) finishSavePath() {
	m.mode = modeNavigate
	m.editBuffer = m.editBuffer[:0]
	m.editDraft = m.editDraft[:0]
	m.editCaret = 0
	m.saveHint = ""
}

func (m *Model) save(path string) bool {
	data, err := document.Marshal(m.geo)
	if err != nil {
		m.status = err.Error()
		return false
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		m.status = fmt.Sprintf("save diagram: %v", err)
		return false
	}
	m.path = path
	m.status = "saved " + path
	return true
}

func (m *Model) completeSavePath() {
	prefix := string(m.editBuffer[:m.editCaret])
	dir, partial := filepath.Split(prefix)
	searchDir := dir
	if searchDir == "" {
		searchDir = "."
	}
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		m.saveHint = fmt.Sprintf("complete path: %v", err)
		return
	}

	matches := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), partial) {
			matches = append(matches, entry.Name())
		}
	}
	if len(matches) == 0 {
		m.saveHint = "no matching path"
		return
	}
	slices.Sort(matches)
	completed := dir + commonPrefix(matches)
	if len(matches) == 1 {
		info, err := os.Stat(filepath.Join(searchDir, matches[0]))
		if err == nil && info.IsDir() {
			completed += string(filepath.Separator)
		}
	}
	m.replaceSavePathRange(0, m.editCaret, []byte(completed))
	if len(matches) > 1 {
		m.saveHint = strings.Join(matches, "  ")
	}
}

func commonPrefix(values []string) string {
	prefix := []rune(values[0])
	for _, value := range values[1:] {
		next := []rune(value)
		prefix = prefix[:min(len(prefix), len(next))]
		for i := range prefix {
			if prefix[i] != next[i] {
				prefix = prefix[:i]
				break
			}
		}
	}
	return string(prefix)
}
