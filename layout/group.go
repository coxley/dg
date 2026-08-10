package layout

import (
	"fmt"
	"math"
	"slices"

	"github.com/coxley/dg/ir"
)

// NewGroup wraps sibling members in one logical group.
func (l *Layout) NewGroup(members []ir.Member) (uint32, error) {
	before := cloneGroups(l.graph.Groups)
	groupID, err := l.graph.NewGroup(members)
	if err != nil {
		return 0, err
	}
	if l.recordingChanges() {
		l.recordChange(historyChange{
			Kind:   historySetGroups,
			Before: historyChangeState{Groups: before},
			After:  historyChangeState{Groups: cloneGroups(l.graph.Groups)},
		})
	}
	return groupID, nil
}

// Ungroup removes exactly one group level and returns its former children.
func (l *Layout) Ungroup(groupID uint32) ([]ir.Member, error) {
	before := cloneGroups(l.graph.Groups)
	wasSelected := l.selection.DirectlyContains(Hit{ID: groupID, Kind: HitGroup})
	members, err := l.graph.Ungroup(groupID)
	if err != nil {
		return nil, err
	}
	if l.recordingChanges() {
		l.recordChange(historyChange{
			Kind:   historySetGroups,
			Before: historyChangeState{Groups: before},
			After:  historyChangeState{Groups: cloneGroups(l.graph.Groups)},
		})
	}
	if wasSelected {
		l.selection.discard(Hit{ID: groupID, Kind: HitGroup})
		l.selection.ensureCapacity()
		for _, member := range members {
			l.selection.Toggle(memberHit(member))
		}
	}
	return members, nil
}

// DeleteGroup deletes every descendant node and its incident edges.
func (l *Layout) DeleteGroup(groupID uint32) error {
	if !l.graph.GroupExists(groupID) {
		return fmt.Errorf("%w: %d", ir.ErrGroupNotFound, groupID)
	}
	nodes := slices.Collect(l.graph.DescendantNodes(groupID))
	for _, nodeID := range nodes {
		if err := l.DeleteNode(nodeID); err != nil {
			return err
		}
	}
	return nil
}

// GroupExists reports whether groupID identifies a live logical group.
func (l *Layout) GroupExists(groupID uint32) bool {
	return l.graph.GroupExists(groupID)
}

// GroupMembers returns an independent copy of a group's immediate members.
func (l *Layout) GroupMembers(groupID uint32) ([]ir.Member, bool) {
	if !l.graph.GroupExists(groupID) {
		return nil, false
	}
	return slices.Clone(l.graph.Groups[groupID].Members), true
}

// OutermostMember returns member's outermost logical selection item.
func (l *Layout) OutermostMember(member ir.Member) ir.Member {
	return l.graph.Outermost(member)
}

// MemberParent returns member's immediate containing group.
func (l *Layout) MemberParent(member ir.Member) (uint32, bool) {
	return l.graph.Parent(member)
}

// GroupChildContaining returns the immediate child containing member.
func (l *Layout) GroupChildContaining(groupID uint32, member ir.Member) (ir.Member, bool) {
	return l.graph.ChildContaining(groupID, member)
}

// GroupNodes yields a group's descendant nodes in member order.
func (l *Layout) GroupNodes(groupID uint32) func(func(uint32) bool) {
	return l.graph.DescendantNodes(groupID)
}

// GroupBounds returns the union of a group's descendant node bounds.
func (l *Layout) GroupBounds(groupID uint32) (Rect, bool) {
	if !l.graph.GroupExists(groupID) {
		return Rect{}, false
	}
	minX, minY := uint32(math.MaxUint32), uint32(math.MaxUint32)
	var maxX, maxY uint32
	for nodeID := range l.graph.DescendantNodes(groupID) {
		rect := l.Nodes[nodeID].Rect
		minX, minY = min(minX, rect.Min.X), min(minY, rect.Min.Y)
		limit := rect.Max()
		maxX, maxY = max(maxX, limit.X), max(maxY, limit.Y)
	}
	if minX == math.MaxUint32 {
		return Rect{}, false
	}
	return Rect{
		Min:  NewPoint(minX, minY),
		Size: Size{Width: maxX - minX, Height: maxY - minY},
	}, true
}

func (l *Layout) setGroups(groups []ir.Group) error {
	l.graph.Groups = cloneGroups(groups)
	l.graph = l.graph.Clone()
	return nil
}

func cloneGroups(groups []ir.Group) []ir.Group {
	cloned := make([]ir.Group, len(groups))
	for groupID, group := range groups {
		cloned[groupID] = ir.Group{Members: slices.Clone(group.Members)}
	}
	return cloned
}

func (l *Layout) rootGroups() []uint32 {
	groups := make([]uint32, 0)
	for groupID := range l.graph.Groups {
		id := uint32(groupID)
		if !l.graph.GroupExists(id) {
			continue
		}
		if _, hasParent := l.graph.Parent(ir.Member{ID: id, Kind: ir.MemberGroup}); !hasParent {
			groups = append(groups, id)
		}
	}
	return groups
}
