package modal

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
)

// ConfirmationDeclaration defines a prompt and its semantic actions.
type ConfirmationDeclaration struct {
	ID      chrome.ID
	Title   string
	Message string
	Confirm chrome.Button
	Cancel  chrome.Button
}

// ConfirmationStyles defines prompt and action appearance.
type ConfirmationStyles struct {
	Title   lipgloss.Style
	Message lipgloss.Style
	Actions chrome.FormStyles
}

// Confirmation is a reusable declarative confirmation-dialog body.
type Confirmation struct {
	declaration ConfirmationDeclaration
	styles      ConfirmationStyles
	actions     *chrome.Form
	bounds      chrome.Rect
	lines       []string
}

// NewConfirmation returns a confirmation body with semantic action IDs.
func NewConfirmation(
	declaration ConfirmationDeclaration,
	styles ConfirmationStyles,
) *Confirmation {
	confirmation := &Confirmation{
		declaration: declaration,
		styles:      styles,
	}
	confirmation.actions = chrome.NewForm(
		confirmation.actionDeclaration(),
		styles.Actions,
	)
	confirmation.arrange()
	return confirmation
}

// Init implements tea.Model.
func (*Confirmation) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (c *Confirmation) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	actions, command := c.actions.Update(message)
	c.actions = actions.(*chrome.Form)
	c.arrange()
	return c, command
}

// View implements tea.Model.
func (c *Confirmation) View() tea.View {
	return tea.NewView(strings.Join(c.lines, "\n"))
}

// SetBounds arranges prompt content and bottom-anchored actions.
func (c *Confirmation) SetBounds(bounds chrome.Rect) {
	bounds.Width = max(bounds.Width, 0)
	bounds.Height = max(bounds.Height, 0)
	if c.bounds == bounds {
		return
	}
	c.bounds = bounds
	c.arrange()
}

// SetStyles replaces prompt and action styles.
func (c *Confirmation) SetStyles(styles ConfirmationStyles) {
	c.styles = styles
	c.actions.SetStyles(styles.Actions)
	c.arrange()
}

// Click routes one arranged pointer cell to the action form.
func (c *Confirmation) Click(point chrome.Point) tea.Cmd {
	return c.actions.Click(point)
}

// Plan returns the retained action layout.
func (c *Confirmation) Plan() chrome.FormPlan {
	return c.actions.Plan()
}

// AccessibleLines describes the prompt and every action.
func (c *Confirmation) AccessibleLines() []string {
	actions := c.actions.AccessibleLines()
	lines := make([]string, 0, 2+len(actions))
	lines = append(lines, c.declaration.Title, c.declaration.Message)
	return append(lines, actions...)
}

func (c *Confirmation) arrange() {
	prompt := c.promptLines()
	c.actions.SetBounds(chrome.Rect{Width: c.bounds.Width})
	actionHeight := min(c.actions.Plan().Bounds.Height, c.bounds.Height)
	promptHeight := max(c.bounds.Height-actionHeight, 0)
	if len(prompt) > promptHeight {
		prompt = prompt[:promptHeight]
	}
	actionHeight = c.bounds.Height - len(prompt)
	if actionHeight == 0 {
		c.actions.SetBounds(chrome.Rect{
			X: c.bounds.X, Y: c.bounds.Y, Height: 1,
		})
		c.lines = c.lines[:0]
		return
	}
	c.actions.SetBounds(chrome.Rect{
		X:      c.bounds.X,
		Y:      c.bounds.Y + len(prompt),
		Width:  c.bounds.Width,
		Height: actionHeight,
	})
	c.lines = append(c.lines[:0], prompt...)
	c.lines = append(c.lines, strings.Split(c.actions.View().Content, "\n")...)
	if len(c.lines) > c.bounds.Height {
		c.lines = c.lines[:c.bounds.Height]
	}
}

func (c *Confirmation) promptLines() []string {
	if c.bounds.Width == 0 {
		return nil
	}
	title := c.styles.Title.
		Width(c.bounds.Width).
		MaxWidth(c.bounds.Width).
		Render(ansi.Wordwrap(c.declaration.Title, c.bounds.Width, ""))
	message := c.styles.Message.
		Width(c.bounds.Width).
		MaxWidth(c.bounds.Width).
		Render(ansi.Wordwrap(c.declaration.Message, c.bounds.Width, ""))
	return append(strings.Split(title, "\n"), strings.Split(message, "\n")...)
}

func (c *Confirmation) actionDeclaration() chrome.FormDeclaration {
	return chrome.FormDeclaration{
		Spacer: chrome.FormSpacer{
			ID:   c.declaration.ID + ".spacer",
			Grow: 1,
		},
		Actions: chrome.ButtonListDeclaration{
			ID: c.declaration.ID + ".actions",
			Buttons: []chrome.Button{
				c.declaration.Confirm,
				c.declaration.Cancel,
			},
		},
	}
}
