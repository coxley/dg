// Package clipboard implements diagram copy and export interaction.
package clipboard

import (
	"strings"
	"sync"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	systemclipboard "golang.design/x/clipboard"
)

const (
	probeTimeout = 100 * time.Millisecond
	// DebounceDuration is the interval for distinguishing copy from export.
	DebounceDuration = 300 * time.Millisecond
)

type mode uint8

const (
	unknown mode = iota
	terminal
	fallback
)

// Style identifies an export wrapper.
type Style uint8

const (
	LineSlash Style = iota
	LineHash
	Block
	Markdown
)

type requestCopyMsg struct {
	text            string
	preferredPrefix string
}

type probeExpiredMsg struct {
	generation uint64
}

type debounceExpiredMsg struct {
	generation uint64
}

type fallbackMsg struct {
	err error
}

// UpdateMsg routes a child command back to Model.Update.
type UpdateMsg struct {
	message tea.Msg
}

// OpenExportMsg requests presentation of the export form.
type OpenExportMsg struct{}

// CloseExportMsg requests closure of the export form.
type CloseExportMsg struct{}

// CopiedMsg reports a successful clipboard write.
type CopiedMsg struct{}

// ErrorMsg reports a clipboard failure.
type ErrorMsg struct {
	Err error
}

var (
	initOnce sync.Once
	errInit  error
)

func writeFallback(text string) error {
	initOnce.Do(func() {
		errInit = systemclipboard.Init()
	})
	if errInit != nil {
		return errInit
	}
	systemclipboard.Write(systemclipboard.FmtText, []byte(text))
	return nil
}

// Model owns clipboard capability, debounce, and export-form state.
type Model struct {
	mode       mode
	fallback   func(string) error
	pending    string
	probe      uint64
	armed      bool
	copy       string
	generation uint64
	form       *huh.Form
	exportText string
	style      Style
	theme      huh.Theme
}

// New returns a clipboard model.
func New(theme huh.Theme) *Model {
	return &Model{
		fallback: writeFallback,
		theme:    theme,
	}
}

// RequestCopy returns a message that begins or advances a copy interaction.
func RequestCopy(text, preferredPrefix string) tea.Msg {
	return requestCopyMsg{text: text, preferredPrefix: preferredPrefix}
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
	switch message := message.(type) {
	case requestCopyMsg:
		return m, m.request(message)
	case debounceExpiredMsg:
		return m, m.handleDebounce(message)
	case tea.ClipboardMsg:
		return m, m.handleTerminalResponse()
	case probeExpiredMsg:
		return m, m.handleProbeTimeout(message)
	case fallbackMsg:
		if message.err != nil {
			m.CancelPending()
			return m, func() tea.Msg { return ErrorMsg{Err: message.err} }
		}
		return m, func() tea.Msg { return CopiedMsg{} }
	default:
		if m.form != nil {
			form, command := m.form.Update(message)
			m.form = form.(*huh.Form)
			if m.form.State == huh.StateCompleted {
				text := Format(m.exportText, m.style)
				m.exportText = ""
				m.form = nil
				return m, tea.Batch(
					func() tea.Msg { return CloseExportMsg{} },
					m.write(text),
				)
			}
			return m, wrap(command)
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	if m.form == nil {
		return tea.View{}
	}
	return tea.NewView(m.form.View())
}

// SetTheme replaces the export form theme.
func (m *Model) SetTheme(theme huh.Theme) {
	m.theme = theme
	if m.form != nil {
		m.form.WithTheme(theme)
	}
}

// CancelPending invalidates a provisional first copy.
func (m *Model) CancelPending() {
	if !m.armed && m.copy == "" {
		return
	}
	m.armed = false
	m.copy = ""
	m.generation++
}

// CancelExport clears the export form.
func (m *Model) CancelExport() {
	m.form = nil
	m.exportText = ""
}

// UseFallback configures a fallback writer and skips terminal probing.
func (m *Model) UseFallback(write func(string) error) {
	m.mode = fallback
	m.fallback = write
}

// CopyGeneration returns the active debounce generation.
func (m *Model) CopyGeneration() uint64 {
	return m.generation
}

// Style returns the selected export style.
func (m *Model) Style() Style {
	return m.style
}

func (m *Model) request(message requestCopyMsg) tea.Cmd {
	if m.armed {
		m.CancelPending()
		m.openExport(message.text, message.preferredPrefix)
		return func() tea.Msg { return OpenExportMsg{} }
	}
	m.armed = true
	m.copy = message.text
	m.generation++
	generation := m.generation
	return tea.Tick(DebounceDuration, func(time.Time) tea.Msg {
		return UpdateMsg{
			message: debounceExpiredMsg{generation: generation},
		}
	})
}

func (m *Model) handleDebounce(message debounceExpiredMsg) tea.Cmd {
	if !m.armed || m.copy == "" || message.generation != m.generation {
		return nil
	}
	text := m.copy
	m.armed = false
	m.copy = ""
	return m.write(text)
}

func (m *Model) write(text string) tea.Cmd {
	switch m.mode {
	case terminal:
		return tea.Batch(
			tea.SetClipboard(text),
			func() tea.Msg { return CopiedMsg{} },
		)
	case fallback:
		return wrap(func() tea.Msg {
			return fallbackMsg{err: m.fallback(text)}
		})
	case unknown:
		m.pending = text
		m.probe++
		generation := m.probe
		return tea.Batch(
			func() tea.Msg { return tea.ReadClipboard() },
			tea.Tick(probeTimeout, func(time.Time) tea.Msg {
				return UpdateMsg{
					message: probeExpiredMsg{generation: generation},
				}
			}),
		)
	default:
		return nil
	}
}

func (m *Model) handleTerminalResponse() tea.Cmd {
	if m.mode != unknown || m.pending == "" {
		return nil
	}
	text := m.pending
	m.pending = ""
	m.mode = terminal
	return tea.Batch(
		tea.SetClipboard(text),
		func() tea.Msg { return CopiedMsg{} },
	)
}

func (m *Model) handleProbeTimeout(message probeExpiredMsg) tea.Cmd {
	if m.mode != unknown ||
		m.pending == "" ||
		message.generation != m.probe {
		return nil
	}
	text := m.pending
	m.pending = ""
	m.mode = fallback
	return m.write(text)
}

func (m *Model) openExport(text, preferredPrefix string) {
	m.exportText = text
	m.style = styleForPrefix(preferredPrefix)
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[Style]().
				Title("Copy selection as").
				Options(options(m.style)...).
				Value(&m.style),
		),
	).
		WithWidth(46).
		WithHeight(7).
		WithShowHelp(true).
		WithTheme(m.theme)
	_ = m.form.Init()
}

func styleForPrefix(prefix string) Style {
	switch normalizePrefix(prefix) {
	case "# ":
		return LineHash
	case "/* */":
		return Block
	default:
		return LineSlash
	}
}

func options(preferred Style) []huh.Option[Style] {
	labels := [...]string{
		"Line comments  //",
		"Line comments  #",
		"Block comment  /* ... */",
	}
	options := make([]huh.Option[Style], 0, len(labels)+1)
	options = append(options, huh.NewOption(labels[preferred], preferred))
	for style, label := range labels {
		value := Style(style)
		if value != preferred {
			options = append(options, huh.NewOption(label, value))
		}
	}
	return append(options, huh.NewOption("Markdown code block", Markdown))
}

// Format trims line endings and applies style.
func Format(text string, style Style) string {
	text = TrimTrailingWhitespace(text)
	switch style {
	case LineSlash:
		return prefixLines(text, "// ")
	case LineHash:
		return prefixLines(text, "# ")
	case Block:
		return "/*\n" + text + "\n*/"
	case Markdown:
		return "```\n" + text + "\n```"
	default:
		return text
	}
}

// TrimTrailingWhitespace removes trailing whitespace from every line.
func TrimTrailingWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

func prefixLines(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(prefix+lines[i], unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

func normalizePrefix(prefix string) string {
	switch prefix {
	case "# ", "/* */":
		return prefix
	default:
		return "// "
	}
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
