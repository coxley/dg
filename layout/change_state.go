package layout

import (
	"errors"
	"fmt"
	"slices"

	"github.com/coxley/dg/ir"
)

type historyKind uint8

const (
	historyCreateNode historyKind = iota + 1
	historyDeleteNode
	historySetLabel
	historyPlaceNode
	historyCreateEdge
	historyDeleteEdge
	historyReconnectEdge
	historySetNodeSize
	historySetLayer
	historySetNodeStyle
	historySetEdgeStyle
	historySetRouter
	historySetAttachment
	historySetPinnedBends
)

type historyPort struct {
	ID   uint32  `json:"id"`
	Port ir.Port `json:"port"`
}

type historyEdge struct {
	ID    uint32       `json:"id"`
	Edge  ir.Edge      `json:"edge"`
	Style EdgeStyle    `json:"style,omitzero"`
	Bends []PinnedBend `json:"bends,omitempty"`
}

type historyLayer struct {
	Hit   Hit    `json:"hit"`
	Index uint32 `json:"index"`
}

type historyNode struct {
	ID          uint32         `json:"id"`
	Label       string         `json:"label,omitempty"`
	Origin      Point          `json:"origin"`
	Size        Size           `json:"size,omitzero"`
	Style       NodeStyle      `json:"style,omitzero"`
	Ports       []historyPort  `json:"ports,omitempty"`
	Edges       []historyEdge  `json:"edges,omitempty"`
	Layers      []historyLayer `json:"layers,omitempty"`
	Attachments []Attachment   `json:"attachments,omitempty"`
}

type historyChange struct {
	Kind     historyKind        `json:"kind"`
	ID       uint32             `json:"id"`
	LayerHit Hit                `json:"layer_hit,omitzero"`
	Before   historyChangeState `json:"before"`
	After    historyChangeState `json:"after"`
}

type historyChangeState struct {
	Point       Point        `json:"point,omitzero"`
	Size        Size         `json:"size,omitzero"`
	Label       string       `json:"label,omitempty"`
	Edge        ir.Edge      `json:"edge,omitzero"`
	NodeStyle   NodeStyle    `json:"node_style,omitzero"`
	EdgeStyle   EdgeStyle    `json:"edge_style,omitzero"`
	Router      Router       `json:"router,omitzero"`
	Layer       uint32       `json:"layer,omitempty"`
	Node        historyNode  `json:"node,omitzero"`
	Attachment  Attachment   `json:"attachment,omitzero"`
	Attached    bool         `json:"attached,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Bends       []PinnedBend `json:"bends,omitempty"`
}

type layoutHistoryState struct {
	Graph       ir.Graph       `json:"graph"`
	Origins     []Point        `json:"origins"`
	Sizes       []Size         `json:"sizes"`
	NodeStyles  []NodeStyle    `json:"node_styles"`
	EdgeStyles  []EdgeStyle    `json:"edge_styles"`
	EdgeBends   [][]PinnedBend `json:"edge_bends"`
	Order       []Hit          `json:"order"`
	Router      Router         `json:"router"`
	Attachments []Attachment   `json:"attachments"`
}

func (l *Layout) historyState() layoutHistoryState {
	return layoutHistoryState{
		Graph:       l.graph.Clone(),
		Origins:     slices.Clone(l.origins),
		Sizes:       slices.Clone(l.explicitSizes),
		NodeStyles:  slices.Clone(l.nodeStyles),
		EdgeStyles:  slices.Clone(l.edgeStyles),
		EdgeBends:   clonePinnedBends(l.edgeBends),
		Order:       slices.Clone(l.drawOrder),
		Router:      l.router,
		Attachments: slices.Clone(l.attachments),
	}
}

func (l *Layout) restoreHistoryState(state layoutHistoryState) error {
	l.graph = state.Graph.Clone()
	l.router = state.Router
	l.drawOrder = slices.Clone(state.Order)
	if err := l.initializeGeometry(); err != nil {
		return err
	}
	l.selection.Clear()
	l.explicitSizes = slices.Clone(state.Sizes)
	l.nodeStyles = slices.Clone(state.NodeStyles)
	l.edgeStyles = slices.Clone(state.EdgeStyles)
	l.edgeBends = clonePinnedBends(state.EdgeBends)
	l.attachments = slices.Clone(state.Attachments)
	for nodeID := range l.graph.Nodes {
		if !l.graph.NodeExists(uint32(nodeID)) {
			continue
		}
		node, err := l.prepareNode(
			uint32(nodeID),
			l.graph.Nodes[nodeID].Label,
			state.Origins[nodeID],
		)
		if err != nil {
			return err
		}
		l.origins[nodeID] = state.Origins[nodeID]
		l.Nodes[nodeID] = node
		l.commitNodePorts(uint32(nodeID))
	}
	return l.Build()
}

func (l *Layout) historyNode(nodeID uint32) historyNode {
	source := l.graph.Nodes[nodeID]
	node := historyNode{
		ID:     nodeID,
		Label:  source.Label,
		Origin: l.origins[nodeID],
		Size:   l.explicitSizes[nodeID],
		Style:  l.nodeStyles[nodeID],
		Ports:  make([]historyPort, 0, len(source.Ports)),
	}
	for _, portID := range source.Ports {
		node.Ports = append(node.Ports, historyPort{
			ID:   portID,
			Port: l.graph.Ports[portID],
		})
	}
	for edgeID := range l.graph.Edges {
		if l.graph.EdgeExists(uint32(edgeID)) &&
			l.graph.EdgeIncidentTo(uint32(edgeID), nodeID) {
			node.Edges = append(node.Edges, historyEdge{
				ID:    uint32(edgeID),
				Edge:  l.graph.Edges[edgeID],
				Style: l.edgeStyles[edgeID],
				Bends: slices.Clone(l.edgeBends[edgeID]),
			})
		}
	}
	for index, hit := range l.drawOrder {
		if hit == (Hit{ID: nodeID, Kind: HitNode}) {
			node.Layers = append(node.Layers, historyLayer{
				Hit:   hit,
				Index: uint32(index),
			})
			continue
		}
		if hit.Kind == HitEdge &&
			l.graph.EdgeExists(hit.ID) &&
			l.graph.EdgeIncidentTo(hit.ID, nodeID) {
			node.Layers = append(node.Layers, historyLayer{
				Hit:   hit,
				Index: uint32(index),
			})
		}
	}
	for attachment := range l.attachmentsForNodeAndEdges(nodeID, node.Edges) {
		node.Attachments = append(node.Attachments, attachment)
	}
	return node
}

func (l *Layout) restoreHistoryNode(node historyNode) error {
	if err := l.validateHistoryNodeTarget(node); err != nil {
		return err
	}
	l.graph.Nodes = growTo(l.graph.Nodes, int(node.ID)+1)
	l.graph.Ports = growTo(l.graph.Ports, maxHistoryPort(node)+1)
	l.graph.Edges = growTo(l.graph.Edges, maxHistoryEdge(node)+1)

	ports := slices.Grow(l.graph.Nodes[node.ID].Ports[:0], len(node.Ports))
	for _, port := range node.Ports {
		l.graph.Ports[port.ID] = port.Port
		ports = append(ports, port.ID)
	}
	l.graph.Nodes[node.ID] = ir.Node{Label: node.Label, Ports: ports}
	for _, edge := range node.Edges {
		l.graph.Edges[edge.ID] = edge.Edge
	}
	l.attachments = growTo(l.attachments, len(l.graph.Nodes))
	for _, attachment := range node.Attachments {
		l.setAttachmentState(attachment.NodeID, attachment, true)
	}
	l.graph = l.graph.Clone()

	l.origins = growTo(l.origins, len(l.graph.Nodes))
	l.explicitSizes = growTo(l.explicitSizes, len(l.graph.Nodes))
	l.nodeStyles = growTo(l.nodeStyles, len(l.graph.Nodes))
	l.Nodes = growTo(l.Nodes, len(l.graph.Nodes))
	l.Ports = growTo(l.Ports, len(l.graph.Ports))
	l.portUsable = growTo(l.portUsable, len(l.graph.Ports))
	l.Edges = growTo(l.Edges, len(l.graph.Edges))
	l.edgeStyles = growTo(l.edgeStyles, len(l.graph.Edges))
	l.edgeBends = growTo(l.edgeBends, len(l.graph.Edges))
	l.explicitSizes[node.ID] = node.Size
	l.nodeStyles[node.ID] = node.Style
	resolved, err := l.prepareNode(node.ID, node.Label, node.Origin)
	if err != nil {
		return err
	}
	l.origins[node.ID] = node.Origin
	l.Nodes[node.ID] = resolved
	l.commitNodePorts(node.ID)
	for _, edge := range node.Edges {
		l.Edges[edge.ID] = Edge{}
		l.edgeStyles[edge.ID] = edge.Style
		l.edgeBends[edge.ID] = slices.Clone(edge.Bends)
	}
	if len(node.Layers) == 0 {
		l.appendLayer(Hit{ID: node.ID, Kind: HitNode})
		for _, edge := range node.Edges {
			l.appendLayer(Hit{ID: edge.ID, Kind: HitEdge})
		}
	} else {
		for _, layer := range node.Layers {
			l.insertLayer(layer.Hit, int(layer.Index))
		}
	}
	return nil
}

func (l *Layout) validateHistoryNodeTarget(node historyNode) error {
	if uint64(node.ID) > uint64(len(l.graph.Nodes)) ||
		uint64(node.ID) < uint64(len(l.graph.Nodes)) && l.graph.NodeExists(node.ID) {
		return fmt.Errorf("restore node into occupied or sparse slot %d", node.ID)
	}
	if err := l.validateHistoryNodePorts(node); err != nil {
		return err
	}
	return l.validateHistoryNodeEdges(node)
}

func (l *Layout) validateHistoryNodePorts(node historyNode) error {
	portLimit := uint64(len(l.graph.Ports)) + uint64(len(node.Ports))
	appendedPorts := 0
	for i, port := range node.Ports {
		if port.Port.Node != node.ID || uint64(port.ID) >= portLimit ||
			uint64(port.ID) < uint64(len(l.graph.Ports)) && l.graph.PortExists(port.ID) {
			return fmt.Errorf("restore node %d with invalid port %d", node.ID, port.ID)
		}
		if uint64(port.ID) >= uint64(len(l.graph.Ports)) {
			appendedPorts++
		}
		for _, previous := range node.Ports[:i] {
			if previous.ID == port.ID {
				return fmt.Errorf("restore node %d with duplicate port %d", node.ID, port.ID)
			}
		}
	}
	portLimit = uint64(len(l.graph.Ports)) + uint64(appendedPorts)
	for _, port := range node.Ports {
		if uint64(port.ID) >= portLimit {
			return fmt.Errorf("restore node %d with sparse port %d", node.ID, port.ID)
		}
	}
	for portID := uint32(len(l.graph.Ports)); uint64(portID) < portLimit; portID++ {
		if !historyNodeHasPort(node, portID) {
			return fmt.Errorf("restore node %d with sparse port %d", node.ID, portID)
		}
	}
	return nil
}

func (l *Layout) validateHistoryNodeEdges(node historyNode) error {
	for i, edge := range node.Edges {
		if !l.validHistoryNodeEdgeTarget(node, edge) {
			return fmt.Errorf("restore node %d with invalid edge %d", node.ID, edge.ID)
		}
		for _, previous := range node.Edges[:i] {
			if previous.ID == edge.ID || previous.Edge.Connects(edge.Edge.PortA, edge.Edge.PortB) {
				return fmt.Errorf("restore node %d with duplicate edge %d", node.ID, edge.ID)
			}
		}
		for edgeID, existing := range l.graph.Edges {
			if l.graph.EdgeExists(uint32(edgeID)) && existing.Connects(edge.Edge.PortA, edge.Edge.PortB) {
				return fmt.Errorf("restore node %d with duplicate edge endpoints", node.ID)
			}
		}
	}
	return nil
}

func (l *Layout) validHistoryNodeEdgeTarget(node historyNode, edge historyEdge) bool {
	portAOnNode := historyNodeHasPort(node, edge.Edge.PortA)
	portBOnNode := historyNodeHasPort(node, edge.Edge.PortB)
	return uint64(edge.ID) < uint64(len(l.graph.Edges)) &&
		!l.graph.EdgeExists(edge.ID) &&
		edge.Edge.PortA != edge.Edge.PortB &&
		(portAOnNode || l.graph.PortExists(edge.Edge.PortA)) &&
		(portBOnNode || l.graph.PortExists(edge.Edge.PortB)) &&
		(portAOnNode || portBOnNode)
}

func historyNodeHasPort(node historyNode, portID uint32) bool {
	for _, port := range node.Ports {
		if port.ID == portID {
			return true
		}
	}
	return false
}

func (l *Layout) restoreHistoryEdge(
	edgeID uint32,
	edge ir.Edge,
	style EdgeStyle,
	bends []PinnedBend,
	layer int,
	replace bool,
) error {
	if !l.graph.PortExists(edge.PortA) || !l.graph.PortExists(edge.PortB) {
		return errors.New("restore edge with deleted port")
	}
	if uint64(edgeID) > uint64(len(l.graph.Edges)) ||
		uint64(edgeID) < uint64(len(l.graph.Edges)) && l.graph.EdgeExists(edgeID) != replace ||
		uint64(edgeID) == uint64(len(l.graph.Edges)) && replace {
		return fmt.Errorf("restore edge into occupied or sparse slot %d", edgeID)
	}
	for otherID, existing := range l.graph.Edges {
		if uint32(otherID) != edgeID && l.graph.EdgeExists(uint32(otherID)) &&
			existing.Connects(edge.PortA, edge.PortB) {
			return errors.New("restore duplicate edge")
		}
	}
	l.graph.Edges = growTo(l.graph.Edges, int(edgeID)+1)
	l.graph.Edges[edgeID] = edge
	l.graph = l.graph.Clone()
	l.Edges = growTo(l.Edges, len(l.graph.Edges))
	l.edgeStyles = growTo(l.edgeStyles, len(l.graph.Edges))
	l.edgeBends = growTo(l.edgeBends, len(l.graph.Edges))
	l.Edges[edgeID] = Edge{}
	l.edgeStyles[edgeID] = style
	l.edgeBends[edgeID] = slices.Clone(bends)
	hit := Hit{ID: edgeID, Kind: HitEdge}
	if !l.hasLayer(hit) {
		if layer < 0 {
			l.appendLayer(hit)
		} else {
			l.insertLayer(hit, layer)
		}
	}
	return nil
}

func maxHistoryPort(node historyNode) int {
	maxID := -1
	for _, port := range node.Ports {
		maxID = max(maxID, int(port.ID))
	}
	return maxID
}

func maxHistoryEdge(node historyNode) int {
	maxID := -1
	for _, edge := range node.Edges {
		maxID = max(maxID, int(edge.ID))
	}
	return maxID
}
