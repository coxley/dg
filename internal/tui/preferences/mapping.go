package preferences

import (
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// MappingPillStyles defines every visual state for a key mapping.
type MappingPillStyles struct {
	Normal          lipgloss.Style
	Hovered         lipgloss.Style
	Focused         lipgloss.Style
	Active          lipgloss.Style
	Empty           lipgloss.Style
	EmptyHovered    lipgloss.Style
	Conflict        lipgloss.Style
	ConflictHovered lipgloss.Style
	ConflictFocused lipgloss.Style
}

// MappingPill renders one configurable key mapping.
type MappingPill struct {
	value    string
	focused  bool
	active   bool
	empty    bool
	conflict bool
	hovered  bool
	styles   MappingPillStyles
}

// NewMappingPill returns a mapping pill with value.
func NewMappingPill(value string, styles MappingPillStyles) MappingPill {
	return MappingPill{value: value, empty: value == "", styles: styles}
}

// SetState replaces the pill's application-owned interaction state.
func (p *MappingPill) SetState(value string, focused, active, conflict, hovered bool) {
	p.value = value
	p.focused = focused
	p.active = active
	p.empty = value == ""
	p.conflict = conflict
	p.hovered = hovered
}

// SetStyles replaces every visual state.
func (p *MappingPill) SetStyles(styles MappingPillStyles) {
	p.styles = styles
}

// View renders the pill within width terminal cells.
func (p MappingPill) View(width int) string {
	style := p.style()
	contentWidth := max(width-style.GetHorizontalFrameSize(), 0)
	label := mappingLabel(p.value, runtime.GOOS)
	if p.active {
		label = "..."
	}
	label = ansi.Truncate(label, contentWidth, "…")
	return style.
		Width(width).
		Render(label)
}

func mappingLabel(value, goos string) string {
	if goos != "darwin" {
		return value
	}
	parts := strings.Split(value, "+")
	for i := range parts {
		if parts[i] == "super" {
			parts[i] = "cmd"
		}
	}
	return strings.Join(parts, "+")
}

func (p MappingPill) style() lipgloss.Style {
	switch {
	case p.active:
		return p.styles.Active
	case p.conflict && p.focused:
		return p.styles.ConflictFocused
	case p.conflict && p.hovered:
		return p.styles.ConflictHovered
	case p.conflict:
		return p.styles.Conflict
	case p.empty && p.hovered:
		return p.styles.EmptyHovered
	case p.empty:
		return p.styles.Empty
	case p.focused:
		return p.styles.Focused
	case p.hovered:
		return p.styles.Hovered
	default:
		return p.styles.Normal
	}
}
