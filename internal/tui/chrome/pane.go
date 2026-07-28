package chrome

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// PanePlan arranges sticky header and footer rows around one body.
type PanePlan struct {
	Version uint64
	ID      ID
	Bounds  Rect
	Header  Rect
	Body    Rect
	Footer  Rect
}

// Pane retains an optional header, one body viewport or nested pane, and an
// optional footer.
type Pane struct {
	id ID

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
	headerHeight := min(len(p.header), p.bounds.Height)
	remaining := p.bounds.Height - headerHeight
	footerHeight := min(len(p.footer), remaining)
	bodyHeight := max(remaining-footerHeight, 0)
	plan := PanePlan{
		Version: p.version,
		ID:      p.id,
		Bounds:  p.bounds,
		Header: Rect{
			X:      p.bounds.X,
			Y:      p.bounds.Y,
			Width:  p.bounds.Width,
			Height: headerHeight,
		},
		Body: Rect{
			X:      p.bounds.X,
			Y:      p.bounds.Y + headerHeight,
			Width:  p.bounds.Width,
			Height: bodyHeight,
		},
		Footer: Rect{
			X:      p.bounds.X,
			Y:      p.bounds.Bottom() - footerHeight,
			Width:  p.bounds.Width,
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
	lines := make([]string, 0, p.bounds.Height)
	lines = appendSlot(lines, p.header[:p.plan.Header.Height], p.bounds.Width)
	switch {
	case p.nested != nil:
		lines = append(lines, p.nested.Lines()...)
	case p.body != nil:
		lines = append(lines, p.body.Lines()...)
	default:
		for range p.plan.Body.Height {
			lines = append(lines, strings.Repeat(" ", p.bounds.Width))
		}
	}
	return appendSlot(lines, p.footer[:p.plan.Footer.Height], p.bounds.Width)
}

func appendSlot(dst, lines []string, width int) []string {
	for _, line := range lines {
		dst = append(dst, padLine(ansi.Truncate(line, width, ""), width))
	}
	return dst
}
