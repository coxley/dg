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

// Port contains a boundary cell and its outward neighbor.
type Port struct {
	Anchor Point // Cell on a node's border
	Exit   Point // First cell outside of the node
}

type Edge struct {
	Points []Point
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
	nodeID := uint32(len(l.graph.Nodes))
	node, err := l.nodeGeometry(nodeID, label, point)
	if err != nil {
		return 0, err
	}

	oldPortCount := len(l.graph.Ports)
	l.graph.NewNode(label)
	if err := l.resolveNodePorts(nodeID, node.Rect); err != nil {
		l.graph.Nodes = l.graph.Nodes[:nodeID]
		l.graph.Ports = l.graph.Ports[:oldPortCount]
		return 0, err
	}

	l.origins = append(l.origins, point)
	l.Nodes = append(l.Nodes, node)

	newPortCount := len(l.graph.Ports) - oldPortCount
	l.Ports = slices.Grow(l.Ports, newPortCount)[:len(l.graph.Ports)]
	l.commitNodePorts(nodeID)
	return nodeID, nil
}

// PlaceNode changes a node's origin if the resulting geometry is valid.
func (l *Layout) PlaceNode(nodeID uint32, point Point) error {
	node, err := l.nodeGeometry(nodeID, l.graph.Nodes[nodeID].Label, point)
	if err != nil {
		return err
	}
	if err := l.resolveNodePorts(nodeID, node.Rect); err != nil {
		return err
	}

	l.origins[nodeID] = point
	l.Nodes[nodeID] = node
	l.commitNodePorts(nodeID)
	return nil
}

// ConnectNodes connects side-constrained center ports and returns the edge index.
func (l *Layout) ConnectNodes(nodeA uint32, sideA, sideB ir.Side, nodeB uint32) uint32 {
	edgeCount := len(l.graph.Edges)
	edgeID := l.graph.ConnectNodes(nodeA, sideA, sideB, nodeB)
	if len(l.graph.Edges) != edgeCount {
		l.Edges = append(l.Edges, Edge{})
	}
	return edgeID
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
			if l.Nodes[i].Rect.Contains(point) &&
				!yield(Hit{ID: uint32(i), Kind: HitNode}) {
				return
			}
		}
		for i := range l.Ports {
			if l.Ports[i].Anchor == point &&
				!yield(Hit{ID: uint32(i), Kind: HitPort}) {
				return
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
	if err := validateGraph(&l.graph); err != nil {
		return fmt.Errorf("load graph: %w", err)
	}

	l.origins = make([]Point, len(l.graph.Nodes))
	l.Nodes = make([]Node, len(l.graph.Nodes))
	l.Ports = make([]Port, len(l.graph.Ports))
	l.Edges = make([]Edge, len(l.graph.Edges))

	for i := range l.graph.Nodes {
		nodeID := uint32(i)
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
	cloned := ir.Graph{
		Nodes: slices.Clone(graph.Nodes),
		Edges: slices.Clone(graph.Edges),
		Ports: slices.Clone(graph.Ports),
	}
	for i := range cloned.Nodes {
		cloned.Nodes[i].Ports = slices.Clone(cloned.Nodes[i].Ports)
	}
	return cloned
}

func validateGraph(graph *ir.Graph) error {
	seenPorts := make([]bool, len(graph.Ports))
	for nodeID := range graph.Nodes {
		for _, portID := range graph.Nodes[nodeID].Ports {
			if uint64(portID) >= uint64(len(graph.Ports)) {
				return fmt.Errorf("node %d references unknown port %d", nodeID, portID)
			}
			if seenPorts[portID] {
				return fmt.Errorf("port %d belongs to multiple nodes", portID)
			}
			if graph.Ports[portID].Node != uint32(nodeID) {
				return fmt.Errorf(
					"node %d references port %d owned by node %d",
					nodeID,
					portID,
					graph.Ports[portID].Node,
				)
			}
			seenPorts[portID] = true
		}
	}
	for portID, seen := range seenPorts {
		if !seen {
			return fmt.Errorf("port %d has no owning node", portID)
		}
	}
	for edgeID, edge := range graph.Edges {
		if uint64(edge.PortA) >= uint64(len(graph.Ports)) ||
			uint64(edge.PortB) >= uint64(len(graph.Ports)) {
			return fmt.Errorf("edge %d references an unknown port", edgeID)
		}
		if edge.PortA == edge.PortB {
			return fmt.Errorf("edge %d connects port %d to itself", edgeID, edge.PortA)
		}
	}
	return nil
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
