// Package document converts editable layouts to and from a versioned JSON format.
package document

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
)

const CurrentVersion uint32 = 1

var ErrUnsupportedVersion = errors.New("unsupported document version")

// Document is the persisted representation of a diagram.
type Document struct {
	Version uint32  `json:"version"`
	Nodes   []Node  `json:"nodes"`
	Ports   []Port  `json:"ports"`
	Edges   []Edge  `json:"edges"`
	Layers  []Layer `json:"layers,omitempty"`
	Options Options `json:"options"`
}

// Node stores label, placement, and ordered port membership.
type Node struct {
	Label  string   `json:"label"`
	Origin Point    `json:"origin"`
	Size   Size     `json:"size,omitzero"`
	Ports  []uint32 `json:"ports"`
}

// Size stores fixed outer node dimensions. The zero value enables auto sizing.
type Size struct {
	Width  uint32 `json:"width"`
	Height uint32 `json:"height"`
}

// Point identifies a document cell.
type Point struct {
	X uint32 `json:"x"`
	Y uint32 `json:"y"`
}

// Side identifies a node boundary in the persisted schema.
type Side string

const (
	SideTop    Side = "top"
	SideRight  Side = "right"
	SideBottom Side = "bottom"
	SideLeft   Side = "left"
)

// Port stores a side and normalized offset.
type Port struct {
	Side   Side    `json:"side"`
	Offset float32 `json:"offset"`
}

// Edge stores two port indices.
type Edge struct {
	PortA uint32 `json:"port_a"`
	PortB uint32 `json:"port_b"`
}

// Layer identifies one node or edge in back-to-front order.
type Layer struct {
	Kind LayerKind `json:"kind"`
	ID   uint32    `json:"id"`
}

// LayerKind identifies a drawable object type.
type LayerKind string

const (
	LayerNode LayerKind = "node"
	LayerEdge LayerKind = "edge"
)

// Options stores layout and routing configuration.
type Options struct {
	Padding Padding `json:"padding"`
	Router  Router  `json:"router"`
}

// Padding stores symmetric horizontal and vertical node padding.
type Padding struct {
	Horizontal uint8 `json:"horizontal"`
	Vertical   uint8 `json:"vertical"`
}

// Router stores orthogonal routing configuration.
type Router struct {
	Costs         Costs `json:"costs"`
	ReroutePasses uint8 `json:"reroute_passes"`
}

// Costs stores route-scoring dimensions.
type Costs struct {
	Step         uint32 `json:"step"`
	SharedStep   uint32 `json:"shared_step"`
	Bend         uint32 `json:"bend"`
	Crossing     uint32 `json:"crossing"`
	EndpointStep uint32 `json:"endpoint_step"`
}

// FromLayout returns a compact document containing the layout's live objects.
func FromLayout(geo *layout.Layout) Document {
	graph := geo.Graph()
	padding := geo.Padding()
	router := geo.Router()
	doc := Document{
		Version: CurrentVersion,
		Nodes:   make([]Node, 0, len(graph.Nodes)),
		Ports:   make([]Port, 0, len(graph.Ports)),
		Edges:   make([]Edge, 0, len(graph.Edges)),
		Options: Options{
			Padding: Padding{
				Horizontal: padding.Left,
				Vertical:   padding.Top,
			},
			Router: Router{
				Costs: Costs{
					Step:         router.Costs.Step,
					SharedStep:   router.Costs.SharedStep,
					Bend:         router.Costs.Bend,
					Crossing:     router.Costs.Crossing,
					EndpointStep: router.Costs.EndpointStep,
				},
				ReroutePasses: router.ReroutePasses,
			},
		},
	}

	portIDs := make([]uint32, len(graph.Ports))
	nodeIDs := make([]uint32, len(graph.Nodes))
	for nodeID := range graph.Nodes {
		if !graph.NodeExists(uint32(nodeID)) {
			continue
		}
		source := &graph.Nodes[nodeID]
		nodeIDs[nodeID] = uint32(len(doc.Nodes))
		node := Node{
			Label: source.Label,
			Origin: Point{
				X: geo.Nodes[nodeID].Rect.Min.X,
				Y: geo.Nodes[nodeID].Rect.Min.Y,
			},
			Ports: make([]uint32, 0, len(source.Ports)),
		}
		if size, ok := geo.ExplicitNodeSize(uint32(nodeID)); ok {
			node.Size = Size{Width: size.Width, Height: size.Height}
		}
		for _, portID := range source.Ports {
			port := graph.Ports[portID]
			portIDs[portID] = uint32(len(doc.Ports))
			node.Ports = append(node.Ports, portIDs[portID])
			doc.Ports = append(doc.Ports, Port{
				Side:   Side(port.Side.String()),
				Offset: port.Offset,
			})
		}
		doc.Nodes = append(doc.Nodes, node)
	}
	edgeIDs := make([]uint32, len(graph.Edges))
	for edgeID := range graph.Edges {
		if !graph.EdgeExists(uint32(edgeID)) {
			continue
		}
		edge := graph.Edges[edgeID]
		edgeIDs[edgeID] = uint32(len(doc.Edges))
		doc.Edges = append(doc.Edges, Edge{
			PortA: portIDs[edge.PortA],
			PortB: portIDs[edge.PortB],
		})
	}
	for hit := range geo.DrawOrder() {
		switch hit.Kind {
		case layout.HitNode:
			doc.Layers = append(doc.Layers, Layer{
				Kind: LayerNode,
				ID:   nodeIDs[hit.ID],
			})
		case layout.HitEdge:
			doc.Layers = append(doc.Layers, Layer{
				Kind: LayerEdge,
				ID:   edgeIDs[hit.ID],
			})
		case layout.HitPort:
			continue
		}
	}
	return doc
}

// Marshal returns an indented JSON encoding of geo.
func Marshal(geo *layout.Layout) ([]byte, error) {
	data, err := json.MarshalIndent(FromLayout(geo), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal document: %w", err)
	}
	return append(data, '\n'), nil
}

// Unmarshal decodes and validates one JSON document.
func Unmarshal(data []byte) (Document, error) {
	var header struct {
		Version uint32 `json:"version"`
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&header); err != nil {
		return Document{}, fmt.Errorf("decode document header: %w", err)
	}
	if header.Version != CurrentVersion {
		return Document{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, header.Version)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	doc := Document{}
	doc.Options.Router.Costs.EndpointStep = layout.DefaultRouter().Costs.EndpointStep
	if err := decoder.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("decode document: %w", err)
	}
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	switch {
	case err == nil:
		return Document{}, errors.New("decode document: trailing JSON value")
	case !errors.Is(err, io.EOF):
		return Document{}, fmt.Errorf("decode document trailing data: %w", err)
	}
	if _, err := doc.Layout(); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// Layout validates d and returns an independent editable layout.
func (d Document) Layout(options ...layout.Option) (*layout.Layout, error) {
	if d.Version != CurrentVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, d.Version)
	}
	graph, err := d.graph()
	if err != nil {
		return nil, err
	}
	router := layout.Router{
		Costs: layout.Costs{
			Step:         d.Options.Router.Costs.Step,
			SharedStep:   d.Options.Router.Costs.SharedStep,
			Bend:         d.Options.Router.Costs.Bend,
			Crossing:     d.Options.Router.Costs.Crossing,
			EndpointStep: d.Options.Router.Costs.EndpointStep,
		},
		ReroutePasses: d.Options.Router.ReroutePasses,
	}
	drawOrder, err := d.drawOrder()
	if err != nil {
		return nil, err
	}
	baseOptions := []layout.Option{
		layout.WithGraph(graph),
		layout.WithDrawOrder(drawOrder),
		layout.WithPadding(
			d.Options.Padding.Horizontal,
			d.Options.Padding.Vertical,
		),
		layout.WithRouter(router),
	}
	geo, err := layout.New(slices.Concat(options, baseOptions)...)
	if err != nil {
		return nil, fmt.Errorf("create layout: %w", err)
	}
	for nodeID, node := range d.Nodes {
		if node.Size.Width != 0 || node.Size.Height != 0 {
			if err := geo.SetNodeSize(uint32(nodeID), layout.Size{
				Width:  node.Size.Width,
				Height: node.Size.Height,
			}); err != nil {
				return nil, fmt.Errorf("size node %d: %w", nodeID, err)
			}
		}
		if err := geo.PlaceNode(uint32(nodeID), layout.NewPoint(node.Origin.X, node.Origin.Y)); err != nil {
			return nil, fmt.Errorf("place node %d: %w", nodeID, err)
		}
	}
	if history := geo.History(); history != nil {
		history.Clear()
	}
	return geo, nil
}

func (d Document) drawOrder() ([]layout.Hit, error) {
	if len(d.Layers) == 0 {
		return nil, nil
	}
	want := len(d.Nodes) + len(d.Edges)
	if len(d.Layers) != want {
		return nil, fmt.Errorf(
			"document contains %d layers, want %d",
			len(d.Layers),
			want,
		)
	}
	order := make([]layout.Hit, len(d.Layers))
	seenNodes := make([]bool, len(d.Nodes))
	seenEdges := make([]bool, len(d.Edges))
	for i, layer := range d.Layers {
		var seen []bool
		switch layer.Kind {
		case LayerNode:
			order[i] = layout.Hit{ID: layer.ID, Kind: layout.HitNode}
			seen = seenNodes
		case LayerEdge:
			order[i] = layout.Hit{ID: layer.ID, Kind: layout.HitEdge}
			seen = seenEdges
		default:
			return nil, fmt.Errorf("layer %d has unknown kind %q", i, layer.Kind)
		}
		if uint64(layer.ID) >= uint64(len(seen)) {
			return nil, fmt.Errorf(
				"layer %d references unknown %s %d",
				i,
				layer.Kind,
				layer.ID,
			)
		}
		if seen[layer.ID] {
			return nil, fmt.Errorf(
				"layer %d duplicates %s %d",
				i,
				layer.Kind,
				layer.ID,
			)
		}
		seen[layer.ID] = true
	}
	return order, nil
}

func (d Document) graph() (ir.Graph, error) {
	if uint64(len(d.Nodes)) > math.MaxUint32 ||
		uint64(len(d.Ports)) > math.MaxUint32 ||
		uint64(len(d.Edges)) > math.MaxUint32 {
		return ir.Graph{}, errors.New("document contains too many objects")
	}

	graph := ir.Graph{
		Nodes: make([]ir.Node, len(d.Nodes)),
		Ports: make([]ir.Port, len(d.Ports)),
		Edges: make([]ir.Edge, len(d.Edges)),
	}
	seenPorts := make([]bool, len(d.Ports))
	for nodeID, node := range d.Nodes {
		if node.Label == "" && len(node.Ports) == 0 {
			return ir.Graph{}, fmt.Errorf("node %d is empty", nodeID)
		}
		graph.Nodes[nodeID] = ir.Node{
			Label: node.Label,
			Ports: slices.Clone(node.Ports),
		}
		for _, portID := range node.Ports {
			if uint64(portID) >= uint64(len(d.Ports)) {
				return ir.Graph{}, fmt.Errorf("node %d references unknown port %d", nodeID, portID)
			}
			if seenPorts[portID] {
				return ir.Graph{}, fmt.Errorf("port %d belongs to multiple nodes", portID)
			}
			port := d.Ports[portID]
			side, ok := parseSide(port.Side)
			if !ok {
				return ir.Graph{}, fmt.Errorf("port %d has unknown side %q", portID, port.Side)
			}
			if math.IsNaN(float64(port.Offset)) ||
				math.IsInf(float64(port.Offset), 0) ||
				port.Offset < 0 ||
				port.Offset > 1 {
				return ir.Graph{}, fmt.Errorf("port %d offset %v outside [0, 1]", portID, port.Offset)
			}
			graph.Ports[portID] = ir.NewPort(uint32(nodeID), side, port.Offset)
			seenPorts[portID] = true
		}
	}
	for portID, seen := range seenPorts {
		if !seen {
			return ir.Graph{}, fmt.Errorf("port %d has no owning node", portID)
		}
	}

	seenEdges := make(map[[2]uint32]struct{}, len(d.Edges))
	for edgeID, edge := range d.Edges {
		if uint64(edge.PortA) >= uint64(len(d.Ports)) ||
			uint64(edge.PortB) >= uint64(len(d.Ports)) {
			return ir.Graph{}, fmt.Errorf("edge %d references an unknown port", edgeID)
		}
		if edge.PortA == edge.PortB {
			return ir.Graph{}, fmt.Errorf("edge %d connects port %d to itself", edgeID, edge.PortA)
		}
		key := [2]uint32{min(edge.PortA, edge.PortB), max(edge.PortA, edge.PortB)}
		if _, ok := seenEdges[key]; ok {
			return ir.Graph{}, fmt.Errorf("edge %d duplicates ports %d and %d", edgeID, edge.PortA, edge.PortB)
		}
		seenEdges[key] = struct{}{}
		graph.Edges[edgeID] = ir.Edge{PortA: edge.PortA, PortB: edge.PortB}
	}
	if err := graph.Validate(); err != nil {
		return ir.Graph{}, fmt.Errorf("validate graph: %w", err)
	}
	return graph, nil
}

func parseSide(value Side) (ir.Side, bool) {
	switch value {
	case SideTop:
		return ir.Top, true
	case SideRight:
		return ir.RightSide, true
	case SideBottom:
		return ir.Bottom, true
	case SideLeft:
		return ir.LeftSide, true
	default:
		return ir.Side{}, false
	}
}
