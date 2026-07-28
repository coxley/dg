package tui

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

const stepperFlashDuration = 120 * time.Millisecond

type stepperField struct {
	title      string
	value      *string
	bits       int
	theme      Theme
	focused    bool
	flash      int
	generation uint64
	width      int
	height     int
	position   huh.FieldPosition
	previous   key.Binding
	next       key.Binding
}

type stepperFlashMsg struct {
	field      *stepperField
	generation uint64
}

func newStepperField(title string, value *string, bits int, theme Theme) *stepperField {
	return &stepperField{
		title: title,
		value: value,
		bits:  bits,
		theme: theme,
		previous: key.NewBinding(
			key.WithKeys("up", "shift+tab"),
			key.WithHelp("↑", "previous"),
		),
		next: key.NewBinding(
			key.WithKeys("down", "enter", "tab"),
			key.WithHelp("↓", "next"),
		),
	}
}

func (f *stepperField) Init() tea.Cmd {
	return nil
}

func (f *stepperField) Update(message tea.Msg) (huh.Model, tea.Cmd) {
	keyPress, ok := message.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}
	switch {
	case key.Matches(keyPress, f.previous):
		return f, huh.PrevField
	case key.Matches(keyPress, f.next):
		return f, huh.NextField
	case keyPress.Code == tea.KeyLeft:
		return f, f.step(-1)
	case keyPress.Code == tea.KeyRight:
		return f, f.step(1)
	default:
		return f, nil
	}
}

func (f *stepperField) step(delta int) tea.Cmd {
	value, _ := strconv.ParseUint(*f.value, 10, f.bits)
	limit := uint64(1)<<f.bits - 1
	if delta < 0 {
		if value != 0 {
			value--
		}
		f.flash = -1
	} else {
		if value != limit {
			value++
		}
		f.flash = 1
	}
	*f.value = strconv.FormatUint(value, 10)
	f.generation++
	generation := f.generation
	return tea.Tick(stepperFlashDuration, func(time.Time) tea.Msg {
		return stepperFlashMsg{field: f, generation: generation}
	})
}

func (f *stepperField) clearFlash(generation uint64) {
	if f.generation == generation {
		f.flash = 0
	}
}

func (f *stepperField) View() string {
	title := f.theme.StepperTitle.Render(f.title)
	if !f.focused {
		return title + "  " + *f.value
	}
	title = f.theme.StepperTitleFocus.Render(f.title)
	left, right := f.theme.Button.Render("⇽"), f.theme.Button.Render("⇾")
	if f.flash < 0 {
		left = f.theme.ButtonActive.Render("⇽")
	} else if f.flash > 0 {
		right = f.theme.ButtonActive.Render("⇾")
	}
	return title + "  " + left + " " + *f.value + " " + right
}

func (f *stepperField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *stepperField) Blur() tea.Cmd {
	f.focused = false
	f.flash = 0
	return nil
}

func (*stepperField) Error() error {
	return nil
}

func (*stepperField) Run() error {
	return nil
}

func (f *stepperField) RunAccessible(writer io.Writer, _ io.Reader) error {
	_, err := fmt.Fprintf(writer, "%s: %s\n", f.title, *f.value)
	return err
}

func (*stepperField) Skip() bool {
	return false
}

func (*stepperField) Zoom() bool {
	return false
}

func (f *stepperField) KeyBinds() []key.Binding {
	return []key.Binding{f.previous, f.next}
}

func (f *stepperField) WithTheme(huh.Theme) huh.Field {
	return f
}

func (f *stepperField) WithKeyMap(*huh.KeyMap) huh.Field {
	return f
}

func (f *stepperField) WithWidth(width int) huh.Field {
	f.width = width
	return f
}

func (f *stepperField) WithHeight(height int) huh.Field {
	f.height = height
	return f
}

func (f *stepperField) WithPosition(position huh.FieldPosition) huh.Field {
	f.position = position
	f.previous.SetEnabled(!position.IsFirst())
	f.next.SetEnabled(!position.IsLast())
	return f
}

func (f *stepperField) GetKey() string {
	return f.title
}

func (f *stepperField) GetValue() any {
	return *f.value
}
