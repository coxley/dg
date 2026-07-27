// Package ir [...]
package ir

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/orsinium-labs/enum"
)

var (
	ErrNodeNotFound  = errors.New("node not found")
	ErrEdgeNotFound  = errors.New("edge not found")
	ErrPortNotFound  = errors.New("port not found")
	ErrSamePort      = errors.New("cannot connect a port to itself")
	ErrPortNotOnEdge = errors.New("port is not an edge endpoint")
	ErrDuplicateEdge = errors.New("edge already connects ports")
)

const deletedPortNode = math.MaxUint32

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

// Empty reports whether the node is a tombstone.
func (n Node) Empty() bool {
	return n.Label == "" && len(n.Ports) == 0
}

type Edge struct {
	PortA uint32
	PortB uint32
}

// Empty reports whether the edge is a tombstone.
func (e Edge) Empty() bool {
	var zero Edge
	return e == zero
}

// HasPort reports whether portID is one of the edge's endpoints.
func (e Edge) HasPort(portID uint32) bool {
	return e.PortA == portID || e.PortB == portID
}

// Connects reports whether the edge connects portA and portB in either order.
func (e Edge) Connects(portA, portB uint32) bool {
	return e.PortA == portA && e.PortB == portB ||
		e.PortA == portB && e.PortB == portA
}

// SharesPort reports whether the edges have an endpoint in common.
func (e Edge) SharesPort(other Edge) bool {
	return e.HasPort(other.PortA) || e.HasPort(other.PortB)
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

// Empty reports whether the port is a tombstone.
func (p Port) Empty() bool {
	return p.Node == deletedPortNode
}

// Graph is an undirected representation of diagram nodes connected by edges and ports.
type Graph struct {
	Nodes []Node
	Edges []Edge
	Ports []Port

	freeNodes []uint32
	freeEdges []uint32
	freePorts []uint32
}

func (g *Graph) String() string {
	var buf bytes.Buffer

	for i := range g.Edges {
		if !g.EdgeExists(uint32(i)) {
			continue
		}
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
	offsets := [...]float32{.5, .25, .75}
	sides := [...]Side{Top, RightSide, Bottom, LeftSide}
	portCount := len(offsets) * len(sides)
	nid := g.NextNodeID()
	var ports []uint32
	if int(nid) < len(g.Nodes) {
		ports = g.Nodes[nid].Ports
	}
	ports = slices.Grow(ports[:0], portCount)
	node := Node{
		Label: label,
		Ports: ports[:0],
	}

	if missing := portCount - len(g.freePorts); missing > 0 {
		g.Ports = slices.Grow(g.Ports, missing)
	}
	for _, offset := range offsets {
		for _, side := range sides {
			portID := g.newPort(NewPort(nid, side, offset))
			node.Ports = append(node.Ports, portID)
		}
	}

	if int(nid) == len(g.Nodes) {
		g.Nodes = append(g.Nodes, node)
	} else {
		g.freeNodes = g.freeNodes[:len(g.freeNodes)-1]
		g.Nodes[nid] = node
	}
	return nid
}

// NextNodeID returns the ID that NewNode will allocate.
func (g *Graph) NextNodeID() uint32 {
	if len(g.freeNodes) != 0 {
		return g.freeNodes[len(g.freeNodes)-1]
	}
	return uint32(len(g.Nodes))
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
	if !g.PortExists(portA) || !g.PortExists(portB) {
		panic("connect unknown port")
	}
	for i := range g.Edges {
		if !g.EdgeExists(uint32(i)) {
			continue
		}
		edge := g.Edges[i]
		if edge.Connects(portA, portB) {
			// Already exists
			return uint32(i)
		}
	}

	edge := Edge{PortA: portA, PortB: portB}
	if len(g.freeEdges) == 0 {
		g.Edges = append(g.Edges, edge)
		return uint32(len(g.Edges) - 1)
	}
	edgeID := g.freeEdges[len(g.freeEdges)-1]
	g.freeEdges = g.freeEdges[:len(g.freeEdges)-1]
	g.Edges[edgeID] = edge
	return edgeID
}

// ReconnectEdge replaces one endpoint while preserving the edge ID.
func (g *Graph) ReconnectEdge(edgeID, oldPort, newPort uint32) error {
	if !g.EdgeExists(edgeID) {
		return fmt.Errorf("%w: %d", ErrEdgeNotFound, edgeID)
	}
	if !g.PortExists(newPort) {
		return fmt.Errorf("%w: %d", ErrPortNotFound, newPort)
	}
	edge := g.Edges[edgeID]
	if !edge.HasPort(oldPort) {
		return fmt.Errorf("%w: %d", ErrPortNotOnEdge, oldPort)
	}
	if oldPort == newPort {
		return nil
	}

	otherPort := edge.PortA
	if oldPort == edge.PortA {
		otherPort = edge.PortB
	}
	if otherPort == newPort {
		return ErrSamePort
	}
	for i := range g.Edges {
		if uint32(i) != edgeID &&
			g.EdgeExists(uint32(i)) &&
			g.Edges[i].Connects(otherPort, newPort) {
			return ErrDuplicateEdge
		}
	}
	if oldPort == edge.PortA {
		g.Edges[edgeID].PortA = newPort
	} else {
		g.Edges[edgeID].PortB = newPort
	}
	return nil
}

// DeleteEdge removes an edge and makes its ID available for reuse.
func (g *Graph) DeleteEdge(edgeID uint32) error {
	if !g.EdgeExists(edgeID) {
		return fmt.Errorf("%w: %d", ErrEdgeNotFound, edgeID)
	}
	g.Edges[edgeID] = Edge{}
	g.freeEdges = append(g.freeEdges, edgeID)
	return nil
}

// DeleteNode removes a node, its ports, and its incident edges.
func (g *Graph) DeleteNode(nodeID uint32) error {
	if !g.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ErrNodeNotFound, nodeID)
	}

	for edgeID := range g.Edges {
		if !g.EdgeExists(uint32(edgeID)) {
			continue
		}
		if g.EdgeIncidentTo(uint32(edgeID), nodeID) {
			if err := g.DeleteEdge(uint32(edgeID)); err != nil {
				return err
			}
		}
	}
	ports := g.Nodes[nodeID].Ports
	for _, portID := range ports {
		g.Ports[portID] = Port{Node: deletedPortNode}
		g.freePorts = append(g.freePorts, portID)
	}
	g.Nodes[nodeID] = Node{Ports: ports[:0]}
	g.freeNodes = append(g.freeNodes, nodeID)
	return nil
}

// NodeExists reports whether nodeID identifies a live node.
func (g *Graph) NodeExists(nodeID uint32) bool {
	return uint64(nodeID) < uint64(len(g.Nodes)) && !g.Nodes[nodeID].Empty()
}

// EdgeExists reports whether edgeID identifies a live edge.
func (g *Graph) EdgeExists(edgeID uint32) bool {
	return uint64(edgeID) < uint64(len(g.Edges)) && !g.Edges[edgeID].Empty()
}

// PortExists reports whether portID identifies a live port.
func (g *Graph) PortExists(portID uint32) bool {
	return uint64(portID) < uint64(len(g.Ports)) && !g.Ports[portID].Empty()
}

// AllPortsLive reports whether the graph contains no deleted ports.
func (g *Graph) AllPortsLive() bool {
	return len(g.freePorts) == 0
}

// EdgeIncidentTo reports whether edgeID connects to a port owned by nodeID.
// Both IDs must identify live graph entries.
func (g *Graph) EdgeIncidentTo(edgeID, nodeID uint32) bool {
	edge := g.Edges[edgeID]
	return g.Ports[edge.PortA].Node == nodeID || g.Ports[edge.PortB].Node == nodeID
}

// Validate checks graph ownership and edge endpoint invariants.
func (g *Graph) Validate() error {
	seenPorts := make([]bool, len(g.Ports))
	for nodeID := range g.Nodes {
		if !g.NodeExists(uint32(nodeID)) {
			continue
		}
		for _, portID := range g.Nodes[nodeID].Ports {
			if uint64(portID) >= uint64(len(g.Ports)) {
				return fmt.Errorf("node %d references unknown port %d", nodeID, portID)
			}
			if seenPorts[portID] {
				return fmt.Errorf("port %d belongs to multiple nodes", portID)
			}
			if g.Ports[portID].Node != uint32(nodeID) {
				return fmt.Errorf(
					"node %d references port %d owned by node %d",
					nodeID,
					portID,
					g.Ports[portID].Node,
				)
			}
			seenPorts[portID] = true
		}
	}
	for portID, seen := range seenPorts {
		if g.PortExists(uint32(portID)) && !seen {
			return fmt.Errorf("port %d has no owning node", portID)
		}
	}
	for edgeID, edge := range g.Edges {
		if !g.EdgeExists(uint32(edgeID)) {
			continue
		}
		if uint64(edge.PortA) >= uint64(len(g.Ports)) ||
			uint64(edge.PortB) >= uint64(len(g.Ports)) {
			return fmt.Errorf("edge %d references an unknown port", edgeID)
		}
		if !g.PortExists(edge.PortA) || !g.PortExists(edge.PortB) {
			return fmt.Errorf("edge %d references a deleted port", edgeID)
		}
		if edge.PortA == edge.PortB {
			return fmt.Errorf("edge %d connects port %d to itself", edgeID, edge.PortA)
		}
	}
	return nil
}

// Clone returns an independent graph with reconstructed free lists.
func (g Graph) Clone() Graph {
	cloned := Graph{
		Nodes: slices.Clone(g.Nodes),
		Edges: slices.Clone(g.Edges),
		Ports: slices.Clone(g.Ports),
	}
	for i := range cloned.Nodes {
		cloned.Nodes[i].Ports = slices.Clone(cloned.Nodes[i].Ports)
	}
	cloned.rebuildFreeLists()
	return cloned
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

func (g *Graph) newPort(port Port) uint32 {
	if len(g.freePorts) == 0 {
		g.Ports = append(g.Ports, port)
		return uint32(len(g.Ports) - 1)
	}
	portID := g.freePorts[len(g.freePorts)-1]
	g.freePorts = g.freePorts[:len(g.freePorts)-1]
	g.Ports[portID] = port
	return portID
}

func (g *Graph) rebuildFreeLists() {
	g.freeNodes = g.freeNodes[:0]
	g.freeEdges = g.freeEdges[:0]
	g.freePorts = g.freePorts[:0]
	for i := range g.Nodes {
		if !g.NodeExists(uint32(i)) {
			g.freeNodes = append(g.freeNodes, uint32(i))
		}
	}
	for i := range g.Edges {
		if !g.EdgeExists(uint32(i)) {
			g.freeEdges = append(g.freeEdges, uint32(i))
		}
	}
	for i := range g.Ports {
		if !g.PortExists(uint32(i)) {
			g.freePorts = append(g.freePorts, uint32(i))
		}
	}
}
