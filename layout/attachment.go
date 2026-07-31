package layout

import (
	"errors"
	"fmt"
	"iter"
	"slices"
)

const (
	attachmentBuildPasses = 8
	attachmentPositionMax = ^uint16(0)
)

// Attachment keeps a node at a stable relative position along a routed edge.
// Anchor is the edge cell's offset from the node origin.
type Attachment struct {
	NodeID   uint32
	EdgeID   uint32
	Position uint16
	Anchor   Point
}

// NodeAttachment returns nodeID's attachment.
func (l *Layout) NodeAttachment(nodeID uint32) (Attachment, bool) {
	if !l.NodeExists(nodeID) ||
		uint64(nodeID) >= uint64(len(l.attachments)) ||
		l.attachments[nodeID] == (Attachment{}) {
		return Attachment{}, false
	}
	return l.attachments[nodeID], true
}

// Attachments yields the nodes attached to edgeID in node ID order.
func (l *Layout) Attachments(edgeID uint32) iter.Seq[Attachment] {
	return func(yield func(Attachment) bool) {
		if !l.EdgeExists(edgeID) {
			return
		}
		for nodeID, attachment := range l.attachments {
			if attachment != (Attachment{}) &&
				attachment.EdgeID == edgeID &&
				!yield(l.attachments[nodeID]) {
				return
			}
		}
	}
}

// CanAttachNode reports whether edgeID can host nodeID.
func (l *Layout) CanAttachNode(nodeID, edgeID uint32) bool {
	if !l.canHostNode(nodeID, edgeID) || l.Edges[edgeID].Empty() {
		return false
	}
	return true
}

func (l *Layout) canHostNode(nodeID, edgeID uint32) bool {
	if !l.NodeExists(nodeID) || !l.EdgeExists(edgeID) {
		return false
	}
	nodeA, nodeB, err := l.EdgeNodes(edgeID)
	return err == nil && nodeID != nodeA && nodeID != nodeB
}

// CanAttachNodeAt reports whether point can anchor nodeID to edgeID.
func (l *Layout) CanAttachNodeAt(nodeID, edgeID uint32, point Point) bool {
	if !l.CanAttachNode(nodeID, edgeID) ||
		!l.Nodes[nodeID].Rect.Contains(point) {
		return false
	}
	offset, length, ok := edgePosition(l.Edges[edgeID].Points, point)
	if !ok {
		return false
	}
	position := attachmentPosition(offset, length)
	return position != 0 && position != attachmentPositionMax
}

// AttachNode attaches nodeID to the edge cell at point. The node keeps its
// current origin until the host route changes.
func (l *Layout) AttachNode(nodeID, edgeID uint32, point Point) error {
	if !l.CanAttachNode(nodeID, edgeID) {
		return fmt.Errorf("node %d cannot attach to edge %d", nodeID, edgeID)
	}
	if !l.Nodes[nodeID].Rect.Contains(point) {
		return fmt.Errorf("attachment point %+v outside node %d", point, nodeID)
	}
	offset, length, ok := edgePosition(l.Edges[edgeID].Points, point)
	if !ok {
		return fmt.Errorf("point %+v is not on edge %d", point, edgeID)
	}
	origin := l.Nodes[nodeID].Rect.Min
	position := attachmentPosition(offset, length)
	if position == 0 || position == attachmentPositionMax {
		return errors.New("attachment cannot overlap an edge endpoint")
	}
	return l.SetAttachment(Attachment{
		NodeID:   nodeID,
		EdgeID:   edgeID,
		Position: position,
		Anchor:   NewPoint(point.X-origin.X, point.Y-origin.Y),
	})
}

// SetAttachment restores or updates an attachment from its stable route
// position. The mutation succeeds only when the complete layout remains
// buildable.
func (l *Layout) SetAttachment(attachment Attachment) error {
	return l.SetAttachments(attachment)
}

// SetAttachments atomically restores or updates attachments before routing.
func (l *Layout) SetAttachments(attachments ...Attachment) error {
	for i, attachment := range attachments {
		if err := l.validateAttachment(attachment); err != nil {
			return err
		}
		for _, previous := range attachments[:i] {
			if previous.NodeID == attachment.NodeID {
				return fmt.Errorf(
					"multiple attachments for node %d",
					attachment.NodeID,
				)
			}
		}
	}
	l.snapshotAttachmentBuildState(&l.attachmentMutationRollback)
	type attachmentChange struct {
		before      Attachment
		after       Attachment
		beforePoint Point
		had         bool
	}
	changes := make([]attachmentChange, 0, len(attachments))
	for _, attachment := range attachments {
		previous, hadPrevious := l.NodeAttachment(attachment.NodeID)
		if hadPrevious && previous == attachment {
			continue
		}
		l.attachments[attachment.NodeID] = attachment
		changes = append(changes, attachmentChange{
			before:      previous,
			after:       attachment,
			beforePoint: l.origins[attachment.NodeID],
			had:         hadPrevious,
		})
	}
	if len(changes) == 0 {
		return nil
	}
	if err := l.Build(); err != nil {
		l.restoreAttachmentBuildState(&l.attachmentMutationRollback)
		return fmt.Errorf("set attachments: %w", err)
	}
	if l.recordingChanges() {
		for _, change := range changes {
			l.recordChange(historyChange{
				Kind: historySetAttachment,
				ID:   change.after.NodeID,
				Before: historyChangeState{
					Attachment: change.before,
					Attached:   change.had,
					Point:      change.beforePoint,
				},
				After: historyChangeState{
					Attachment: change.after,
					Attached:   true,
					Point:      l.origins[change.after.NodeID],
				},
			})
		}
	}
	return nil
}

// DetachNode removes nodeID's attachment while keeping its current origin.
// The mutation succeeds only when the complete layout remains buildable.
func (l *Layout) DetachNode(nodeID uint32) error {
	previous, ok := l.NodeAttachment(nodeID)
	if !ok {
		if !l.NodeExists(nodeID) {
			return fmt.Errorf("node not found: %d", nodeID)
		}
		return nil
	}
	l.snapshotAttachmentBuildState(&l.attachmentMutationRollback)
	l.attachments[nodeID] = Attachment{}
	if err := l.Build(); err != nil {
		l.restoreAttachmentBuildState(&l.attachmentMutationRollback)
		return fmt.Errorf("detach node %d: %w", nodeID, err)
	}
	if l.recordingChanges() {
		l.recordChange(historyChange{
			Kind: historySetAttachment,
			ID:   nodeID,
			Before: historyChangeState{
				Attachment: previous,
				Attached:   true,
				Point:      l.attachmentMutationRollback.origins[nodeID],
			},
			After: historyChangeState{Point: l.origins[nodeID]},
		})
	}
	return nil
}

func (l *Layout) validateAttachment(attachment Attachment) error {
	if !l.canHostNode(attachment.NodeID, attachment.EdgeID) {
		return fmt.Errorf(
			"node %d cannot attach to edge %d",
			attachment.NodeID,
			attachment.EdgeID,
		)
	}
	if attachment.Position == 0 ||
		attachment.Position == attachmentPositionMax {
		return errors.New("attachment cannot overlap an edge endpoint")
	}
	rect := l.Nodes[attachment.NodeID].Rect
	if attachment.Anchor.X >= rect.Size.Width ||
		attachment.Anchor.Y >= rect.Size.Height {
		return errors.New("attachment anchor outside node")
	}
	return nil
}

func (l *Layout) setAttachmentState(
	nodeID uint32,
	attachment Attachment,
	attached bool,
) {
	l.attachments = growTo(l.attachments, int(nodeID)+1)
	if !attached {
		attachment = Attachment{}
	}
	l.attachments[nodeID] = attachment
}

func (l *Layout) attachmentsForNodeAndEdges(
	nodeID uint32,
	edges []historyEdge,
) iter.Seq[Attachment] {
	return func(yield func(Attachment) bool) {
		for candidate, attachment := range l.attachments {
			if attachment == (Attachment{}) {
				continue
			}
			hosted := slices.ContainsFunc(edges, func(edge historyEdge) bool {
				return edge.ID == attachment.EdgeID
			})
			if uint32(candidate) == nodeID || hosted {
				if !yield(attachment) {
					return
				}
			}
		}
	}
}

func (l *Layout) hasAttachments() bool {
	return slices.ContainsFunc(l.attachments, func(attachment Attachment) bool {
		return attachment != (Attachment{})
	})
}

func (l *Layout) buildAttachments(skipSelected bool) error {
	for range attachmentBuildPasses {
		if err := l.router.route(l); err != nil {
			return err
		}
		changed, err := l.placeAttachedNodes(skipSelected)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
	}
	return errors.New("attachment routing did not converge")
}

func (l *Layout) placeAttachedNodes(skipSelected bool) (bool, error) {
	changed := false
	for nodeID, attachment := range l.attachments {
		if attachment == (Attachment{}) ||
			skipSelected &&
				l.selection.Contains(Hit{ID: uint32(nodeID), Kind: HitNode}) {
			continue
		}
		if err := l.validateAttachment(attachment); err != nil {
			return false, fmt.Errorf("validate attachment for node %d: %w", nodeID, err)
		}
		point, err := attachmentPoint(
			l.Edges[attachment.EdgeID].Points,
			attachment.Position,
		)
		if err != nil {
			return false, fmt.Errorf("position attached node %d: %w", nodeID, err)
		}
		points := l.Edges[attachment.EdgeID].Points
		if point == points[0] || point == points[len(points)-1] {
			return false, fmt.Errorf(
				"attachment for node %d overlaps an edge endpoint",
				nodeID,
			)
		}
		if point.X < attachment.Anchor.X || point.Y < attachment.Anchor.Y {
			return false, fmt.Errorf("attachment for node %d exceeds coordinate space", nodeID)
		}
		origin := NewPoint(
			point.X-attachment.Anchor.X,
			point.Y-attachment.Anchor.Y,
		)
		if origin == l.origins[nodeID] {
			continue
		}
		node, err := l.prepareNode(uint32(nodeID), l.graph.Nodes[nodeID].Label, origin)
		if err != nil {
			return false, err
		}
		l.origins[nodeID] = origin
		l.Nodes[nodeID] = node
		l.commitNodePorts(uint32(nodeID))
		changed = true
	}
	return changed, nil
}

func edgePosition(points []Point, target Point) (uint64, uint64, bool) {
	if len(points) < 2 {
		return 0, 0, false
	}
	var offset uint64
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		length := manhattan(a, b)
		if pointOnSegment(target, a, b) {
			return offset + manhattan(a, target), pathLength(points), true
		}
		offset += length
	}
	return 0, 0, false
}

func pathLength(points []Point) uint64 {
	var length uint64
	for i := 1; i < len(points); i++ {
		length += manhattan(points[i-1], points[i])
	}
	return length
}

func attachmentPoint(points []Point, position uint16) (Point, error) {
	total := pathLength(points)
	if total == 0 {
		return Point{}, errors.New("empty attachment route")
	}
	distance := uint64(position)*total +
		uint64(attachmentPositionMax)/2
	distance /= uint64(attachmentPositionMax)
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		segment := manhattan(a, b)
		if distance > segment {
			distance -= segment
			continue
		}
		switch {
		case a.X == b.X && b.Y >= a.Y:
			return NewPoint(a.X, a.Y+uint32(distance)), nil
		case a.X == b.X:
			return NewPoint(a.X, a.Y-uint32(distance)), nil
		case b.X >= a.X:
			return NewPoint(a.X+uint32(distance), a.Y), nil
		default:
			return NewPoint(a.X-uint32(distance), a.Y), nil
		}
	}
	return points[len(points)-1], nil
}

func attachmentPosition(offset, length uint64) uint16 {
	if length == 0 {
		return 0
	}
	position := (offset*uint64(attachmentPositionMax) + length/2) / length
	return uint16(min(position, uint64(attachmentPositionMax)))
}

type attachmentBuildSnapshot struct {
	origins     []Point
	nodes       []Node
	ports       []Port
	portUsable  []bool
	edges       []Edge
	attachments []Attachment
}

func (l *Layout) snapshotAttachmentBuildState(
	state *attachmentBuildSnapshot,
) {
	state.origins = append(state.origins[:0], l.origins...)
	state.nodes = append(state.nodes[:0], l.Nodes...)
	state.ports = append(state.ports[:0], l.Ports...)
	state.portUsable = append(state.portUsable[:0], l.portUsable...)
	state.edges = growTo(state.edges, len(l.Edges))
	state.edges = state.edges[:len(l.Edges)]
	for i, edge := range l.Edges {
		state.edges[i].Points = append(state.edges[i].Points[:0], edge.Points...)
	}
	state.attachments = append(state.attachments[:0], l.attachments...)
}

func (l *Layout) restoreAttachmentBuildState(state *attachmentBuildSnapshot) {
	l.origins, state.origins = state.origins, l.origins
	l.Nodes, state.nodes = state.nodes, l.Nodes
	l.Ports, state.ports = state.ports, l.Ports
	l.portUsable, state.portUsable = state.portUsable, l.portUsable
	l.Edges, state.edges = state.edges, l.Edges
	l.attachments, state.attachments = state.attachments, l.attachments
	l.rebuildPortLookup()
}
