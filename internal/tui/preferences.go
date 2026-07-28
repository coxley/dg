package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/layout"
	bubbletab "github.com/mJehanno/bubble-tab"
	tabmodel "github.com/mJehanno/bubble-tab/pkg/model"
	tabtheme "github.com/mJehanno/bubble-tab/pkg/theme"
)

type preferencesFile struct {
	Router        layout.Router `json:"router"`
	ApplyToFuture bool          `json:"apply_to_future"`
	SaveDirectory string        `json:"save_directory,omitempty"`
	CommentPrefix string        `json:"comment_prefix,omitempty"`
}

type preferenceState struct {
	router                layout.Router
	originalRouter        layout.Router
	applyToFuture         bool
	originalApplyToFuture bool
	saveDirectory         string
	originalSaveDirectory string
	commentPrefix         string
	originalCommentPrefix string
	path                  string
}

type preferenceFormValues struct {
	step          string
	sharedStep    string
	bend          string
	crossing      string
	endpoint      string
	reroutePasses string
	applyToFuture bool
	saveDirectory string
	commentPrefix string
}

type preferenceField struct {
	title string
	value *string
	input *huh.Input
}

type componentKind uint8

const (
	settingsComponent componentKind = iota
	exportComponent
	saveComponent
)

type componentMsg struct {
	kind    componentKind
	message tea.Msg
}

type settingsTabMouseMsg struct {
	tab     modal
	message tea.Msg
}

type staticTabBody string

func (staticTabBody) Init() tea.Cmd {
	return nil
}

func (body staticTabBody) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return body, nil
}

func (body staticTabBody) View() tea.View {
	return tea.NewView(string(body))
}

type formTabBody struct {
	form *huh.Form
}

func (body formTabBody) Init() tea.Cmd {
	return body.form.Init()
}

func (body formTabBody) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	form, command := body.form.Update(message)
	body.form = form.(*huh.Form)
	return body, command
}

func (body formTabBody) View() tea.View {
	return tea.NewView(body.form.View())
}

func preferencesPath() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	name := "dg"
	if runtime.GOOS == "darwin" {
		name = "org.coxley.dg"
	}
	return filepath.Join(cache, name, "preferences.json"), nil
}

func readPreferences() (preferencesFile, string, error) {
	path, err := preferencesPath()
	if err != nil {
		return preferencesFile{}, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return preferencesFile{}, path, nil
	}
	if err != nil {
		return preferencesFile{}, path, err
	}
	var preferences preferencesFile
	if err := json.Unmarshal(data, &preferences); err != nil {
		return preferencesFile{}, path, err
	}
	return preferences, path, nil
}

func (m *Model) loadPreferences() {
	preferences, path, err := readPreferences()
	if err != nil {
		m.setError("load preferences: " + err.Error())
		return
	}
	m.preferences.path = path
	m.preferences.applyToFuture = preferences.ApplyToFuture
	m.preferences.saveDirectory = preferences.SaveDirectory
	m.preferences.commentPrefix = normalizeCommentPrefix(preferences.CommentPrefix)
}

// PreferredRouter returns the persisted router for newly created diagrams.
func PreferredRouter() (layout.Router, bool) {
	preferences, _, err := readPreferences()
	return preferences.Router, err == nil && preferences.ApplyToFuture
}

func (m *Model) openHelp() {
	if m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	m.resetSettingsTabs(modalHelp)
	m.modal = modalHelp
	m.preferenceEdit = false
	m.status = ""
}

func (m *Model) openPreferences() {
	if m.preferenceForm == nil {
		m.resetSettingsTabs(modalPreferences)
	} else {
		m.selectSettingsTab(modalPreferences)
	}
	m.beginPreferenceEdit()
	m.modal = modalPreferences
}

func (m *Model) beginPreferenceEdit() {
	if m.preferenceEdit {
		return
	}
	m.preferenceEdit = true
	m.preferences.originalRouter = m.geo.Router()
	m.preferences.router = m.preferences.originalRouter
	m.preferences.originalApplyToFuture = m.preferences.applyToFuture
	m.preferences.originalSaveDirectory = m.preferences.saveDirectory
	m.preferences.originalCommentPrefix = m.preferences.commentPrefix
	m.beginTransaction()
}

func (m *Model) updateModal(message tea.KeyPressMsg) tea.Cmd {
	key := message.Key()
	if m.settingsModalCloses(key) {
		m.closeSettingsModal()
		return nil
	}
	switch m.modal {
	case modalNone:
		return nil
	case modalHelp:
		switch {
		case key.Code == tea.KeyEscape || key.Code == '?' || key.Code == tea.KeyEnter:
			m.closeSettingsModal()
			return nil
		case key.Code == 'p' && key.Mod == 0:
			m.openPreferences()
			return nil
		}
	case modalPreferences:
		if key.Code == tea.KeyEscape {
			m.closeSettingsModal()
			return nil
		}
	case modalExport:
		if key.Code == tea.KeyEscape {
			m.modal = modalNone
			m.exportText = ""
			return nil
		}
		return m.updateExportForm(message)
	case modalSave:
		if key.Code == tea.KeyEscape {
			m.closeSaveForm()
			return nil
		}
		if key.Code == 's' && key.Mod == tea.ModCtrl {
			m.commitSaveForm()
			return nil
		}
		return m.updateSaveForm(message)
	case modalNotice:
		return nil
	}
	if key.Code == tea.KeyTab && (key.Mod == 0 || key.Mod == tea.ModShift) {
		if m.modal == modalHelp {
			m.beginPreferenceEdit()
			m.modal = modalPreferences
		} else {
			m.modal = modalHelp
		}
	} else if m.modal == modalHelp && key.Code == '1' && key.Mod == 0 {
		m.modal = modalHelp
	} else if m.modal == modalHelp && key.Code == '2' && key.Mod == 0 {
		m.openPreferences()
	}
	return m.updateSettingsTabs(message)
}

func (m *Model) settingsModalCloses(key tea.Key) bool {
	return (m.modal == modalHelp || m.modal == modalPreferences) &&
		key.Code == 'q' && key.Mod == 0
}

func (m *Model) closeSettingsModal() {
	var err error
	if m.preferenceEdit {
		hadTransaction := m.transactionOpen
		err = m.cancelTransaction()
		if !hadTransaction {
			m.geo.SetRouter(m.preferences.originalRouter)
			err = errors.Join(err, m.geo.Build())
		}
		m.preferences.router = m.preferences.originalRouter
		m.preferences.applyToFuture = m.preferences.originalApplyToFuture
		m.preferences.saveDirectory = m.preferences.originalSaveDirectory
		m.preferences.commentPrefix = m.preferences.originalCommentPrefix
		err = errors.Join(err, m.render())
	}
	m.preferenceEdit = false
	m.modal = modalNone
	if err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) applyPreferences() tea.Cmd {
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return nil
	}
	m.preferenceEdit = false
	if m.preferences.path == "" {
		path, err := preferencesPath()
		if err != nil {
			m.setError(err.Error())
			return nil
		}
		m.preferences.path = path
	}
	data, err := json.MarshalIndent(preferencesFile{
		Router:        m.preferences.router,
		ApplyToFuture: m.preferences.applyToFuture,
		SaveDirectory: m.preferences.saveDirectory,
		CommentPrefix: m.preferences.commentPrefix,
	}, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(m.preferences.path), 0o700)
	}
	if err == nil {
		err = os.WriteFile(m.preferences.path, append(data, '\n'), 0o600)
	}
	m.resetSettingsTabs(modalHelp)
	if err != nil {
		m.setError("save preferences: " + err.Error())
		m.modal = modalHelp
		return nil
	}
	m.status = ""
	return m.showNotice("Preferences saved", modalHelp)
}

func (m *Model) resetSettingsTabs(active modal) {
	m.preferenceInput = preferenceFormValuesFrom(
		m.geo.Router(),
		m.preferences.applyToFuture,
		m.preferences.saveDirectory,
		m.preferences.commentPrefix,
	)
	m.preferenceForm, m.preferenceFields = newPreferenceForm(
		&m.preferenceInput,
		m.preferenceFormHeight(),
	)
	tabs := []tabmodel.Tab{
		*tabmodel.NewTab(
			tabmodel.WithName("Shortcuts"),
			tabmodel.WithBody(staticTabBody(shortcutContent())),
		),
		*tabmodel.NewTab(
			tabmodel.WithName("Preferences"),
			tabmodel.WithBody(formTabBody{form: m.preferenceForm}),
		),
	}
	m.settingsTabs = *bubbletab.New(
		bubbletab.WithTabs(tabs),
		bubbletab.WithCurrent(settingsTabIndex(active)),
		bubbletab.WithStyles(settingsTabStyles()),
		bubbletab.WithMouseMode(tea.MouseModeNone),
	)
	_ = m.settingsTabs.Init()
}

func (m *Model) selectSettingsTab(tab modal) {
	code := rune('1' + settingsTabIndex(tab))
	updated, _ := m.settingsTabs.Update(
		tea.KeyPressMsg(tea.Key{Code: code, Text: string(code)}),
	)
	m.settingsTabs = updated.(bubbletab.TabModel)
}

func (m *Model) updateSettingsTabs(message tea.Msg) tea.Cmd {
	var command tea.Cmd
	if key, ok := message.(tea.KeyPressMsg); ok &&
		m.modal == modalPreferences &&
		key.Code >= '0' && key.Code <= '9' {
		_, command = m.preferenceForm.Update(message)
	} else {
		updated, cmd := m.settingsTabs.Update(message)
		m.settingsTabs = updated.(bubbletab.TabModel)
		command = cmd
	}
	m.syncPreferenceForm()
	if m.preferenceForm != nil && m.preferenceForm.State == huh.StateCompleted {
		return tea.Batch(
			componentCommand(settingsComponent, command),
			m.applyPreferences(),
		)
	}
	return componentCommand(settingsComponent, command)
}

func (m *Model) updateSettingsWheel(message tea.MouseWheelMsg) tea.Cmd {
	var code rune
	switch message.Mouse().Button {
	case tea.MouseWheelUp:
		code = tea.KeyUp
	case tea.MouseWheelDown:
		code = tea.KeyDown
	default:
		return nil
	}
	return m.updateSettingsTabs(tea.KeyPressMsg(tea.Key{Code: code}))
}

func componentCommand(kind componentKind, command tea.Cmd) tea.Cmd {
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
				batch[i] = componentCommand(kind, batch[i])
			}
			return batch
		}
		return componentMsg{kind: kind, message: message}
	}
}

func settingsTabIndex(tab modal) int {
	if tab == modalPreferences {
		return 1
	}
	return 0
}

func settingsTabStyles() tabtheme.Styles {
	active := lipgloss.NewStyle().Bold(true).Underline(true).Padding(0, 1)
	inactive := lipgloss.NewStyle().Faint(true).Padding(0, 1)
	body := lipgloss.NewStyle().PaddingTop(1)
	return tabtheme.Styles{
		ActiveHeader:   active,
		InactiveHeader: inactive,
		DisabledHeader: inactive,
		ActiveBody:     body,
		InactiveBody:   body,
		DisabledBody:   body,
	}
}

func preferenceFormValuesFrom(
	router layout.Router,
	applyToFuture bool,
	saveDirectory string,
	commentPrefix string,
) preferenceFormValues {
	return preferenceFormValues{
		step:          strconv.FormatUint(uint64(router.Costs.Step), 10),
		sharedStep:    strconv.FormatUint(uint64(router.Costs.SharedStep), 10),
		bend:          strconv.FormatUint(uint64(router.Costs.Bend), 10),
		crossing:      strconv.FormatUint(uint64(router.Costs.Crossing), 10),
		endpoint:      strconv.FormatUint(uint64(router.Costs.EndpointStep), 10),
		reroutePasses: strconv.FormatUint(uint64(router.ReroutePasses), 10),
		applyToFuture: applyToFuture,
		saveDirectory: saveDirectory,
		commentPrefix: normalizeCommentPrefix(commentPrefix),
	}
}

func newPreferenceForm(
	values *preferenceFormValues,
	height int,
) (*huh.Form, []preferenceField) {
	keymap := preferenceKeyMap()
	inputs := []preferenceField{
		preferenceNumberField("Step cost", &values.step, 32),
		preferenceNumberField("Shared-step cost", &values.sharedStep, 32),
		preferenceNumberField("Bend cost", &values.bend, 32),
		preferenceNumberField("Crossing cost", &values.crossing, 32),
		preferenceNumberField("Endpoint cost", &values.endpoint, 32),
		preferenceNumberField("Reroute passes", &values.reroutePasses, 8),
		preferenceTextField("Default save directory", &values.saveDirectory),
	}
	fields := []huh.Field{
		inputs[0].input,
		inputs[1].input,
		inputs[2].input,
		inputs[3].input,
		inputs[4].input,
		inputs[5].input,
		huh.NewSelect[bool]().
			Options(
				huh.NewOption(
					preferenceOption("Apply to future diagrams?", "Yes"),
					true,
				),
				huh.NewOption(
					preferenceOption("Apply to future diagrams?", "No"),
					false,
				),
			).
			Inline(true).
			Value(&values.applyToFuture),
		inputs[6].input,
		huh.NewSelect[string]().
			Options(
				huh.NewOption(
					preferenceOption("Preferred comments", "//"),
					"// ",
				),
				huh.NewOption(
					preferenceOption("Preferred comments", "#"),
					"# ",
				),
				huh.NewOption(
					preferenceOption("Preferred comments", "/* */"),
					"/* */",
				),
			).
			Inline(true).
			Value(&values.commentPrefix),
	}
	form := huh.NewForm(huh.NewGroup(fields...)).
		WithWidth(preferenceModalWidth - 4).
		WithHeight(height).
		WithShowHelp(false).
		WithKeyMap(keymap).
		WithTheme(preferenceFormTheme())
	return form, inputs
}

func preferenceKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Input.Prev = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "previous"))
	keymap.Input.Next = key.NewBinding(key.WithKeys("down", "enter"), key.WithHelp("↓", "next"))
	keymap.Confirm.Prev = keymap.Input.Prev
	keymap.Confirm.Next = keymap.Input.Next
	keymap.Select.Prev = keymap.Input.Prev
	keymap.Select.Next = keymap.Input.Next
	keymap.Select.Up = key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "choice"))
	keymap.Select.Down = key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "choice"))
	return keymap
}

func preferenceNumberField(title string, value *string, bits int) preferenceField {
	field := preferenceTextField(title, value)
	field.input.Validate(func(text string) error {
		if _, err := strconv.ParseUint(text, 10, bits); err != nil {
			return fmt.Errorf("enter an unsigned %d-bit integer", bits)
		}
		return nil
	})
	return field
}

func preferenceTextField(title string, value *string) preferenceField {
	input := huh.NewInput().
		Title(title).
		Prompt(preferencePrompt(title, *value)).
		Inline(true).
		Value(value)
	return preferenceField{title: title, value: value, input: input}
}

func preferencePrompt(title, value string) string {
	const valueColumnEnd = 58
	return strings.Repeat(" ", max(valueColumnEnd-len(title)-len(value), 1))
}

func preferenceOption(title, value string) string {
	return title + preferencePrompt(title, value) + value
}

func preferenceFormTheme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		styles := huh.ThemeCharm(isDark)
		styles.FieldSeparator = lipgloss.NewStyle().SetString("\n")
		styles.Focused.Base = lipgloss.NewStyle()
		styles.Blurred.Base = lipgloss.NewStyle()
		styles.Focused.Title = styles.Focused.Title.Bold(true)
		return styles
	})
}

func (m *Model) syncPreferenceForm() {
	if m.preferenceForm == nil {
		return
	}
	for _, field := range m.preferenceFields {
		field.input.Prompt(preferencePrompt(field.title, *field.value))
	}
	router := m.preferences.router
	parse32 := func(text string, destination *uint32) {
		if value, err := strconv.ParseUint(text, 10, 32); err == nil {
			*destination = uint32(value)
		}
	}
	parse32(m.preferenceInput.step, &router.Costs.Step)
	parse32(m.preferenceInput.sharedStep, &router.Costs.SharedStep)
	parse32(m.preferenceInput.bend, &router.Costs.Bend)
	parse32(m.preferenceInput.crossing, &router.Costs.Crossing)
	parse32(m.preferenceInput.endpoint, &router.Costs.EndpointStep)
	if value, err := strconv.ParseUint(m.preferenceInput.reroutePasses, 10, 8); err == nil {
		router.ReroutePasses = uint8(value)
	}
	m.preferences.applyToFuture = m.preferenceInput.applyToFuture
	m.preferences.saveDirectory = m.preferenceInput.saveDirectory
	m.preferences.commentPrefix = normalizeCommentPrefix(m.preferenceInput.commentPrefix)
	if router == m.preferences.router {
		return
	}
	m.preferences.router = router
	m.geo.SetRouter(router)
	if err := m.rebuild(); err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) preferenceFormHeight() int {
	const (
		preferenceCount = 9
		modalFrameRows  = 4
	)
	if m.height == 0 {
		return preferenceCount + 1
	}
	return min(preferenceCount+1, max(m.diagramHeight()-modalFrameRows, 1))
}

func (m *Model) resizePreferenceForm() {
	if m.preferenceForm != nil {
		m.preferenceForm.WithHeight(m.preferenceFormHeight())
	}
}
