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

// Styles defines every directory-picker visual state.
type Styles struct {
	Container    lipgloss.Style
	Title        lipgloss.Style
	Item         lipgloss.Style
	HoveredItem  lipgloss.Style
	SelectedItem lipgloss.Style
	Empty        lipgloss.Style
	Error        lipgloss.Style
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
	hovered  bool
	hover    int
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
	case tea.MouseMotionMsg:
		m.hoverAt(message.Mouse())
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

// SetStyles replaces every directory-picker visual state.
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
	m.hovered = false
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
	return max(m.contentHeight()-1, 1)
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
	row := mouse.Y - m.contentTop() - 1
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

func (m *Model) hoverAt(mouse tea.Mouse) {
	row := mouse.Y - m.contentTop() - 1
	index := m.offset + row
	hovered := row >= 0 && row < m.visibleRows() && index < len(m.entries)
	if m.hovered == hovered && (!hovered || m.hover == index) {
		return
	}
	m.hovered = hovered
	m.hover = index
}

func (m *Model) render() string {
	width := m.contentWidth()
	height := m.contentHeight()
	if width == 0 {
		return ""
	}
	lines := make([]string, 0, height)
	lines = append(lines, m.styles.Title.Render(
		ansi.Truncate(m.config.Title, width, ""),
	))
	switch {
	case m.err != "":
		lines = append(lines, m.styles.Error.Render(
			ansi.Truncate(m.err, width, ""),
		))
	case len(m.entries) == 0:
		lines = append(lines, m.styles.Empty.Render("  No visible directories"))
	default:
		end := min(m.offset+m.visibleRows(), len(m.entries))
		for i := m.offset; i < end; i++ {
			prefix := "  "
			style := m.styles.Item
			switch {
			case i == m.selected:
				prefix = "> "
				style = m.styles.SelectedItem
			case m.hovered && i == m.hover:
				style = m.styles.HoveredItem
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
	outerHeight := m.height
	if outerHeight == 0 {
		outerHeight = height + m.styles.Container.GetVerticalFrameSize()
	}
	return m.styles.Container.
		Width(m.width).
		Height(outerHeight).
		MaxWidth(m.width).
		MaxHeight(outerHeight).
		Render(strings.Join(lines, "\n"))
}

func (m *Model) contentWidth() int {
	return max(m.width-m.styles.Container.GetHorizontalFrameSize(), 0)
}

func (m *Model) contentHeight() int {
	if m.height > 0 {
		return max(m.height-m.styles.Container.GetVerticalFrameSize(), 0)
	}
	return min(max(len(m.entries)+1, 2), 10)
}

func (m *Model) contentTop() int {
	style := m.styles.Container
	return style.GetMarginTop() +
		style.GetBorderTopSize() +
		style.GetPaddingTop()
}
