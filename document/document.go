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
	"github.com/google/uuid"
)

const CurrentVersion uint32 = 4

var ErrUnsupportedVersion = errors.New("unsupported document version")

// Document is the persisted representation of a diagram.
type Document struct {
	Version     uint32       `json:"version"`
	ID          uuid.UUID    `json:"id"`
	Nodes       []Node       `json:"nodes"`
	Ports       []Port       `json:"ports"`
	Edges       []Edge       `json:"edges"`
	Groups      []Group      `json:"groups,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Layers      []Layer      `json:"layers,omitempty"`
	Options     Options      `json:"options"`
}

// Group stores ordered immediate node and subgroup membership.
type Group struct {
	Members []GroupMember `json:"members"`
}

// GroupMember stores one node or subgroup reference.
type GroupMember struct {
	Kind GroupMemberKind `json:"kind"`
	ID   uint32          `json:"id"`
}

// GroupMemberKind identifies a persisted group member type.
type GroupMemberKind string

const (
	GroupMemberNode  GroupMemberKind = "node"
	GroupMemberGroup GroupMemberKind = "group"
)

// Node stores label, placement, and ordered port membership.
type Node struct {
	Label  string    `json:"label"`
	Origin Point     `json:"origin"`
	Size   Size      `json:"size,omitzero"`
	Style  NodeStyle `json:"style,omitzero"`
	Ports  []uint32  `json:"ports"`
}

// NodeStyle stores node rendering choices.
type NodeStyle struct {
	Border     BorderStyle     `json:"border,omitempty"`
	Stroke     StrokeStyle     `json:"stroke,omitempty"`
	Horizontal HorizontalAlign `json:"horizontal,omitempty"`
	Vertical   VerticalAlign   `json:"vertical,omitempty"`
	Padding    PaddingLevel    `json:"padding,omitempty"`
}

// PaddingLevel identifies persisted node label spacing.
type PaddingLevel string

const (
	PaddingNone  PaddingLevel = "none"
	PaddingExtra PaddingLevel = "extra"
)

// BorderStyle identifies a persisted node border.
type BorderStyle string

const (
	BorderRounded BorderStyle = "rounded"
	BorderDouble  BorderStyle = "double"
	BorderNone    BorderStyle = "none"
)

// StrokeStyle identifies a persisted line pattern.
type StrokeStyle string

const StrokeDashed StrokeStyle = "dashed"

// HorizontalAlign identifies persisted horizontal label placement.
type HorizontalAlign string

const (
	AlignCenter HorizontalAlign = "center"
	AlignRight  HorizontalAlign = "right"
)

// VerticalAlign identifies persisted vertical label placement.
type VerticalAlign string

const (
	AlignMiddle VerticalAlign = "middle"
	AlignBottom VerticalAlign = "bottom"
)

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

// Edge stores two port indices and ordered routing constraints.
type Edge struct {
	PortA uint32       `json:"port_a"`
	PortB uint32       `json:"port_b"`
	Style EdgeStyle    `json:"style,omitzero"`
	Bends []PinnedBend `json:"bends,omitempty"`
}

// PinnedBend fixes one orthogonal turn in an edge route.
type PinnedBend struct {
	Point    Point     `json:"point"`
	Incoming Direction `json:"incoming"`
	Outgoing Direction `json:"outgoing"`
}

// Direction identifies cardinal travel through a pinned bend.
type Direction string

const (
	DirectionNorth Direction = "north"
	DirectionEast  Direction = "east"
	DirectionSouth Direction = "south"
	DirectionWest  Direction = "west"
)

// AttachmentEnd identifies the route endpoint used to address a landmark.
type AttachmentEnd string

const (
	AttachmentPortA AttachmentEnd = "port_a"
	AttachmentPortB AttachmentEnd = "port_b"
)

// AttachmentReference identifies an endpoint or bend in route order.
type AttachmentReference struct {
	End      AttachmentEnd `json:"end"`
	Bend     uint32        `json:"bend,omitempty"`
	Incoming Direction     `json:"incoming,omitempty"`
	Outgoing Direction     `json:"outgoing,omitempty"`
}

// Attachment stores a node's cell offset from an edge landmark.
type Attachment struct {
	Node      uint32              `json:"node"`
	Edge      uint32              `json:"edge"`
	Reference AttachmentReference `json:"reference"`
	Offset    int64               `json:"offset"`
	Anchor    Point               `json:"anchor"`
}

// EdgeStyle stores endpoint arrow choices.
type EdgeStyle struct {
	PortAArrow ArrowStyle  `json:"port_a_arrow,omitempty"`
	PortBArrow ArrowStyle  `json:"port_b_arrow,omitempty"`
	Stroke     StrokeStyle `json:"stroke,omitempty"`
}

// ArrowStyle identifies a persisted endpoint marker.
type ArrowStyle string

const (
	ArrowOpen         ArrowStyle = "open"
	ArrowFilled       ArrowStyle = "filled"
	ArrowCircle       ArrowStyle = "circle"
	ArrowCircleBullet ArrowStyle = "circle_bullet"
)

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
	Router Router `json:"router"`
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

// New returns a compact document with a fresh identity.
func New(geo *layout.Layout) Document {
	doc := Document{ID: uuid.New()}
	doc.Update(geo)
	return doc
}

// Update replaces the document contents while preserving its identity and capacity.
func (d *Document) Update(geo *layout.Layout) {
	graph := geo.Graph()
	router := geo.Router()
	previousNodes := d.Nodes[:cap(d.Nodes)]
	previousEdges := d.Edges[:cap(d.Edges)]
	previousGroups := d.Groups[:cap(d.Groups)]
	d.Version = CurrentVersion
	d.Nodes = d.Nodes[:0]
	d.Ports = d.Ports[:0]
	d.Edges = d.Edges[:0]
	d.Groups = d.Groups[:0]
	d.Attachments = d.Attachments[:0]
	d.Layers = d.Layers[:0]
	d.Options = Options{
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
	}

	portIDs := make([]uint32, len(graph.Ports))
	nodeIDs := make([]uint32, len(graph.Nodes))
	for nodeID := range graph.Nodes {
		if !graph.NodeExists(uint32(nodeID)) {
			continue
		}
		source := &graph.Nodes[nodeID]
		compactID := len(d.Nodes)
		nodeIDs[nodeID] = uint32(compactID)
		var ports []uint32
		if compactID < len(previousNodes) {
			ports = previousNodes[compactID].Ports
		}
		node := Node{
			Label: source.Label,
			Origin: Point{
				X: geo.Nodes[nodeID].Rect.Min.X,
				Y: geo.Nodes[nodeID].Rect.Min.Y,
			},
			Ports: slices.Grow(ports[:0], len(source.Ports)),
		}
		style, _ := geo.NodeStyle(uint32(nodeID))
		node.Style = documentNodeStyle(style)
		if size, ok := geo.ExplicitNodeSize(uint32(nodeID)); ok {
			node.Size = Size{Width: size.Width, Height: size.Height}
		}
		for _, portID := range source.Ports {
			port := graph.Ports[portID]
			portIDs[portID] = uint32(len(d.Ports))
			node.Ports = append(node.Ports, portIDs[portID])
			d.Ports = append(d.Ports, Port{
				Side:   Side(port.Side.String()),
				Offset: port.Offset,
			})
		}
		d.Nodes = append(d.Nodes, node)
	}
	groupIDs := make([]uint32, len(graph.Groups))
	for groupID := range graph.Groups {
		if graph.GroupExists(uint32(groupID)) {
			groupIDs[groupID] = uint32(len(d.Groups))
			d.Groups = append(d.Groups, Group{})
		}
	}
	for groupID, source := range graph.Groups {
		if !graph.GroupExists(uint32(groupID)) {
			continue
		}
		compactID := groupIDs[groupID]
		var members []GroupMember
		if int(compactID) < len(previousGroups) {
			members = previousGroups[compactID].Members
		}
		members = slices.Grow(members[:0], len(source.Members))
		for _, member := range source.Members {
			switch member.Kind {
			case ir.MemberNode:
				members = append(members, GroupMember{Kind: GroupMemberNode, ID: nodeIDs[member.ID]})
			case ir.MemberGroup:
				members = append(members, GroupMember{Kind: GroupMemberGroup, ID: groupIDs[member.ID]})
			}
		}
		d.Groups[compactID] = Group{Members: members}
	}
	edgeIDs := make([]uint32, len(graph.Edges))
	for edgeID := range graph.Edges {
		if !graph.EdgeExists(uint32(edgeID)) {
			continue
		}
		edge := graph.Edges[edgeID]
		style, _ := geo.EdgeStyle(uint32(edgeID))
		bends, _ := geo.PinnedBends(uint32(edgeID))
		compactID := len(d.Edges)
		edgeIDs[edgeID] = uint32(compactID)
		var previousBends []PinnedBend
		if compactID < len(previousEdges) {
			previousBends = previousEdges[compactID].Bends
		}
		d.Edges = append(d.Edges, Edge{
			PortA: portIDs[edge.PortA],
			PortB: portIDs[edge.PortB],
			Style: documentEdgeStyle(style),
			Bends: documentPinnedBendsInto(previousBends[:0], bends),
		})
	}
	for nodeID := range graph.Nodes {
		if !graph.NodeExists(uint32(nodeID)) {
			continue
		}
		attachment, ok := geo.NodeAttachment(uint32(nodeID))
		if !ok {
			continue
		}
		d.Attachments = append(d.Attachments, Attachment{
			Node:      nodeIDs[attachment.NodeID],
			Edge:      edgeIDs[attachment.EdgeID],
			Reference: documentAttachmentReference(attachment.Reference),
			Offset:    attachment.Offset,
			Anchor:    Point{X: attachment.Anchor.X, Y: attachment.Anchor.Y},
		})
	}
	for hit := range geo.DrawOrder() {
		switch hit.Kind {
		case layout.HitNode:
			d.Layers = append(d.Layers, Layer{
				Kind: LayerNode,
				ID:   nodeIDs[hit.ID],
			})
		case layout.HitEdge:
			d.Layers = append(d.Layers, Layer{
				Kind: LayerEdge,
				ID:   edgeIDs[hit.ID],
			})
		case layout.HitPort, layout.HitGroup:
			continue
		}
	}
}

// Marshal returns an indented JSON encoding of d.
func Marshal(d Document) ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal document: %w", err)
	}
	return append(data, '\n'), nil
}

// Unmarshal decodes and validates one JSON document.
func Unmarshal(data []byte) (Document, error) {
	doc := Document{}
	if err := UnmarshalInto(data, &doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// UnmarshalInto decodes and validates one JSON document while reusing dst capacity.
// An error may leave dst partially decoded.
func UnmarshalInto(data []byte, dst *Document) error {
	_, err := Migrate(data, dst)
	return err
}

// Migrate decodes data into the current schema and reports whether it upgraded
// an older supported version. An error may leave dst partially decoded.
func Migrate(data []byte, dst *Document) (bool, error) {
	if dst == nil {
		return false, errors.New("decode document into nil destination")
	}
	dst.resetForDecode()
	decodeErr := decodeJSONInto(data, dst)
	sourceVersion := dst.Version
	if decodeErr != nil && sourceVersion == 0 {
		version, err := encodedVersion(data)
		if err != nil {
			return false, decodeErr
		}
		sourceVersion = version
	}
	if sourceVersion == CurrentVersion {
		if decodeErr != nil {
			return false, decodeErr
		}
		if _, err := dst.Convert(); err != nil {
			return false, err
		}
		return false, nil
	}
	if sourceVersion != 2 && sourceVersion != 3 {
		return false, fmt.Errorf("%w: %d", ErrUnsupportedVersion, sourceVersion)
	}
	data, err := migrateJSON(data, sourceVersion)
	if err != nil {
		return false, err
	}
	dst.resetForDecode()
	if err := decodeJSONInto(data, dst); err != nil {
		return false, err
	}
	if dst.Version != CurrentVersion {
		return false, fmt.Errorf("%w: %d", ErrUnsupportedVersion, dst.Version)
	}
	if _, err := dst.Convert(); err != nil {
		return false, err
	}
	return true, nil
}

func decodeJSONInto(data []byte, dst *Document) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode document: %w", err)
	}
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	switch {
	case err == nil:
		return errors.New("decode document: trailing JSON value")
	case !errors.Is(err, io.EOF):
		return fmt.Errorf("decode document trailing data: %w", err)
	}
	return nil
}

func (d *Document) resetForDecode() {
	nodes := d.Nodes[:cap(d.Nodes)]
	for i := range nodes {
		ports := nodes[i].Ports[:0]
		nodes[i] = Node{Ports: ports}
	}
	edges := d.Edges[:cap(d.Edges)]
	for i := range edges {
		bends := edges[i].Bends[:0]
		edges[i] = Edge{Bends: bends}
	}
	groups := d.Groups[:cap(d.Groups)]
	for i := range groups {
		members := groups[i].Members[:0]
		groups[i] = Group{Members: members}
	}
	d.Version = 0
	d.ID = uuid.Nil
	d.Nodes = nodes[:0]
	d.Ports = d.Ports[:0]
	d.Edges = edges[:0]
	d.Groups = groups[:0]
	d.Attachments = d.Attachments[:0]
	d.Layers = d.Layers[:0]
	d.Options = Options{}
	d.Options.Router.Costs.EndpointStep = layout.DefaultRouter().Costs.EndpointStep
}

// Convert validates d and returns an independent editable layout.
func (d Document) Convert(options ...layout.Option) (*layout.Layout, error) {
	options, err := d.conversionOptions(options)
	if err != nil {
		return nil, err
	}
	geo, err := layout.New(options...)
	if err != nil {
		return nil, fmt.Errorf("create layout: %w", err)
	}
	if err := d.populate(geo); err != nil {
		return nil, err
	}
	return geo, nil
}

// ConvertInto atomically replaces dst while retaining its reusable storage.
func (d Document) ConvertInto(dst *layout.Layout, options ...layout.Option) error {
	if dst == nil {
		return errors.New("convert document into nil layout")
	}
	options, err := d.conversionOptions(options)
	if err != nil {
		return err
	}
	if err := dst.Replace(d.populate, options...); err != nil {
		return fmt.Errorf("replace layout: %w", err)
	}
	return nil
}

func (d Document) conversionOptions(options []layout.Option) ([]layout.Option, error) {
	if d.Version != CurrentVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, d.Version)
	}
	if d.ID == uuid.Nil || d.ID.Version() != 4 {
		return nil, errors.New("document requires a UUIDv4 identity")
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
		layout.WithRouter(router),
	}
	return slices.Concat(options, baseOptions), nil
}

func (d Document) populate(geo *layout.Layout) error {
	for nodeID, node := range d.Nodes {
		style, err := node.Style.layoutStyle()
		if err != nil {
			return fmt.Errorf("style node %d: %w", nodeID, err)
		}
		if err := geo.SetNodeStyle(uint32(nodeID), style); err != nil {
			return fmt.Errorf("style node %d: %w", nodeID, err)
		}
		if node.Size.Width != 0 || node.Size.Height != 0 {
			if err := geo.SetNodeSize(uint32(nodeID), layout.Size{
				Width:  node.Size.Width,
				Height: node.Size.Height,
			}); err != nil {
				return fmt.Errorf("size node %d: %w", nodeID, err)
			}
		}
		if err := geo.PlaceNode(uint32(nodeID), layout.NewPoint(node.Origin.X, node.Origin.Y)); err != nil {
			return fmt.Errorf("place node %d: %w", nodeID, err)
		}
	}
	for edgeID, edge := range d.Edges {
		style, err := edge.Style.layoutStyle()
		if err != nil {
			return fmt.Errorf("style edge %d: %w", edgeID, err)
		}
		if err := geo.SetEdgeStyle(uint32(edgeID), style); err != nil {
			return fmt.Errorf("style edge %d: %w", edgeID, err)
		}
		bends, err := edge.layoutPinnedBends()
		if err != nil {
			return fmt.Errorf("bend edge %d: %w", edgeID, err)
		}
		if err := geo.SetPinnedBends(uint32(edgeID), bends); err != nil {
			return fmt.Errorf("bend edge %d: %w", edgeID, err)
		}
	}
	seenAttachments := make([]uint32, 0, len(d.Attachments))
	attachments := make([]layout.Attachment, 0, len(d.Attachments))
	for i, attachment := range d.Attachments {
		if uint64(attachment.Node) >= uint64(len(d.Nodes)) {
			return fmt.Errorf(
				"attachment %d references unknown node %d",
				i,
				attachment.Node,
			)
		}
		if uint64(attachment.Edge) >= uint64(len(d.Edges)) {
			return fmt.Errorf(
				"attachment %d references unknown edge %d",
				i,
				attachment.Edge,
			)
		}
		if slices.Contains(seenAttachments, attachment.Node) {
			return fmt.Errorf(
				"attachment %d duplicates node %d",
				i,
				attachment.Node,
			)
		}
		seenAttachments = append(seenAttachments, attachment.Node)
		reference, err := attachment.Reference.layoutReference()
		if err != nil {
			return fmt.Errorf("attachment %d reference: %w", i, err)
		}
		attachments = append(attachments, layout.Attachment{
			NodeID:    attachment.Node,
			EdgeID:    attachment.Edge,
			Reference: reference,
			Offset:    attachment.Offset,
			Anchor:    layout.NewPoint(attachment.Anchor.X, attachment.Anchor.Y),
		})
	}
	if err := geo.SetAttachments(attachments...); err != nil {
		return fmt.Errorf("restore attachments: %w", err)
	}
	return nil
}

func documentAttachmentReference(reference layout.AttachmentReference) AttachmentReference {
	var end AttachmentEnd
	switch reference.End {
	case layout.AttachmentPortA:
		end = AttachmentPortA
	case layout.AttachmentPortB:
		end = AttachmentPortB
	}
	return AttachmentReference{
		End:      end,
		Bend:     reference.Bend,
		Incoming: documentDirection(reference.Incoming),
		Outgoing: documentDirection(reference.Outgoing),
	}
}

func (r AttachmentReference) layoutReference() (layout.AttachmentReference, error) {
	var end layout.AttachmentEnd
	switch r.End {
	case AttachmentPortA:
		end = layout.AttachmentPortA
	case AttachmentPortB:
		end = layout.AttachmentPortB
	default:
		return layout.AttachmentReference{}, fmt.Errorf("unknown attachment end %q", r.End)
	}
	reference := layout.AttachmentReference{End: end, Bend: r.Bend}
	if r.Bend == 0 {
		if r.Incoming != "" || r.Outgoing != "" {
			return layout.AttachmentReference{}, errors.New("endpoint has bend directions")
		}
		return reference, nil
	}
	var err error
	reference.Incoming, err = r.Incoming.layoutConnection()
	if err != nil {
		return layout.AttachmentReference{}, fmt.Errorf("incoming: %w", err)
	}
	reference.Outgoing, err = r.Outgoing.layoutConnection()
	if err != nil {
		return layout.AttachmentReference{}, fmt.Errorf("outgoing: %w", err)
	}
	if !reference.Valid() {
		return layout.AttachmentReference{}, errors.New("invalid bend turn")
	}
	return reference, nil
}

func documentNodeStyle(style layout.NodeStyle) NodeStyle {
	var border BorderStyle
	switch style.Border {
	case layout.BorderSolid:
	case layout.BorderRounded:
		border = BorderRounded
	case layout.BorderDouble:
		border = BorderDouble
	case layout.BorderNone:
		border = BorderNone
	}
	var horizontal HorizontalAlign
	switch style.Horizontal {
	case layout.AlignLeft:
	case layout.AlignCenter:
		horizontal = AlignCenter
	case layout.AlignRight:
		horizontal = AlignRight
	}
	var vertical VerticalAlign
	switch style.Vertical {
	case layout.AlignTop:
	case layout.AlignMiddle:
		vertical = AlignMiddle
	case layout.AlignBottom:
		vertical = AlignBottom
	}
	var padding PaddingLevel
	switch style.Padding {
	case layout.PaddingDefault:
	case layout.PaddingNone:
		padding = PaddingNone
	case layout.PaddingExtra:
		padding = PaddingExtra
	}
	return NodeStyle{
		Border:     border,
		Stroke:     documentStrokeStyle(style.Stroke),
		Horizontal: horizontal,
		Vertical:   vertical,
		Padding:    padding,
	}
}

func (s NodeStyle) layoutStyle() (layout.NodeStyle, error) {
	var border layout.BorderStyle
	switch s.Border {
	case "":
		border = layout.BorderSolid
	case BorderRounded:
		border = layout.BorderRounded
	case BorderDouble:
		border = layout.BorderDouble
	case BorderNone:
		border = layout.BorderNone
	default:
		return layout.NodeStyle{}, fmt.Errorf("unknown border %q", s.Border)
	}
	if s.Stroke != "" && s.Stroke != StrokeDashed {
		return layout.NodeStyle{}, fmt.Errorf("unknown stroke %q", s.Stroke)
	}
	var horizontal layout.HorizontalAlign
	switch s.Horizontal {
	case "":
		horizontal = layout.AlignLeft
	case AlignCenter:
		horizontal = layout.AlignCenter
	case AlignRight:
		horizontal = layout.AlignRight
	default:
		return layout.NodeStyle{}, fmt.Errorf("unknown horizontal alignment %q", s.Horizontal)
	}
	var vertical layout.VerticalAlign
	switch s.Vertical {
	case "":
		vertical = layout.AlignTop
	case AlignMiddle:
		vertical = layout.AlignMiddle
	case AlignBottom:
		vertical = layout.AlignBottom
	default:
		return layout.NodeStyle{}, fmt.Errorf("unknown vertical alignment %q", s.Vertical)
	}
	var padding layout.PaddingLevel
	switch s.Padding {
	case "":
		padding = layout.PaddingDefault
	case PaddingNone:
		padding = layout.PaddingNone
	case PaddingExtra:
		padding = layout.PaddingExtra
	default:
		return layout.NodeStyle{}, fmt.Errorf("unknown padding %q", s.Padding)
	}
	return layout.NodeStyle{
		Border:     border,
		Stroke:     s.Stroke.layoutStyle(),
		Horizontal: horizontal,
		Vertical:   vertical,
		Padding:    padding,
	}, nil
}

func documentEdgeStyle(style layout.EdgeStyle) EdgeStyle {
	return EdgeStyle{
		PortAArrow: documentArrowStyle(style.PortAArrow),
		PortBArrow: documentArrowStyle(style.PortBArrow),
		Stroke:     documentStrokeStyle(style.Stroke),
	}
}

func documentPinnedBendsInto(dst []PinnedBend, bends []layout.PinnedBend) []PinnedBend {
	if len(bends) == 0 {
		return dst[:0]
	}
	result := slices.Grow(dst[:0], len(bends))[:len(bends)]
	for i, bend := range bends {
		result[i] = PinnedBend{
			Point:    Point{X: bend.Point.X, Y: bend.Point.Y},
			Incoming: documentDirection(bend.Incoming),
			Outgoing: documentDirection(bend.Outgoing),
		}
	}
	return result
}

func documentDirection(direction layout.Connections) Direction {
	switch direction {
	case layout.North:
		return DirectionNorth
	case layout.East:
		return DirectionEast
	case layout.South:
		return DirectionSouth
	case layout.West:
		return DirectionWest
	default:
		return ""
	}
}

func (e Edge) layoutPinnedBends() ([]layout.PinnedBend, error) {
	if len(e.Bends) == 0 {
		return nil, nil
	}
	result := make([]layout.PinnedBend, len(e.Bends))
	for i, bend := range e.Bends {
		incoming, err := bend.Incoming.layoutConnection()
		if err != nil {
			return nil, fmt.Errorf("bend %d incoming: %w", i, err)
		}
		outgoing, err := bend.Outgoing.layoutConnection()
		if err != nil {
			return nil, fmt.Errorf("bend %d outgoing: %w", i, err)
		}
		result[i] = layout.PinnedBend{
			Point:    layout.NewPoint(bend.Point.X, bend.Point.Y),
			Incoming: incoming,
			Outgoing: outgoing,
		}
	}
	return result, nil
}

func (d Direction) layoutConnection() (layout.Connections, error) {
	switch d {
	case DirectionNorth:
		return layout.North, nil
	case DirectionEast:
		return layout.East, nil
	case DirectionSouth:
		return layout.South, nil
	case DirectionWest:
		return layout.West, nil
	default:
		return 0, fmt.Errorf("unknown direction %q", d)
	}
}

func documentStrokeStyle(style layout.StrokeStyle) StrokeStyle {
	if style == layout.StrokeDashed {
		return StrokeDashed
	}
	return ""
}

func (s StrokeStyle) layoutStyle() layout.StrokeStyle {
	if s == StrokeDashed {
		return layout.StrokeDashed
	}
	return layout.StrokeSolid
}

func documentArrowStyle(style layout.ArrowStyle) ArrowStyle {
	switch style {
	case layout.ArrowOpen:
		return ArrowOpen
	case layout.ArrowFilled:
		return ArrowFilled
	case layout.ArrowCircle:
		return ArrowCircle
	case layout.ArrowCircleBullet:
		return ArrowCircleBullet
	default:
		return ""
	}
}

func (s EdgeStyle) layoutStyle() (layout.EdgeStyle, error) {
	portA, err := s.PortAArrow.layoutStyle()
	if err != nil {
		return layout.EdgeStyle{}, fmt.Errorf("port A: %w", err)
	}
	portB, err := s.PortBArrow.layoutStyle()
	if err != nil {
		return layout.EdgeStyle{}, fmt.Errorf("port B: %w", err)
	}
	if s.Stroke != "" && s.Stroke != StrokeDashed {
		return layout.EdgeStyle{}, fmt.Errorf("unknown stroke %q", s.Stroke)
	}
	return layout.EdgeStyle{
		PortAArrow: portA,
		PortBArrow: portB,
		Stroke:     s.Stroke.layoutStyle(),
	}, nil
}

func (s ArrowStyle) layoutStyle() (layout.ArrowStyle, error) {
	switch s {
	case "":
		return layout.ArrowNone, nil
	case ArrowOpen:
		return layout.ArrowOpen, nil
	case ArrowFilled:
		return layout.ArrowFilled, nil
	case ArrowCircle:
		return layout.ArrowCircle, nil
	case ArrowCircleBullet:
		return layout.ArrowCircleBullet, nil
	default:
		return layout.ArrowNone, fmt.Errorf("unknown arrow %q", s)
	}
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
		uint64(len(d.Edges)) > math.MaxUint32 ||
		uint64(len(d.Groups)) > math.MaxUint32 {
		return ir.Graph{}, errors.New("document contains too many objects")
	}

	graph := ir.Graph{
		Nodes:  make([]ir.Node, len(d.Nodes)),
		Ports:  make([]ir.Port, len(d.Ports)),
		Edges:  make([]ir.Edge, len(d.Edges)),
		Groups: make([]ir.Group, len(d.Groups)),
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
	for groupID, group := range d.Groups {
		members := make([]ir.Member, len(group.Members))
		for memberID, member := range group.Members {
			switch member.Kind {
			case GroupMemberNode:
				members[memberID] = ir.Member{Kind: ir.MemberNode, ID: member.ID}
			case GroupMemberGroup:
				members[memberID] = ir.Member{Kind: ir.MemberGroup, ID: member.ID}
			default:
				return ir.Graph{}, fmt.Errorf(
					"group %d member %d has unknown kind %q",
					groupID,
					memberID,
					member.Kind,
				)
			}
		}
		graph.Groups[groupID] = ir.Group{Members: members}
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
