// Package numinput implements an unsigned numeric stepper.
package numinput

import (
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const flashDuration = 120 * time.Millisecond

// Styles defines all numeric-input appearance.
type Styles struct {
	Title        lipgloss.Style
	FocusedTitle lipgloss.Style
	Button       lipgloss.Style
	ActiveButton lipgloss.Style
}

// FlashExpiredMsg clears directional feedback for one input generation.
type FlashExpiredMsg struct {
	input      *Model
	generation uint64
}

// Model owns a bounded unsigned value and its interaction state.
type Model struct {
	title      string
	value      *string
	bits       int
	styles     Styles
	focused    bool
	flash      int
	generation uint64
}

// New returns a numeric input bound to value.
func New(title string, value *string, bits int, styles Styles) *Model {
	return &Model{
		title:  title,
		value:  value,
		bits:   bits,
		styles: styles,
	}
}

// Init implements tea.Model.
func (*Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		switch message.Code {
		case tea.KeyLeft:
			return m, m.step(-1)
		case tea.KeyRight:
			return m, m.step(1)
		}
	case FlashExpiredMsg:
		m.HandleFlash(message)
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	return tea.NewView(m.Render())
}

// Render returns the styled input content for embedding in another model.
func (m *Model) Render() string {
	title := m.styles.Title.Render(m.title)
	if !m.focused {
		return title + "  " + *m.value
	}
	title = m.styles.FocusedTitle.Render(m.title)
	left, right := m.styles.Button.Render("⇽"), m.styles.Button.Render("⇾")
	if m.flash < 0 {
		left = m.styles.ActiveButton.Render("⇽")
	} else if m.flash > 0 {
		right = m.styles.ActiveButton.Render("⇾")
	}
	return title + "  " + left + " " + *m.value + " " + right
}

// SetStyles replaces the input's visual styles.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
}

// SetFocused controls focused rendering.
func (m *Model) SetFocused(focused bool) {
	m.focused = focused
	if !focused {
		m.flash = 0
	}
}

// Flash reports the active direction: -1 for left, 1 for right, or 0.
func (m *Model) Flash() int {
	return m.flash
}

// HandleFlash consumes a matching flash-expiration message.
func (m *Model) HandleFlash(message FlashExpiredMsg) bool {
	if message.input != m {
		return false
	}
	if m.generation == message.generation {
		m.flash = 0
	}
	return true
}

func (m *Model) step(delta int) tea.Cmd {
	value, _ := strconv.ParseUint(*m.value, 10, m.bits)
	limit := uint64(1)<<m.bits - 1
	if delta < 0 {
		if value != 0 {
			value--
		}
		m.flash = -1
	} else {
		if value != limit {
			value++
		}
		m.flash = 1
	}
	*m.value = strconv.FormatUint(value, 10)
	m.generation++
	generation := m.generation
	return tea.Tick(flashDuration, func(time.Time) tea.Msg {
		return FlashExpiredMsg{input: m, generation: generation}
	})
}
