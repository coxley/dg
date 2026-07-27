// Package layout resolves diagram IR into terminal-cell geometry.
package layout

import (
	"fmt"
	"iter"
	"math"
	"slices"

	"github.com/coxley/dg/ir"
)

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

	graph      ir.Graph
	origins    []Point
	padding    Padding
	router     Router
	scratch    routeScratch
	draftPorts []Port
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
	if err := l.initializeGeometry(); err != nil {
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

// NewNode adds a node at the origin and returns its index. It returns an error
// when the label or resulting geometry is invalid.
func (l *Layout) NewNode(label string) (uint32, error) {
	return l.NewNodeAt(label, Point{})
}

// NewNodeAt adds a node at point and returns its index. It returns an error
// when the label or resulting geometry is invalid.
func (l *Layout) NewNodeAt(label string, point Point) (uint32, error) {
	nodeID := l.graph.NextNodeID()
	node, err := l.nodeGeometry(nodeID, label, point)
	if err != nil {
		return 0, err
	}

	nodeID = l.graph.NewNode(label)
	if err := l.resolveNodePorts(nodeID, node.Rect); err != nil {
		if deleteErr := l.graph.DeleteNode(nodeID); deleteErr != nil {
			return 0, fmt.Errorf("rollback node %d: %w", nodeID, deleteErr)
		}
		return 0, err
	}

	l.origins = growTo(l.origins, len(l.graph.Nodes))
	l.Nodes = growTo(l.Nodes, len(l.graph.Nodes))
	l.Ports = growTo(l.Ports, len(l.graph.Ports))
	l.origins[nodeID] = point
	l.Nodes[nodeID] = node
	l.commitNodePorts(nodeID)
	return nodeID, nil
}

// SetNodeLabel changes a node's label if the resulting geometry is valid.
func (l *Layout) SetNodeLabel(nodeID uint32, label string) error {
	if !l.graph.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ir.ErrNodeNotFound, nodeID)
	}
	node, err := l.prepareNode(nodeID, label, l.origins[nodeID])
	if err != nil {
		return err
	}

	l.graph.Nodes[nodeID].Label = label
	l.Nodes[nodeID] = node
	l.commitNodePorts(nodeID)
	return nil
}

// PlaceNode changes a node's origin if the resulting geometry is valid.
func (l *Layout) PlaceNode(nodeID uint32, point Point) error {
	if !l.graph.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ir.ErrNodeNotFound, nodeID)
	}
	node, err := l.prepareNode(nodeID, l.graph.Nodes[nodeID].Label, point)
	if err != nil {
		return err
	}

	l.origins[nodeID] = point
	l.Nodes[nodeID] = node
	l.commitNodePorts(nodeID)
	return nil
}

// ConnectNodes connects side-constrained center ports and returns the edge index.
func (l *Layout) ConnectNodes(nodeA uint32, sideA, sideB ir.Side, nodeB uint32) uint32 {
	edgeID := l.graph.ConnectNodes(nodeA, sideA, sideB, nodeB)
	l.Edges = growTo(l.Edges, len(l.graph.Edges))
	return edgeID
}

// DeleteEdge removes an edge and makes its ID available for reuse.
func (l *Layout) DeleteEdge(edgeID uint32) error {
	if err := l.graph.DeleteEdge(edgeID); err != nil {
		return err
	}
	l.Edges[edgeID] = Edge{}
	return nil
}

// DeleteNode removes a node, its ports, and its incident edges.
func (l *Layout) DeleteNode(nodeID uint32) error {
	if !l.graph.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ir.ErrNodeNotFound, nodeID)
	}

	for edgeID := range l.graph.Edges {
		if !l.graph.EdgeExists(uint32(edgeID)) {
			continue
		}
		if l.graph.EdgeIncidentTo(uint32(edgeID), nodeID) {
			l.Edges[edgeID] = Edge{}
		}
	}
	for _, portID := range l.graph.Nodes[nodeID].Ports {
		l.Ports[portID] = Port{}
	}
	if err := l.graph.DeleteNode(nodeID); err != nil {
		return err
	}
	l.origins[nodeID] = Point{}
	l.Nodes[nodeID] = Node{}
	return nil
}

// NodeExists reports whether nodeID identifies a live node.
func (l *Layout) NodeExists(nodeID uint32) bool {
	return l.graph.NodeExists(nodeID)
}

// EdgeExists reports whether edgeID identifies a live edge.
func (l *Layout) EdgeExists(edgeID uint32) bool {
	return l.graph.EdgeExists(edgeID)
}

// Label returns a node's source label.
func (l *Layout) Label(nodeID uint32) string {
	return l.graph.Nodes[nodeID].Label
}

// Build routes edges using the current node and port geometry.
func (l *Layout) Build() error {
	if err := l.router.route(l); err != nil {
		return fmt.Errorf("route edges: %w", err)
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

// Hits yields all geometry occupying point in node, port, then edge order.
func (l *Layout) Hits(point Point) iter.Seq[Hit] {
	return func(yield func(Hit) bool) {
		for i := range l.Nodes {
			if !l.Nodes[i].Empty() &&
				l.Nodes[i].Rect.Contains(point) &&
				!yield(Hit{ID: uint32(i), Kind: HitNode}) {
				return
			}
		}
		if l.graph.AllPortsLive() {
			for i := range l.Ports {
				if l.Ports[i].Anchor == point &&
					!yield(Hit{ID: uint32(i), Kind: HitPort}) {
					return
				}
			}
		} else {
			for i := range l.Ports {
				if l.graph.PortExists(uint32(i)) &&
					l.Ports[i].Anchor == point &&
					!yield(Hit{ID: uint32(i), Kind: HitPort}) {
					return
				}
			}
		}
		for i := range l.Edges {
			for j := 1; j < len(l.Edges[i].Points); j++ {
				if pointOnSegment(point, l.Edges[i].Points[j-1], l.Edges[i].Points[j]) {
					if !yield(Hit{ID: uint32(i), Kind: HitEdge}) {
						return
					}
					break
				}
			}
		}
	}
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
	l.Nodes = make([]Node, len(l.graph.Nodes))
	l.Ports = make([]Port, len(l.graph.Ports))
	l.Edges = make([]Edge, len(l.graph.Edges))

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
	rect, err := NodeRect(point, size, l.padding)
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
	for i, portID := range source.Ports {
		port := l.graph.Ports[portID]
		resolved, err := ResolvePort(rect, port.Side, port.Offset)
		if err != nil {
			return fmt.Errorf("resolve port %d: %w", portID, err)
		}
		l.draftPorts[i] = resolved
	}
	return nil
}

func (l *Layout) commitNodePorts(nodeID uint32) {
	for i, portID := range l.graph.Nodes[nodeID].Ports {
		l.Ports[portID] = l.draftPorts[i]
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
		return NewRect(origin, origin.Add(3, 2))
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
