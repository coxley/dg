// Package numinput implements a bounded integer stepper.
package numinput

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/exp/constraints"
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
	generationAddress *uint64
	generation        uint64
}

// Model owns a bounded integer value and its interaction state.
type Model[T constraints.Integer] struct {
	title      string
	value      *T
	max        T
	styles     Styles
	width      int
	focused    bool
	flash      int
	generation uint64
}

// New returns a numeric input bound to value.
func New[T constraints.Integer](
	title string,
	value *T,
	maxValue T,
	styles Styles,
) *Model[T] {
	var zero T
	maxValue = max(maxValue, zero)
	*value = min(max(*value, zero), maxValue)
	return &Model[T]{
		title:  title,
		value:  value,
		max:    maxValue,
		styles: styles,
	}
}

// Init implements tea.Model.
func (*Model[T]) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model[T]) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		switch message.Code {
		case tea.KeyLeft:
			return m, m.step(-1)
		case tea.KeyRight:
			return m, m.step(1)
		default:
			switch message.Text {
			case "h":
				return m, m.step(-1)
			case "l":
				return m, m.step(1)
			}
		}
	case FlashExpiredMsg:
		m.HandleFlash(message)
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model[T]) View() tea.View {
	return tea.NewView(m.Render())
}

// Render returns the styled input content for embedding in another model.
func (m *Model[T]) Render() string {
	value := strconv.FormatUint(uint64(*m.value), 10)
	title := m.styles.Title.Render(m.title)
	if !m.focused {
		return justifyApart(title, value, m.width)
	}
	title = m.styles.FocusedTitle.Render(m.title)
	left, right := m.styles.Button.Render("⇽"), m.styles.Button.Render("⇾")
	if m.flash < 0 {
		left = m.styles.ActiveButton.Render("⇽")
	} else if m.flash > 0 {
		right = m.styles.ActiveButton.Render("⇾")
	}
	return justifyApart(
		title,
		left+" "+value+" "+right,
		m.width,
	)
}

// SetStyles replaces the input's visual styles.
func (m *Model[T]) SetStyles(styles Styles) {
	m.styles = styles
}

// SetWidth sets the rendered row width.
func (m *Model[T]) SetWidth(width int) {
	m.width = max(width, 0)
}

// SetFocused controls focused rendering.
func (m *Model[T]) SetFocused(focused bool) {
	m.focused = focused
	if !focused {
		m.flash = 0
	}
}

// Flash reports the active direction: -1 for left, 1 for right, or 0.
func (m *Model[T]) Flash() int {
	return m.flash
}

// HandleFlash consumes a matching flash-expiration message.
func (m *Model[T]) HandleFlash(message FlashExpiredMsg) bool {
	if message.generationAddress != &m.generation {
		return false
	}
	if m.generation == message.generation {
		m.flash = 0
	}
	return true
}

func (m *Model[T]) step(delta int) tea.Cmd {
	if delta < 0 {
		if *m.value != 0 {
			*m.value--
		}
		m.flash = -1
	} else {
		if *m.value != m.max {
			*m.value++
		}
		m.flash = 1
	}
	m.generation++
	generation := m.generation
	return tea.Tick(flashDuration, func(time.Time) tea.Msg {
		return FlashExpiredMsg{
			generationAddress: &m.generation,
			generation:        generation,
		}
	})
}

func justifyApart(left, right string, width int) string {
	if width <= 0 {
		return left + "  " + right
	}
	rightWidth := ansi.StringWidth(right)
	leftWidth := max(width-rightWidth-1, 0)
	left = ansi.Truncate(left, leftWidth, "")
	return left +
		strings.Repeat(" ", max(width-ansi.StringWidth(left)-rightWidth, 0)) +
		right
}
