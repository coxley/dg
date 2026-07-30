package chrome

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ScrollbarPolicy controls scrollbar reservation.
type ScrollbarPolicy uint8

const (
	// ScrollbarNever never reserves scrollbar cells.
	ScrollbarNever ScrollbarPolicy = iota
	// ScrollbarAutomatic reserves cells when scrollable content overflows.
	ScrollbarAutomatic
	// ScrollbarAlways always reserves scrollbar cells.
	ScrollbarAlways
)

// ScrollbarStyles configures finite viewport scrollbar cells.
type ScrollbarStyles struct {
	Track        lipgloss.Style
	Thumb        lipgloss.Style
	HoveredTrack lipgloss.Style
	HoveredThumb lipgloss.Style
	FocusedTrack lipgloss.Style
	FocusedThumb lipgloss.Style
	ActiveTrack  lipgloss.Style
	ActiveThumb  lipgloss.Style
}

type renderedScrollbarStyles struct {
	verticalTrack   string
	verticalThumb   string
	horizontalTrack string
	horizontalThumb string
	corner          string
}

// ViewportPlan is one finite viewport arrangement.
type ViewportPlan struct {
	Version uint64
	ID      ID
	Bounds  Rect
	Content Rect
	Offset  Point
	Extent  Size

	HorizontalBar   Rect
	VerticalBar     Rect
	HorizontalThumb Rect
	VerticalThumb   Rect
	lines           []string
}

type scrollbarAxis uint8

const (
	scrollbarNone scrollbarAxis = iota
	scrollbarHorizontal
	scrollbarVertical
)

// Viewport retains finite text content, scroll offsets, and its arranged plan.
type Viewport struct {
	id ID

	content    []string
	overflow   HorizontalOverflow
	horizontal ScrollbarPolicy
	vertical   ScrollbarPolicy
	offset     Point
	bounds     Rect
	version    uint64
	plan       ViewportPlan
	scrollbar  ScrollbarStyles
	focused    bool
	hoverAxis  scrollbarAxis

	dragAxis scrollbarAxis
	dragGrab int
}

// NewViewport returns a retained finite viewport.
func NewViewport(id ID) *Viewport {
	v := &Viewport{
		id:         id,
		overflow:   WrapText,
		horizontal: ScrollbarAutomatic,
		vertical:   ScrollbarAutomatic,
	}
	v.arrange()
	return v
}

// SetScrollbarStyles replaces the retained track and thumb styles.
func (v *Viewport) SetScrollbarStyles(styles ScrollbarStyles) {
	v.scrollbar = styles
	v.invalidate()
}

// SetFocused controls whether the scrollbar displays keyboard focus.
func (v *Viewport) SetFocused(focused bool) {
	if v.focused == focused {
		return
	}
	v.focused = focused
	v.invalidate()
}

// SetContent replaces logical content lines.
func (v *Viewport) SetContent(lines []string) {
	v.content = append(v.content[:0], lines...)
	v.invalidate()
}

// SetOverflow sets horizontal text overflow behavior.
func (v *Viewport) SetOverflow(overflow HorizontalOverflow) {
	if v.overflow == overflow {
		return
	}
	v.overflow = overflow
	v.invalidate()
}

// SetScrollbars sets horizontal and vertical reservation policies.
func (v *Viewport) SetScrollbars(horizontal, vertical ScrollbarPolicy) {
	if v.horizontal == horizontal && v.vertical == vertical {
		return
	}
	v.horizontal, v.vertical = horizontal, vertical
	v.invalidate()
}

// SetBounds arranges the viewport immediately.
func (v *Viewport) SetBounds(bounds Rect) {
	bounds.Width = max(bounds.Width, 0)
	bounds.Height = max(bounds.Height, 0)
	if v.bounds == bounds {
		return
	}
	v.bounds = bounds
	v.invalidate()
}

// Plan returns the current viewport arrangement.
func (v *Viewport) Plan() ViewportPlan {
	return v.plan
}

// Scroll moves the viewport offset and clamps it to arranged content.
func (v *Viewport) Scroll(dx, dy int) {
	v.offset.X += dx
	v.offset.Y += dy
	v.invalidate()
}

// Reveal moves the least distance needed to show target content cells.
func (v *Viewport) Reveal(target Rect) {
	if target.X < v.offset.X {
		v.offset.X = target.X
	} else if target.Right() > v.offset.X+v.plan.Content.Width {
		v.offset.X = target.Right() - v.plan.Content.Width
	}
	if target.Y < v.offset.Y {
		v.offset.Y = target.Y
	} else if target.Bottom() > v.offset.Y+v.plan.Content.Height {
		v.offset.Y = target.Bottom() - v.plan.Content.Height
	}
	v.invalidate()
}

// ContentPoint translates a visible terminal cell into content coordinates.
func (v *Viewport) ContentPoint(point Point) (Point, bool) {
	if !v.plan.Content.Contains(point) {
		return Point{}, false
	}
	return Point{
		X: point.X - v.plan.Content.X + v.plan.Offset.X,
		Y: point.Y - v.plan.Content.Y + v.plan.Offset.Y,
	}, true
}

// BeginScrollbarDrag starts dragging the scrollbar at point.
func (v *Viewport) BeginScrollbarDrag(point Point) bool {
	switch {
	case v.plan.VerticalBar.Contains(point):
		v.dragAxis = scrollbarVertical
		v.dragGrab = point.Y - v.plan.VerticalThumb.Y
		if !v.plan.VerticalThumb.Contains(point) {
			v.dragGrab = v.plan.VerticalThumb.Height / 2
			v.dragScrollbar(point)
		}
		v.invalidate()
		return true
	case v.plan.HorizontalBar.Contains(point):
		v.dragAxis = scrollbarHorizontal
		v.dragGrab = point.X - v.plan.HorizontalThumb.X
		if !v.plan.HorizontalThumb.Contains(point) {
			v.dragGrab = v.plan.HorizontalThumb.Width / 2
			v.dragScrollbar(point)
		}
		v.invalidate()
		return true
	default:
		return false
	}
}

// HoverScrollbar updates pointer emphasis for one scrollbar cell.
func (v *Viewport) HoverScrollbar(point Point) bool {
	axis := scrollbarNone
	switch {
	case v.plan.VerticalBar.Contains(point):
		axis = scrollbarVertical
	case v.plan.HorizontalBar.Contains(point):
		axis = scrollbarHorizontal
	}
	if v.hoverAxis == axis {
		return axis != scrollbarNone
	}
	v.hoverAxis = axis
	v.invalidate()
	return axis != scrollbarNone
}

// ClearScrollbarHover removes pointer emphasis.
func (v *Viewport) ClearScrollbarHover() {
	if v.hoverAxis == scrollbarNone {
		return
	}
	v.hoverAxis = scrollbarNone
	v.invalidate()
}

// DragScrollbar moves an active scrollbar drag to point.
func (v *Viewport) DragScrollbar(point Point) bool {
	if v.dragAxis == scrollbarNone {
		return false
	}
	v.dragScrollbar(point)
	return true
}

// EndScrollbarDrag ends an active scrollbar drag.
func (v *Viewport) EndScrollbarDrag() {
	if v.dragAxis == scrollbarNone {
		return
	}
	v.dragAxis = scrollbarNone
	v.dragGrab = 0
	v.invalidate()
}

// ScrollbarDragging reports whether a scrollbar owns the pointer.
func (v *Viewport) ScrollbarDragging() bool {
	return v.dragAxis != scrollbarNone
}

// Lines renders the exact current plan.
func (v *Viewport) Lines() []string {
	return append([]string(nil), v.plan.lines...)
}

func (v *Viewport) invalidate() {
	v.version++
	v.arrange()
}

func (v *Viewport) arrange() {
	horizontal := v.horizontal == ScrollbarAlways
	vertical := v.vertical == ScrollbarAlways
	var content []string
	var extent Size
	for range 3 {
		width := max(v.bounds.Width-boolCell(vertical), 0)
		height := max(v.bounds.Height-boolCell(horizontal), 0)
		_, extent = v.measureContent(width)
		needHorizontal := v.horizontal == ScrollbarAutomatic &&
			v.overflow == ScrollText &&
			extent.Width > width
		needVertical := v.vertical == ScrollbarAutomatic && extent.Height > height
		nextHorizontal := horizontal || needHorizontal
		nextVertical := vertical || needVertical
		if nextHorizontal == horizontal && nextVertical == vertical {
			break
		}
		horizontal, vertical = nextHorizontal, nextVertical
	}

	contentRect := v.bounds
	if vertical {
		contentRect.Width = max(contentRect.Width-1, 0)
	}
	if horizontal {
		contentRect.Height = max(contentRect.Height-1, 0)
	}
	content, extent = v.measureContent(contentRect.Width)
	v.offset.X = min(max(v.offset.X, 0), max(extent.Width-contentRect.Width, 0))
	v.offset.Y = min(max(v.offset.Y, 0), max(extent.Height-contentRect.Height, 0))
	if v.overflow != ScrollText {
		v.offset.X = 0
	}

	plan := ViewportPlan{
		Version: v.version,
		ID:      v.id,
		Bounds:  v.bounds,
		Content: contentRect,
		Offset:  v.offset,
		Extent:  extent,
	}
	if vertical {
		plan.VerticalBar = Rect{
			X:      contentRect.Right(),
			Y:      contentRect.Y,
			Width:  min(1, v.bounds.Width),
			Height: contentRect.Height,
		}
		start, size := scrollbarThumb(
			plan.VerticalBar.Height,
			plan.Offset.Y,
			plan.Extent.Height,
			plan.Content.Height,
		)
		plan.VerticalThumb = Rect{
			X: plan.VerticalBar.X, Y: plan.VerticalBar.Y + start,
			Width: plan.VerticalBar.Width, Height: size,
		}
	}
	if horizontal {
		plan.HorizontalBar = Rect{
			X:      contentRect.X,
			Y:      contentRect.Bottom(),
			Width:  contentRect.Width,
			Height: min(1, v.bounds.Height),
		}
		start, size := scrollbarThumb(
			plan.HorizontalBar.Width,
			plan.Offset.X,
			plan.Extent.Width,
			plan.Content.Width,
		)
		plan.HorizontalThumb = Rect{
			X: plan.HorizontalBar.X + start, Y: plan.HorizontalBar.Y,
			Width: size, Height: plan.HorizontalBar.Height,
		}
	}
	plan.lines = renderViewport(plan, content, v.renderedScrollbarStyles())
	v.plan = plan
}

func (v *Viewport) renderedScrollbarStyles() renderedScrollbarStyles {
	track, thumb := v.scrollbar.Track, v.scrollbar.Thumb
	switch {
	case v.dragAxis != scrollbarNone:
		track, thumb = v.scrollbar.ActiveTrack, v.scrollbar.ActiveThumb
	case v.hoverAxis != scrollbarNone:
		track, thumb = v.scrollbar.HoveredTrack, v.scrollbar.HoveredThumb
	case v.focused:
		track, thumb = v.scrollbar.FocusedTrack, v.scrollbar.FocusedThumb
	}
	return renderScrollbarStyles(track, thumb)
}

func (v *Viewport) dragScrollbar(point Point) {
	switch v.dragAxis {
	case scrollbarVertical:
		v.offset.Y = scrollbarOffset(
			point.Y-v.plan.VerticalBar.Y-v.dragGrab,
			v.plan.VerticalBar.Height,
			v.plan.VerticalThumb.Height,
			v.plan.Extent.Height,
			v.plan.Content.Height,
		)
	case scrollbarHorizontal:
		v.offset.X = scrollbarOffset(
			point.X-v.plan.HorizontalBar.X-v.dragGrab,
			v.plan.HorizontalBar.Width,
			v.plan.HorizontalThumb.Width,
			v.plan.Extent.Width,
			v.plan.Content.Width,
		)
	case scrollbarNone:
		return
	}
	v.invalidate()
}

func (v *Viewport) measureContent(width int) ([]string, Size) {
	var lines []string
	switch v.overflow {
	case WrapText:
		for _, line := range v.content {
			if width == 0 {
				continue
			}
			lines = append(lines, FitText(line, width, WrapText)...)
		}
	case ClipText, ScrollText:
		lines = append(lines, v.content...)
	}
	extent := Size{Height: len(lines)}
	for _, line := range lines {
		extent.Width = max(extent.Width, ansi.StringWidth(line))
	}
	if v.overflow == ClipText {
		extent.Width = min(extent.Width, width)
	}
	return lines, extent
}

func renderViewport(
	plan ViewportPlan,
	content []string,
	scrollbar renderedScrollbarStyles,
) []string {
	lines := make([]string, 0, plan.Bounds.Height)
	for row := range plan.Content.Height {
		contentRow := row + plan.Offset.Y
		line := ""
		if contentRow < len(content) {
			line = ansi.Cut(content[contentRow], plan.Offset.X, plan.Offset.X+plan.Content.Width)
		}
		line = padLine(line, plan.Content.Width)
		if plan.VerticalBar.Width != 0 {
			line += verticalScrollbarCell(
				row,
				plan.VerticalBar.Height,
				plan.Offset.Y,
				plan.Extent.Height,
				plan.Content.Height,
				scrollbar,
			)
		}
		lines = append(lines, line)
	}
	if plan.HorizontalBar.Height != 0 {
		line := horizontalScrollbarLine(
			plan.HorizontalBar.Width,
			plan.Offset.X,
			plan.Extent.Width,
			plan.Content.Width,
			scrollbar,
		)
		if plan.VerticalBar.Width != 0 {
			line += scrollbar.corner
		}
		lines = append(lines, line)
	}
	for len(lines) < plan.Bounds.Height {
		lines = append(lines, strings.Repeat(" ", plan.Bounds.Width))
	}
	return lines
}

func verticalScrollbarCell(
	row, track, offset, extent, visible int,
	scrollbar renderedScrollbarStyles,
) string {
	start, size := scrollbarThumb(track, offset, extent, visible)
	if row >= start && row < start+size {
		return scrollbar.verticalThumb
	}
	return scrollbar.verticalTrack
}

func horizontalScrollbarLine(
	track, offset, extent, visible int,
	scrollbar renderedScrollbarStyles,
) string {
	start, size := scrollbarThumb(track, offset, extent, visible)
	var line strings.Builder
	line.Grow(track * len(scrollbar.horizontalTrack))
	for column := range track {
		if column >= start && column < start+size {
			line.WriteString(scrollbar.horizontalThumb)
		} else {
			line.WriteString(scrollbar.horizontalTrack)
		}
	}
	return line.String()
}

func renderScrollbarStyles(
	track lipgloss.Style,
	thumb lipgloss.Style,
) renderedScrollbarStyles {
	return renderedScrollbarStyles{
		verticalTrack:   track.Render("│"),
		verticalThumb:   thumb.Render("█"),
		horizontalTrack: track.Render("─"),
		horizontalThumb: thumb.Render("█"),
		corner:          track.Render("┘"),
	}
}

func scrollbarThumb(track, offset, extent, visible int) (int, int) {
	if track <= 0 {
		return 0, 0
	}
	if extent <= visible || extent == 0 {
		return 0, track
	}
	size := max(track*visible/extent, 1)
	start := offset * (track - size) / max(extent-visible, 1)
	return start, size
}

func scrollbarOffset(start, track, thumb, extent, visible int) int {
	travel := max(track-thumb, 0)
	maxOffset := max(extent-visible, 0)
	if travel == 0 || maxOffset == 0 {
		return 0
	}
	start = min(max(start, 0), travel)
	return (start*maxOffset + travel/2) / travel
}

func padLine(line string, width int) string {
	return line + strings.Repeat(" ", max(width-ansi.StringWidth(line), 0))
}

func boolCell(value bool) int {
	if value {
		return 1
	}
	return 0
}
