// Command chrome-lab exercises completed declarative chrome mechanics.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
)

const (
	scenarioLayout   = "layout"
	scenarioPane     = "pane"
	scenarioViewport = "viewport"
	scenarioMenu     = "menu"
	scenarioDensity  = "density"
	scenarioOverflow = "overflow"
	scenarioFocus    = "focus"
)

var scenarioNames = [...]string{
	scenarioLayout,
	scenarioPane,
	scenarioViewport,
	scenarioMenu,
	scenarioDensity,
	scenarioOverflow,
	scenarioFocus,
}

type labModel struct {
	width    int
	height   int
	scenario int
	density  chrome.Density

	menu            *chrome.Menu
	contentViewport *chrome.Viewport
	contentPane     *chrome.Pane
	diagnostics     *chrome.Viewport
	diagnosticsPane *chrome.Pane

	pointer         chrome.Point
	pointerCaptured bool
	clicks          int
	profile         chrome.KeyProfile
	text            string
	resolver        *chrome.Resolver
	focus           *chrome.FocusRegistry
}

func newLabModel(scenario string) *labModel {
	index := 0
	for i, name := range scenarioNames {
		if name == scenario {
			index = i
			break
		}
	}
	items := make([]chrome.MenuItem, len(scenarioNames))
	for i, name := range scenarioNames {
		items[i] = chrome.MenuItem{
			ID:    chrome.ID(name),
			Label: " " + abbreviatedScenario(name) + " ",
		}
	}
	menu := chrome.NewMenu(
		"scenario-menu",
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
		items,
	)
	content := chrome.NewViewport("scenario-body")
	contentPane := chrome.NewPane("scenario-pane", content)
	contentPane.SetHeader([]string{"SCENARIO"})
	contentPane.SetFooter([]string{"arrows/wheel scroll"})
	diagnostics := chrome.NewViewport("diagnostics-body")
	diagnosticsPane := chrome.NewPane("diagnostics-pane", diagnostics)
	diagnosticsPane.SetHeader([]string{"DIAGNOSTICS"})
	resolver, err := chrome.NewResolver([]chrome.Binding{
		{Scope: "field", Chords: chrome.Keys("q"), Command: "field-q", Label: "type q"},
		{Scope: "global", Chords: []chrome.Chord{chrome.Primary(",")}, Command: "preferences", Label: "preferences"},
	})
	if err != nil {
		panic(err)
	}
	resolver.SetSuperAvailable(true)
	focus := chrome.NewFocusRegistry()
	focus.Register("field", []chrome.FocusTarget{{ID: "text", Enabled: true}})
	focus.Open("field")
	return &labModel{
		scenario:        index,
		menu:            menu,
		contentViewport: content,
		contentPane:     contentPane,
		diagnostics:     diagnostics,
		diagnosticsPane: diagnosticsPane,
		resolver:        resolver,
		focus:           focus,
	}
}

func abbreviatedScenario(name string) string {
	switch name {
	case scenarioViewport:
		return "View"
	case scenarioDensity:
		return "Dense"
	case scenarioOverflow:
		return "Over"
	default:
		return strings.ToUpper(name[:1]) + name[1:]
	}
}

func (m *labModel) Init() tea.Cmd {
	return nil
}

func (m *labModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = max(message.Width, 0)
		m.height = max(message.Height, 0)
		m.reflow()
	case tea.KeyPressMsg:
		if scenarioNames[m.scenario] == scenarioFocus && message.String() == "p" {
			if m.profile == chrome.ProfileMac {
				m.profile = chrome.ProfileStandard
			} else {
				m.profile = chrome.ProfileMac
			}
			m.resolver.SetProfile(m.profile)
			m.reflow()
			return m, nil
		}
		if scenarioNames[m.scenario] == scenarioFocus &&
			message.Text != "" && message.Mod == 0 {
			m.text += message.Text
			m.reflow()
			return m, nil
		}
		switch message.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right":
			m.selectScenario((m.scenario + 1) % len(scenarioNames))
		case "shift+tab", "left":
			m.selectScenario((m.scenario + len(scenarioNames) - 1) % len(scenarioNames))
		case "up":
			m.contentViewport.Scroll(0, -1)
			m.refreshDiagnostics()
		case "down":
			m.contentViewport.Scroll(0, 1)
			m.refreshDiagnostics()
		case "d":
			if m.density == chrome.Regular {
				m.density = chrome.Compact
			} else {
				m.density = chrome.Regular
			}
			m.reflow()
		case "1", "2", "3", "4", "5", "6", "7":
			index, err := strconv.Atoi(message.String())
			if err == nil {
				m.selectScenario(index - 1)
			}
		}
	case tea.PasteMsg:
		if scenarioNames[m.scenario] == scenarioFocus {
			m.text += message.Content
			m.reflow()
		}
	case tea.MouseWheelMsg:
		switch message.Button {
		case tea.MouseWheelUp:
			m.contentViewport.Scroll(0, -1)
		case tea.MouseWheelDown:
			m.contentViewport.Scroll(0, 1)
		}
		m.refreshDiagnostics()
	case tea.MouseClickMsg:
		m.pointer = chrome.Point{X: message.X, Y: message.Y}
		m.clicks++
		if message.Button == tea.MouseLeft {
			m.pointerCaptured = true
			if item, ok := m.menu.ItemAt(m.pointer); ok {
				m.selectScenario(m.scenarioIndex(string(item.ID)))
			}
		}
		m.refreshDiagnostics()
	case tea.MouseMotionMsg:
		if m.pointerCaptured {
			m.pointer = chrome.Point{X: message.X, Y: message.Y}
			m.refreshDiagnostics()
		}
	case tea.MouseReleaseMsg:
		m.pointer = chrome.Point{X: message.X, Y: message.Y}
		m.pointerCaptured = false
		m.refreshDiagnostics()
	}
	return m, nil
}

func (m *labModel) View() tea.View {
	rows := make([]string, 0, m.height)
	menuBounds := m.menu.Bounds()
	for _, line := range m.menu.Lines(func(item chrome.MenuItem) string {
		if string(item.ID) == scenarioNames[m.scenario] {
			return lipgloss.NewStyle().Reverse(true).Render(item.Label)
		}
		return item.Label
	}) {
		rows = append(rows, placeLine(line, menuBounds.X, m.width))
	}
	workspaceHeight := max(m.height-len(rows), 0)
	contentLines := m.contentPane.Lines()
	diagnosticLines := m.diagnosticsPane.Lines()
	if m.width >= 70 {
		for row := range workspaceHeight {
			rows = append(rows, lineAt(contentLines, row)+lineAt(diagnosticLines, row))
		}
	} else {
		rows = append(rows, contentLines...)
		rows = append(rows, diagnosticLines...)
	}
	for len(rows) < m.height {
		rows = append(rows, strings.Repeat(" ", m.width))
	}
	content := strings.Join(rows[:min(len(rows), m.height)], "\n")
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeAllMotion
	view.WindowTitle = "dg chrome lab"
	return view
}

func (m *labModel) selectScenario(index int) {
	if index < 0 || index >= len(scenarioNames) {
		return
	}
	m.scenario = index
	m.contentViewport.Scroll(-m.contentViewport.Plan().Offset.X, -m.contentViewport.Plan().Offset.Y)
	m.reflow()
}

func (m *labModel) scenarioIndex(name string) int {
	for i, candidate := range scenarioNames {
		if candidate == name {
			return i
		}
	}
	return m.scenario
}

func (m *labModel) reflow() {
	m.menu.SetViewport(m.width, 0)
	menuHeight := m.menu.Bounds().Height
	workspace := chrome.Rect{
		Y:      menuHeight,
		Width:  m.width,
		Height: max(m.height-menuHeight, 0),
	}
	m.contentViewport.SetContent(m.scenarioContent())
	if scenarioNames[m.scenario] == scenarioOverflow {
		m.contentViewport.SetOverflow(chrome.ScrollText)
	} else {
		m.contentViewport.SetOverflow(chrome.WrapText)
	}
	if m.width >= 70 {
		diagnosticsWidth := min(34, m.width/2)
		m.contentPane.SetBounds(chrome.Rect{
			X:      workspace.X,
			Y:      workspace.Y,
			Width:  workspace.Width - diagnosticsWidth,
			Height: workspace.Height,
		})
		m.diagnosticsPane.SetBounds(chrome.Rect{
			X:      workspace.Right() - diagnosticsWidth,
			Y:      workspace.Y,
			Width:  diagnosticsWidth,
			Height: workspace.Height,
		})
	} else {
		contentHeight := workspace.Height / 2
		m.contentPane.SetBounds(chrome.Rect{
			X:      workspace.X,
			Y:      workspace.Y,
			Width:  workspace.Width,
			Height: contentHeight,
		})
		m.diagnosticsPane.SetBounds(chrome.Rect{
			X:      workspace.X,
			Y:      workspace.Y + contentHeight,
			Width:  workspace.Width,
			Height: workspace.Height - contentHeight,
		})
	}
	m.refreshDiagnostics()
}

func (m *labModel) refreshDiagnostics() {
	density := "regular"
	if m.density == chrome.Compact {
		density = "compact"
	}
	contentPlan := m.contentViewport.Plan()
	capture := "none"
	if m.pointerCaptured {
		capture = "scenario"
	}
	m.diagnostics.SetContent([]string{
		"CHROME LAB",
		"scenario: " + scenarioNames[m.scenario],
		fmt.Sprintf("terminal: %dx%d", m.width, m.height),
		"density: " + density,
		"pointer-capture: " + capture,
		fmt.Sprintf("events: click=%d", m.clicks),
		fmt.Sprintf("pointer: %d,%d", m.pointer.X, m.pointer.Y),
		fmt.Sprintf("scroll: %d,%d", contentPlan.Offset.X, contentPlan.Offset.Y),
		"focus: unavailable (phase 5)",
		"scopes: unavailable (phase 5)",
		formatRect("rect.workspace", chrome.Rect{Width: m.width, Height: m.height}),
		formatRect("rect.body", contentPlan.Content),
		"animation: unavailable (phase 9)",
		"bindings: 1-7/tab scenario",
		"bindings: d density; q quit",
	})
}

func (m *labModel) scenarioContent() []string {
	switch scenarioNames[m.scenario] {
	case scenarioLayout:
		root := chrome.Box(
			"box",
			lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
			chrome.Row("row", chrome.Text("left", "left"), chrome.Spacer("space"), chrome.Text("right", "right")),
		)
		plan, err := chrome.Arrange(root, chrome.Rect{Width: 28, Height: 3}, 1)
		if err != nil {
			return []string{"layout error: " + err.Error()}
		}
		lines := []string{"Box > Row > Text/Spacer/Text"}
		for _, entry := range plan.Entries() {
			lines = append(lines, formatRect(string(entry.ID), entry.Rect))
		}
		return lines
	case scenarioPane:
		return []string{
			"Pane",
			"header: sticky",
			"body: one viewport",
			"footer: sticky",
			"nested panes provide independent bodies",
		}
	case scenarioViewport:
		return []string{
			"Viewport wraps display cells.",
			"Wide glyphs: A界B",
			"Combining: e\u0301",
			"Use arrows or the mouse wheel.",
			"row 5", "row 6", "row 7", "row 8", "row 9",
		}
	case scenarioMenu:
		plan, err := m.menu.Plan()
		if err != nil {
			return []string{"menu error: " + err.Error()}
		}
		lines := []string{"Menu render and input share this plan:"}
		for _, entry := range plan.Entries() {
			lines = append(lines, formatRect(string(entry.ID), entry.Rect))
		}
		return lines
	case scenarioDensity:
		return []string{
			"Global density is environment data.",
			"Press d to toggle regular/compact.",
			"Components do not choose breakpoints.",
		}
	case scenarioOverflow:
		return []string{
			"0123456789012345678901234567890123456789",
			"horizontal scroll reserves a bar when automatic",
			"vertical content row 3",
			"vertical content row 4",
			"vertical content row 5",
			"vertical content row 6",
			"vertical content row 7",
			"vertical content row 8",
		}
	case scenarioFocus:
		profile := "standard"
		if m.profile == chrome.ProfileMac {
			profile = "mac"
		}
		effective := m.resolver.Effective([]chrome.ScopeID{"field", "global"})
		lines := []string{
			"Focus and semantic commands",
			"focus: text",
			"profile: " + profile,
			"text: " + m.text,
			"press p to change profile",
		}
		for _, binding := range effective {
			lines = append(lines, fmt.Sprintf(
				"effective: %s %s",
				binding.Chord,
				binding.Label,
			))
		}
		return lines
	default:
		return nil
	}
}

func formatRect(label string, rect chrome.Rect) string {
	return fmt.Sprintf("%s: %d,%d %dx%d", label, rect.X, rect.Y, rect.Width, rect.Height)
}

func lineAt(lines []string, row int) string {
	if row < len(lines) {
		return lines[row]
	}
	return ""
}

func placeLine(line string, left, width int) string {
	lineWidth := ansi.StringWidth(line)
	return strings.Repeat(" ", max(left, 0)) +
		line +
		strings.Repeat(" ", max(width-left-lineWidth, 0))
}

func main() {
	scenario := flag.String("scenario", "layout", "initial scenario")
	flag.Parse()
	if !validScenario(*scenario) {
		fmt.Fprintf(os.Stderr, "unknown scenario %q\n", *scenario)
		os.Exit(2)
	}
	if _, err := tea.NewProgram(newLabModel(*scenario)).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run chrome lab: %v\n", err)
		os.Exit(1)
	}
}

func validScenario(name string) bool {
	for _, candidate := range scenarioNames {
		if candidate == name {
			return true
		}
	}
	return false
}
