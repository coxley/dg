package chrome

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// PanePlan arranges sticky header and footer rows around one body.
type PanePlan struct {
	Version uint64
	ID      ID
	Bounds  Rect
	Content Rect
	Header  Rect
	Body    Rect
	Footer  Rect
}

// Pane retains an optional header, one body viewport or nested pane, and an
// optional footer.
type Pane struct {
	id ID

	style  lipgloss.Style
	header []string
	footer []string
	body   *Viewport
	nested *Pane
	bounds Rect

	version uint64
	plan    PanePlan
}

// NewPane returns a pane whose body is viewport.
func NewPane(id ID, viewport *Viewport) *Pane {
	p := &Pane{id: id, body: viewport}
	p.arrange()
	return p
}

// SetStyle replaces the pane frame style and arranges immediately.
func (p *Pane) SetStyle(style lipgloss.Style) {
	p.style = style
	p.invalidate()
}

// SetHeader replaces sticky header rows.
func (p *Pane) SetHeader(lines []string) {
	p.header = append(p.header[:0], lines...)
	p.invalidate()
}

// SetFooter replaces sticky footer rows.
func (p *Pane) SetFooter(lines []string) {
	p.footer = append(p.footer[:0], lines...)
	p.invalidate()
}

// SetNested replaces the body with a nested pane. Passing nil restores the
// viewport body.
func (p *Pane) SetNested(nested *Pane) {
	p.nested = nested
	p.invalidate()
}

// SetBounds arranges all pane slots immediately.
func (p *Pane) SetBounds(bounds Rect) {
	bounds.Width = max(bounds.Width, 0)
	bounds.Height = max(bounds.Height, 0)
	if p.bounds == bounds {
		return
	}
	p.bounds = bounds
	p.invalidate()
}

// Plan returns the current pane arrangement.
func (p *Pane) Plan() PanePlan {
	return p.plan
}

// Lines renders the exact current pane plan.
func (p *Pane) Lines() []string {
	return p.renderLines()
}

func (p *Pane) invalidate() {
	p.version++
	p.arrange()
}

func (p *Pane) arrange() {
	content := p.bounds.Inset(styleFrameInsets(p.style))
	headerHeight := min(len(p.header), content.Height)
	remaining := content.Height - headerHeight
	footerHeight := min(len(p.footer), remaining)
	bodyHeight := max(remaining-footerHeight, 0)
	plan := PanePlan{
		Version: p.version,
		ID:      p.id,
		Bounds:  p.bounds,
		Content: content,
		Header: Rect{
			X:      content.X,
			Y:      content.Y,
			Width:  content.Width,
			Height: headerHeight,
		},
		Body: Rect{
			X:      content.X,
			Y:      content.Y + headerHeight,
			Width:  content.Width,
			Height: bodyHeight,
		},
		Footer: Rect{
			X:      content.X,
			Y:      content.Bottom() - footerHeight,
			Width:  content.Width,
			Height: footerHeight,
		},
	}
	switch {
	case p.nested != nil:
		p.nested.SetBounds(plan.Body)
	case p.body != nil:
		p.body.SetBounds(plan.Body)
	}
	p.plan = plan
}

func (p *Pane) renderLines() []string {
	if p.bounds.Width == 0 || p.bounds.Height == 0 {
		return nil
	}
	lines := make([]string, 0, p.plan.Content.Height)
	lines = appendSlot(lines, p.header[:p.plan.Header.Height], p.plan.Content.Width)
	switch {
	case p.nested != nil:
		lines = append(lines, p.nested.Lines()...)
	case p.body != nil:
		lines = append(lines, p.body.Lines()...)
	default:
		for range p.plan.Body.Height {
			lines = append(lines, strings.Repeat(" ", p.plan.Content.Width))
		}
	}
	lines = appendSlot(lines, p.footer[:p.plan.Footer.Height], p.plan.Content.Width)
	style := p.style.
		Width(max(p.bounds.Width-p.style.GetHorizontalMargins(), 0)).
		Height(max(p.bounds.Height-p.style.GetVerticalMargins(), 0))
	return fitBlock(style.Render(strings.Join(lines, "\n")), p.bounds.Width, p.bounds.Height)
}

func appendSlot(dst, lines []string, width int) []string {
	for _, line := range lines {
		dst = append(dst, padLine(ansi.Truncate(line, width, ""), width))
	}
	return dst
}

func styleFrameInsets(style lipgloss.Style) Insets {
	return Insets{
		Top: style.GetMarginTop() +
			style.GetBorderTopSize() +
			style.GetPaddingTop(),
		Right: style.GetMarginRight() +
			style.GetBorderRightSize() +
			style.GetPaddingRight(),
		Bottom: style.GetMarginBottom() +
			style.GetBorderBottomSize() +
			style.GetPaddingBottom(),
		Left: style.GetMarginLeft() +
			style.GetBorderLeftSize() +
			style.GetPaddingLeft(),
	}
}

func fitBlock(block string, width, height int) []string {
	rendered := strings.Split(block, "\n")
	lines := make([]string, height)
	for i := range height {
		line := ""
		if i < len(rendered) {
			line = ansi.Truncate(rendered[i], width, "")
		}
		lines[i] = padLine(line, width)
	}
	return lines
}
