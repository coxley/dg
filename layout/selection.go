package layout

import (
	"iter"
	"slices"

	"github.com/coxley/dg/ir"
)

// Selection tracks selected layout objects by their index-aligned IDs.
type Selection struct {
	layout *Layout

	nodes []bool
	edges []bool

	nodeCount int
	edgeCount int
	expanded  bool

	components        ir.Components
	selectedComponent []bool
}

// Selection returns the layout's current selection.
func (l *Layout) Selection() *Selection {
	return &l.selection
}

// Clear removes every object from the selection.
func (s *Selection) Clear() {
	clear(s.nodes)
	clear(s.edges)
	s.nodeCount = 0
	s.edgeCount = 0
	s.expanded = false
}

// Empty reports whether the selection contains no objects.
func (s *Selection) Empty() bool {
	return s.nodeCount == 0 && s.edgeCount == 0
}

// HasNodes reports whether the selection contains at least one node.
func (s *Selection) HasNodes() bool {
	return s.nodeCount != 0
}

// Counts returns the number of selected nodes and edges.
func (s *Selection) Counts() (nodes, edges int) {
	return s.nodeCount, s.edgeCount
}

// Contains reports whether hit is selected.
func (s *Selection) Contains(hit Hit) bool {
	switch hit.Kind {
	case HitNode:
		return uint64(hit.ID) < uint64(len(s.nodes)) && s.nodes[hit.ID]
	case HitEdge:
		return uint64(hit.ID) < uint64(len(s.edges)) && s.edges[hit.ID]
	default:
		return false
	}
}

// SelectOnly replaces the selection with hit. Ports remain connection handles
// and cannot join a selection.
func (s *Selection) SelectOnly(hit Hit) bool {
	s.ensureCapacity()
	s.Clear()
	return s.selectHit(hit)
}

// Toggle adds hit when unselected and removes it when selected. Ports remain
// connection handles and cannot join a selection.
func (s *Selection) Toggle(hit Hit) bool {
	s.ensureCapacity()
	switch hit.Kind {
	case HitNode:
		if !s.layout.NodeExists(hit.ID) {
			return false
		}
		s.nodes[hit.ID] = !s.nodes[hit.ID]
		s.nodeCount += countDelta(s.nodes[hit.ID])
	case HitEdge:
		if !s.layout.EdgeExists(hit.ID) {
			return false
		}
		s.edges[hit.ID] = !s.edges[hit.ID]
		s.edgeCount += countDelta(s.edges[hit.ID])
	default:
		return false
	}
	s.expanded = false
	return true
}

// FirstNode returns the lowest selected live node ID.
func (s *Selection) FirstNode() (uint32, bool) {
	for nodeID, selected := range s.nodes {
		if selected && s.layout.NodeExists(uint32(nodeID)) {
			return uint32(nodeID), true
		}
	}
	return 0, false
}

// Nodes yields selected live node IDs.
func (s *Selection) Nodes() iter.Seq[uint32] {
	return func(yield func(uint32) bool) {
		for nodeID, selected := range s.nodes {
			if selected && s.layout.NodeExists(uint32(nodeID)) &&
				!yield(uint32(nodeID)) {
				return
			}
		}
	}
}

// Edges yields selected live edge IDs.
func (s *Selection) Edges() iter.Seq[uint32] {
	return func(yield func(uint32) bool) {
		for edgeID, selected := range s.edges {
			if selected && s.layout.EdgeExists(uint32(edgeID)) &&
				!yield(uint32(edgeID)) {
				return
			}
		}
	}
}

// SelectAll selects every live node and edge.
func (s *Selection) SelectAll() {
	s.ensureCapacity()
	s.Clear()
	for nodeID := range s.layout.Nodes {
		if s.layout.NodeExists(uint32(nodeID)) {
			s.nodes[nodeID] = true
			s.nodeCount++
		}
	}
	for edgeID := range s.layout.Edges {
		if s.layout.EdgeExists(uint32(edgeID)) {
			s.edges[edgeID] = true
			s.edgeCount++
		}
	}
	s.expanded = true
}

// Expand selects each connected component containing a selected object. A
// second call without an intervening selection change selects every object.
func (s *Selection) Expand() {
	s.ensureCapacity()
	if s.Empty() || s.expanded {
		s.SelectAll()
		return
	}

	s.components.Build(&s.layout.graph)
	clear(s.selectedComponent)
	for nodeID := range s.Nodes() {
		componentID, _ := s.components.ID(nodeID)
		s.selectedComponent[componentID] = true
	}
	for edgeID := range s.Edges() {
		nodeA, _, err := s.layout.EdgeNodes(edgeID)
		if err == nil {
			componentID, _ := s.components.ID(nodeA)
			s.selectedComponent[componentID] = true
		}
	}
	for nodeID := range s.layout.Nodes {
		componentID, live := s.components.ID(uint32(nodeID))
		s.nodes[nodeID] = live && s.selectedComponent[componentID]
	}
	for edgeID := range s.layout.Edges {
		if !s.layout.EdgeExists(uint32(edgeID)) {
			s.edges[edgeID] = false
			continue
		}
		nodeA, _, err := s.layout.EdgeNodes(uint32(edgeID))
		if err != nil {
			s.edges[edgeID] = false
			continue
		}
		componentID, live := s.components.ID(nodeA)
		s.edges[edgeID] = live && s.selectedComponent[componentID]
	}
	s.recount()
	s.expanded = true
}

// SelectArea selects nodes and routed edges intersecting the inclusive area
// between a and b.
func (s *Selection) SelectArea(a, b Point) {
	s.ensureCapacity()
	s.Clear()
	area := selectionArea{
		min: NewPoint(min(a.X, b.X), min(a.Y, b.Y)),
		max: NewPoint(max(a.X, b.X), max(a.Y, b.Y)),
	}
	for nodeID, node := range s.layout.Nodes {
		if s.layout.NodeExists(uint32(nodeID)) && area.intersectsRect(node.Rect) {
			s.nodes[nodeID] = true
			s.nodeCount++
		}
	}
	for edgeID, edge := range s.layout.Edges {
		if s.layout.EdgeExists(uint32(edgeID)) && area.intersectsEdge(edge) {
			s.edges[edgeID] = true
			s.edgeCount++
		}
	}
}

func (s *Selection) attach(l *Layout) {
	s.layout = l
}

func (s *Selection) ensureCapacity() {
	s.nodes = growSelection(s.nodes, len(s.layout.Nodes))
	s.edges = growSelection(s.edges, len(s.layout.Edges))
	s.selectedComponent = growSelection(
		s.selectedComponent,
		len(s.layout.Nodes),
	)
}

func growSelection[S ~[]E, E any](values S, length int) S {
	if length <= len(values) {
		return values
	}
	return slices.Grow(values, length-len(values))[:length]
}

func (s *Selection) selectHit(hit Hit) bool {
	switch hit.Kind {
	case HitNode:
		if s.layout.NodeExists(hit.ID) {
			s.nodes[hit.ID] = true
			s.nodeCount = 1
			return true
		}
	case HitEdge:
		if s.layout.EdgeExists(hit.ID) {
			s.edges[hit.ID] = true
			s.edgeCount = 1
			return true
		}
	case HitPort:
	}
	return false
}

func (s *Selection) discard(hit Hit) {
	if !s.Contains(hit) {
		return
	}
	switch hit.Kind {
	case HitNode:
		s.nodes[hit.ID] = false
		s.nodeCount--
	case HitEdge:
		s.edges[hit.ID] = false
		s.edgeCount--
	case HitPort:
	}
	s.expanded = false
}

func (s *Selection) recount() {
	s.nodeCount = 0
	s.edgeCount = 0
	for _, selected := range s.nodes {
		if selected {
			s.nodeCount++
		}
	}
	for _, selected := range s.edges {
		if selected {
			s.edgeCount++
		}
	}
}

func countDelta(selected bool) int {
	if selected {
		return 1
	}
	return -1
}

type selectionArea struct {
	min Point
	max Point
}

func (a selectionArea) intersectsRect(rect Rect) bool {
	if rect.Empty() {
		return false
	}
	limit := rect.Max()
	return rect.Min.X <= a.max.X && limit.X-1 >= a.min.X &&
		rect.Min.Y <= a.max.Y && limit.Y-1 >= a.min.Y
}

func (a selectionArea) intersectsEdge(edge Edge) bool {
	for i := 1; i < len(edge.Points); i++ {
		start, end := edge.Points[i-1], edge.Points[i]
		if min(start.X, end.X) <= a.max.X &&
			max(start.X, end.X) >= a.min.X &&
			min(start.Y, end.Y) <= a.max.Y &&
			max(start.Y, end.Y) >= a.min.Y {
			return true
		}
	}
	return false
}
