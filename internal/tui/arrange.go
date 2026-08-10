package tui

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
)

const (
	arrangeHorizontalField chrome.ID = "align-horizontal"
	arrangeVerticalField   chrome.ID = "align-vertical"
	arrangeDistributeField chrome.ID = "distribute"
	arrangeSpacingField    chrome.ID = "spacing"

	arrangeAlignLeft            chrome.ID = "align-left"
	arrangeAlignCenter          chrome.ID = "align-center"
	arrangeAlignRight           chrome.ID = "align-right"
	arrangeAlignTop             chrome.ID = "align-top"
	arrangeAlignMiddle          chrome.ID = "align-middle"
	arrangeAlignBottom          chrome.ID = "align-bottom"
	arrangeDistributeHorizontal chrome.ID = "distribute-horizontal"
	arrangeDistributeVertical   chrome.ID = "distribute-vertical"

	arrangeFormWidth                = 26
	arrangeSpacingAttentionDuration = 500 * time.Millisecond
)

type arrangeSpacing struct {
	cells uint64
	fixed bool
}

type arrangeSettings struct {
	horizontal chrome.ID
	vertical   chrome.ID
	distribute chrome.ID
	spacing    arrangeSpacing
}

func (s arrangeSettings) empty() bool {
	return s.horizontal == "" && s.vertical == "" && s.distribute == ""
}

type arrangeSpacingScale struct {
	active      bool
	approximate int64
}

func (s arrangeSpacingScale) maximum() uint64 {
	if s.active && s.approximate < 0 {
		return uint64(math.MaxUint32) + 1
	}
	if s.active {
		return math.MaxUint32
	}
	return 0
}

func (s arrangeSpacingScale) evenIndex() uint64 {
	if !s.active || s.approximate < 0 {
		return 0
	}
	return uint64(s.approximate)
}

func (s arrangeSpacingScale) numberText(number uint64) string {
	if number == s.evenIndex() {
		if s.active {
			return fmt.Sprintf("Even (~%d)", s.approximate)
		}
		return "Even"
	}
	if s.approximate < 0 {
		return strconv.FormatUint(number-1, 10)
	}
	return strconv.FormatUint(number, 10)
}

func (s arrangeSpacingScale) spacing(number uint64) arrangeSpacing {
	if !s.active || number == s.evenIndex() {
		return arrangeSpacing{}
	}
	if s.approximate < 0 {
		return arrangeSpacing{cells: number - 1, fixed: true}
	}
	return arrangeSpacing{cells: number, fixed: true}
}

func (s arrangeSpacingScale) number(spacing arrangeSpacing) uint64 {
	if !s.active || !spacing.fixed {
		return s.evenIndex()
	}
	if s.approximate < 0 {
		return spacing.cells + 1
	}
	return spacing.cells
}

type arrangeForm struct {
	form      *chrome.Form
	container lipgloss.Style
	styles    chrome.FormStyles
	spacing   arrangeSpacingScale
	lines     []string
	bounds    chrome.Rect
}

func newArrangeForm(container lipgloss.Style, styles chrome.FormStyles) arrangeForm {
	f := arrangeForm{container: container, styles: styles}
	f.Reset()
	return f
}

func (f *arrangeForm) Reset() {
	f.spacing = arrangeSpacingScale{}
	f.form = chrome.NewForm(arrangeDeclaration(f.spacing), f.styles)
	f.resize()
}

func (f *arrangeForm) SetStyles(container lipgloss.Style, styles chrome.FormStyles) {
	f.container = container
	f.styles = styles
	f.form.SetStyles(styles)
	f.resize()
}

func (f *arrangeForm) Update(message tea.Msg) tea.Cmd {
	updated, command := f.form.Update(message)
	f.form = updated.(*chrome.Form)
	f.render()
	return command
}

func (f *arrangeForm) Click(point chrome.Point) tea.Cmd {
	command := f.form.Click(point)
	f.render()
	return command
}

func (f arrangeForm) Settings() arrangeSettings {
	horizontal, _ := f.form.Selected(arrangeHorizontalField)
	vertical, _ := f.form.Selected(arrangeVerticalField)
	distribute, _ := f.form.Selected(arrangeDistributeField)
	spacing, _ := f.form.Number(arrangeSpacingField)
	return arrangeSettings{
		horizontal: chrome.ID(horizontal),
		vertical:   chrome.ID(vertical),
		distribute: chrome.ID(distribute),
		spacing:    f.spacing.spacing(spacing),
	}
}

func (f *arrangeForm) configureSpacing(items []arrangeItem) {
	current := f.Settings().spacing
	distribute, _ := f.form.Selected(arrangeDistributeField)
	var next arrangeSpacingScale
	switch chrome.ID(distribute) {
	case arrangeDistributeHorizontal:
		next = arrangeSpacingScale{active: true, approximate: arrangeEvenSpacing(items, true)}
	case arrangeDistributeVertical:
		next = arrangeSpacingScale{active: true, approximate: arrangeEvenSpacing(items, false)}
	}
	f.spacing = next
	f.form.SetNumber(
		arrangeSpacingField,
		next.number(current),
		next.maximum(),
		next.numberText,
	)
	f.render()
}

func (f arrangeForm) spacingBlocked(message tea.KeyPressMsg) bool {
	if f.form.FocusID() != arrangeSpacingField || f.spacing.active {
		return false
	}
	intent := chrome.ResolveControlIntent(message, false)
	return intent == chrome.NavigateLeft || intent == chrome.NavigateRight
}

func (f *arrangeForm) highlightDistribution() tea.Cmd {
	command := f.form.Highlight(arrangeDistributeField, arrangeSpacingAttentionDuration)
	f.render()
	return command
}

func (f arrangeForm) Lines() []string {
	return f.lines
}

func (f arrangeForm) Bounds() chrome.Rect {
	return f.bounds
}

func (f *arrangeForm) resize() {
	left := f.container.GetBorderLeftSize() + f.container.GetPaddingLeft()
	top := f.container.GetBorderTopSize() + f.container.GetPaddingTop()
	f.form.SetBounds(chrome.Rect{
		X: left, Y: top,
		Width: arrangeFormWidth, Height: 4,
	})
	f.render()
}

func (f *arrangeForm) render() {
	view := f.container.Render(f.form.View().Content)
	f.lines = strings.Split(view, "\n")
	f.bounds = chrome.Rect{Width: lipgloss.Width(view), Height: lipgloss.Height(view)}
}

func arrangeDeclaration(spacing arrangeSpacingScale) chrome.FormDeclaration {
	placeholder := chrome.FormOption{Label: "—"}
	return chrome.FormDeclaration{Fields: []chrome.FormField{
		{
			ID: arrangeHorizontalField, Label: "Align (h)", Kind: chrome.SelectField,
			Options: []chrome.FormOption{
				placeholder,
				{Label: "Left", Value: string(arrangeAlignLeft)},
				{Label: "Center", Value: string(arrangeAlignCenter)},
				{Label: "Right", Value: string(arrangeAlignRight)},
			},
		},
		{
			ID: arrangeVerticalField, Label: "Align (v)", Kind: chrome.SelectField,
			Options: []chrome.FormOption{
				placeholder,
				{Label: "Top", Value: string(arrangeAlignTop)},
				{Label: "Middle", Value: string(arrangeAlignMiddle)},
				{Label: "Bottom", Value: string(arrangeAlignBottom)},
			},
		},
		{
			ID: arrangeDistributeField, Label: "Distribute", Kind: chrome.SelectField,
			Options: []chrome.FormOption{
				placeholder,
				{Label: "Horizontal", Value: string(arrangeDistributeHorizontal)},
				{Label: "Vertical", Value: string(arrangeDistributeVertical)},
			},
		},
		{
			ID: arrangeSpacingField, Label: "Spacing", Kind: chrome.NumberField,
			Maximum: spacing.maximum(), Number: spacing.evenIndex(), NumberText: spacing.numberText,
		},
	}}
}

type arrangeItem struct {
	hit     layout.Hit
	bounds  layout.Rect
	nodes   []uint32
	origins []layout.Point
}

func (m *Model) toggleArrange() {
	if m.arrangeOpen {
		m.commitArrange()
		return
	}
	if !m.arrangeSelectionAvailable() {
		m.setError("select at least two nodes or groups to arrange")
		return
	}
	items, err := m.arrangeItems()
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.arrange.Reset()
	m.arrangePreview = items
	m.arrangeOpen = true
	m.beginTransaction(transactionArrange)
	m.status = "preview arrangement; enter applies, esc cancels"
	m.statusError = ""
}

func (m *Model) arrangeSelectionAvailable() bool {
	if !m.interaction.idle() {
		return false
	}
	nodes, groups, edges := m.geo.Selection().LogicalCounts()
	return edges == 0 && nodes+groups >= 2
}

func (m *Model) cancelArrange() {
	if !m.arrangeOpen {
		return
	}
	err := errors.Join(m.restoreArrangePreview(), m.cancelTransaction())
	m.finishArrange()
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.status = ""
	m.statusError = ""
}

func (m *Model) commitArrange() {
	if !m.arrangeOpen {
		return
	}
	if err := m.previewArrange(); err != nil {
		m.setError(err.Error())
		return
	}
	settings := m.arrange.Settings()
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.finishArrange()
	if settings.empty() {
		m.status = "arrangement unchanged"
	} else {
		m.status = "arranged selection"
	}
	m.statusError = ""
}

func (m *Model) finishArrange() {
	m.arrangeOpen = false
	clear(m.arrangePreview)
	m.arrangePreview = m.arrangePreview[:0]
}

func (m *Model) updateArrangeKey(message tea.KeyPressMsg) tea.Cmd {
	switch {
	case message.Code == tea.KeyEscape:
		m.cancelArrange()
		return nil
	case m.bindings.MatchesKey(message, commandArrange):
		m.commitArrange()
		return nil
	case message.Code == tea.KeyEnter && message.Mod == 0:
		m.commitArrange()
		return nil
	}
	if m.arrange.spacingBlocked(message) {
		return m.arrange.highlightDistribution()
	}
	before := m.arrange.Settings()
	command := m.arrange.Update(message)
	after := m.arrange.Settings()
	if after.distribute != before.distribute {
		m.arrange.configureSpacing(m.arrangePreview)
		after = m.arrange.Settings()
	}
	if after != before {
		if err := m.previewArrange(); err != nil {
			m.setError(err.Error())
		}
	}
	return command
}

func (m *Model) previewArrange() error {
	deltas, err := arrangeDeltas(m.arrangePreview, m.arrange.Settings())
	if err != nil {
		return errors.Join(err, m.restoreArrangePreview())
	}
	if err := m.placeArrangePreview(deltas); err != nil {
		return errors.Join(err, m.restoreArrangePreview())
	}
	if err := m.rebuild(); err != nil {
		return errors.Join(err, m.restoreArrangePreview())
	}
	m.refreshHits()
	m.status = "preview arrangement; enter applies, esc cancels"
	m.statusError = ""
	return nil
}

func (m *Model) restoreArrangePreview() error {
	if err := m.placeArrangePreview(nil); err != nil {
		return err
	}
	if err := m.rebuild(); err != nil {
		return err
	}
	m.refreshHits()
	return nil
}

func (m *Model) placeArrangePreview(deltas [][2]int64) error {
	for i, item := range m.arrangePreview {
		var dx, dy int64
		if deltas != nil {
			dx, dy = deltas[i][0], deltas[i][1]
		}
		for j, nodeID := range item.nodes {
			origin, ok := offsetArrangePoint(item.origins[j], dx, dy)
			if !ok {
				return errors.New("arrangement exceeds coordinate space")
			}
			if err := m.geo.PlaceNode(nodeID, origin); err != nil {
				return fmt.Errorf("place arranged node %d: %w", nodeID, err)
			}
		}
	}
	return nil
}

func (m *Model) arrangeItems() ([]arrangeItem, error) {
	if !m.arrangeSelectionAvailable() {
		return nil, errors.New("select at least two nodes or groups to arrange")
	}
	nodes, groups, _ := m.geo.Selection().LogicalCounts()
	items := make([]arrangeItem, 0, nodes+groups)
	for nodeID := range m.geo.Selection().DirectNodes() {
		items = append(items, arrangeItem{
			hit:     layout.Hit{ID: nodeID, Kind: layout.HitNode},
			bounds:  m.geo.Nodes[nodeID].Rect,
			nodes:   []uint32{nodeID},
			origins: []layout.Point{m.geo.Nodes[nodeID].Rect.Min},
		})
	}
	for groupID := range m.geo.Selection().Groups() {
		bounds, ok := m.geo.GroupBounds(groupID)
		if !ok {
			return nil, fmt.Errorf("group %d has no bounds", groupID)
		}
		groupNodes := slices.Collect(m.geo.GroupNodes(groupID))
		origins := make([]layout.Point, len(groupNodes))
		for i, nodeID := range groupNodes {
			origins[i] = m.geo.Nodes[nodeID].Rect.Min
		}
		items = append(items, arrangeItem{
			hit:     layout.Hit{ID: groupID, Kind: layout.HitGroup},
			bounds:  bounds,
			nodes:   groupNodes,
			origins: origins,
		})
	}
	return items, nil
}

func arrangeDeltas(items []arrangeItem, settings arrangeSettings) ([][2]int64, error) {
	deltas := make([][2]int64, len(items))
	minX, minY := uint32(math.MaxUint32), uint32(math.MaxUint32)
	var maxX, maxY uint32
	for _, item := range items {
		minX, minY = min(minX, item.bounds.Min.X), min(minY, item.bounds.Min.Y)
		limit := item.bounds.Max()
		maxX, maxY = max(maxX, limit.X), max(maxY, limit.Y)
	}
	for i, item := range items {
		bounds, limit := item.bounds, item.bounds.Max()
		switch settings.horizontal {
		case "":
		case arrangeAlignLeft:
			deltas[i][0] = int64(minX) - int64(bounds.Min.X)
		case arrangeAlignCenter:
			target := (int64(minX) + int64(maxX) - int64(bounds.Size.Width)) / 2
			deltas[i][0] = target - int64(bounds.Min.X)
		case arrangeAlignRight:
			deltas[i][0] = int64(maxX) - int64(limit.X)
		default:
			return nil, fmt.Errorf("unknown horizontal alignment %q", settings.horizontal)
		}
		switch settings.vertical {
		case "":
		case arrangeAlignTop:
			deltas[i][1] = int64(minY) - int64(bounds.Min.Y)
		case arrangeAlignMiddle:
			target := (int64(minY) + int64(maxY) - int64(bounds.Size.Height)) / 2
			deltas[i][1] = target - int64(bounds.Min.Y)
		case arrangeAlignBottom:
			deltas[i][1] = int64(maxY) - int64(limit.Y)
		default:
			return nil, fmt.Errorf("unknown vertical alignment %q", settings.vertical)
		}
	}
	switch settings.distribute {
	case "":
	case arrangeDistributeHorizontal, arrangeDistributeVertical:
		if len(items) < 3 {
			return nil, errors.New("distribution requires at least three selected items")
		}
		distributeArrange(
			items,
			deltas,
			settings.distribute == arrangeDistributeHorizontal,
			settings.spacing,
		)
	default:
		return nil, fmt.Errorf("unknown distribution %q", settings.distribute)
	}
	return deltas, nil
}

func distributeArrange(
	items []arrangeItem,
	deltas [][2]int64,
	horizontal bool,
	spacing arrangeSpacing,
) {
	order := arrangeOrder(items, horizontal)
	if spacing.fixed {
		distributeArrangeFixed(items, order, deltas, horizontal, spacing.cells)
		return
	}
	distributeArrangeEven(items, order, deltas, horizontal)
}

func arrangeOrder(items []arrangeItem, horizontal bool) []int {
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		aBounds, bBounds := items[a].bounds, items[b].bounds
		aPrimary, bPrimary := aBounds.Min.X, bBounds.Min.X
		aSecondary, bSecondary := aBounds.Min.Y, bBounds.Min.Y
		if !horizontal {
			aPrimary, bPrimary = aBounds.Min.Y, bBounds.Min.Y
			aSecondary, bSecondary = aBounds.Min.X, bBounds.Min.X
		}
		if result := cmp.Compare(aPrimary, bPrimary); result != 0 {
			return result
		}
		if result := cmp.Compare(aSecondary, bSecondary); result != 0 {
			return result
		}
		if result := cmp.Compare(items[a].hit.Kind, items[b].hit.Kind); result != 0 {
			return result
		}
		return cmp.Compare(items[a].hit.ID, items[b].hit.ID)
	})
	return order
}

func distributeArrangeEven(
	items []arrangeItem,
	order []int,
	deltas [][2]int64,
	horizontal bool,
) {
	start, end, content := arrangeDistributionSpan(items, order, horizontal)
	gapTotal := end - start - content
	position := start
	contentBefore := int64(0)
	for rank, index := range order {
		bounds := items[index].bounds
		if rank != 0 {
			position = start + contentBefore +
				gapTotal*int64(rank)/int64(len(order)-1)
		}
		if horizontal {
			deltas[index][0] = position - int64(bounds.Min.X)
			contentBefore += int64(bounds.Size.Width)
		} else {
			deltas[index][1] = position - int64(bounds.Min.Y)
			contentBefore += int64(bounds.Size.Height)
		}
	}
}

func distributeArrangeFixed(
	items []arrangeItem,
	order []int,
	deltas [][2]int64,
	horizontal bool,
	spacing uint64,
) {
	bounds := items[order[0]].bounds
	position := int64(bounds.Min.X)
	if !horizontal {
		position = int64(bounds.Min.Y)
	}
	for rank, index := range order {
		bounds = items[index].bounds
		if rank != 0 {
			position += int64(spacing)
		}
		if horizontal {
			deltas[index][0] = position - int64(bounds.Min.X)
			position += int64(bounds.Size.Width) - 1
		} else {
			deltas[index][1] = position - int64(bounds.Min.Y)
			position += int64(bounds.Size.Height) - 1
		}
	}
}

func arrangeEvenSpacing(items []arrangeItem, horizontal bool) int64 {
	if len(items) < 2 {
		return 0
	}
	order := arrangeOrder(items, horizontal)
	start, end, content := arrangeDistributionSpan(items, order, horizontal)
	gaps := int64(len(order) - 1)
	spacingTotal := end - start - content + gaps
	if spacingTotal < 0 {
		return -((-spacingTotal + gaps/2) / gaps)
	}
	return (spacingTotal + gaps/2) / gaps
}

func arrangeDistributionSpan(
	items []arrangeItem,
	order []int,
	horizontal bool,
) (int64, int64, int64) {
	first, last := items[order[0]].bounds, items[order[len(order)-1]].bounds
	start, end := int64(first.Min.X), int64(last.Max().X)
	if !horizontal {
		start, end = int64(first.Min.Y), int64(last.Max().Y)
	}
	content := int64(0)
	for _, index := range order {
		if horizontal {
			content += int64(items[index].bounds.Size.Width)
		} else {
			content += int64(items[index].bounds.Size.Height)
		}
	}
	return start, end, content
}

func offsetArrangePoint(point layout.Point, dx, dy int64) (layout.Point, bool) {
	x, y := int64(point.X)+dx, int64(point.Y)+dy
	if x < 0 || y < 0 || x > math.MaxUint32 || y > math.MaxUint32 {
		return layout.Point{}, false
	}
	return layout.NewPoint(uint32(x), uint32(y)), true
}
