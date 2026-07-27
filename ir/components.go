package ir

import (
	"math"
	"slices"
)

// Components indexes node connectivity for a graph. Build reuses its storage,
// so callers can retain an index across repeated connectivity queries.
type Components struct {
	parents []uint32
	ranks   []uint8
}

// Build replaces the index with the connected components in graph.
func (c *Components) Build(graph *Graph) {
	c.parents = growComponents(c.parents, len(graph.Nodes))
	c.ranks = growComponents(c.ranks, len(graph.Nodes))
	for nodeID := range graph.Nodes {
		if graph.NodeExists(uint32(nodeID)) {
			c.parents[nodeID] = uint32(nodeID)
		} else {
			c.parents[nodeID] = math.MaxUint32
		}
		c.ranks[nodeID] = 0
	}
	for edgeID, edge := range graph.Edges {
		if !graph.EdgeExists(uint32(edgeID)) {
			continue
		}
		c.union(
			graph.Ports[edge.PortA].Node,
			graph.Ports[edge.PortB].Node,
		)
	}
}

// ID returns the representative ID for nodeID's connected component.
func (c *Components) ID(nodeID uint32) (uint32, bool) {
	if uint64(nodeID) >= uint64(len(c.parents)) ||
		c.parents[nodeID] == math.MaxUint32 {
		return 0, false
	}
	return c.root(nodeID), true
}

func (c *Components) root(nodeID uint32) uint32 {
	root := nodeID
	for c.parents[root] != root {
		root = c.parents[root]
	}
	for c.parents[nodeID] != nodeID {
		parent := c.parents[nodeID]
		c.parents[nodeID] = root
		nodeID = parent
	}
	return root
}

func (c *Components) union(nodeA, nodeB uint32) {
	rootA := c.root(nodeA)
	rootB := c.root(nodeB)
	if rootA == rootB {
		return
	}
	if c.ranks[rootA] < c.ranks[rootB] {
		rootA, rootB = rootB, rootA
	}
	c.parents[rootB] = rootA
	if c.ranks[rootA] == c.ranks[rootB] {
		c.ranks[rootA]++
	}
}

func growComponents[S ~[]E, E any](values S, size int) S {
	if size <= len(values) {
		return values
	}
	return slices.Grow(values, size-len(values))[:size]
}
