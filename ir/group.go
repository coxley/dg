package ir

import (
	"errors"
	"fmt"
	"iter"
	"slices"
)

var (
	ErrGroupNotFound      = errors.New("group not found")
	ErrGroupTooSmall      = errors.New("group requires at least two members")
	ErrMemberNotFound     = errors.New("group member not found")
	ErrMembersNotSiblings = errors.New("group members must share one parent")
	ErrDuplicateMember    = errors.New("duplicate group member")
)

// MemberKind identifies one semantic object that may belong to a group.
type MemberKind uint8

const (
	// MemberNode identifies a node.
	MemberNode MemberKind = iota + 1
	// MemberGroup identifies a nested group.
	MemberGroup
)

// Member identifies one node or nested group.
type Member struct {
	ID   uint32
	Kind MemberKind
}

// Group stores ordered immediate members.
type Group struct {
	Members []Member
}

// Empty reports whether the group is a tombstone.
func (g Group) Empty() bool {
	return len(g.Members) == 0
}

// NewGroup wraps live sibling members in one new parent.
func (g *Graph) NewGroup(members []Member) (uint32, error) {
	if len(members) < 2 {
		return 0, ErrGroupTooSmall
	}
	parentID, hasParent := g.Parent(members[0])
	for i, member := range members {
		if !g.memberExists(member) {
			return 0, fmt.Errorf("%w: %s %d", ErrMemberNotFound, member.Kind, member.ID)
		}
		candidateParent, candidateHasParent := g.Parent(member)
		if candidateHasParent != hasParent || hasParent && candidateParent != parentID {
			return 0, ErrMembersNotSiblings
		}
		if slices.Contains(members[:i], member) {
			return 0, fmt.Errorf("%w: %s %d", ErrDuplicateMember, member.Kind, member.ID)
		}
	}

	groupID := g.NextGroupID()
	var retained []Member
	if int(groupID) < len(g.Groups) {
		retained = g.Groups[groupID].Members
	}
	group := Group{Members: append(retained[:0], members...)}
	if int(groupID) == len(g.Groups) {
		g.Groups = append(g.Groups, group)
	} else {
		g.freeGroups = g.freeGroups[:len(g.freeGroups)-1]
		g.Groups[groupID] = group
	}
	if hasParent {
		g.wrapMembers(parentID, members, Member{ID: groupID, Kind: MemberGroup})
	}
	return groupID, nil
}

// NextGroupID returns the ID that NewGroup will allocate.
func (g *Graph) NextGroupID() uint32 {
	if len(g.freeGroups) != 0 {
		return g.freeGroups[len(g.freeGroups)-1]
	}
	return uint32(len(g.Groups))
}

// GroupExists reports whether groupID identifies a live group.
func (g *Graph) GroupExists(groupID uint32) bool {
	return uint64(groupID) < uint64(len(g.Groups)) && !g.Groups[groupID].Empty()
}

// Parent returns the immediate group containing member.
func (g *Graph) Parent(member Member) (uint32, bool) {
	for groupID, group := range g.Groups {
		if g.GroupExists(uint32(groupID)) && slices.Contains(group.Members, member) {
			return uint32(groupID), true
		}
	}
	return 0, false
}

// Outermost returns member's outermost containing group, or member itself when
// it has no parent.
func (g *Graph) Outermost(member Member) Member {
	for {
		parentID, ok := g.Parent(member)
		if !ok {
			return member
		}
		member = Member{ID: parentID, Kind: MemberGroup}
	}
}

// ChildContaining returns the immediate child of groupID that contains member.
func (g *Graph) ChildContaining(groupID uint32, member Member) (Member, bool) {
	if !g.GroupExists(groupID) || !g.memberExists(member) {
		return Member{}, false
	}
	for _, child := range g.Groups[groupID].Members {
		if child == member || child.Kind == MemberGroup && g.contains(child.ID, member) {
			return child, true
		}
	}
	return Member{}, false
}

func (g *Graph) contains(groupID uint32, member Member) bool {
	for _, child := range g.Groups[groupID].Members {
		if child == member || child.Kind == MemberGroup && g.contains(child.ID, member) {
			return true
		}
	}
	return false
}

// DescendantNodes yields every live descendant node in group order.
func (g *Graph) DescendantNodes(groupID uint32) iter.Seq[uint32] {
	return func(yield func(uint32) bool) {
		if !g.GroupExists(groupID) {
			return
		}
		g.yieldDescendantNodes(groupID, yield)
	}
}

func (g *Graph) yieldDescendantNodes(groupID uint32, yield func(uint32) bool) bool {
	for _, member := range g.Groups[groupID].Members {
		switch member.Kind {
		case MemberNode:
			if !yield(member.ID) {
				return false
			}
		case MemberGroup:
			if !g.yieldDescendantNodes(member.ID, yield) {
				return false
			}
		}
	}
	return true
}

// Ungroup removes one group level and returns its former immediate members.
func (g *Graph) Ungroup(groupID uint32) ([]Member, error) {
	if !g.GroupExists(groupID) {
		return nil, fmt.Errorf("%w: %d", ErrGroupNotFound, groupID)
	}
	members := slices.Clone(g.Groups[groupID].Members)
	if parentID, ok := g.Parent(Member{ID: groupID, Kind: MemberGroup}); ok {
		g.replaceMember(parentID, Member{ID: groupID, Kind: MemberGroup}, members)
	}
	g.deleteGroupSlot(groupID)
	return members, nil
}

func (g *Graph) removeFromParent(member Member) {
	parentID, ok := g.Parent(member)
	if !ok {
		return
	}
	group := &g.Groups[parentID]
	for i, candidate := range group.Members {
		if candidate == member {
			copy(group.Members[i:], group.Members[i+1:])
			group.Members = group.Members[:len(group.Members)-1]
			break
		}
	}
	g.cleanupGroup(parentID)
}

func (g *Graph) cleanupGroup(groupID uint32) {
	if uint64(groupID) >= uint64(len(g.Groups)) {
		return
	}
	switch len(g.Groups[groupID].Members) {
	case 0:
		g.removeFromParent(Member{ID: groupID, Kind: MemberGroup})
		g.deleteGroupSlot(groupID)
	case 1:
		member := g.Groups[groupID].Members[0]
		if parentID, ok := g.Parent(Member{ID: groupID, Kind: MemberGroup}); ok {
			g.replaceMember(parentID, Member{ID: groupID, Kind: MemberGroup}, []Member{member})
			g.deleteGroupSlot(groupID)
			g.cleanupGroup(parentID)
			return
		}
		g.deleteGroupSlot(groupID)
	}
}

func (g *Graph) wrapMembers(groupID uint32, members []Member, replacement Member) {
	group := &g.Groups[groupID]
	first := len(group.Members)
	for i, member := range group.Members {
		if slices.Contains(members, member) {
			first = min(first, i)
		}
	}
	kept := group.Members[:0]
	for _, member := range group.Members {
		if !slices.Contains(members, member) {
			kept = append(kept, member)
		}
	}
	kept = append(kept, Member{})
	copy(kept[first+1:], kept[first:])
	kept[first] = replacement
	group.Members = kept
}

func (g *Graph) replaceMember(groupID uint32, old Member, replacements []Member) {
	group := &g.Groups[groupID]
	for i, member := range group.Members {
		if member != old {
			continue
		}
		members := slices.Grow(group.Members, len(replacements)-1)
		members = members[:len(group.Members)+len(replacements)-1]
		copy(members[i+len(replacements):], group.Members[i+1:])
		copy(members[i:], replacements)
		group.Members = members
		return
	}
}

func (g *Graph) deleteGroupSlot(groupID uint32) {
	members := g.Groups[groupID].Members
	g.Groups[groupID] = Group{Members: members[:0]}
	g.freeGroups = append(g.freeGroups, groupID)
}

func (g *Graph) memberExists(member Member) bool {
	switch member.Kind {
	case MemberNode:
		return g.NodeExists(member.ID)
	case MemberGroup:
		return g.GroupExists(member.ID)
	default:
		return false
	}
}

func (g *Graph) validateGroups() error {
	nodeParents := make([]bool, len(g.Nodes))
	groupParents := make([]bool, len(g.Groups))
	for groupID, group := range g.Groups {
		if !g.GroupExists(uint32(groupID)) {
			continue
		}
		if len(group.Members) < 2 {
			return fmt.Errorf("group %d has fewer than two members", groupID)
		}
		for _, member := range group.Members {
			if !g.memberExists(member) {
				return fmt.Errorf("group %d references unknown %s %d", groupID, member.Kind, member.ID)
			}
			var seen *bool
			switch member.Kind {
			case MemberNode:
				seen = &nodeParents[member.ID]
			case MemberGroup:
				seen = &groupParents[member.ID]
			}
			if *seen {
				return fmt.Errorf("%s %d belongs to multiple groups", member.Kind, member.ID)
			}
			*seen = true
		}
	}
	states := make([]uint8, len(g.Groups))
	for groupID := range g.Groups {
		if g.GroupExists(uint32(groupID)) && g.groupCycle(uint32(groupID), states) {
			return fmt.Errorf("group %d creates a membership cycle", groupID)
		}
	}
	return nil
}

func (g *Graph) groupCycle(groupID uint32, states []uint8) bool {
	if states[groupID] != 0 {
		return states[groupID] == 1
	}
	states[groupID] = 1
	for _, member := range g.Groups[groupID].Members {
		if member.Kind == MemberGroup && g.groupCycle(member.ID, states) {
			return true
		}
	}
	states[groupID] = 2
	return false
}

func (k MemberKind) String() string {
	switch k {
	case MemberNode:
		return "node"
	case MemberGroup:
		return "group"
	default:
		return "unknown"
	}
}
