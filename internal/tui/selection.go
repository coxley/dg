package tui

import (
	"cmp"
	"errors"
	"slices"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
)

type selectionArea struct {
	min layout.Point
	max layout.Point
}

func newSelectionArea(a, b layout.Point) selectionArea {
	return selectionArea{
		min: layout.NewPoint(min(a.X, b.X), min(a.Y, b.Y)),
		max: layout.NewPoint(max(a.X, b.X), max(a.Y, b.Y)),
	}
}

func (a selectionArea) contains(point layout.Point) bool {
	return point.X >= a.min.X && point.X <= a.max.X &&
		point.Y >= a.min.Y && point.Y <= a.max.Y
}

func (m *Model) clearSelection() {
	m.geo.Selection().Clear()
	m.interaction.render.selectionHighlight = m.interaction.render.selectionHighlight[:0]
}

func (m *Model) hasSelection() bool {
	return !m.geo.Selection().Empty()
}

func (m *Model) hasSelectedNodes() bool {
	return m.geo.Selection().HasNodes()
}

func (m *Model) selectedCounts() (nodes, edges int) {
	return m.geo.Selection().Counts()
}

func (m *Model) lineToolEdgeEditReady() bool {
	if m.interaction.tool != toolConnect ||
		m.interaction.session.kind != sessionNone ||
		m.interaction.gesture.kind != gestureNone {
		return false
	}
	nodes, edges := m.selectedCounts()
	return nodes == 0 && edges != 0
}

func (m *Model) selectOnly(hit layout.Hit) {
	m.geo.Selection().SelectOnly(hit)
	m.refreshSelectionHighlight()
}

func (m *Model) hitSelected(hit layout.Hit) bool {
	return m.geo.Selection().Contains(hit)
}

func (m *Model) firstSelectedNode() (layout.Hit, bool) {
	nodeID, ok := m.geo.Selection().FirstNode()
	return layout.Hit{ID: nodeID, Kind: layout.HitNode}, ok
}

func (m *Model) expandSelection() {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	if !m.hasSelection() {
		if hit, ok := m.activeHit(); ok {
			m.selectOnly(hit)
		}
	}
	beforeNodes, beforeEdges := m.selectedCounts()
	m.geo.Selection().Expand()
	afterNodes, afterEdges := m.selectedCounts()
	if beforeNodes == afterNodes &&
		beforeEdges == afterEdges &&
		afterNodes+afterEdges < m.liveObjectCount() {
		m.geo.Selection().Expand()
	}
	m.refreshSelectionHighlight()
	m.status = ""
}

func (m *Model) groupSelection() {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	directNodes, groupCount, edgeCount := m.geo.Selection().LogicalCounts()
	if edgeCount != 0 {
		m.setError("groups can contain nodes and groups, not edges")
		return
	}
	if directNodes == 0 && groupCount == 1 {
		var groupID uint32
		for selected := range m.geo.Selection().Groups() {
			groupID = selected
		}
		m.beginTransaction(transactionImmediate)
		if _, err := m.geo.Ungroup(groupID); err != nil {
			m.setError(errors.Join(err, m.cancelTransaction()).Error())
			return
		}
		if err := m.commitTransaction(); err != nil {
			m.setError(err.Error())
			return
		}
		m.refreshSelectionHighlight()
		m.status = "ungrouped one level"
		return
	}
	if directNodes+groupCount < 2 {
		m.setError("select at least two sibling nodes or groups")
		return
	}
	type item struct {
		member ir.Member
		bounds layout.Rect
	}
	items := make([]item, 0, directNodes+groupCount)
	for nodeID := range m.geo.Selection().DirectNodes() {
		items = append(items, item{
			member: ir.Member{ID: nodeID, Kind: ir.MemberNode},
			bounds: m.geo.Nodes[nodeID].Rect,
		})
	}
	for groupID := range m.geo.Selection().Groups() {
		bounds, _ := m.geo.GroupBounds(groupID)
		items = append(items, item{
			member: ir.Member{ID: groupID, Kind: ir.MemberGroup},
			bounds: bounds,
		})
	}
	slices.SortFunc(items, func(a, b item) int {
		if order := cmp.Compare(a.bounds.Min.Y, b.bounds.Min.Y); order != 0 {
			return order
		}
		if order := cmp.Compare(a.bounds.Min.X, b.bounds.Min.X); order != 0 {
			return order
		}
		if order := cmp.Compare(a.member.Kind, b.member.Kind); order != 0 {
			return order
		}
		return cmp.Compare(a.member.ID, b.member.ID)
	})
	members := make([]ir.Member, len(items))
	for i, selected := range items {
		members[i] = selected.member
	}
	m.beginTransaction(transactionImmediate)
	groupID, err := m.geo.NewGroup(members)
	if err != nil {
		_ = m.cancelTransaction()
		if errors.Is(err, ir.ErrMembersNotSiblings) {
			m.setError("group members must share one parent; drill into the same group first")
		} else {
			m.setError(err.Error())
		}
		return
	}
	m.geo.Selection().SelectOnly(layout.Hit{ID: groupID, Kind: layout.HitGroup})
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.refreshSelectionHighlight()
	m.status = "grouped selection"
}

func (m *Model) beginAreaSelection(point layout.Point) {
	m.clearSelection()
	m.interaction.gesture = pointerGesture{
		kind:  gestureAreaSelection,
		start: point,
		point: point,
	}
	m.status = ""
}

func (m *Model) updateAreaSelection(point layout.Point) {
	m.interaction.gesture.point = point
	m.cursor = point
}

func (m *Model) finishAreaSelection() {
	m.geo.Selection().SelectArea(
		m.interaction.gesture.start,
		m.interaction.gesture.point,
	)
	m.refreshSelectionHighlight()
	m.interaction.resetGesture()
	m.refreshHits()
	m.status = ""
}

func (m *Model) marqueeArea() selectionArea {
	return newSelectionArea(
		m.interaction.gesture.start,
		m.interaction.gesture.point,
	)
}

func (m *Model) duplicateSelection(dx, dy int64) {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	if !m.hasSelectedNodes() {
		hit, ok := m.activeHit()
		if !ok || hit.Kind != layout.HitNode {
			m.setError("select at least one node to duplicate")
			return
		}
		m.selectOnly(hit)
	}
	m.beginTransaction(transactionDuplicate)
	if err := m.geo.DuplicateSelection(dx, dy); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.rebuild(); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction(), m.render()).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	if hit, ok := m.firstSelectedNode(); ok {
		m.target = hit
		m.cursor = m.geo.Nodes[hit.ID].LabelPoint
	}
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) duplicateSelectionDefault() {
	var (
		minX  uint32
		maxX  uint32
		found bool
	)
	for nodeID := range m.geo.Selection().Nodes() {
		rect := m.geo.Nodes[nodeID].Rect
		if !found {
			minX = rect.Min.X
			found = true
		}
		minX = min(minX, rect.Min.X)
		maxX = max(maxX, rect.Max().X)
	}
	if !found {
		if hit, ok := m.activeHit(); ok && hit.Kind == layout.HitNode {
			rect := m.geo.Nodes[hit.ID].Rect
			minX, maxX = rect.Min.X, rect.Max().X
		}
	}
	m.duplicateSelection(int64(maxX-minX)+2, 0)
}
