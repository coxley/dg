// Package clipboard implements diagram copy and export interaction.
package clipboard

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	native "github.com/coxley/dg/internal/tui/clipboard/native"
)

const (
	probeTimeout             = 100 * time.Millisecond
	payloadMIME              = "application/vnd.dg.fragment"
	payloadCompressThreshold = 4 << 10
	payloadLimit             = 64 << 20
	// DebounceDuration is the interval for distinguishing copy from export.
	DebounceDuration = 175 * time.Millisecond
)

var (
	payloadFormat = native.Register(payloadMIME)
	payloadMagic  = [4]byte{'D', 'G', 'F', 2}
)

const (
	payloadFlagGZIP   uint8 = 1 << iota
	payloadHeaderSize       = len(payloadMagic) + 1 + 4 + 4
)

type mode uint8

const (
	unknown mode = iota
	nativeClipboard
	probingTerminal
	terminal
)

// Style identifies an export wrapper.
type Style uint8

const (
	LineSlash Style = iota
	LineHash
	Block
	Markdown
)

const (
	exportStyle        chrome.ID = "export-style"
	exportCopy         chrome.ID = "export-copy"
	styleBlockValue              = "block"
	styleMarkdownValue           = "markdown"
)

type requestCopyMsg struct {
	text            string
	preferredPrefix string
	payload         []byte
	modifier        tea.KeyMod
}

type releaseCopyMsg struct {
	modifier tea.KeyMod
}

type probeExpiredMsg struct {
	generation uint64
}

type debounceExpiredMsg struct {
	generation uint64
}

type nativeWriteMsg struct {
	copy requestCopyMsg
	err  error
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

// PasteMsg contains structural clipboard data matching the pasted text.
type PasteMsg struct {
	Data []byte
}

// ErrorMsg reports a clipboard failure.
type ErrorMsg struct {
	Err error
}

func writeNative(text string, payload []byte) error {
	if err := native.Init(); err != nil {
		return err
	}
	values := []native.Data{{Format: native.FmtText, Bytes: []byte(text)}}
	if len(payload) != 0 {
		encoded, err := encodePayload(text, payload)
		if err != nil {
			return err
		}
		values = append(values, native.Data{
			Format: payloadFormat,
			Bytes:  encoded,
		})
	}
	_, err := native.WriteMany(values...)
	return err
}

func readNative() []byte {
	if native.Init() != nil {
		return nil
	}
	return native.Read(payloadFormat)
}

// Model owns clipboard capability, copy gestures, and export-form state.
type Model struct {
	mode          mode
	nativeWrite   func(string, []byte) error
	nativeRead    func() []byte
	pending       requestCopyMsg
	nativeErr     error
	probe         uint64
	armed         bool
	releaseEvents bool
	releaseArmed  bool
	copy          requestCopyMsg
	generation    uint64
	form          *chrome.Form
	exportText    string
	exportData    []byte
	style         Style
	styles        chrome.FormStyles
}

// New returns a clipboard model.
func New(styles chrome.FormStyles) *Model {
	return &Model{
		nativeWrite: writeNative,
		nativeRead:  readNative,
		styles:      styles,
	}
}

// RequestCopy returns a message that begins or advances a copy interaction.
func RequestCopy(
	text, preferredPrefix string,
	payload []byte,
	modifier tea.KeyMod,
) tea.Msg {
	return requestCopyMsg{
		text:            text,
		preferredPrefix: preferredPrefix,
		payload:         append([]byte(nil), payload...),
		modifier:        modifier,
	}
}

// ReleaseCopy returns a message that completes an armed copy on modifier release.
func ReleaseCopy(modifier tea.KeyMod) tea.Msg {
	return releaseCopyMsg{modifier: modifier}
}

// ReadPaste reads structural data when it matches the terminal's pasted text.
func (m *Model) ReadPaste(text string) tea.Cmd {
	return func() tea.Msg {
		return PasteMsg{Data: decodePayload(text, m.nativeRead())}
	}
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
	case releaseCopyMsg:
		return m, m.release(message)
	case debounceExpiredMsg:
		return m, m.handleDebounce(message)
	case tea.ClipboardMsg:
		return m, m.handleTerminalResponse()
	case probeExpiredMsg:
		return m, m.handleProbeTimeout(message)
	case nativeWriteMsg:
		if message.err != nil {
			return m, m.probeTerminal(message.copy, message.err)
		}
		m.mode = nativeClipboard
		return m, func() tea.Msg { return CopiedMsg{} }
	case chrome.FormSubmitMsg:
		if m.form != nil && message.ID == exportCopy {
			return m, m.completeExport()
		}
	default:
		if m.form != nil {
			form, command := m.form.Update(message)
			m.form = form.(*chrome.Form)
			if selected, ok := m.form.Selected(exportStyle); ok {
				m.style = parseStyle(selected)
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
	return m.form.View()
}

// SetStyles replaces the export form styles.
func (m *Model) SetStyles(styles chrome.FormStyles) {
	m.styles = styles
	if m.form != nil {
		m.form.SetStyles(styles)
	}
}

// SetBounds arranges the export form.
func (m *Model) SetBounds(width, height int) {
	if m.form != nil {
		m.form.SetBounds(chrome.Rect{Width: width, Height: height})
	}
}

// Click routes a local pointer cell through retained form geometry.
func (m *Model) Click(point chrome.Point) tea.Cmd {
	if m.form == nil {
		return nil
	}
	return wrap(m.form.Click(point))
}

// AccessibleLines returns export fields and executable actions.
func (m *Model) AccessibleLines() []string {
	if m.form == nil {
		return nil
	}
	return m.form.AccessibleLines()
}

// CancelPending invalidates a provisional first copy.
func (m *Model) CancelPending() {
	if !m.armed && m.copy.text == "" {
		return
	}
	m.armed = false
	m.releaseArmed = false
	m.copy = requestCopyMsg{}
	m.generation++
}

// CancelExport clears the export form.
func (m *Model) CancelExport() {
	m.form = nil
	m.exportText = ""
	m.exportData = nil
}

// UseNative configures the native writer for tests.
func (m *Model) UseNative(write func(string, []byte) error) {
	m.mode = nativeClipboard
	m.nativeWrite = write
}

// UseNativeReader configures the native reader for tests.
func (m *Model) UseNativeReader(read func() []byte) {
	m.nativeRead = read
}

// SetReleaseEvents selects modifier-release copy completion when supported.
func (m *Model) SetReleaseEvents(supported bool) {
	m.releaseEvents = supported
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
		m.openExport(message.text, message.preferredPrefix, message.payload)
		return func() tea.Msg { return OpenExportMsg{} }
	}
	m.armed = true
	m.releaseArmed = m.releaseEvents && message.modifier != 0
	m.copy = message
	m.generation++
	if m.releaseArmed {
		return nil
	}
	generation := m.generation
	return tea.Tick(DebounceDuration, func(time.Time) tea.Msg {
		return UpdateMsg{
			message: debounceExpiredMsg{generation: generation},
		}
	})
}

func (m *Model) release(message releaseCopyMsg) tea.Cmd {
	if !m.releaseArmed || !m.armed || message.modifier != m.copy.modifier {
		return nil
	}
	copy := m.copy
	m.armed = false
	m.releaseArmed = false
	m.copy = requestCopyMsg{}
	return m.write(copy)
}

func (m *Model) handleDebounce(message debounceExpiredMsg) tea.Cmd {
	if !m.armed || m.copy.text == "" || message.generation != m.generation {
		return nil
	}
	copy := m.copy
	m.armed = false
	m.releaseArmed = false
	m.copy = requestCopyMsg{}
	return m.write(copy)
}

func (m *Model) write(copy requestCopyMsg) tea.Cmd {
	switch m.mode {
	case terminal:
		return tea.Batch(
			tea.SetClipboard(copy.text),
			func() tea.Msg { return CopiedMsg{} },
		)
	case unknown, nativeClipboard:
		return wrap(func() tea.Msg {
			return nativeWriteMsg{
				copy: copy,
				err:  m.nativeWrite(copy.text, copy.payload),
			}
		})
	default:
		return nil
	}
}

func (m *Model) handleTerminalResponse() tea.Cmd {
	if m.mode != probingTerminal || m.pending.text == "" {
		return nil
	}
	copy := m.pending
	m.pending = requestCopyMsg{}
	m.nativeErr = nil
	m.mode = terminal
	return tea.Batch(
		tea.SetClipboard(copy.text),
		func() tea.Msg { return CopiedMsg{} },
	)
}

func (m *Model) handleProbeTimeout(message probeExpiredMsg) tea.Cmd {
	if m.mode != probingTerminal ||
		m.pending.text == "" ||
		message.generation != m.probe {
		return nil
	}
	err := m.nativeErr
	m.pending = requestCopyMsg{}
	m.nativeErr = nil
	m.mode = unknown
	return func() tea.Msg { return ErrorMsg{Err: err} }
}

func (m *Model) probeTerminal(copy requestCopyMsg, err error) tea.Cmd {
	m.nativeErr = err
	m.pending = copy
	if m.pending.text == "" {
		return func() tea.Msg { return ErrorMsg{Err: err} }
	}
	m.mode = probingTerminal
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
}

func (m *Model) openExport(text, preferredPrefix string, payload []byte) {
	m.exportText = text
	m.exportData = append(m.exportData[:0], payload...)
	m.style = styleForPrefix(preferredPrefix)
	m.form = chrome.NewForm(chrome.FormDeclaration{
		DefaultAction: exportCopy,
		Fields: []chrome.FormField{{
			ID: exportStyle, Label: "Copy selection as", Kind: chrome.SelectField,
			Options: exportOptions(m.style),
		}},
		Spacer: chrome.FormSpacer{ID: "export-spacer", Grow: 1},
		Actions: chrome.ButtonListDeclaration{
			ID: "export-actions",
			Buttons: []chrome.Button{{
				ID: exportCopy, Label: "Copy",
			}},
		},
	}, m.styles)
	m.form.SetBounds(chrome.Rect{Width: 46, Height: 5})
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

func exportOptions(preferred Style) []chrome.FormOption {
	labels := [...]string{
		"Line comments  //",
		"Line comments  #",
		"Block comment  /* ... */",
	}
	options := make([]chrome.FormOption, 0, len(labels)+1)
	options = append(options, chrome.FormOption{
		Label: labels[preferred], Value: styleValue(preferred),
	})
	for style, label := range labels {
		value := Style(style)
		if value != preferred {
			options = append(options, chrome.FormOption{
				Label: label, Value: styleValue(value),
			})
		}
	}
	return append(options, chrome.FormOption{
		Label: "Markdown code block", Value: styleValue(Markdown),
	})
}

func (m *Model) completeExport() tea.Cmd {
	text := Format(m.exportText, m.style)
	payload := m.exportData
	m.exportText = ""
	m.exportData = nil
	m.form = nil
	return tea.Batch(
		func() tea.Msg { return CloseExportMsg{} },
		m.write(requestCopyMsg{text: text, payload: payload}),
	)
}

func encodePayload(text string, payload []byte) ([]byte, error) {
	if len(payload) > payloadLimit {
		return nil, errors.New("clipboard fragment exceeds size limit")
	}
	if len(payload) < payloadCompressThreshold {
		return payloadEnvelope(text, payload, len(payload), 0), nil
	}
	var compressed bytes.Buffer
	compressed.Grow(len(payload) / 8)
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(payload); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if compressed.Len() >= len(payload) {
		return payloadEnvelope(text, payload, len(payload), 0), nil
	}
	return payloadEnvelope(
		text,
		compressed.Bytes(),
		len(payload),
		payloadFlagGZIP,
	), nil
}

func decodePayload(text string, encoded []byte) []byte {
	if len(encoded) < payloadHeaderSize ||
		string(encoded[:len(payloadMagic)]) != string(payloadMagic[:]) ||
		binary.LittleEndian.Uint32(encoded[len(payloadMagic)+1:len(payloadMagic)+5]) !=
			crc32.ChecksumIEEE([]byte(text)) {
		return nil
	}
	flags := encoded[len(payloadMagic)]
	size := binary.LittleEndian.Uint32(encoded[len(payloadMagic)+5 : payloadHeaderSize])
	if flags & ^payloadFlagGZIP != 0 || size > payloadLimit {
		return nil
	}
	body := encoded[payloadHeaderSize:]
	if flags == 0 {
		if uint64(len(body)) != uint64(size) {
			return nil
		}
		return append([]byte(nil), body...)
	}
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, int64(size)+1))
	closeErr := reader.Close()
	if err != nil || closeErr != nil || uint64(len(decoded)) != uint64(size) {
		return nil
	}
	return decoded
}

func payloadEnvelope(text string, body []byte, size int, flags uint8) []byte {
	encoded := make([]byte, payloadHeaderSize+len(body))
	copy(encoded, payloadMagic[:])
	encoded[len(payloadMagic)] = flags
	binary.LittleEndian.PutUint32(
		encoded[len(payloadMagic)+1:],
		crc32.ChecksumIEEE([]byte(text)),
	)
	binary.LittleEndian.PutUint32(encoded[len(payloadMagic)+5:], uint32(size))
	copy(encoded[payloadHeaderSize:], body)
	return encoded
}

func styleValue(style Style) string {
	switch style {
	case LineHash:
		return "line-hash"
	case Block:
		return styleBlockValue
	case Markdown:
		return styleMarkdownValue
	default:
		return "line-slash"
	}
}

func parseStyle(value string) Style {
	switch value {
	case "line-hash":
		return LineHash
	case styleBlockValue:
		return Block
	case styleMarkdownValue:
		return Markdown
	default:
		return LineSlash
	}
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
