// Package ir [...]
package ir

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/orsinium-labs/enum"
)

type Side enum.Member[uint8]

var (
	sideb     = enum.NewStrictBuilder[uint8, Side]()
	Top       = sideb.AddValue(0)
	RightSide = sideb.AddValue(1)
	Bottom    = sideb.AddValue(2)
	LeftSide  = sideb.AddValue(3)
	Sides     = sideb.Enum()
)

func (d Side) String() string {
	switch d {
	case Top:
		return "top"
	case RightSide:
		return "right"
	case Bottom:
		return "bottom"
	case LeftSide:
		return "left"
	default:
		return "unknown"
	}
}

type Node struct {
	Label string
	Ports []uint32
}

type Edge struct {
	PortA uint32
	PortB uint32
}

type Port struct {
	Node uint32
	Side Side

	// Offset of the port in relation to the side it is placed on.
	// Valid range: [0.0, 1.0]
	//
	// Examples:
	//
	//   - Top, 0.0: Top left corner
	//   - Right, 1.0: Bottom right corner
	Offset float32
}

// Graph is an undirected representation of diagram nodes connected by edges and ports.
type Graph struct {
	Nodes []Node
	Edges []Edge
	Ports []Port
}

func (g *Graph) String() string {
	var buf bytes.Buffer

	for i := range g.Edges {
		edge := g.Edges[i]
		portA := g.Ports[edge.PortA]
		portB := g.Ports[edge.PortB]
		nodeA := g.Nodes[portA.Node]
		nodeB := g.Nodes[portB.Node]
		fmt.Fprintf(&buf, "[%s] <=> [%s]\n", nodeA.Label, nodeB.Label)
	}
	return buf.String()
}

func (g *Graph) NewNode(label string) uint32 {
	offsets := []float32{.5, .25, .75, .1}
	sides := []Side{Top, RightSide, Bottom, LeftSide}
	node := Node{
		Label: label,
		Ports: make([]uint32, 0, len(offsets)*len(sides)),
	}
	nid := uint32(len(g.Nodes))

	g.Ports = slices.Grow(g.Ports, len(offsets)*len(sides))
	for _, offset := range offsets {
		for _, side := range sides {
			node.Ports = append(node.Ports, uint32(len(g.Ports)))
			g.Ports = append(g.Ports, NewPort(nid, side, offset))
		}
	}

	g.Nodes = append(g.Nodes, node)
	return nid
}

// ConnectNodes and return the edge ID. The nodes are connected using the center port
// on each side. Returns the existing ID if the nodes are already connected on that
// side.
func (g *Graph) ConnectNodes(nodeA uint32, sideA Side, sideB Side, nodeB uint32) uint32 {
	portA, ok := g.PickCenterPort(nodeA, sideA)
	if !ok {
		panic("add port if missing")
	}
	portB, ok := g.PickCenterPort(nodeB, sideB)
	if !ok {
		panic("add port if missing")
	}
	return g.ConnectPorts(portA, portB)
}

// ConnectPorts and return the edge ID. Returns the existing ID if the ports are
// already connected. A port represents a visual connection between to ports that
// occupy space, so it doesn't make sense to create duplicates.
func (g *Graph) ConnectPorts(portA, portB uint32) uint32 {
	if portA == portB {
		panic("self-loop on same port")
	}
	for i := range g.Edges {
		edge := g.Edges[i]
		if (edge.PortA == portA || edge.PortA == portB) && (edge.PortB == portA || edge.PortB == portB) {
			// Already exists
			return uint32(i)
		}
	}
	g.Edges = append(g.Edges, Edge{PortA: portA, PortB: portB})
	return uint32(len(g.Edges) - 1)
}

// PickCenterPort returns the first port on the side, which is our center port. This
// logic will need changed if we later support changing the initial ports.
func (g *Graph) PickCenterPort(nodeID uint32, side Side) (uint32, bool) {
	for _, pid := range g.Nodes[nodeID].Ports {
		if g.Ports[pid].Side == side {
			return pid, true
		}
	}
	return 0, false
}

func (g *Graph) PortsOnSide(nodeID uint32, side Side) []uint32 {
	var pids []uint32
	for _, pid := range g.Nodes[nodeID].Ports {
		if g.Ports[pid].Side == side {
			pids = append(pids, pid)
		}
	}
	return pids
}

func NewPort(nodeID uint32, side Side, offset float32) Port {
	if offset < 0.0 {
		offset = 0.0
	}
	if offset > 1.0 {
		offset = 1.0
	}
	return Port{Node: nodeID, Side: side, Offset: offset}
}
