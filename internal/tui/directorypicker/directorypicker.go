// Package directorypicker adapts Huh filesystem navigation behind one bounded
// TUI component.
package directorypicker

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// Styles selects the picker color environment.
type Styles struct {
	Dark bool
}

// Config declares filesystem selection policy.
type Config struct {
	Title      string
	Value      string
	AllowFiles bool
	ShowHidden bool
}

// UpdateMsg routes a child picker command back to Model.Update.
type UpdateMsg struct {
	message tea.Msg
}

// Model owns one bounded filesystem picker.
type Model struct {
	config Config
	value  string
	picker *huh.FilePicker
	width  int
	height int
	styles Styles
	open   bool
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
	if update, ok := message.(UpdateMsg); ok {
		message = update.message
	}
	picker, command := m.picker.Update(message)
	m.picker = picker.(*huh.FilePicker)
	if !m.picker.Zoom() {
		m.open = false
	}
	return m, wrap(command)
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	return tea.NewView(m.picker.View())
}

// Open starts filesystem navigation from the current value.
func (m *Model) Open() {
	m.open = true
	m.picker.Picking(true)
}

// Close ends filesystem navigation.
func (m *Model) Close() {
	m.open = false
	m.picker.Picking(false)
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
	m.picker.WithWidth(m.width)
	m.picker.WithHeight(m.height)
}

// SetStyles replaces the picker color environment.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
	m.picker.WithTheme(pickerTheme(styles.Dark))
}

func (m *Model) reset() {
	current := m.value
	if info, err := os.Stat(current); err != nil {
		current, _ = os.UserHomeDir()
	} else if !info.IsDir() {
		current = filepath.Dir(current)
	}
	m.picker = huh.NewFilePicker().
		Title(m.config.Title).
		DirAllowed(true).
		FileAllowed(m.config.AllowFiles).
		ShowHidden(m.config.ShowHidden).
		CurrentDirectory(current).
		Picking(false).
		Value(&m.value)
	m.picker.WithWidth(m.width)
	m.picker.WithHeight(m.height)
	m.picker.WithTheme(pickerTheme(m.styles.Dark))
	m.open = false
}

func pickerTheme(dark bool) huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles {
		return huh.ThemeCharm(dark)
	})
}

func wrap(command tea.Cmd) tea.Cmd {
	if command == nil {
		return nil
	}
	return func() tea.Msg {
		message := command()
		if message == nil {
			return nil
		}
		if batch, ok := message.(tea.BatchMsg); ok {
			for i := range batch {
				batch[i] = wrap(batch[i])
			}
			return batch
		}
		return UpdateMsg{message: message}
	}
}
