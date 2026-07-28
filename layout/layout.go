// Package layout resolves diagram IR into terminal-cell geometry.
package layout

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"

	"github.com/coxley/dg/ir"
)

var ErrPortUnavailable = errors.New("port unavailable at current node size")

type Point struct {
	X, Y uint32
}

func NewPoint(x, y uint32) Point {
	return Point{X: x, Y: y}
}

// Add returns a point offset by x and y cells.
func (p Point) Add(x, y uint32) Point {
	return Point{X: p.X + x, Y: p.Y + y}
}

type Size struct {
	Width, Height uint32
}

// Empty reports whether the size has no usable area.
func (s Size) Empty() bool {
	return s.Width == 0 || s.Height == 0
}

// Padding describes empty cells between a node's border and its label.
type Padding struct {
	Top, Right, Bottom, Left uint8
}

// Rect occupies cells from [Min, Max)
type Rect struct {
	Min  Point
	Size Size
}

func (r Rect) Max() Point {
	return r.Min.Add(r.Size.Width, r.Size.Height)
}

// Empty reports whether the rectangle contains no cells.
func (r Rect) Empty() bool {
	return r.Size.Width == 0 || r.Size.Height == 0
}

// NewRect returns the half-open rectangle [origin, limit).
func NewRect(origin, limit Point) (Rect, error) {
	if limit.X <= origin.X || limit.Y <= origin.Y {
		return Rect{}, fmt.Errorf("invalid rectangle bounds %+v to %+v", origin, limit)
	}
	return Rect{
		Min: origin,
		Size: Size{
			Width:  limit.X - origin.X,
			Height: limit.Y - origin.Y,
		},
	}, nil
}

func (r Rect) Contains(p Point) bool {
	maxp := r.Max()
	return p.X >= r.Min.X && p.X < maxp.X && p.Y >= r.Min.Y && p.Y < maxp.Y
}

// OnBoundary reports whether p lies on the rectangle's inner boundary.
func (r Rect) OnBoundary(p Point) bool {
	if !r.Contains(p) {
		return false
	}
	maxp := r.Max()
	return p.X == r.Min.X || p.X == maxp.X-1 ||
		p.Y == r.Min.Y || p.Y == maxp.Y-1
}

type Node struct {
	Rect       Rect
	LabelPoint Point
}

// Empty reports whether the node contains no geometry.
func (n Node) Empty() bool {
	return n.Rect.Empty()
}

// Port contains a boundary cell and its outward neighbor.
type Port struct {
	Anchor Point // Cell on a node's border
	Exit   Point // First cell outside of the node
}

type Edge struct {
	Points []Point
}

// Empty reports whether the edge contains no rasterizable segment.
func (e Edge) Empty() bool {
	return len(e.Points) < 2
}

// Contains reports whether point lies on a route segment.
func (e Edge) Contains(point Point) bool {
	for i := 1; i < len(e.Points); i++ {
		if pointOnSegment(point, e.Points[i-1], e.Points[i]) {
			return true
		}
	}
	return false
}

// HitKind identifies the geometry occupying a cell.
type HitKind uint8

const (
	HitNode HitKind = iota + 1
	HitPort
	HitEdge
)

// Hit identifies geometry by its index in the corresponding Layout slice.
type Hit struct {
	ID   uint32
	Kind HitKind
}

// Layout owns editable diagram IR and its index-aligned cell geometry. Its
// methods keep the exported geometry current; callers must not mutate it.
type Layout struct {
	Nodes []Node
	Ports []Port
	Edges []Edge

	graph         ir.Graph
	origins       []Point
	explicitSizes []Size
	// TODO: Should styles be a lookup table instead for more compactness? Or a list of
	// int64 with each entry representing styles for 8 nodes / edges at once?
	nodeStyles  []NodeStyle
	edgeStyles  []EdgeStyle
	padding     Padding
	router      Router
	scratch     routeScratch
	draftPorts  []Port
	portUsable  []bool
	draftUsable []bool
	drawOrder   []Hit
	history     *History
	selection   Selection
}

// Option configures a Layout.
type Option func(*Layout)

// New returns a layout configured with one cell of horizontal padding and the
// default router. New validates and resolves any graph supplied by WithGraph.
func New(options ...Option) (*Layout, error) {
	l := &Layout{
		padding: Padding{Left: 1, Right: 1},
		router:  DefaultRouter(),
	}
	for _, option := range options {
		option(l)
	}
	l.graph = cloneGraph(l.graph)
	l.selection.attach(l)
	if err := l.initializeGeometry(); err != nil {
		return nil, err
	}
	if err := l.history.attach(l); err != nil {
		return nil, err
	}
	return l, nil
}

// WithPadding sets symmetric horizontal and vertical node padding.
func WithPadding(horizontal, vertical uint8) Option {
	return func(l *Layout) {
		l.padding = Padding{
			Top:    vertical,
			Right:  horizontal,
			Bottom: vertical,
			Left:   horizontal,
		}
	}
}

// WithRouter sets the router used by Build.
func WithRouter(router Router) Option {
	return func(l *Layout) {
		l.router = router
	}
}

// WithGraph initializes a Layout from a copy of graph. Nodes initially use the
// origin and can be positioned with PlaceNode.
func WithGraph(graph ir.Graph) Option {
	return func(l *Layout) {
		l.graph = graph
	}
}

// WithHistory records successful mutations in history.
func WithHistory(history *History) Option {
	return func(l *Layout) {
		l.history = history
	}
}

// History returns the mutation history attached to the layout.
func (l *Layout) History() *History {
	return l.history
}

// NewNode adds a node at the origin and returns its index. It returns an error
// when the label or resulting geometry is invalid.
func (l *Layout) NewNode(label string) (uint32, error) {
	return l.NewNodeAt(label, Point{})
}

// NewNodeAt adds a node at point and returns its index. It returns an error
// when the label or resulting geometry is invalid.
func (l *Layout) NewNodeAt(label string, point Point) (uint32, error) {
	return l.newNodeAt(label, point, nil)
}

func (l *Layout) newNodeAt(
	label string,
	point Point,
	ports []ir.Port,
) (uint32, error) {
	nodeID := l.graph.NextNodeID()
	node, err := l.nodeGeometry(nodeID, label, point)
	if err != nil {
		return 0, err
	}

	if ports == nil {
		nodeID = l.graph.NewNode(label)
	} else {
		nodeID = l.graph.NewNodeWithPorts(label, ports)
	}
	if err := l.resolveNodePorts(nodeID, node.Rect); err != nil {
		if deleteErr := l.graph.DeleteNode(nodeID); deleteErr != nil {
			return 0, fmt.Errorf("rollback node %d: %w", nodeID, deleteErr)
		}
		return 0, err
	}

	l.origins = growTo(l.origins, len(l.graph.Nodes))
	l.explicitSizes = growTo(l.explicitSizes, len(l.graph.Nodes))
	l.nodeStyles = growTo(l.nodeStyles, len(l.graph.Nodes))
	l.Nodes = growTo(l.Nodes, len(l.graph.Nodes))
	l.Ports = growTo(l.Ports, len(l.graph.Ports))
	l.portUsable = growTo(l.portUsable, len(l.graph.Ports))
	l.origins[nodeID] = point
	l.explicitSizes[nodeID] = Size{}
	l.nodeStyles[nodeID] = NodeStyle{}
	l.Nodes[nodeID] = node
	l.selection.discard(Hit{ID: nodeID, Kind: HitNode})
	l.commitNodePorts(nodeID)
	l.appendLayer(Hit{ID: nodeID, Kind: HitNode})
	if l.history != nil {
		l.history.record(historyChange{
			kind: historyCreateNode,
			id:   nodeID,
			node: l.historyNode(nodeID),
		})
	}
	return nodeID, nil
}

// DuplicateSelection copies selected nodes and edges wholly contained by those
// nodes. It replaces the selection with the newly created objects.
func (l *Layout) DuplicateSelection(dx, dy int64) error {
	selectedNodes := make([]uint32, 0)
	for nodeID := range l.selection.Nodes() {
		selectedNodes = append(selectedNodes, nodeID)
	}
	if len(selectedNodes) == 0 {
		return errors.New("selection contains no nodes")
	}

	portMap := make([]uint32, len(l.graph.Ports))
	selected := make([]bool, len(l.graph.Nodes))
	l.selection.Clear()
	for _, sourceID := range selectedNodes {
		origin, ok := offsetPoint(l.origins[sourceID], dx, dy)
		if !ok {
			return errors.New("duplicate placement outside coordinate space")
		}
		source := l.graph.Nodes[sourceID]
		ports := make([]ir.Port, len(source.Ports))
		for i, portID := range source.Ports {
			ports[i] = l.graph.Ports[portID]
		}
		nodeID, err := l.newNodeAt(source.Label, origin, ports)
		if err != nil {
			return err
		}
		selected[sourceID] = true
		for i, sourcePort := range source.Ports {
			portMap[sourcePort] = l.graph.Nodes[nodeID].Ports[i]
		}
		if size, explicit := l.ExplicitNodeSize(sourceID); explicit {
			if err := l.SetNodeSize(nodeID, size); err != nil {
				return err
			}
		}
		if err := l.SetNodeStyle(nodeID, l.nodeStyles[sourceID]); err != nil {
			return err
		}
		l.selection.ensureCapacity()
		l.selection.Toggle(Hit{ID: nodeID, Kind: HitNode})
	}
	for edgeID, edge := range l.graph.Edges {
		if uint64(edge.PortA) >= uint64(len(portMap)) ||
			uint64(edge.PortB) >= uint64(len(portMap)) {
			continue
		}
		nodeA := l.graph.Ports[edge.PortA].Node
		nodeB := l.graph.Ports[edge.PortB].Node
		if uint64(nodeA) >= uint64(len(selected)) ||
			uint64(nodeB) >= uint64(len(selected)) ||
			!selected[nodeA] || !selected[nodeB] {
			continue
		}
		duplicateID, err := l.ConnectPorts(portMap[edge.PortA], portMap[edge.PortB])
		if err != nil {
			return err
		}
		if err := l.SetEdgeStyle(duplicateID, l.edgeStyles[edgeID]); err != nil {
			return err
		}
		points := slices.Grow(
			l.Edges[duplicateID].Points[:0],
			len(l.Edges[edgeID].Points),
		)
		for _, point := range l.Edges[edgeID].Points {
			translated, ok := offsetPoint(point, dx, dy)
			if !ok {
				return errors.New("duplicate route outside coordinate space")
			}
			points = append(points, translated)
		}
		l.Edges[duplicateID].Points = points
		l.selection.ensureCapacity()
		l.selection.Toggle(Hit{ID: duplicateID, Kind: HitEdge})
	}
	return nil
}

func offsetPoint(point Point, dx, dy int64) (Point, bool) {
	x, y := int64(point.X)+dx, int64(point.Y)+dy
	if x < 0 || y < 0 || x > math.MaxUint32 || y > math.MaxUint32 {
		return Point{}, false
	}
	return NewPoint(uint32(x), uint32(y)), true
}

// SetNodeLabel changes a node's label if the resulting geometry is valid.
func (l *Layout) SetNodeLabel(nodeID uint32, label string) error {
	if !l.graph.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ir.ErrNodeNotFound, nodeID)
	}
	previous := l.graph.Nodes[nodeID].Label
	if previous == label {
		return nil
	}
	node, err := l.prepareNode(nodeID, label, l.origins[nodeID])
	if err != nil {
		return err
	}

	l.graph.Nodes[nodeID].Label = label
	l.Nodes[nodeID] = node
	l.commitNodePorts(nodeID)
	if l.history != nil {
		l.history.record(historyChange{
			kind:        historySetLabel,
			id:          nodeID,
			beforeLabel: previous,
			afterLabel:  label,
		})
	}
	return nil
}

// SetNodeSize gives a node fixed outer dimensions. Labels wrap to its inner
// width and clip at its inner height.
func (l *Layout) SetNodeSize(nodeID uint32, size Size) error {
	if size.Empty() {
		return fmt.Errorf("invalid explicit node size %+v", size)
	}
	return l.setNodeSize(nodeID, size)
}

// AutoSizeNode restores content-derived dimensions and disables wrapping.
func (l *Layout) AutoSizeNode(nodeID uint32) error {
	return l.setNodeSize(nodeID, Size{})
}

// ExplicitNodeSize returns a node's fixed outer dimensions.
func (l *Layout) ExplicitNodeSize(nodeID uint32) (Size, bool) {
	if !l.graph.NodeExists(nodeID) ||
		uint64(nodeID) >= uint64(len(l.explicitSizes)) {
		return Size{}, false
	}
	size := l.explicitSizes[nodeID]
	return size, !size.Empty()
}

func (l *Layout) setNodeSize(nodeID uint32, size Size) error {
	if !l.graph.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ir.ErrNodeNotFound, nodeID)
	}
	previous := l.explicitSizes[nodeID]
	if previous == size {
		return nil
	}
	l.explicitSizes[nodeID] = size
	node, err := l.prepareNode(
		nodeID,
		l.graph.Nodes[nodeID].Label,
		l.origins[nodeID],
	)
	if err != nil {
		l.explicitSizes[nodeID] = previous
		return err
	}
	l.Nodes[nodeID] = node
	l.commitNodePorts(nodeID)
	if l.history != nil {
		l.history.record(historyChange{
			kind:       historySetNodeSize,
			id:         nodeID,
			beforeSize: previous,
			afterSize:  size,
		})
	}
	return nil
}

// PlaceNode changes a node's origin if the resulting geometry is valid.
func (l *Layout) PlaceNode(nodeID uint32, point Point) error {
	if !l.graph.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ir.ErrNodeNotFound, nodeID)
	}
	previous := l.origins[nodeID]
	if previous == point {
		return nil
	}
	node, err := l.prepareNode(nodeID, l.graph.Nodes[nodeID].Label, point)
	if err != nil {
		return err
	}

	l.origins[nodeID] = point
	l.Nodes[nodeID] = node
	l.commitNodePorts(nodeID)
	if l.history != nil {
		l.history.record(historyChange{
			kind:        historyPlaceNode,
			id:          nodeID,
			beforePoint: previous,
			afterPoint:  point,
		})
	}
	return nil
}

// MoveSelection translates selected nodes and routes whose endpoints are both
// selected. BuildSelection can then validate translated routes and reroute only
// edges whose geometry no longer works.
func (l *Layout) MoveSelection(dx, dy int64) error {
	for nodeID := range l.selection.Nodes() {
		if _, ok := offsetPoint(l.origins[nodeID], dx, dy); !ok {
			return errors.New("selection placement outside coordinate space")
		}
	}
	for edgeID, edge := range l.graph.Edges {
		id := uint32(edgeID)
		if !l.graph.EdgeExists(id) || !l.edgeEndpointsSelected(edge) {
			continue
		}
		for _, point := range l.Edges[id].Points {
			if _, ok := offsetPoint(point, dx, dy); !ok {
				return errors.New("selection route outside coordinate space")
			}
		}
	}
	for nodeID := range l.selection.Nodes() {
		point, _ := offsetPoint(l.origins[nodeID], dx, dy)
		if err := l.PlaceNode(nodeID, point); err != nil {
			return err
		}
	}
	for edgeID, edge := range l.graph.Edges {
		id := uint32(edgeID)
		if !l.graph.EdgeExists(id) || !l.edgeEndpointsSelected(edge) {
			continue
		}
		for i, point := range l.Edges[id].Points {
			l.Edges[id].Points[i], _ = offsetPoint(point, dx, dy)
		}
	}
	return nil
}

// SelectionMovesRigidly reports whether every edge incident to a selected node
// has both endpoint nodes selected.
func (l *Layout) SelectionMovesRigidly() bool {
	if !l.selection.HasNodes() {
		return false
	}
	for edgeID, edge := range l.graph.Edges {
		id := uint32(edgeID)
		if !l.graph.EdgeExists(id) {
			continue
		}
		nodeA := l.graph.Ports[edge.PortA].Node
		nodeB := l.graph.Ports[edge.PortB].Node
		selectedA := l.selection.Contains(Hit{ID: nodeA, Kind: HitNode})
		selectedB := l.selection.Contains(Hit{ID: nodeB, Kind: HitNode})
		if selectedA != selectedB {
			return false
		}
	}
	return true
}

func (l *Layout) edgeEndpointsSelected(edge ir.Edge) bool {
	nodeA := l.graph.Ports[edge.PortA].Node
	nodeB := l.graph.Ports[edge.PortB].Node
	return l.selection.Contains(Hit{ID: nodeA, Kind: HitNode}) &&
		l.selection.Contains(Hit{ID: nodeB, Kind: HitNode})
}

// ConnectNodes connects side-constrained center ports and returns the edge index.
func (l *Layout) ConnectNodes(nodeA uint32, sideA, sideB ir.Side, nodeB uint32) uint32 {
	portA, _ := l.graph.PickCenterPort(nodeA, sideA)
	portB, _ := l.graph.PickCenterPort(nodeB, sideB)
	_, existed := l.connectedEdge(portA, portB)
	edgeID := l.graph.ConnectNodes(nodeA, sideA, sideB, nodeB)
	l.Edges = growTo(l.Edges, len(l.graph.Edges))
	l.edgeStyles = growTo(l.edgeStyles, len(l.graph.Edges))
	if !existed {
		l.edgeStyles[edgeID] = EdgeStyle{}
		l.selection.discard(Hit{ID: edgeID, Kind: HitEdge})
		l.appendLayer(Hit{ID: edgeID, Kind: HitEdge})
	}
	if l.history != nil && !existed {
		l.history.record(historyChange{
			kind:           historyCreateEdge,
			id:             edgeID,
			afterEdge:      l.graph.Edges[edgeID],
			afterEdgeStyle: l.edgeStyles[edgeID],
			afterLayer:     uint32(len(l.drawOrder) - 1),
		})
	}
	return edgeID
}

// ConnectPorts connects two ports and returns the edge index. It returns the
// existing edge when the ports are already connected.
func (l *Layout) ConnectPorts(portA, portB uint32) (uint32, error) {
	if !l.graph.PortExists(portA) {
		return 0, fmt.Errorf("%w: %d", ir.ErrPortNotFound, portA)
	}
	if !l.graph.PortExists(portB) {
		return 0, fmt.Errorf("%w: %d", ir.ErrPortNotFound, portB)
	}
	if !l.PortUsable(portA) {
		return 0, fmt.Errorf("%w: %d", ErrPortUnavailable, portA)
	}
	if !l.PortUsable(portB) {
		return 0, fmt.Errorf("%w: %d", ErrPortUnavailable, portB)
	}
	if portA == portB {
		return 0, ir.ErrSamePort
	}
	_, existed := l.connectedEdge(portA, portB)
	edgeID := l.graph.ConnectPorts(portA, portB)
	l.Edges = growTo(l.Edges, len(l.graph.Edges))
	l.edgeStyles = growTo(l.edgeStyles, len(l.graph.Edges))
	if !existed {
		l.edgeStyles[edgeID] = EdgeStyle{}
		l.selection.discard(Hit{ID: edgeID, Kind: HitEdge})
		l.appendLayer(Hit{ID: edgeID, Kind: HitEdge})
	}
	if l.history != nil && !existed {
		l.history.record(historyChange{
			kind:           historyCreateEdge,
			id:             edgeID,
			afterEdge:      l.graph.Edges[edgeID],
			afterEdgeStyle: l.edgeStyles[edgeID],
			afterLayer:     uint32(len(l.drawOrder) - 1),
		})
	}
	return edgeID, nil
}

// ReconnectEdge replaces one endpoint while preserving the edge index.
func (l *Layout) ReconnectEdge(edgeID, oldPort, newPort uint32) error {
	if !l.graph.EdgeExists(edgeID) {
		return fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	if !l.graph.PortExists(newPort) {
		return fmt.Errorf("%w: %d", ir.ErrPortNotFound, newPort)
	}
	if !l.graph.Edges[edgeID].HasPort(oldPort) {
		return fmt.Errorf("%w: %d", ir.ErrPortNotOnEdge, oldPort)
	}
	if oldPort == newPort {
		return nil
	}
	if !l.PortUsable(newPort) {
		return fmt.Errorf("%w: %d", ErrPortUnavailable, newPort)
	}
	previous := l.graph.Edges[edgeID]
	if err := l.graph.ReconnectEdge(edgeID, oldPort, newPort); err != nil {
		return err
	}
	l.Edges[edgeID] = Edge{}
	l.selection.discard(Hit{ID: edgeID, Kind: HitEdge})
	if l.history != nil && previous != l.graph.Edges[edgeID] {
		style := l.edgeStyles[edgeID]
		l.history.record(historyChange{
			kind:            historyReconnectEdge,
			id:              edgeID,
			beforeEdge:      previous,
			afterEdge:       l.graph.Edges[edgeID],
			beforeEdgeStyle: style,
			afterEdgeStyle:  style,
		})
	}
	return nil
}

// DeleteEdge removes an edge and makes its ID available for reuse.
func (l *Layout) DeleteEdge(edgeID uint32) error {
	if !l.graph.EdgeExists(edgeID) {
		return fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	previous := l.graph.Edges[edgeID]
	previousStyle := l.edgeStyles[edgeID]
	layer, _ := l.removeLayer(Hit{ID: edgeID, Kind: HitEdge})
	if err := l.graph.DeleteEdge(edgeID); err != nil {
		return err
	}
	l.Edges[edgeID] = Edge{}
	l.edgeStyles[edgeID] = EdgeStyle{}
	if l.history != nil {
		l.history.record(historyChange{
			kind:            historyDeleteEdge,
			id:              edgeID,
			beforeEdge:      previous,
			beforeEdgeStyle: previousStyle,
			beforeLayer:     uint32(layer),
		})
	}
	return nil
}

// DeleteNode removes a node, its ports, and its incident edges.
func (l *Layout) DeleteNode(nodeID uint32) error {
	if !l.graph.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ir.ErrNodeNotFound, nodeID)
	}
	var previous historyNode
	if l.history != nil {
		previous = l.historyNode(nodeID)
	}

	for edgeID := range l.graph.Edges {
		if !l.graph.EdgeExists(uint32(edgeID)) {
			continue
		}
		if l.graph.EdgeIncidentTo(uint32(edgeID), nodeID) {
			l.Edges[edgeID] = Edge{}
			l.edgeStyles[edgeID] = EdgeStyle{}
			l.removeLayer(Hit{ID: uint32(edgeID), Kind: HitEdge})
			l.selection.discard(Hit{ID: uint32(edgeID), Kind: HitEdge})
		}
	}
	for _, portID := range l.graph.Nodes[nodeID].Ports {
		l.Ports[portID] = Port{}
		l.portUsable[portID] = false
	}
	if err := l.graph.DeleteNode(nodeID); err != nil {
		return err
	}
	l.origins[nodeID] = Point{}
	l.explicitSizes[nodeID] = Size{}
	l.nodeStyles[nodeID] = NodeStyle{}
	l.Nodes[nodeID] = Node{}
	l.removeLayer(Hit{ID: nodeID, Kind: HitNode})
	l.selection.discard(Hit{ID: nodeID, Kind: HitNode})
	if l.history != nil {
		l.history.record(historyChange{
			kind: historyDeleteNode,
			id:   nodeID,
			node: previous,
		})
	}
	return nil
}

func (l *Layout) connectedEdge(portA, portB uint32) (uint32, bool) {
	for edgeID := range l.graph.Edges {
		if l.graph.EdgeExists(uint32(edgeID)) &&
			l.graph.Edges[edgeID].Connects(portA, portB) {
			return uint32(edgeID), true
		}
	}
	return 0, false
}

// NodeExists reports whether nodeID identifies a live node.
func (l *Layout) NodeExists(nodeID uint32) bool {
	return l.graph.NodeExists(nodeID)
}

// EdgeExists reports whether edgeID identifies a live edge.
func (l *Layout) EdgeExists(edgeID uint32) bool {
	return l.graph.EdgeExists(edgeID)
}

// PortExists reports whether portID identifies a live port.
func (l *Layout) PortExists(portID uint32) bool {
	return l.graph.PortExists(portID)
}

// PortUsable reports whether portID can start or receive a new connection.
// The first port on each side is always usable. Each later port needs one
// boundary cell between itself, both corners, and every earlier usable port.
func (l *Layout) PortUsable(portID uint32) bool {
	return l.graph.PortExists(portID) &&
		uint64(portID) < uint64(len(l.portUsable)) &&
		l.portUsable[portID]
}

// EdgePorts returns an edge's endpoint port IDs.
func (l *Layout) EdgePorts(edgeID uint32) (uint32, uint32, error) {
	if !l.graph.EdgeExists(edgeID) {
		return 0, 0, fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	edge := l.graph.Edges[edgeID]
	return edge.PortA, edge.PortB, nil
}

// EdgeNodes returns the node IDs joined by an edge.
func (l *Layout) EdgeNodes(edgeID uint32) (uint32, uint32, error) {
	portA, portB, err := l.EdgePorts(edgeID)
	if err != nil {
		return 0, 0, err
	}
	return l.graph.Ports[portA].Node, l.graph.Ports[portB].Node, nil
}

// NodePorts yields the IDs of ports owned by nodeID.
func (l *Layout) NodePorts(nodeID uint32) iter.Seq[uint32] {
	return func(yield func(uint32) bool) {
		if !l.graph.NodeExists(nodeID) {
			return
		}
		for _, portID := range l.graph.Nodes[nodeID].Ports {
			if !yield(portID) {
				return
			}
		}
	}
}

// Label returns a node's source label.
func (l *Layout) Label(nodeID uint32) string {
	return l.graph.Nodes[nodeID].Label
}

// LabelBounds returns the cells available for a node's rendered label.
func (l *Layout) LabelBounds(nodeID uint32) Rect {
	node := l.Nodes[nodeID]
	return Rect{
		Min: node.LabelPoint,
		Size: Size{
			Width: node.Rect.Size.Width -
				uint32(l.padding.Left) -
				uint32(l.padding.Right) -
				2,
			Height: node.Rect.Size.Height -
				uint32(l.padding.Top) -
				uint32(l.padding.Bottom) -
				2,
		},
	}
}

// Graph returns an independent copy of the semantic graph.
func (l *Layout) Graph() ir.Graph {
	return l.graph.Clone()
}

// Padding returns the configured node padding.
func (l *Layout) Padding() Padding {
	return l.padding
}

// Router returns the configured router.
func (l *Layout) Router() Router {
	return l.router
}

// SetRouter changes the orthogonal routing configuration.
func (l *Layout) SetRouter(router Router) {
	previous := l.router
	if previous == router {
		return
	}
	l.router = router
	if l.history != nil {
		l.history.record(historyChange{
			kind:         historySetRouter,
			beforeRouter: previous,
			afterRouter:  router,
		})
	}
}

// Clone returns an independent layout with matching semantic, geometry, style,
// layer, and selection state. The clone has no attached history.
func (l *Layout) Clone() (*Layout, error) {
	cloned, err := New(func(cloned *Layout) {
		cloned.graph = l.graph
		cloned.padding = l.padding
		cloned.router = l.router
		cloned.drawOrder = slices.Clone(l.drawOrder)
	})
	if err != nil {
		return nil, err
	}
	for nodeID := range l.graph.Nodes {
		id := uint32(nodeID)
		if !l.graph.NodeExists(id) {
			continue
		}
		if err := cloned.PlaceNode(id, l.origins[id]); err != nil {
			return nil, err
		}
		if size, explicit := l.ExplicitNodeSize(id); explicit {
			if err := cloned.SetNodeSize(id, size); err != nil {
				return nil, err
			}
		}
		if err := cloned.SetNodeStyle(id, l.nodeStyles[id]); err != nil {
			return nil, err
		}
	}
	for edgeID := range l.graph.Edges {
		id := uint32(edgeID)
		if l.graph.EdgeExists(id) {
			if err := cloned.SetEdgeStyle(id, l.edgeStyles[id]); err != nil {
				return nil, err
			}
		}
	}
	for nodeID := range l.selection.Nodes() {
		cloned.selection.Toggle(Hit{ID: nodeID, Kind: HitNode})
	}
	for edgeID := range l.selection.Edges() {
		cloned.selection.Toggle(Hit{ID: edgeID, Kind: HitEdge})
	}
	if err := cloned.Build(); err != nil {
		return nil, err
	}
	return cloned, nil
}

// Build routes edges using the current node and port geometry.
func (l *Layout) Build() error {
	if err := l.router.route(l); err != nil {
		return fmt.Errorf("route edges: %w", err)
	}
	return nil
}

// BuildSelection reroutes selected edges and edges incident to selected nodes
// against existing unrelated routes. It changes no route when routing fails.
func (l *Layout) BuildSelection() error {
	if err := l.router.routeSelection(l); err != nil {
		return fmt.Errorf("route selected edges: %w", err)
	}
	return nil
}

// Obstacles yields the current node rectangles.
func (l *Layout) Obstacles() iter.Seq[Rect] {
	return func(yield func(Rect) bool) {
		for i := range l.Nodes {
			if l.Nodes[i].Empty() {
				continue
			}
			if !yield(l.Nodes[i].Rect) {
				return
			}
		}
	}
}

// Hits yields visible geometry occupying point.
func (l *Layout) Hits(point Point) iter.Seq[Hit] {
	return func(yield func(Hit) bool) {
		owner, ok := l.visibleObjectAt(point)
		if !ok || !yield(owner) {
			return
		}
		if owner.Kind == HitEdge {
			return
		}
		if l.graph.NodeExists(owner.ID) {
			for _, portID := range l.graph.Nodes[owner.ID].Ports {
				if l.portUsable[portID] &&
					l.Ports[portID].Anchor == point &&
					!yield(Hit{ID: portID, Kind: HitPort}) {
					return
				}
			}
			for i := len(l.drawOrder) - 1; i >= 0; i-- {
				hit := l.drawOrder[i]
				if hit.Kind == HitEdge &&
					l.edgeEndpointNodeAt(hit.ID, owner.ID, point) &&
					!yield(hit) {
					return
				}
			}
			return
		}
		for portID, port := range l.Ports {
			if port.Anchor == point &&
				!yield(Hit{ID: uint32(portID), Kind: HitPort}) {
				return
			}
		}
		for edgeID, edge := range l.Edges {
			if edge.Contains(point) &&
				l.edgeEndpointAt(uint32(edgeID), point) &&
				!yield(Hit{ID: uint32(edgeID), Kind: HitEdge}) {
				return
			}
		}
	}
}

func (l *Layout) visibleObjectAt(point Point) (Hit, bool) {
	if len(l.drawOrder) == 0 {
		for edgeID := len(l.Edges) - 1; edgeID >= 0; edgeID-- {
			if l.Edges[edgeID].Contains(point) &&
				!l.edgeEndpointAt(uint32(edgeID), point) {
				return Hit{ID: uint32(edgeID), Kind: HitEdge}, true
			}
		}
		for nodeID := len(l.Nodes) - 1; nodeID >= 0; nodeID-- {
			if l.Nodes[nodeID].Rect.Contains(point) {
				return Hit{ID: uint32(nodeID), Kind: HitNode}, true
			}
		}
		return Hit{}, false
	}
	for i := len(l.drawOrder) - 1; i >= 0; i-- {
		hit := l.drawOrder[i]
		switch hit.Kind {
		case HitNode:
			if l.Nodes[hit.ID].Rect.Contains(point) {
				return hit, true
			}
		case HitEdge:
			if l.Edges[hit.ID].Contains(point) &&
				!l.edgeEndpointAt(hit.ID, point) {
				return hit, true
			}
		case HitPort:
			continue
		}
	}
	return Hit{}, false
}

func (l *Layout) edgeEndpointAt(edgeID uint32, point Point) bool {
	if !l.graph.EdgeExists(edgeID) {
		edge := l.Edges[edgeID]
		if len(edge.Points) < 2 ||
			point != edge.Points[0] && point != edge.Points[len(edge.Points)-1] {
			return false
		}
		for _, node := range l.Nodes {
			if !node.Empty() && node.Rect.OnBoundary(point) {
				return true
			}
		}
		return false
	}
	edge := l.graph.Edges[edgeID]
	return l.Ports[edge.PortA].Anchor == point ||
		l.Ports[edge.PortB].Anchor == point
}

func (l *Layout) edgeEndpointNodeAt(edgeID, nodeID uint32, point Point) bool {
	edge := l.graph.Edges[edgeID]
	return l.graph.Ports[edge.PortA].Node == nodeID &&
		l.Ports[edge.PortA].Anchor == point ||
		l.graph.Ports[edge.PortB].Node == nodeID &&
			l.Ports[edge.PortB].Anchor == point
}

func pointOnSegment(point, a, b Point) bool {
	switch {
	case a.X == b.X:
		return point.X == a.X &&
			point.Y >= min(a.Y, b.Y) &&
			point.Y <= max(a.Y, b.Y)
	case a.Y == b.Y:
		return point.Y == a.Y &&
			point.X >= min(a.X, b.X) &&
			point.X <= max(a.X, b.X)
	default:
		return false
	}
}

func (l *Layout) initializeGeometry() error {
	if err := l.graph.Validate(); err != nil {
		return fmt.Errorf("load graph: %w", err)
	}

	l.origins = make([]Point, len(l.graph.Nodes))
	l.explicitSizes = make([]Size, len(l.graph.Nodes))
	l.nodeStyles = make([]NodeStyle, len(l.graph.Nodes))
	l.Nodes = make([]Node, len(l.graph.Nodes))
	l.Ports = make([]Port, len(l.graph.Ports))
	l.portUsable = make([]bool, len(l.graph.Ports))
	l.Edges = make([]Edge, len(l.graph.Edges))
	l.edgeStyles = make([]EdgeStyle, len(l.graph.Edges))
	if err := l.initializeDrawOrder(); err != nil {
		return err
	}

	for i := range l.graph.Nodes {
		nodeID := uint32(i)
		if !l.graph.NodeExists(nodeID) {
			continue
		}
		node, err := l.nodeGeometry(nodeID, l.graph.Nodes[i].Label, Point{})
		if err != nil {
			return err
		}
		if err := l.resolveNodePorts(nodeID, node.Rect); err != nil {
			return err
		}
		l.Nodes[i] = node
		l.commitNodePorts(nodeID)
	}
	return nil
}

func (l *Layout) nodeGeometry(nodeID uint32, label string, point Point) (Node, error) {
	size, err := MeasureLabel(label)
	if err != nil {
		return Node{}, fmt.Errorf("measure node %d label: %w", nodeID, err)
	}
	rect, err := l.nodeRect(nodeID, point, size)
	if err != nil {
		return Node{}, fmt.Errorf("size node %d: %w", nodeID, err)
	}
	return Node{
		Rect: rect,
		LabelPoint: Point{
			X: rect.Min.X + 1 + uint32(l.padding.Left),
			Y: rect.Min.Y + 1 + uint32(l.padding.Top),
		},
	}, nil
}

func (l *Layout) nodeRect(nodeID uint32, origin Point, label Size) (Rect, error) {
	if uint64(nodeID) >= uint64(len(l.explicitSizes)) ||
		l.explicitSizes[nodeID].Empty() {
		return NodeRect(origin, label, l.padding)
	}
	size := l.explicitSizes[nodeID]
	minWidth := uint32(l.padding.Left) + uint32(l.padding.Right) + 2
	minHeight := uint32(l.padding.Top) + uint32(l.padding.Bottom) + 2
	if size.Width < minWidth || size.Height < minHeight {
		return Rect{}, fmt.Errorf(
			"explicit node size %dx%d smaller than minimum %dx%d",
			size.Width,
			size.Height,
			minWidth,
			minHeight,
		)
	}
	if origin.X > math.MaxUint32-size.Width ||
		origin.Y > math.MaxUint32-size.Height {
		return Rect{}, fmt.Errorf(
			"node size %dx%d exceeds supported size",
			size.Width,
			size.Height,
		)
	}
	return NewRect(origin, origin.Add(size.Width, size.Height))
}

func (l *Layout) prepareNode(nodeID uint32, label string, point Point) (Node, error) {
	node, err := l.nodeGeometry(nodeID, label, point)
	if err != nil {
		return Node{}, err
	}
	if err := l.resolveNodePorts(nodeID, node.Rect); err != nil {
		return Node{}, err
	}
	return node, nil
}

func (l *Layout) resolveNodePorts(nodeID uint32, rect Rect) error {
	source := &l.graph.Nodes[nodeID]
	l.draftPorts = slices.Grow(l.draftPorts[:0], len(source.Ports))[:len(source.Ports)]
	l.draftUsable = slices.Grow(l.draftUsable[:0], len(source.Ports))[:len(source.Ports)]
	clear(l.draftUsable)
	for i, portID := range source.Ports {
		port := l.graph.Ports[portID]
		resolved, err := ResolvePort(rect, port.Side, port.Offset)
		if err != nil {
			return fmt.Errorf("resolve port %d: %w", portID, err)
		}
		l.draftPorts[i] = resolved
		l.draftUsable[i] = l.canUseDraftPort(source, i, rect)
	}
	return nil
}

func (l *Layout) canUseDraftPort(node *ir.Node, index int, rect Rect) bool {
	candidate := l.graph.Ports[node.Ports[index]]
	primary := true
	for i := range index {
		if l.graph.Ports[node.Ports[i]].Side == candidate.Side {
			primary = false
			break
		}
	}
	if primary {
		return true
	}

	position, length := sidePosition(l.draftPorts[index].Anchor, rect, candidate.Side)
	if length < 3 || position < 2 || position > length-3 {
		return false
	}
	for i := range index {
		previous := l.graph.Ports[node.Ports[i]]
		if previous.Side != candidate.Side || !l.draftUsable[i] {
			continue
		}
		previousPosition, _ := sidePosition(l.draftPorts[i].Anchor, rect, previous.Side)
		if max(position, previousPosition)-min(position, previousPosition) < 2 {
			return false
		}
	}
	return true
}

func sidePosition(point Point, rect Rect, side ir.Side) (position, length uint32) {
	switch side {
	case ir.Top, ir.Bottom:
		return point.X - rect.Min.X, rect.Size.Width
	case ir.RightSide, ir.LeftSide:
		return point.Y - rect.Min.Y, rect.Size.Height
	default:
		return 0, 0
	}
}

func (l *Layout) commitNodePorts(nodeID uint32) {
	for i, portID := range l.graph.Nodes[nodeID].Ports {
		l.Ports[portID] = l.draftPorts[i]
		l.portUsable[portID] = l.draftUsable[i]
	}
}

func cloneGraph(graph ir.Graph) ir.Graph {
	return graph.Clone()
}

func growTo[S ~[]E, E any](values S, length int) S {
	if len(values) >= length {
		return values
	}
	return slices.Grow(values, length-len(values))[:length]
}

// NodeRect returns the cells occupied by a bordered label and its padding.
func NodeRect(origin Point, label Size, padding Padding) (Rect, error) {
	if label.Width == 0 && label.Height == 0 {
		label.Height = 1
	}
	hpad := uint32(padding.Left) + uint32(padding.Right) + 2
	vpad := uint32(padding.Top) + uint32(padding.Bottom) + 2
	if label.Width > math.MaxUint32-hpad || label.Height > math.MaxUint32-vpad {
		return Rect{}, fmt.Errorf("label size %+v exceeds supported size", label)
	}
	width := label.Width + hpad
	height := label.Height + vpad
	if origin.X > math.MaxUint32-width || origin.Y > math.MaxUint32-height {
		return Rect{}, fmt.Errorf("node size %dx%d exceeds supported size", width, height)
	}
	return NewRect(origin, origin.Add(width, height))
}
