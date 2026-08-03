package layout

import (
	"errors"
	"fmt"
	"iter"
	"slices"
)

const (
	attachmentBuildPasses = 8
)

// AttachmentEnd identifies the route endpoint used to address a landmark.
type AttachmentEnd uint8

const (
	AttachmentPortA AttachmentEnd = iota + 1
	AttachmentPortB
)

// AttachmentReference identifies an endpoint or bend in route order. Bend zero
// identifies End itself; positive values count bends away from End.
type AttachmentReference struct {
	End      AttachmentEnd
	Bend     uint32
	Incoming Connections
	Outgoing Connections
}

// Valid reports whether the reference contains a valid endpoint or bend.
func (r AttachmentReference) Valid() bool {
	if r.End != AttachmentPortA && r.End != AttachmentPortB {
		return false
	}
	if r.Bend == 0 {
		return r.Incoming == 0 && r.Outgoing == 0
	}
	return (PinnedBend{Incoming: r.Incoming, Outgoing: r.Outgoing}).Valid()
}

func validAttachmentLocation(attachment Attachment) bool {
	if !attachment.Reference.Valid() {
		return false
	}
	if attachment.Reference.Bend != 0 {
		return true
	}
	return attachment.Reference.End == AttachmentPortA && attachment.Offset > 0 ||
		attachment.Reference.End == AttachmentPortB && attachment.Offset < 0
}

// Attachment keeps a node at a signed cell offset from an edge landmark.
// Anchor is the attached edge cell's offset from the node origin.
type Attachment struct {
	NodeID    uint32
	EdgeID    uint32
	Reference AttachmentReference
	Offset    int64
	Anchor    Point
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
	_, _, ok := attachmentLocation(l.Edges[edgeID].Points, point)
	if !ok {
		return false
	}
	points := l.Edges[edgeID].Points
	return point != points[0] && point != points[len(points)-1]
}

// AttachmentAt returns the attachment represented by an edge cell inside a
// node without changing the layout.
func (l *Layout) AttachmentAt(nodeID, edgeID uint32, point Point) (Attachment, error) {
	if !l.CanAttachNode(nodeID, edgeID) {
		return Attachment{}, fmt.Errorf("node %d cannot attach to edge %d", nodeID, edgeID)
	}
	if !l.Nodes[nodeID].Rect.Contains(point) {
		return Attachment{}, fmt.Errorf("attachment point %+v outside node %d", point, nodeID)
	}
	reference, offset, ok := attachmentLocation(l.Edges[edgeID].Points, point)
	if !ok {
		return Attachment{}, fmt.Errorf("point %+v is not on edge %d", point, edgeID)
	}
	origin := l.Nodes[nodeID].Rect.Min
	points := l.Edges[edgeID].Points
	if point == points[0] || point == points[len(points)-1] {
		return Attachment{}, errors.New("attachment cannot overlap an edge endpoint")
	}
	return Attachment{
		NodeID:    nodeID,
		EdgeID:    edgeID,
		Reference: reference,
		Offset:    offset,
		Anchor:    NewPoint(point.X-origin.X, point.Y-origin.Y),
	}, nil
}

// AttachNode attaches nodeID to the edge cell at point. The node keeps its
// current origin until the host route changes.
func (l *Layout) AttachNode(nodeID, edgeID uint32, point Point) error {
	attachment, err := l.AttachmentAt(nodeID, edgeID, point)
	if err != nil {
		return err
	}
	return l.SetAttachment(attachment)
}

// SetAttachment restores or updates an attachment from its route landmark.
// The mutation succeeds only when the complete layout remains buildable.
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
	changed := false
	for _, attachment := range attachments {
		previous, _ := l.NodeAttachment(attachment.NodeID)
		if previous == attachment {
			continue
		}
		l.attachments[attachment.NodeID] = attachment
		changed = true
	}
	if !changed {
		return nil
	}
	if err := l.build(false); err != nil {
		l.restoreAttachmentBuildState(&l.attachmentMutationRollback)
		return fmt.Errorf("set attachments: %w", err)
	}
	l.recordAttachmentChanges(&l.attachmentMutationRollback)
	return nil
}

// DetachNode removes nodeID's attachment while keeping its current origin.
// The mutation succeeds only when the complete layout remains buildable.
func (l *Layout) DetachNode(nodeID uint32) error {
	_, ok := l.NodeAttachment(nodeID)
	if !ok {
		if !l.NodeExists(nodeID) {
			return fmt.Errorf("node not found: %d", nodeID)
		}
		return nil
	}
	l.snapshotAttachmentBuildState(&l.attachmentMutationRollback)
	l.attachments[nodeID] = Attachment{}
	if err := l.build(false); err != nil {
		l.restoreAttachmentBuildState(&l.attachmentMutationRollback)
		return fmt.Errorf("detach node %d: %w", nodeID, err)
	}
	l.recordAttachmentChanges(&l.attachmentMutationRollback)
	return nil
}

func (l *Layout) recordAttachmentChanges(before *attachmentBuildSnapshot) {
	if !l.recordingChanges() {
		return
	}
	for nodeID := range max(len(before.attachments), len(l.attachments)) {
		var previous, current Attachment
		var previousPoint, currentPoint Point
		if nodeID < len(before.attachments) {
			previous = before.attachments[nodeID]
		}
		if nodeID < len(l.attachments) {
			current = l.attachments[nodeID]
		}
		if previous == current {
			continue
		}
		if nodeID < len(before.origins) {
			previousPoint = before.origins[nodeID]
		}
		if nodeID < len(l.origins) {
			currentPoint = l.origins[nodeID]
		}
		l.recordChange(historyChange{
			Kind: historySetAttachment,
			ID:   uint32(nodeID),
			Before: historyChangeState{
				Attachment: previous,
				Attached:   previous != (Attachment{}),
				Point:      previousPoint,
			},
			After: historyChangeState{
				Attachment: current,
				Attached:   current != (Attachment{}),
				Point:      currentPoint,
			},
		})
	}
}

func (l *Layout) validateAttachment(attachment Attachment) error {
	if !l.canHostNode(attachment.NodeID, attachment.EdgeID) {
		return fmt.Errorf(
			"node %d cannot attach to edge %d",
			attachment.NodeID,
			attachment.EdgeID,
		)
	}
	if !validAttachmentLocation(attachment) {
		return errors.New("invalid attachment location")
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
		previousPoint, err := attachmentCell(
			l.origins[nodeID],
			attachment.Anchor,
		)
		if err != nil {
			return false, fmt.Errorf("position attached node %d: %w", nodeID, err)
		}
		var previousRoute []Point
		if uint64(attachment.EdgeID) < uint64(len(l.attachmentBuildRollback.edges)) {
			previousRoute = l.attachmentBuildRollback.edges[attachment.EdgeID].Points
		}
		attachment, point, err := resolveAttachment(
			previousRoute,
			l.Edges[attachment.EdgeID].Points,
			attachment,
			previousPoint,
		)
		if err != nil {
			return false, fmt.Errorf("position attached node %d: %w", nodeID, err)
		}
		l.attachments[nodeID] = attachment
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

func attachmentLocation(
	points []Point,
	target Point,
) (AttachmentReference, int64, bool) {
	if len(points) < 2 {
		return AttachmentReference{}, 0, false
	}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		if !pointOnSegment(target, a, b) {
			continue
		}
		fromA, fromB := manhattan(a, target), manhattan(target, b)
		if fromA <= fromB {
			return attachmentReferenceAt(points, i-1), int64(fromA), true
		}
		return attachmentReferenceAt(points, i), -int64(fromB), true
	}
	return AttachmentReference{}, 0, false
}

func attachmentReferenceAt(points []Point, index int) AttachmentReference {
	last := len(points) - 1
	if index == 0 {
		return AttachmentReference{End: AttachmentPortA}
	}
	if index == last {
		return AttachmentReference{End: AttachmentPortB}
	}
	bend := routeBendAt(points, index)
	reference := AttachmentReference{
		End:      AttachmentPortA,
		Bend:     uint32(index),
		Incoming: bend.Incoming,
		Outgoing: bend.Outgoing,
	}
	if last-index < index {
		reference.End = AttachmentPortB
		reference.Bend = uint32(last - index)
	}
	return reference
}

func routeBendAt(points []Point, index int) PinnedBend {
	if index <= 0 || index+1 >= len(points) {
		return PinnedBend{}
	}
	incoming, incomingOK := routeSegmentDirection(points[index-1], points[index])
	outgoing, outgoingOK := routeSegmentDirection(points[index], points[index+1])
	if !incomingOK || !outgoingOK {
		return PinnedBend{}
	}
	incomingConnection, _ := directionConnections(incoming)
	outgoingConnection, _ := directionConnections(outgoing)
	return PinnedBend{
		Point:    points[index],
		Incoming: incomingConnection,
		Outgoing: outgoingConnection,
	}
}

func routeSegmentDirection(a, b Point) (direction, bool) {
	switch {
	case a.X == b.X && b.Y < a.Y:
		return north, true
	case a.Y == b.Y && b.X > a.X:
		return east, true
	case a.X == b.X && b.Y > a.Y:
		return south, true
	case a.Y == b.Y && b.X < a.X:
		return west, true
	default:
		return 0, false
	}
}

func attachmentReferenceIndex(
	points []Point,
	reference AttachmentReference,
) (int, bool) {
	if len(points) < 2 || !reference.Valid() {
		return 0, false
	}
	last := len(points) - 1
	if reference.Bend == 0 {
		if reference.End == AttachmentPortA {
			return 0, true
		}
		return last, true
	}
	if uint64(reference.Bend) >= uint64(last) {
		return 0, false
	}
	index := int(reference.Bend)
	if reference.End == AttachmentPortB {
		index = last - index
	}
	bend := routeBendAt(points, index)
	if bend.Incoming != reference.Incoming || bend.Outgoing != reference.Outgoing {
		return 0, false
	}
	return index, true
}

func attachmentPointAt(
	points []Point,
	reference AttachmentReference,
	offset int64,
) (Point, bool) {
	index, ok := attachmentReferenceIndex(points, reference)
	if !ok {
		return Point{}, false
	}
	point, ok := routePointAtOffset(points, index, offset)
	if !ok || point == points[0] || point == points[len(points)-1] {
		return Point{}, false
	}
	return point, true
}

func routePointAtOffset(points []Point, index int, offset int64) (Point, bool) {
	distance := offsetMagnitude(offset)
	if offset >= 0 {
		for next := index + 1; next < len(points); next++ {
			segment := manhattan(points[next-1], points[next])
			if distance <= segment {
				return pointAlongSegment(points[next-1], points[next], distance), true
			}
			distance -= segment
		}
		return Point{}, false
	}
	for previous := index - 1; previous >= 0; previous-- {
		segment := manhattan(points[previous+1], points[previous])
		if distance <= segment {
			return pointAlongSegment(points[previous+1], points[previous], distance), true
		}
		distance -= segment
	}
	return Point{}, false
}

func offsetMagnitude(offset int64) uint64 {
	if offset >= 0 {
		return uint64(offset)
	}
	return uint64(-(offset + 1)) + 1
}

func pointAlongSegment(a, b Point, distance uint64) Point {
	delta := uint32(distance)
	switch {
	case a.X == b.X && b.Y >= a.Y:
		return NewPoint(a.X, a.Y+delta)
	case a.X == b.X:
		return NewPoint(a.X, a.Y-delta)
	case b.X >= a.X:
		return NewPoint(a.X+delta, a.Y)
	default:
		return NewPoint(a.X-delta, a.Y)
	}
}

func pathLength(points []Point) uint64 {
	var length uint64
	for index := 1; index < len(points); index++ {
		length += manhattan(points[index-1], points[index])
	}
	return length
}

func routePointAtDistance(points []Point, distance uint64) (Point, bool) {
	if len(points) < 2 {
		return Point{}, false
	}
	for index := 1; index < len(points); index++ {
		segment := manhattan(points[index-1], points[index])
		if distance <= segment {
			return pointAlongSegment(points[index-1], points[index], distance), true
		}
		distance -= segment
	}
	return Point{}, false
}

func resolveAttachment(
	previousRoute, route []Point,
	attachment Attachment,
	previousPoint Point,
) (Attachment, Point, error) {
	if point, ok := attachmentPointAt(route, attachment.Reference, attachment.Offset); ok {
		return attachment, point, nil
	}
	landmark := previousPoint
	if index, ok := attachmentReferenceIndex(previousRoute, attachment.Reference); ok {
		landmark = previousRoute[index]
	}
	bestDistance := uint64(0)
	var bestReference AttachmentReference
	var bestPoint Point
	found := false
	for index := 1; index+1 < len(route); index++ {
		reference := attachmentReferenceAt(route, index)
		point, ok := attachmentPointAt(route, reference, attachment.Offset)
		if !ok {
			continue
		}
		distance := manhattan(landmark, route[index])
		if found && distance >= bestDistance {
			continue
		}
		bestDistance = distance
		bestReference = reference
		bestPoint = point
		found = true
	}
	if found {
		attachment.Reference = bestReference
		return attachment, bestPoint, nil
	}
	projected, ok := projectAttachmentPoint(route, previousPoint)
	if !ok {
		return Attachment{}, Point{}, errors.New("attachment route has no interior cell")
	}
	reference, offset, ok := attachmentLocation(route, projected)
	if !ok {
		return Attachment{}, Point{}, errors.New("project attachment onto route")
	}
	attachment.Reference = reference
	attachment.Offset = offset
	return attachment, projected, nil
}

func projectAttachmentPoint(points []Point, target Point) (Point, bool) {
	if len(points) < 2 {
		return Point{}, false
	}
	bestDistance := uint64(0)
	bestOffset := uint64(0)
	pathOffset := uint64(0)
	var best Point
	found := false
	for index := 1; index < len(points); index++ {
		a, b := points[index-1], points[index]
		candidate := projectPointToSegment(target, a, b)
		if candidate == points[0] && manhattan(a, b) != 0 {
			candidate = pointAlongSegment(a, b, 1)
		}
		if candidate == points[len(points)-1] && manhattan(a, b) != 0 {
			candidate = pointAlongSegment(b, a, 1)
		}
		if candidate == points[0] || candidate == points[len(points)-1] {
			pathOffset += manhattan(a, b)
			continue
		}
		distance := manhattan(target, candidate)
		offset := pathOffset + manhattan(a, candidate)
		if found && (distance > bestDistance ||
			distance == bestDistance && offset >= bestOffset) {
			pathOffset += manhattan(a, b)
			continue
		}
		bestDistance = distance
		bestOffset = offset
		best = candidate
		found = true
		pathOffset += manhattan(a, b)
	}
	return best, found
}

func projectPointToSegment(point, a, b Point) Point {
	if a.X == b.X {
		return NewPoint(a.X, min(max(point.Y, min(a.Y, b.Y)), max(a.Y, b.Y)))
	}
	return NewPoint(min(max(point.X, min(a.X, b.X)), max(a.X, b.X)), a.Y)
}

func attachmentCell(origin, anchor Point) (Point, error) {
	x, y := uint64(origin.X)+uint64(anchor.X), uint64(origin.Y)+uint64(anchor.Y)
	if x > uint64(^uint32(0)) || y > uint64(^uint32(0)) {
		return Point{}, errors.New("attachment exceeds coordinate space")
	}
	return NewPoint(uint32(x), uint32(y)), nil
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
