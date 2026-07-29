// Package directorypicker implements bounded filesystem navigation.
package directorypicker

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Styles selects the picker color environment.
type Styles struct {
	Dark bool
}

// Config declares filesystem selection policy.
type Config struct {
	Title string
	Value string
}

// Model owns one bounded directory picker.
type Model struct {
	config   Config
	value    string
	current  string
	entries  []string
	err      string
	width    int
	height   int
	selected int
	offset   int
	styles   Styles
	open     bool
}

// New returns a closed picker.
func New(config Config, styles Styles) *Model {
	m := &Model{config: config, value: config.Value, styles: styles}
	m.reset()
	return m
}

// Init implements tea.Model.
func (*Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if !m.open {
		return m, nil
	}
	switch message := message.(type) {
	case tea.KeyPressMsg:
		m.updateKey(message)
	case tea.MouseWheelMsg:
		switch message.Button {
		case tea.MouseWheelUp:
			m.move(-1)
		case tea.MouseWheelDown:
			m.move(1)
		}
	case tea.MouseClickMsg:
		m.click(message.Mouse())
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	return tea.NewView(m.render())
}

// Open starts filesystem navigation from the current value.
func (m *Model) Open() {
	m.open = true
	m.load(m.startDirectory())
}

// Close ends filesystem navigation.
func (m *Model) Close() {
	m.open = false
}

// Opened reports whether the picker owns input.
func (m *Model) Opened() bool {
	return m.open
}

// Value returns the selected path.
func (m *Model) Value() string {
	return m.value
}

// SetValue replaces the selected path and navigation root.
func (m *Model) SetValue(value string) {
	m.config.Value = value
	m.value = value
	m.reset()
}

// SetBounds replaces the picker dimensions.
func (m *Model) SetBounds(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
	m.reveal()
}

// SetStyles replaces the picker color environment.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
}

func (m *Model) reset() {
	m.current = m.startDirectory()
	m.selected = 0
	m.offset = 0
	m.load(m.current)
	m.open = false
}

func (m *Model) startDirectory() string {
	current := m.value
	if info, err := os.Stat(current); err == nil {
		if !info.IsDir() {
			return filepath.Dir(current)
		}
		return current
	}
	current, _ = os.UserHomeDir()
	return current
}

func (m *Model) load(directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		m.err = err.Error()
		m.entries = m.entries[:0]
		return
	}
	m.current = directory
	m.err = ""
	m.entries = m.entries[:0]
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") ||
			!directoryEntry(directory, entry) {
			continue
		}
		m.entries = append(m.entries, entry.Name())
	}
	m.selected = min(m.selected, max(len(m.entries)-1, 0))
	m.reveal()
}

func directoryEntry(directory string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	info, err := os.Stat(filepath.Join(directory, entry.Name()))
	return err == nil && info.IsDir()
}

func (m *Model) updateKey(message tea.KeyPressMsg) {
	switch {
	case message.Mod == 0 && (message.Code == tea.KeyUp || message.Code == 'k'):
		m.move(-1)
	case message.Mod == 0 && (message.Code == tea.KeyDown || message.Code == 'j'):
		m.move(1)
	case message.Mod == 0 && (message.Code == tea.KeyPgUp || message.Code == 'K'):
		m.move(-m.visibleRows())
	case message.Mod == 0 && (message.Code == tea.KeyPgDown || message.Code == 'J'):
		m.move(m.visibleRows())
	case message.Mod == 0 &&
		(message.Code == tea.KeyLeft ||
			message.Code == tea.KeyBackspace ||
			message.Code == 'h'):
		m.load(filepath.Dir(m.current))
	case message.Mod == 0 && (message.Code == tea.KeyRight || message.Code == 'l'):
		m.openSelected()
	case message.Mod == 0 && message.Code == tea.KeyEnter:
		m.selectCurrent()
	case message.Mod == 0 && (message.Code == tea.KeyEscape || message.Code == 'q'):
		m.Close()
	}
}

func (m *Model) move(delta int) {
	if len(m.entries) == 0 {
		return
	}
	m.selected = min(max(m.selected+delta, 0), len(m.entries)-1)
	m.reveal()
}

func (m *Model) reveal() {
	rows := m.visibleRows()
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+rows {
		m.offset = m.selected - rows + 1
	}
	maxOffset := max(len(m.entries)-rows, 0)
	m.offset = min(max(m.offset, 0), maxOffset)
}

func (m *Model) visibleRows() int {
	return max(m.renderHeight()-1, 1)
}

func (m *Model) openSelected() {
	if len(m.entries) == 0 {
		return
	}
	next := filepath.Join(m.current, m.entries[m.selected])
	m.selected = 0
	m.offset = 0
	m.load(next)
}

func (m *Model) selectCurrent() {
	if len(m.entries) == 0 {
		m.value = m.current
	} else {
		m.value = filepath.Join(m.current, m.entries[m.selected])
	}
	m.Close()
}

func (m *Model) click(mouse tea.Mouse) {
	if mouse.Button != tea.MouseLeft {
		return
	}
	row := mouse.Y - 1
	if row < 0 || row >= m.visibleRows() {
		return
	}
	index := m.offset + row
	if index >= len(m.entries) {
		return
	}
	m.selected = index
	m.reveal()
}

func (m *Model) render() string {
	width := max(m.width, 0)
	height := m.renderHeight()
	if width == 0 {
		return ""
	}
	title, cursor, directory, muted, errStyle := pickerStyles(m.styles.Dark)
	lines := make([]string, 0, height)
	lines = append(lines, title.Render(ansi.Truncate(m.config.Title, width, "")))
	switch {
	case m.err != "":
		lines = append(lines, errStyle.Render(ansi.Truncate(m.err, width, "")))
	case len(m.entries) == 0:
		lines = append(lines, muted.Render("  No visible directories"))
	default:
		end := min(m.offset+m.visibleRows(), len(m.entries))
		for i := m.offset; i < end; i++ {
			prefix := "  "
			style := directory
			if i == m.selected {
				prefix = "> "
				style = cursor
			}
			lines = append(
				lines,
				style.Render(ansi.Truncate(prefix+m.entries[i], width, "")),
			)
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxWidth(width).
		MaxHeight(height).
		Render(strings.Join(lines, "\n"))
}

func (m *Model) renderHeight() int {
	if m.height > 0 {
		return m.height
	}
	return min(max(len(m.entries)+1, 2), 10)
}

func pickerStyles(dark bool) (
	title, cursor, directory, muted, errStyle lipgloss.Style,
) {
	if dark {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")),
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("36")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("57")),
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("127")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("29")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
}
