// Package layout resolves diagram IR into terminal-cell geometry.
package layout

import (
	"fmt"
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

// Padding describe empty cells between a node's border and its label.
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

// Layout owns editable diagram IR and its index-aligned cell geometry.
type Layout struct {
	Padding Padding
	Nodes   []Node
	Ports   []Port
	Edges   []Edge

	graph   ir.Graph
	origins []Point
}

// NewNode adds a node at the origin and returns its index.
func (l *Layout) NewNode(label string) uint32 {
	return l.NewNodeAt(label, Point{})
}

// NewNodeAt adds a node at point and returns its index.
func (l *Layout) NewNodeAt(label string, point Point) uint32 {
	nodeID := l.graph.NewNode(label)
	l.origins = append(l.origins, point)
	l.invalidate()
	return nodeID
}

// PlaceNode changes a node's origin.
func (l *Layout) PlaceNode(nodeID uint32, point Point) {
	l.origins[nodeID] = point
	l.invalidate()
}

// ConnectNodes connects side-constrained center ports and returns the edge index.
func (l *Layout) ConnectNodes(nodeA uint32, sideA, sideB ir.Side, nodeB uint32) uint32 {
	edgeID := l.graph.ConnectNodes(nodeA, sideA, sideB, nodeB)
	l.invalidate()
	return edgeID
}

// Label returns a node's source label.
func (l *Layout) Label(nodeID uint32) string {
	return l.graph.Nodes[nodeID].Label
}

// Build resolves the current diagram into node, port, and edge geometry.
func (l *Layout) Build() error {
	// Re-use existing capacity after calls to invalidate, if available.
	nodes := slices.Grow(l.Nodes, len(l.graph.Nodes))[:len(l.graph.Nodes)]
	ports := slices.Grow(l.Ports, len(l.graph.Ports))[:len(l.graph.Ports)]
	edges := slices.Grow(l.Edges, len(l.graph.Edges))[:len(l.graph.Edges)]

	for i := range l.graph.Nodes {
		size, err := MeasureLabel(l.graph.Nodes[i].Label)
		if err != nil {
			return fmt.Errorf("measure node %d label: %w", i, err)
		}
		rect, err := NodeRect(l.origins[i], size, l.Padding)
		if err != nil {
			return fmt.Errorf("size node %d: %w", i, err)
		}
		nodes[i] = Node{
			Rect: rect,
			LabelPoint: Point{
				X: rect.Min.X + 1 + uint32(l.Padding.Left),
				Y: rect.Min.Y + 1 + uint32(l.Padding.Top),
			},
		}
	}

	for i := range l.graph.Ports {
		port := l.graph.Ports[i]
		resolved, err := ResolvePort(nodes[port.Node].Rect, port.Side, port.Offset)
		if err != nil {
			return fmt.Errorf("resolve port %d: %w", i, err)
		}
		ports[i] = resolved
	}

	obstacles := make([]Rect, len(nodes))
	for i := range nodes {
		obstacles[i] = nodes[i].Rect
	}
	for i := range l.graph.Edges {
		edge := l.graph.Edges[i]
		routed, err := RouteOrthogonal(ports[edge.PortA], ports[edge.PortB], obstacles)
		if err != nil {
			return fmt.Errorf("route edge %d: %w", i, err)
		}
		edges[i] = routed
	}

	l.Nodes = nodes
	l.Ports = ports
	l.Edges = edges
	return nil
}

func (l *Layout) invalidate() {
	l.Nodes = l.Nodes[:0]
	l.Ports = l.Ports[:0]
	l.Edges = l.Edges[:0]
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
