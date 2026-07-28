// Package modal implements a movable modal shell with optional tabs.
package modal

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TabID identifies modal content without coupling it to a parent model.
type TabID uint8

// Tab defines one optional modal tab.
type Tab struct {
	ID    TabID
	Label string
}

// Variant selects a modal container style.
type Variant uint8

const (
	Standard Variant = iota
	Notice
)

const (
	minimumWidth  = 12
	minimumHeight = 3
)

// Styles defines all modal-owned appearance.
type Styles struct {
	Container lipgloss.Style
	Notice    lipgloss.Style
	Body      lipgloss.Style
	Tab       lipgloss.Style
	ActiveTab lipgloss.Style
}

type geometry struct {
	frameWidth  int
	frameHeight int
	contentLeft int
	contentTop  int
}

// Overlay is a rendered and positioned modal layer.
type Overlay struct {
	Content     string
	Left        int
	Top         int
	Width       int
	Height      int
	ContentLeft int
	ContentTop  int
}

// Contains reports whether a terminal cell falls inside the overlay.
func (o Overlay) Contains(x, y int) bool {
	return x >= o.Left && x < o.Left+o.Width &&
		y >= o.Top && y < o.Top+o.Height
}

// SwitchTabMsg requests a tab change.
type SwitchTabMsg struct {
	ID TabID
}

// CloseMsg requests that the parent close the modal.
type CloseMsg struct{}

// SwitchTab returns a command that changes the active tab through Model.Update.
func SwitchTab(id TabID) tea.Cmd {
	return func() tea.Msg {
		return SwitchTabMsg{ID: id}
	}
}

// Close returns a command that requests modal closure.
func Close() tea.Cmd {
	return func() tea.Msg {
		return CloseMsg{}
	}
}

// Model owns modal styles, geometry, position, pointer interaction, and tabs.
type Model struct {
	styles Styles
	normal geometry
	notice geometry
	body   geometry

	screenWidth  int
	screenHeight int
	avoidTop     int
	width        int
	content      string
	variant      Variant
	tabs         []Tab
	activeTab    TabID
	visible      bool
	fullscreen   bool

	left        int
	top         int
	positioned  bool
	dragPending bool
	dragging    bool
	dragOffsetX int
	dragOffsetY int
	resized     bool
	height      int
	resize      resizeState
	overlay     Overlay
}

type resizeState struct {
	pending bool
	active  bool
	east    bool
	south   bool
	fixedX  int
	fixedY  int
	offsetX int
	offsetY int
}

// New returns a modal model.
func New(styles Styles) Model {
	var model Model
	model.SetStyles(styles)
	return model
}

// SetStyles replaces modal styles and precomputes their cell geometry.
func (m *Model) SetStyles(styles Styles) {
	m.styles = styles
	m.normal = measure(styles.Container)
	m.notice = measure(styles.Notice)
	m.body = measure(styles.Body)
}

// Configure supplies parent-owned content and available terminal geometry.
func (m *Model) Configure(
	screenWidth, screenHeight, avoidTop, width int,
	content string,
	variant Variant,
	tabs []Tab,
	activeTab TabID,
) {
	m.screenWidth = max(screenWidth, 0)
	m.screenHeight = max(screenHeight, 0)
	m.avoidTop = max(avoidTop, 0)
	if !m.resized {
		m.width = min(max(width, 0), m.screenWidth)
	} else {
		m.width = min(
			max(m.width, min(minimumWidth, m.screenWidth)),
			m.screenWidth,
		)
		m.height = min(
			max(m.height, min(minimumHeight, m.screenHeight)),
			m.screenHeight,
		)
	}
	m.content = strings.TrimSuffix(content, "\n")
	m.variant = variant
	m.tabs = tabs
	m.activeTab = activeTab
	m.visible = m.width >= 2
	m.layout()
}

// Hide removes the modal and resets its placement and pointer state.
func (m *Model) Hide() {
	m.visible = false
	m.dragPending = false
	m.dragging = false
	m.resized = false
	m.height = 0
	m.resize = resizeState{}
	m.positioned = false
	m.overlay = Overlay{}
}

// ActiveTab returns the selected tab.
func (m Model) ActiveTab() TabID {
	return m.activeTab
}

// Overlay returns the current rendered modal layer.
func (m Model) Overlay() Overlay {
	return m.overlay
}

// BodyHeight returns the rows available after shell, tab, and body framing.
func (m Model) BodyHeight() int {
	geo := m.normal
	if m.variant == Notice {
		geo = m.notice
	}
	height := m.overlay.Height - geo.frameHeight
	if len(m.tabs) != 0 {
		height -= lipgloss.Height(m.tabsView()) + m.body.frameHeight
	}
	return max(height, 1)
}

// BodyOrigin returns the terminal position of the modal body content.
func (m Model) BodyOrigin() (x, y int) {
	x = m.overlay.ContentLeft
	y = m.overlay.ContentTop
	if len(m.tabs) != 0 {
		x += m.body.contentLeft
		y += lipgloss.Height(m.tabsView()) + m.body.contentTop
	}
	return x, y
}

// Dragging reports whether pointer motion is moving the modal.
func (m Model) Dragging() bool {
	return m.dragging
}

// Resizing reports whether pointer motion is resizing the modal.
func (m Model) Resizing() bool {
	return m.resize.active
}

// CapturesPointer reports whether the modal owns the current pointer gesture.
func (m Model) CapturesPointer() bool {
	return m.dragPending || m.dragging ||
		m.resize.pending || m.resize.active
}

// Update handles tab commands and modal pointer interaction.
func (m Model) Update(message tea.Msg) (Model, tea.Cmd) {
	switch message := message.(type) {
	case SwitchTabMsg:
		m.activeTab = message.ID
	case tea.MouseClickMsg:
		m.dragPending = false
		m.resize.pending = false
		if !m.overlay.Contains(message.X, message.Y) {
			return m, Close()
		}
		if message.Button == tea.MouseRight {
			if !m.fullscreen && m.variant != Notice {
				m.beginResize(message.X, message.Y)
			}
			return m, nil
		}
		if message.Button != tea.MouseLeft {
			return m, nil
		}
		if message.Y == m.overlay.ContentTop {
			if id, ok := m.tabAt(message.X); ok {
				return m, SwitchTab(id)
			}
		}
		if !m.fullscreen && (message.Y == m.overlay.Top ||
			!m.cellOccupied(message.X, message.Y)) {
			m.dragPending = true
			m.dragOffsetX = message.X - m.overlay.Left
			m.dragOffsetY = message.Y - m.overlay.Top
		}
	case tea.MouseMotionMsg:
		if m.resize.pending && message.Button == tea.MouseRight {
			m.resize.pending = false
			m.resize.active = true
		}
		if m.resize.active && message.Button == tea.MouseRight {
			m.resizeTo(message.X, message.Y)
			return m, nil
		}
		if m.dragPending {
			m.dragPending = false
			m.dragging = true
		}
		if m.dragging {
			m.left = message.X - m.dragOffsetX
			m.top = message.Y - m.dragOffsetY
			m.positioned = true
			m.layout()
		}
	case tea.MouseReleaseMsg:
		m.dragPending = false
		m.dragging = false
		m.resize.pending = false
		m.resize.active = false
	}
	return m, nil
}

// View returns the positioned modal content without compositing it.
func (m Model) View() string {
	return m.overlay.Content
}

func (m *Model) layout() {
	if !m.visible {
		m.overlay = Overlay{}
		return
	}
	style, geo := m.styles.Container, m.normal
	if m.variant == Notice {
		style, geo = m.styles.Notice, m.notice
	}
	content := m.content
	if len(m.tabs) != 0 {
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			m.tabsView(),
			m.styles.Body.
				Width(max(m.width-geo.frameWidth-m.body.frameWidth, 0)).
				MaxWidth(max(m.width-geo.frameWidth, 0)).
				Render(content),
		)
	}
	contentHeight := lipgloss.Height(content)
	height := contentHeight + geo.frameHeight
	if m.resized {
		height = min(max(m.height, minimumHeight), m.screenHeight)
	}
	style = style.
		Width(m.width).
		Height(height).
		MaxWidth(m.width).
		MaxHeight(height)
	rendered := style.Render(content)
	width, height := lipgloss.Width(rendered), lipgloss.Height(rendered)
	m.fullscreen = !m.resized && lipgloss.Height(content) > 3 &&
		height+m.avoidTop > m.screenHeight
	if m.fullscreen {
		style = style.
			Width(m.screenWidth).
			Height(m.screenHeight).
			MaxWidth(m.screenWidth).
			MaxHeight(m.screenHeight)
		rendered = style.Render(content)
		width, height = lipgloss.Width(rendered), lipgloss.Height(rendered)
	}
	left := max((m.screenWidth-width)/2, 0)
	top := max((m.screenHeight-height)/2, m.avoidTop)
	if m.fullscreen {
		left, top = 0, 0
	} else if m.positioned {
		left = min(max(m.left, 0), max(m.screenWidth-width, 0))
		top = min(max(m.top, 0), max(m.screenHeight-height, 0))
	}
	m.overlay = Overlay{
		Content:     rendered,
		Left:        left,
		Top:         top,
		Width:       width,
		Height:      height,
		ContentLeft: left + geo.contentLeft,
		ContentTop:  top + geo.contentTop,
	}
}

func (m *Model) beginResize(x, y int) {
	right := m.overlay.Left + m.overlay.Width - 1
	bottom := m.overlay.Top + m.overlay.Height - 1
	east := x-m.overlay.Left >= m.overlay.Width/2
	south := y-m.overlay.Top >= m.overlay.Height/2
	cornerX, fixedX := m.overlay.Left, right
	if east {
		cornerX, fixedX = right, m.overlay.Left
	}
	cornerY, fixedY := m.overlay.Top, bottom
	if south {
		cornerY, fixedY = bottom, m.overlay.Top
	}
	m.resize = resizeState{
		pending: true,
		east:    east,
		south:   south,
		fixedX:  fixedX,
		fixedY:  fixedY,
		offsetX: x - cornerX,
		offsetY: y - cornerY,
	}
}

func (m *Model) resizeTo(x, y int) {
	cornerX := min(max(x-m.resize.offsetX, 0), m.screenWidth-1)
	cornerY := min(max(y-m.resize.offsetY, 0), m.screenHeight-1)
	left, width := resizeAxis(
		cornerX,
		m.resize.fixedX,
		m.resize.east,
		min(minimumWidth, m.screenWidth),
	)
	top, height := resizeAxis(
		cornerY,
		m.resize.fixedY,
		m.resize.south,
		min(minimumHeight, m.screenHeight),
	)
	m.left = left
	m.top = top
	m.width = width
	m.height = height
	m.positioned = true
	m.resized = true
	m.layout()
}

func resizeAxis(point, fixed int, positive bool, minimum int) (origin, size int) {
	if positive {
		point = max(point, fixed+minimum-1)
		return fixed, point - fixed + 1
	}
	point = min(point, fixed-(minimum-1))
	return point, fixed - point + 1
}

func (m Model) tabsView() string {
	rendered := make([]string, 0, len(m.tabs))
	for _, tab := range m.tabs {
		style := m.styles.Tab
		if tab.ID == m.activeTab {
			style = m.styles.ActiveTab
		}
		rendered = append(rendered, style.Render(tab.Label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m Model) tabAt(x int) (TabID, bool) {
	x -= m.overlay.ContentLeft
	for _, tab := range m.tabs {
		width := lipgloss.Width(m.styles.Tab.Render(tab.Label))
		if tab.ID == m.activeTab {
			width = lipgloss.Width(m.styles.ActiveTab.Render(tab.Label))
		}
		if x >= 0 && x < width {
			return tab.ID, true
		}
		x -= width
	}
	return 0, false
}

func (m Model) cellOccupied(x, y int) bool {
	x -= m.overlay.Left
	y -= m.overlay.Top
	canvas := lipgloss.NewCanvas(m.overlay.Width, m.overlay.Height).
		Compose(lipgloss.NewLayer(m.overlay.Content))
	cell := canvas.CellAt(x, y)
	if cell == nil {
		return false
	}
	hasContent := cell.Content != "" && cell.Content != " "
	return hasContent || !cell.Style.IsZero()
}

func measure(style lipgloss.Style) geometry {
	return geometry{
		frameWidth:  style.GetHorizontalFrameSize(),
		frameHeight: style.GetVerticalFrameSize(),
		contentLeft: style.GetBorderLeftSize() +
			style.GetPaddingLeft(),
		contentTop: style.GetBorderTopSize() +
			style.GetPaddingTop(),
	}
}
