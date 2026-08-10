package layout

import (
	"iter"
	"slices"

	"github.com/coxley/dg/ir"
)

// Selection tracks selected layout objects by their index-aligned IDs.
type Selection struct {
	layout *Layout

	nodes  []bool
	edges  []bool
	groups []bool

	nodeCount  int
	edgeCount  int
	groupCount int
	expanded   bool

	attachmentEdge     uint32
	attachmentExpanded bool

	components        ir.Components
	selectedComponent []bool
}

type selectionState struct {
	nodes              []bool
	edges              []bool
	groups             []bool
	nodeCount          int
	edgeCount          int
	groupCount         int
	expanded           bool
	attachmentEdge     uint32
	attachmentExpanded bool
}

// SelectionSnapshot owns one independent logical selection state.
type SelectionSnapshot struct {
	state selectionState
}

// Snapshot returns an independent copy of the selection.
func (s *Selection) Snapshot() SelectionSnapshot {
	return SelectionSnapshot{state: s.state()}
}

// Restore replaces the selection with snapshot and discards deleted objects.
func (s *Selection) Restore(snapshot SelectionSnapshot) {
	s.restore(snapshot.state)
	s.ensureCapacity()
	s.prune()
}

func (s *Selection) state() selectionState {
	return selectionState{
		nodes:              slices.Clone(s.nodes),
		edges:              slices.Clone(s.edges),
		groups:             slices.Clone(s.groups),
		nodeCount:          s.nodeCount,
		edgeCount:          s.edgeCount,
		groupCount:         s.groupCount,
		expanded:           s.expanded,
		attachmentEdge:     s.attachmentEdge,
		attachmentExpanded: s.attachmentExpanded,
	}
}

func (s *Selection) restore(state selectionState) {
	s.nodes = slices.Clone(state.nodes)
	s.edges = slices.Clone(state.edges)
	s.groups = slices.Clone(state.groups)
	s.nodeCount = state.nodeCount
	s.edgeCount = state.edgeCount
	s.groupCount = state.groupCount
	s.expanded = state.expanded
	s.attachmentEdge = state.attachmentEdge
	s.attachmentExpanded = state.attachmentExpanded
}

// Selection returns the layout's current selection.
func (l *Layout) Selection() *Selection {
	return &l.selection
}

// Clear removes every object from the selection.
func (s *Selection) Clear() {
	clear(s.nodes)
	clear(s.edges)
	clear(s.groups)
	s.nodeCount = 0
	s.edgeCount = 0
	s.groupCount = 0
	s.expanded = false
	s.attachmentExpanded = false
}

// Empty reports whether the selection contains no objects.
func (s *Selection) Empty() bool {
	return s.nodeCount == 0 && s.edgeCount == 0 && s.groupCount == 0
}

// HasNodes reports whether the selection contains at least one node.
func (s *Selection) HasNodes() bool {
	return s.nodeCount != 0 || s.groupCount != 0
}

// Counts returns the number of selected nodes and edges.
func (s *Selection) Counts() (nodes, edges int) {
	for range s.Nodes() {
		nodes++
	}
	return nodes, s.edgeCount
}

// LogicalCounts returns directly selected node, group, and edge counts.
func (s *Selection) LogicalCounts() (nodes, groups, edges int) {
	return s.nodeCount, s.groupCount, s.edgeCount
}

// Contains reports whether hit is selected.
func (s *Selection) Contains(hit Hit) bool {
	switch hit.Kind {
	case HitNode:
		if uint64(hit.ID) >= uint64(len(s.nodes)) || !s.layout.NodeExists(hit.ID) {
			return false
		}
		if s.nodes[hit.ID] {
			return true
		}
		member := ir.Member{ID: hit.ID, Kind: ir.MemberNode}
		for {
			parentID, ok := s.layout.graph.Parent(member)
			if !ok {
				return false
			}
			if uint64(parentID) < uint64(len(s.groups)) && s.groups[parentID] {
				return true
			}
			member = ir.Member{ID: parentID, Kind: ir.MemberGroup}
		}
	case HitEdge:
		return uint64(hit.ID) < uint64(len(s.edges)) && s.edges[hit.ID]
	case HitGroup:
		return uint64(hit.ID) < uint64(len(s.groups)) && s.groups[hit.ID]
	default:
		return false
	}
}

// DirectlyContains reports whether hit is a logical selection item rather than
// a descendant of one.
func (s *Selection) DirectlyContains(hit Hit) bool {
	switch hit.Kind {
	case HitNode:
		return uint64(hit.ID) < uint64(len(s.nodes)) && s.nodes[hit.ID]
	case HitEdge:
		return uint64(hit.ID) < uint64(len(s.edges)) && s.edges[hit.ID]
	case HitGroup:
		return uint64(hit.ID) < uint64(len(s.groups)) && s.groups[hit.ID]
	default:
		return false
	}
}

// SelectOnly replaces the selection with hit. Ports remain connection handles
// and cannot join a selection.
func (s *Selection) SelectOnly(hit Hit) bool {
	s.ensureCapacity()
	collapseAttachments := hit.Kind == HitEdge &&
		s.attachmentExpanded &&
		s.attachmentEdge == hit.ID
	s.Clear()
	if !s.selectHit(hit) {
		return false
	}
	if hit.Kind != HitEdge || collapseAttachments {
		return true
	}
	for attachment := range s.layout.Attachments(hit.ID) {
		if s.selectHit(Hit{ID: attachment.NodeID, Kind: HitNode}) {
			s.attachmentExpanded = true
			s.attachmentEdge = hit.ID
		}
	}
	return true
}

// Toggle adds hit when unselected and removes it when selected. Ports remain
// connection handles and cannot join a selection.
func (s *Selection) Toggle(hit Hit) bool {
	s.ensureCapacity()
	switch hit.Kind {
	case HitNode:
		return s.toggleMember(ir.Member{ID: hit.ID, Kind: ir.MemberNode})
	case HitGroup:
		return s.toggleMember(ir.Member{ID: hit.ID, Kind: ir.MemberGroup})
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
	s.attachmentExpanded = false
	return true
}

func (s *Selection) toggleMember(member ir.Member) bool {
	if member.Kind == ir.MemberNode && !s.layout.NodeExists(member.ID) ||
		member.Kind == ir.MemberGroup && !s.layout.GroupExists(member.ID) {
		return false
	}
	hit := memberHit(member)
	if s.DirectlyContains(hit) {
		s.discard(hit)
		return true
	}
	s.discardMemberAncestors(member)
	if member.Kind == ir.MemberGroup {
		s.discardGroupDescendants(member.ID)
		s.groups[member.ID] = true
		s.groupCount++
	} else {
		s.nodes[member.ID] = true
		s.nodeCount++
	}
	s.expanded = false
	s.attachmentExpanded = false
	return true
}

// FirstNode returns the lowest selected live node ID.
func (s *Selection) FirstNode() (uint32, bool) {
	for nodeID := range s.Nodes() {
		return nodeID, true
	}
	return 0, false
}

// Nodes yields selected live node IDs.
func (s *Selection) Nodes() iter.Seq[uint32] {
	return func(yield func(uint32) bool) {
		for nodeID, selected := range s.nodes {
			if selected && s.layout.NodeExists(uint32(nodeID)) && !yield(uint32(nodeID)) {
				return
			}
		}
		for groupID, selected := range s.groups {
			if !selected || !s.layout.GroupExists(uint32(groupID)) {
				continue
			}
			for nodeID := range s.layout.graph.DescendantNodes(uint32(groupID)) {
				if !yield(nodeID) {
					return
				}
			}
		}
	}
}

// DirectNodes yields directly selected live nodes.
func (s *Selection) DirectNodes() iter.Seq[uint32] {
	return func(yield func(uint32) bool) {
		for nodeID, selected := range s.nodes {
			if selected && s.layout.NodeExists(uint32(nodeID)) && !yield(uint32(nodeID)) {
				return
			}
		}
	}
}

// Groups yields directly selected live groups.
func (s *Selection) Groups() iter.Seq[uint32] {
	return func(yield func(uint32) bool) {
		for groupID, selected := range s.groups {
			if selected && s.layout.GroupExists(uint32(groupID)) && !yield(uint32(groupID)) {
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
			member := s.layout.graph.Outermost(ir.Member{ID: uint32(nodeID), Kind: ir.MemberNode})
			s.selectMember(member)
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
	clear(s.groups)
	s.groupCount = 0
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
	selectedNodes := slices.Clone(s.nodes)
	clear(s.nodes)
	s.nodeCount = 0
	for nodeID, selected := range selectedNodes {
		if selected {
			s.selectMember(s.layout.graph.Outermost(ir.Member{ID: uint32(nodeID), Kind: ir.MemberNode}))
		}
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
			member := s.layout.graph.Outermost(ir.Member{ID: uint32(nodeID), Kind: ir.MemberNode})
			s.selectMember(member)
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
	s.groups = growSelection(s.groups, len(s.layout.graph.Groups))
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
	case HitGroup:
		if s.layout.GroupExists(hit.ID) {
			s.groups[hit.ID] = true
			s.groupCount = 1
			return true
		}
	case HitPort:
	}
	return false
}

func (s *Selection) discard(hit Hit) {
	if !s.DirectlyContains(hit) {
		return
	}
	switch hit.Kind {
	case HitNode:
		s.nodes[hit.ID] = false
		s.nodeCount--
	case HitEdge:
		s.edges[hit.ID] = false
		s.edgeCount--
	case HitGroup:
		s.groups[hit.ID] = false
		s.groupCount--
	case HitPort:
	}
	s.expanded = false
}

func (s *Selection) recount() {
	s.nodeCount = 0
	s.edgeCount = 0
	s.groupCount = 0
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
	for _, selected := range s.groups {
		if selected {
			s.groupCount++
		}
	}
}

func (s *Selection) prune() {
	for nodeID, selected := range s.nodes {
		if selected && !s.layout.NodeExists(uint32(nodeID)) {
			s.nodes[nodeID] = false
		}
	}
	for edgeID, selected := range s.edges {
		if selected && !s.layout.EdgeExists(uint32(edgeID)) {
			s.edges[edgeID] = false
		}
	}
	for groupID, selected := range s.groups {
		if selected && !s.layout.GroupExists(uint32(groupID)) {
			s.groups[groupID] = false
		}
	}
	s.recount()
}

func (s *Selection) selectMember(member ir.Member) {
	hit := memberHit(member)
	if s.DirectlyContains(hit) {
		return
	}
	if member.Kind == ir.MemberGroup {
		s.groups[member.ID] = true
		s.groupCount++
	} else {
		s.nodes[member.ID] = true
		s.nodeCount++
	}
}

func (s *Selection) discardMemberAncestors(member ir.Member) {
	for {
		parentID, ok := s.layout.graph.Parent(member)
		if !ok {
			return
		}
		if uint64(parentID) < uint64(len(s.groups)) && s.groups[parentID] {
			s.groups[parentID] = false
			s.groupCount--
		}
		member = ir.Member{ID: parentID, Kind: ir.MemberGroup}
	}
}

func (s *Selection) discardGroupDescendants(groupID uint32) {
	for _, member := range s.layout.graph.Groups[groupID].Members {
		hit := memberHit(member)
		if s.DirectlyContains(hit) {
			s.discard(hit)
		}
		if member.Kind == ir.MemberGroup {
			s.discardGroupDescendants(member.ID)
		}
	}
}

func memberHit(member ir.Member) Hit {
	kind := HitNode
	if member.Kind == ir.MemberGroup {
		kind = HitGroup
	}
	return Hit{ID: member.ID, Kind: kind}
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
