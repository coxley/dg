package chrome

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Kind selects an element's layout behavior.
type Kind uint8

const (
	// TextKind measures display-cell-aware text.
	TextKind Kind = iota
	// BoxKind frames one child.
	BoxKind
	// RowKind arranges children horizontally.
	RowKind
	// ColumnKind arranges children vertically.
	ColumnKind
	// SpacerKind absorbs available main-axis space.
	SpacerKind
)

// SizePolicy selects intrinsic or available size.
type SizePolicy uint8

const (
	// Hug uses the preferred content size.
	Hug SizePolicy = iota
	// Fill uses the available size.
	Fill
)

// Justify distributes children on a container's main axis.
type Justify uint8

const (
	// JustifyStart packs children at the start.
	JustifyStart Justify = iota
	// JustifyCenter centers packed children.
	JustifyCenter
	// JustifyEnd packs children at the end.
	JustifyEnd
	// JustifySpaceBetween distributes space between children.
	JustifySpaceBetween
)

// Align positions children on a container's cross axis.
type Align uint8

const (
	// AlignStart places children at the start.
	AlignStart Align = iota
	// AlignCenter centers children.
	AlignCenter
	// AlignEnd places children at the end.
	AlignEnd
	// AlignStretch fills the cross axis.
	AlignStretch
	// AlignBaseline aligns single-line text baselines.
	AlignBaseline
)

// Node is one concrete declarative layout element.
type Node struct {
	ID       ID
	Kind     Kind
	Text     string
	Style    lipgloss.Style
	Children []Node

	Width   SizePolicy
	Height  SizePolicy
	Grow    int
	Shrink  int
	Gap     int
	Justify Justify
	Align   Align
}

// Text returns a display-cell-aware text node.
func Text(id ID, content string) Node {
	return Node{ID: id, Kind: TextKind, Text: content}
}

// Box returns a styled single-child node.
func Box(id ID, style lipgloss.Style, child Node) Node {
	return Node{ID: id, Kind: BoxKind, Style: style, Children: []Node{child}}
}

// Row returns a horizontal container.
func Row(id ID, children ...Node) Node {
	return Node{ID: id, Kind: RowKind, Children: children, Align: AlignStart}
}

// Column returns a vertical container.
func Column(id ID, children ...Node) Node {
	return Node{ID: id, Kind: ColumnKind, Children: children, Align: AlignStart}
}

// Spacer returns a zero-minimum element that grows on a container's main axis.
func Spacer(id ID) Node {
	return Node{ID: id, Kind: SpacerKind, Grow: 1, Width: Fill, Height: Fill}
}

type measuredNode struct {
	node     Node
	metrics  Metrics
	children []measuredNode
}

// Measure computes intrinsic minimum and preferred sizes under constraints.
func Measure(root Node, constraints Constraints) Metrics {
	return measure(root, constraints.normalize()).metrics
}

// Arrange measures and arranges root within bounds.
func Arrange(root Node, bounds Rect, version uint64) (Plan, error) {
	constraints := Tight(Size{Width: bounds.Width, Height: bounds.Height})
	measured := measure(root, constraints)
	size := chooseSize(root, measured.metrics, constraints)
	rect := Rect{X: bounds.X, Y: bounds.Y, Width: size.Width, Height: size.Height}
	entries := make([]Arrangement, 0, countNodes(root))
	arrange(measured, rect, &entries)
	return newPlan(version, rect, entries)
}

// ArrangePreferred arranges root at its preferred size, bounded by available.
func ArrangePreferred(root Node, available Rect, version uint64) (Plan, error) {
	constraints := Constraints{Max: Size{Width: available.Width, Height: available.Height}}
	measured := measure(root, constraints.normalize())
	size := chooseSize(root, measured.metrics, constraints)
	rect := Rect{X: available.X, Y: available.Y, Width: size.Width, Height: size.Height}
	entries := make([]Arrangement, 0, countNodes(root))
	arrange(measured, rect, &entries)
	return newPlan(version, rect, entries)
}

func measure(node Node, constraints Constraints) measuredNode {
	frame := Size{
		Width:  node.Style.GetHorizontalFrameSize(),
		Height: node.Style.GetVerticalFrameSize(),
	}
	inner := Constraints{
		Max: Size{
			Width:  max(constraints.Max.Width-frame.Width, 0),
			Height: max(constraints.Max.Height-frame.Height, 0),
		},
	}
	result := measuredNode{node: node}
	switch node.Kind {
	case TextKind:
		result.metrics = measureText(node.Text, inner.Max.Width)
	case BoxKind:
		if len(node.Children) != 0 {
			child := measure(node.Children[0], inner)
			result.children = append(result.children, child)
			result.metrics = child.metrics
		}
	case RowKind, ColumnKind:
		result.children = make([]measuredNode, len(node.Children))
		for i, child := range node.Children {
			result.children[i] = measure(child, inner)
		}
		result.metrics = measureLinear(node.Kind, result.children, max(node.Gap, 0))
	case SpacerKind:
	}
	result.metrics.Min.Width += frame.Width
	result.metrics.Min.Height += frame.Height
	result.metrics.Preferred.Width += frame.Width
	result.metrics.Preferred.Height += frame.Height
	result.metrics.Min.Width = max(result.metrics.Min.Width, constraints.Min.Width)
	result.metrics.Min.Height = max(result.metrics.Min.Height, constraints.Min.Height)
	result.metrics.Preferred = constraints.constrain(result.metrics.Preferred)
	result.metrics.Preferred.Width = max(result.metrics.Preferred.Width, result.metrics.Min.Width)
	result.metrics.Preferred.Height = max(result.metrics.Preferred.Height, result.metrics.Min.Height)
	return result
}

func measureText(text string, width int) Metrics {
	lines := strings.Split(text, "\n")
	preferredWidth := 0
	minWidth := 0
	for _, line := range lines {
		preferredWidth = max(preferredWidth, ansi.StringWidth(line))
		for _, r := range ansi.Strip(line) {
			minWidth = max(minWidth, ansi.StringWidth(string(r)))
		}
	}
	height := len(lines)
	if width > 0 && preferredWidth > width {
		height = len(strings.Split(ansi.Hardwrap(text, width, true), "\n"))
	}
	return Metrics{
		Min:       Size{Width: minWidth, Height: max(height, 1)},
		Preferred: Size{Width: preferredWidth, Height: max(height, 1)},
	}
}

func measureLinear(kind Kind, children []measuredNode, gap int) Metrics {
	var metrics Metrics
	for _, child := range children {
		if kind == RowKind {
			metrics.Min.Width += child.metrics.Min.Width
			metrics.Preferred.Width += child.metrics.Preferred.Width
			metrics.Min.Height = max(metrics.Min.Height, child.metrics.Min.Height)
			metrics.Preferred.Height = max(metrics.Preferred.Height, child.metrics.Preferred.Height)
			continue
		}
		metrics.Min.Width = max(metrics.Min.Width, child.metrics.Min.Width)
		metrics.Preferred.Width = max(metrics.Preferred.Width, child.metrics.Preferred.Width)
		metrics.Min.Height += child.metrics.Min.Height
		metrics.Preferred.Height += child.metrics.Preferred.Height
	}
	if len(children) > 1 {
		totalGap := gap * (len(children) - 1)
		if kind == RowKind {
			metrics.Min.Width += totalGap
			metrics.Preferred.Width += totalGap
		} else {
			metrics.Min.Height += totalGap
			metrics.Preferred.Height += totalGap
		}
	}
	return metrics
}

func chooseSize(node Node, metrics Metrics, constraints Constraints) Size {
	size := metrics.Preferred
	if node.Width == Fill {
		size.Width = constraints.Max.Width
	}
	if node.Height == Fill {
		size.Height = constraints.Max.Height
	}
	return constraints.constrain(size)
}

func arrange(measured measuredNode, rect Rect, entries *[]Arrangement) {
	*entries = append(*entries, Arrangement{ID: measured.node.ID, Rect: rect})
	if len(measured.children) == 0 {
		return
	}
	frame := Insets{
		Top:    measured.node.Style.GetBorderTopSize() + measured.node.Style.GetPaddingTop(),
		Right:  measured.node.Style.GetBorderRightSize() + measured.node.Style.GetPaddingRight(),
		Bottom: measured.node.Style.GetBorderBottomSize() + measured.node.Style.GetPaddingBottom(),
		Left:   measured.node.Style.GetBorderLeftSize() + measured.node.Style.GetPaddingLeft(),
	}
	content := rect.Inset(frame)
	switch measured.node.Kind {
	case BoxKind:
		arrange(measured.children[0], content, entries)
	case RowKind:
		arrangeLinear(measured, content, true, entries)
	case ColumnKind:
		arrangeLinear(measured, content, false, entries)
	case TextKind, SpacerKind:
	}
}

func arrangeLinear(measured measuredNode, content Rect, horizontal bool, entries *[]Arrangement) {
	count := len(measured.children)
	gap := max(measured.node.Gap, 0)
	mainSize := content.Height
	if horizontal {
		mainSize = content.Width
	}
	if count > 1 {
		gap = min(gap, mainSize/(count-1))
	}
	available := max(mainSize-gap*max(count-1, 0), 0)
	sizes := make([]int, count)
	minimums := make([]int, count)
	total := 0
	for i, child := range measured.children {
		if horizontal {
			sizes[i] = child.metrics.Preferred.Width
			minimums[i] = child.metrics.Min.Width
		} else {
			sizes[i] = child.metrics.Preferred.Height
			minimums[i] = child.metrics.Min.Height
		}
		total += sizes[i]
	}
	if total < available {
		distributeGrow(sizes, measured.children, available-total)
	} else if total > available {
		deficit := distributeShrink(sizes, minimums, measured.children, total-available)
		distributeEmergencyShrink(sizes, deficit)
	}
	used := 0
	for _, size := range sizes {
		used += size
	}
	actualGap := gap
	offset := 0
	extra := max(available-used, 0)
	switch measured.node.Justify {
	case JustifyCenter:
		offset = extra / 2
	case JustifyEnd:
		offset = extra
	case JustifySpaceBetween:
		if count > 1 {
			actualGap += extra / (count - 1)
		}
	case JustifyStart:
	}
	cursor := offset
	for i, child := range measured.children {
		childRect := crossRect(content, child, sizes[i], cursor, horizontal, measured.node.Align)
		arrange(child, childRect, entries)
		cursor += sizes[i] + actualGap
	}
}

func distributeGrow(sizes []int, children []measuredNode, extra int) {
	for extra > 0 {
		weight := 0
		for _, child := range children {
			weight += max(child.node.Grow, 0)
		}
		if weight == 0 {
			return
		}
		progress := 0
		for i, child := range children {
			grow := max(child.node.Grow, 0)
			if grow == 0 {
				continue
			}
			share := max(extra*grow/weight, 1)
			share = min(share, extra-progress)
			sizes[i] += share
			progress += share
			if progress == extra {
				break
			}
		}
		extra -= progress
	}
}

func distributeShrink(sizes, minimums []int, children []measuredNode, deficit int) int {
	for deficit > 0 {
		weight := 0
		for i, child := range children {
			if sizes[i] > minimums[i] {
				weight += max(child.node.Shrink, 0)
			}
		}
		if weight == 0 {
			return deficit
		}
		progress := 0
		for i, child := range children {
			shrink := max(child.node.Shrink, 0)
			room := sizes[i] - minimums[i]
			if shrink == 0 || room == 0 {
				continue
			}
			share := max(deficit*shrink/weight, 1)
			share = min(share, room, deficit-progress)
			sizes[i] -= share
			progress += share
			if progress == deficit {
				break
			}
		}
		deficit -= progress
	}
	return 0
}

func distributeEmergencyShrink(sizes []int, deficit int) {
	for deficit > 0 {
		progress := 0
		for i := range sizes {
			if sizes[i] == 0 {
				continue
			}
			sizes[i]--
			deficit--
			progress++
			if deficit == 0 {
				return
			}
		}
		if progress == 0 {
			return
		}
	}
}

func crossRect(content Rect, child measuredNode, main, offset int, horizontal bool, align Align) Rect {
	if horizontal {
		height := min(child.metrics.Preferred.Height, content.Height)
		y := crossOffset(content.Y, content.Height, height, align)
		if align == AlignStretch {
			y, height = content.Y, content.Height
		}
		return Rect{X: content.X + offset, Y: y, Width: main, Height: height}
	}
	width := min(child.metrics.Preferred.Width, content.Width)
	x := crossOffset(content.X, content.Width, width, align)
	if align == AlignStretch {
		x, width = content.X, content.Width
	}
	return Rect{X: x, Y: content.Y + offset, Width: width, Height: main}
}

func crossOffset(start, available, size int, align Align) int {
	switch align {
	case AlignCenter:
		return start + (available-size)/2
	case AlignEnd:
		return start + available - size
	case AlignStart, AlignStretch, AlignBaseline:
		return start
	default:
		return start
	}
}

func countNodes(node Node) int {
	count := 1
	for _, child := range node.Children {
		count += countNodes(child)
	}
	return count
}
