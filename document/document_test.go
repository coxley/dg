package document

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestRoundTripProperties(t *testing.T) {
	t.Parallel()

	documents := generatedDocument(0)
	rapid.Check(t, func(t *rapid.T) {
		want := documents.Draw(t, "document")
		geo, err := want.Convert()
		require.NoError(t, err)
		updated := want
		updated.Update(geo)
		require.Equal(t, want, updated)

		first, err := Marshal(updated)
		require.NoError(t, err)
		got, err := Unmarshal(first)
		require.NoError(t, err)
		require.Equal(t, want, got)

		loaded, err := got.Convert()
		require.NoError(t, err)
		got.Update(loaded)
		second, err := Marshal(got)
		require.NoError(t, err)
		require.Equal(t, first, second)
	})
}

func TestNewAndUpdatePreserveIdentityAndCapacity(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	for range 8 {
		_, err := geo.NewNode("node")
		require.NoError(t, err)
	}
	doc := New(geo)
	require.Equal(t, uuid.Version(4), doc.ID.Version())
	id := doc.ID
	nodeCapacity := cap(doc.Nodes)
	portCapacity := cap(doc.Ports)

	require.NoError(t, geo.DeleteNode(7))
	doc.Update(geo)
	require.Equal(t, id, doc.ID)
	require.GreaterOrEqual(t, cap(doc.Nodes), nodeCapacity)
	require.GreaterOrEqual(t, cap(doc.Ports), portCapacity)
}

func TestConvertIntoReusesLayoutAndPreservesCallback(t *testing.T) {
	t.Parallel()

	first := validDocument()
	second := validDocument()
	second.Nodes[0].Label = "replacement"
	second.Nodes = append(second.Nodes, Node{Label: "third", Ports: []uint32{2}})
	second.Ports = append(second.Ports, Port{Side: SideTop, Offset: 0.5})
	second.Layers = []Layer{
		{Kind: LayerNode, ID: 0},
		{Kind: LayerNode, ID: 1},
		{Kind: LayerNode, ID: 2},
		{Kind: LayerEdge, ID: 0},
	}

	geo, err := first.Convert()
	require.NoError(t, err)
	require.True(t, geo.Selection().SelectOnly(layout.Hit{ID: 0, Kind: layout.HitNode}))
	changes := 0
	require.NoError(t, geo.SetChangeCallback(func(layout.Change) { changes++ }))
	pointer := geo
	require.NoError(t, second.ConvertInto(geo))
	require.Same(t, pointer, geo)
	require.Equal(t, "replacement", geo.Label(0))
	require.True(t, geo.Selection().Empty())
	require.Zero(t, changes)
	require.NoError(t, geo.SetNodeLabel(0, "edited"))
	require.Equal(t, 1, changes)
}

func TestConvertIntoFailureLeavesLayoutUnchanged(t *testing.T) {
	t.Parallel()

	doc := validDocument()
	geo, err := doc.Convert()
	require.NoError(t, err)
	require.True(t, geo.Selection().SelectOnly(layout.Hit{ID: 0, Kind: layout.HitNode}))
	doc.Nodes[0].Style.Border = "invalid"

	err = doc.ConvertInto(geo)
	require.ErrorContains(t, err, "unknown border")
	require.Equal(t, "source", geo.Label(0))
	require.True(t, geo.Selection().Contains(layout.Hit{ID: 0, Kind: layout.HitNode}))
}

func TestConvertIntoAlternatesRetainedCapacity(t *testing.T) {
	t.Parallel()

	largeLayout, err := layout.New()
	require.NoError(t, err)
	for range 16 {
		_, err := largeLayout.NewNode("node")
		require.NoError(t, err)
	}
	large := New(largeLayout)
	small := validDocument()
	geo, err := layout.New()
	require.NoError(t, err)
	require.NoError(t, large.ConvertInto(geo))
	largeNodeCapacity := cap(geo.Nodes)
	require.NoError(t, small.ConvertInto(geo))
	require.NoError(t, large.ConvertInto(geo))
	require.GreaterOrEqual(t, cap(geo.Nodes), largeNodeCapacity)
}

func TestInvalidPortOffsetProperties(t *testing.T) {
	t.Parallel()

	offsets := rapid.OneOf(
		rapid.Float32Range(-100, -0.0001),
		rapid.Float32Range(1.0001, 100),
		rapid.SampledFrom([]float32{
			float32(math.NaN()),
			float32(math.Inf(1)),
			float32(math.Inf(-1)),
		}),
	)
	rapid.Check(t, func(t *rapid.T) {
		doc := validDocument()
		doc.Ports[0].Offset = offsets.Draw(t, "offset")

		_, err := doc.Convert()
		require.ErrorContains(t, err, "outside [0, 1]")
	})
}

func TestMarshalCompactsDeletedSlots(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", layout.NewPoint(3, 4))
	require.NoError(t, err)
	deleted, err := geo.NewNodeAt("deleted", layout.NewPoint(15, 2))
	require.NoError(t, err)
	sink, err := geo.NewNodeAt("sink", layout.NewPoint(30, 10))
	require.NoError(t, err)
	geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, sink)
	require.NoError(t, geo.DeleteNode(deleted))
	require.NoError(t, geo.Build())

	doc := New(geo)
	require.Equal(t, []string{"source", "sink"}, []string{doc.Nodes[0].Label, doc.Nodes[1].Label})
	require.Len(t, doc.Ports, 24)
	require.Len(t, doc.Edges, 1)
	require.Less(t, doc.Edges[0].PortA, uint32(len(doc.Ports)))
	require.Less(t, doc.Edges[0].PortB, uint32(len(doc.Ports)))

	data, err := Marshal(doc)
	require.NoError(t, err)
	require.Contains(t, string(data), `"version": 4`)
	require.NotContains(t, string(data), "points")
}

func TestUnmarshalRejectsVersionOne(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"version": 1,
		"nodes": [],
		"ports": [],
		"edges": [],
		"options": {
			"router": {
				"costs": {"step": 10, "shared_step": 2, "bend": 5, "crossing": 15},
				"reroute_passes": 1
			}
		}
	}`)
	_, err := Unmarshal(data)
	require.ErrorIs(t, err, ErrUnsupportedVersion)
}

func TestMigrateVersion2Attachment(t *testing.T) {
	t.Parallel()

	want := validAttachmentDocument(t)
	data := version2AttachmentData(t, want, math.MaxUint16/2)
	var got Document
	migrated, err := Migrate(data, &got)
	require.NoError(t, err)
	require.True(t, migrated)
	require.Equal(t, CurrentVersion, got.Version)
	require.Equal(t, want.ID, got.ID)
	require.Len(t, got.Attachments, 1)
	require.True(t, got.Attachments[0].Reference.End == AttachmentPortA ||
		got.Attachments[0].Reference.End == AttachmentPortB)

	geo, err := got.Convert()
	require.NoError(t, err)
	attachment, ok := geo.NodeAttachment(got.Attachments[0].Node)
	require.True(t, ok)
	point := geo.Nodes[attachment.NodeID].Rect.Min.Add(attachment.Anchor.X, attachment.Anchor.Y)
	require.True(t, geo.Edges[attachment.EdgeID].Contains(point))
	wantAttachment := want.Attachments[0]
	wantPoint := layout.NewPoint(
		want.Nodes[wantAttachment.Node].Origin.X+wantAttachment.Anchor.X,
		want.Nodes[wantAttachment.Node].Origin.Y+wantAttachment.Anchor.Y,
	)
	require.Equal(t, wantPoint, point)

	current, err := Marshal(got)
	require.NoError(t, err)
	migrated, err = Migrate(current, &got)
	require.NoError(t, err)
	require.False(t, migrated)
}

func TestMigrateVersion2WithoutAttachments(t *testing.T) {
	t.Parallel()

	want := validDocument()
	want.Version = 2
	data, err := Marshal(want)
	require.NoError(t, err)
	var got Document
	migrated, err := Migrate(data, &got)
	require.NoError(t, err)
	require.True(t, migrated)
	want.Version = CurrentVersion
	require.Equal(t, want, got)
}

func TestMigrateVersion3AddsEmptyGroups(t *testing.T) {
	t.Parallel()

	want := validDocument()
	want.Version = 3
	data, err := Marshal(want)
	require.NoError(t, err)
	var got Document
	migrated, err := Migrate(data, &got)
	require.NoError(t, err)
	require.True(t, migrated)
	want.Version = CurrentVersion
	require.Equal(t, want, got)
}

func TestRoundTripNestedGroupsCompactsIDs(t *testing.T) {
	t.Parallel()

	var graph ir.Graph
	dummyA := graph.NewNode("dummy a")
	dummyB := graph.NewNode("dummy b")
	left := graph.NewNode("left")
	middle := graph.NewNode("middle")
	right := graph.NewNode("right")
	dummy, err := graph.NewGroup([]ir.Member{
		{ID: dummyA, Kind: ir.MemberNode},
		{ID: dummyB, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	shape, err := graph.NewGroup([]ir.Member{
		{ID: left, Kind: ir.MemberNode},
		{ID: middle, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	_, err = graph.NewGroup([]ir.Member{
		{ID: shape, Kind: ir.MemberGroup},
		{ID: right, Kind: ir.MemberNode},
	})
	require.NoError(t, err)
	_, err = graph.Ungroup(dummy)
	require.NoError(t, err)

	geo, err := layout.New(layout.WithGraph(graph))
	require.NoError(t, err)
	doc := New(geo)
	require.Len(t, doc.Groups, 2)
	require.Equal(t, []GroupMember{
		{Kind: GroupMemberNode, ID: left},
		{Kind: GroupMemberNode, ID: middle},
	}, doc.Groups[0].Members)
	require.Equal(t, GroupMember{Kind: GroupMemberGroup, ID: 0}, doc.Groups[1].Members[0])

	data, err := Marshal(doc)
	require.NoError(t, err)
	decoded, err := Unmarshal(data)
	require.NoError(t, err)
	restored, err := decoded.Convert()
	require.NoError(t, err)
	restoredGraph := restored.Graph()
	require.Len(t, restoredGraph.Groups, 2)
	require.Equal(t, []uint32{left, middle, right}, slices.Collect(restoredGraph.DescendantNodes(1)))
}

func TestDocumentRejectsInvalidGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		groups []Group
		want   string
	}{
		{
			name: "singleton",
			groups: []Group{{Members: []GroupMember{
				{Kind: GroupMemberNode, ID: 0},
			}}},
			want: "fewer than two members",
		},
		{
			name: "invalid member kind",
			groups: []Group{{Members: []GroupMember{
				{Kind: "shape", ID: 0},
				{Kind: GroupMemberNode, ID: 1},
			}}},
			want: "unknown kind",
		},
		{
			name: "missing node member",
			groups: []Group{{Members: []GroupMember{
				{Kind: GroupMemberNode, ID: 0},
				{Kind: GroupMemberNode, ID: 99},
			}}},
			want: "references unknown node",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			doc := validDocument()
			doc.Groups = test.groups
			_, err := doc.Convert()
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestMigrateVersion2RejectsEndpointAttachment(t *testing.T) {
	t.Parallel()

	data := version2AttachmentData(t, validAttachmentDocument(t), 0)
	var doc Document
	_, err := Migrate(data, &doc)
	require.ErrorContains(t, err, "overlaps an edge endpoint")
}

func version2AttachmentData(t testing.TB, doc Document, position uint16) []byte {
	t.Helper()

	data, err := Marshal(doc)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fields))
	fields["version"] = json.RawMessage("2")
	attachments := []attachmentV2{{
		Node:     doc.Attachments[0].Node,
		Edge:     doc.Attachments[0].Edge,
		Position: position,
		Anchor:   doc.Attachments[0].Anchor,
	}}
	fields["attachments"], err = json.Marshal(attachments)
	require.NoError(t, err)
	data, err = json.Marshal(fields)
	require.NoError(t, err)
	return data
}

func TestRoundTripMultilineLabelAndExplicitSize(t *testing.T) {
	t.Parallel()

	doc := validDocument()
	doc.Nodes[0].Label = "one\ntwo three"
	doc.Nodes[0].Size = Size{Width: 9, Height: 4}

	geo, err := doc.Convert()
	require.NoError(t, err)
	updated := doc
	updated.Update(geo)
	require.Equal(t, doc, updated)

	data, err := Marshal(updated)
	require.NoError(t, err)
	require.Contains(t, string(data), `"size"`)
	decoded, err := Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, doc, decoded)
}

func TestRoundTripPinnedBends(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", layout.NewPoint(2, 4))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", layout.NewPoint(30, 14))
	require.NoError(t, err)
	edgeID := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	bends := []layout.PinnedBend{
		{
			Point:    layout.NewPoint(15, 5),
			Incoming: layout.East,
			Outgoing: layout.South,
		},
		{
			Point:    layout.NewPoint(15, 15),
			Incoming: layout.South,
			Outgoing: layout.East,
		},
	}
	require.NoError(t, geo.SetPinnedBends(edgeID, bends))
	require.NoError(t, geo.Build())

	doc := New(geo)
	data, err := Marshal(doc)
	require.NoError(t, err)
	require.Contains(t, string(data), `"bends"`)
	doc, err = Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, []PinnedBend{
		{
			Point:    Point{X: 15, Y: 5},
			Incoming: DirectionEast,
			Outgoing: DirectionSouth,
		},
		{
			Point:    Point{X: 15, Y: 15},
			Incoming: DirectionSouth,
			Outgoing: DirectionEast,
		},
	}, doc.Edges[0].Bends)

	loaded, err := doc.Convert()
	require.NoError(t, err)
	got, err := loaded.PinnedBends(0)
	require.NoError(t, err)
	require.Equal(t, bends, got)
	doc.Update(loaded)
	second, err := Marshal(doc)
	require.NoError(t, err)
	require.Equal(t, data, second)
}

func TestDocumentRejectsInvalidPinnedBendDirection(t *testing.T) {
	t.Parallel()

	doc := validDocument()
	doc.Edges[0].Bends = []PinnedBend{{
		Point:    Point{X: 5, Y: 5},
		Incoming: "diagonal",
		Outgoing: DirectionSouth,
	}}

	_, err := doc.Convert()
	require.ErrorContains(t, err, `unknown direction "diagonal"`)
}

func TestRoundTripAttachment(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", layout.NewPoint(2, 4))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", layout.NewPoint(30, 4))
	require.NoError(t, err)
	node, err := geo.NewNodeAt("tag", layout.NewPoint(12, 15))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	point := documentEdgeMiddle(geo.Edges[edge].Points)
	require.NoError(t, geo.PlaceNode(
		node,
		layout.NewPoint(point.X-2, point.Y-1),
	))
	require.NoError(t, geo.AttachNode(node, edge, point))

	doc := New(geo)
	data, err := Marshal(doc)
	require.NoError(t, err)
	require.Contains(t, string(data), `"attachments"`)
	doc, err = Unmarshal(data)
	require.NoError(t, err)
	require.Len(t, doc.Attachments, 1)

	loaded, err := doc.Convert()
	require.NoError(t, err)
	updated := doc
	updated.Update(loaded)
	require.Equal(t, doc, updated)
	got, ok := loaded.NodeAttachment(doc.Attachments[0].Node)
	require.True(t, ok)
	require.Equal(t, doc.Attachments[0].Edge, got.EdgeID)
	require.Equal(t, doc.Attachments[0].Reference, documentAttachmentReference(got.Reference))
	require.Equal(t, doc.Attachments[0].Offset, got.Offset)
	require.Equal(t, doc.Attachments[0].Anchor, Point{X: got.Anchor.X, Y: got.Anchor.Y})
}

func TestRoundTripAttachmentProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		geo, err := layout.New()
		require.NoError(t, err)
		source, err := geo.NewNodeAt("source", layout.NewPoint(20, 20))
		require.NoError(t, err)
		destination, err := geo.NewNodeAt(
			"destination",
			layout.NewPoint(
				rapid.Uint32Range(70, 120).Draw(t, "destination x"),
				rapid.Uint32Range(10, 60).Draw(t, "destination y"),
			),
		)
		require.NoError(t, err)
		node, err := geo.NewNodeAt("tag", layout.NewPoint(40, 70))
		require.NoError(t, err)
		edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
		require.NoError(t, geo.Build())

		length := documentPathLength(geo.Edges[edge].Points)
		require.Greater(t, length, uint64(1))
		point := documentEdgePoint(
			geo.Edges[edge].Points,
			rapid.Uint64Range(1, length-1).Draw(t, "edge offset"),
		)
		size := geo.Nodes[node].Rect.Size
		anchor := layout.NewPoint(
			rapid.Uint32Range(0, min(size.Width-1, point.X)).Draw(t, "anchor x"),
			rapid.Uint32Range(0, min(size.Height-1, point.Y)).Draw(t, "anchor y"),
		)
		require.NoError(t, geo.PlaceNode(
			node,
			layout.NewPoint(point.X-anchor.X, point.Y-anchor.Y),
		))
		require.NoError(t, geo.AttachNode(node, edge, point))

		doc := New(geo)
		data, err := Marshal(doc)
		require.NoError(t, err)
		doc, err = Unmarshal(data)
		require.NoError(t, err)
		loaded, err := doc.Convert()
		require.NoError(t, err)
		doc.Update(loaded)
		second, err := Marshal(doc)
		require.NoError(t, err)
		require.Equal(t, data, second)
	})
}

func TestDocumentAttachmentValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Document)
		want   string
	}{
		{
			name: "unknown node",
			mutate: func(doc *Document) {
				doc.Attachments[0].Node = uint32(len(doc.Nodes))
			},
			want: "unknown node",
		},
		{
			name: "unknown edge",
			mutate: func(doc *Document) {
				doc.Attachments[0].Edge = uint32(len(doc.Edges))
			},
			want: "unknown edge",
		},
		{
			name: "duplicate node",
			mutate: func(doc *Document) {
				doc.Attachments = append(doc.Attachments, doc.Attachments[0])
			},
			want: "duplicates node",
		},
		{
			name: "unknown landmark end",
			mutate: func(doc *Document) {
				doc.Attachments[0].Reference.End = "middle"
			},
			want: "unknown attachment end",
		},
		{
			name: "endpoint without inward offset",
			mutate: func(doc *Document) {
				doc.Attachments[0].Reference = AttachmentReference{End: AttachmentPortA}
				doc.Attachments[0].Offset = 0
			},
			want: "invalid attachment location",
		},
		{
			name: "endpoint with bend directions",
			mutate: func(doc *Document) {
				doc.Attachments[0].Reference = AttachmentReference{
					End:      AttachmentPortA,
					Incoming: DirectionEast,
				}
			},
			want: "endpoint has bend directions",
		},
		{
			name: "invalid bend direction",
			mutate: func(doc *Document) {
				doc.Attachments[0].Reference = AttachmentReference{
					End:      AttachmentPortA,
					Bend:     1,
					Incoming: "diagonal",
					Outgoing: DirectionSouth,
				}
			},
			want: `unknown direction "diagonal"`,
		},
		{
			name: "parallel bend directions",
			mutate: func(doc *Document) {
				doc.Attachments[0].Reference = AttachmentReference{
					End:      AttachmentPortA,
					Bend:     1,
					Incoming: DirectionNorth,
					Outgoing: DirectionSouth,
				}
			},
			want: "invalid bend turn",
		},
		{
			name: "incident node",
			mutate: func(doc *Document) {
				doc.Attachments[0].Node = 0
			},
			want: "cannot attach",
		},
		{
			name: "anchor outside node",
			mutate: func(doc *Document) {
				doc.Attachments[0].Anchor.X = math.MaxUint32
			},
			want: "anchor outside node",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			doc := validAttachmentDocument(t)
			test.mutate(&doc)
			_, err := doc.Convert()
			require.ErrorContains(t, err, test.want)
		})
	}
}

func validAttachmentDocument(t testing.TB) Document {
	t.Helper()

	geo, err := layout.New()
	require.NoError(t, err)
	source, err := geo.NewNodeAt("source", layout.NewPoint(10, 10))
	require.NoError(t, err)
	destination, err := geo.NewNodeAt("destination", layout.NewPoint(50, 10))
	require.NoError(t, err)
	node, err := geo.NewNodeAt("tag", layout.NewPoint(25, 20))
	require.NoError(t, err)
	edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, destination)
	require.NoError(t, geo.Build())
	point := documentEdgeMiddle(geo.Edges[edge].Points)
	require.NoError(t, geo.PlaceNode(node, layout.NewPoint(point.X-1, point.Y-1)))
	require.NoError(t, geo.AttachNode(node, edge, point))
	return New(geo)
}

func documentEdgeMiddle(points []layout.Point) layout.Point {
	return documentEdgePoint(points, documentPathLength(points)/2)
}

func documentPathLength(points []layout.Point) uint64 {
	var length uint64
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		length += uint64(max(a.X, b.X)-min(a.X, b.X)) +
			uint64(max(a.Y, b.Y)-min(a.Y, b.Y))
	}
	return length
}

func documentEdgePoint(points []layout.Point, distance uint64) layout.Point {
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		segment := uint64(max(a.X, b.X)-min(a.X, b.X)) +
			uint64(max(a.Y, b.Y)-min(a.Y, b.Y))
		if distance > segment {
			distance -= segment
			continue
		}
		switch {
		case a.X == b.X && b.Y >= a.Y:
			return layout.NewPoint(a.X, a.Y+uint32(distance))
		case a.X == b.X:
			return layout.NewPoint(a.X, a.Y-uint32(distance))
		case b.X >= a.X:
			return layout.NewPoint(a.X+uint32(distance), a.Y)
		default:
			return layout.NewPoint(a.X-uint32(distance), a.Y)
		}
	}
	return points[len(points)-1]
}

func TestUnmarshalRejectsInvalidJSONShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "unknown field",
			data: `{"version":3,"id":"123e4567-e89b-42d3-a456-426614174000","nodes":[],"ports":[],"edges":[],"options":{},"unknown":true}`,
			want: "unknown field",
		},
		{
			name: "trailing value",
			data: `{"version":3,"id":"123e4567-e89b-42d3-a456-426614174000","nodes":[],"ports":[],"edges":[],"options":{}} {}`,
			want: "trailing JSON value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := Unmarshal([]byte(test.data))
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestUnmarshalIntoReusesCapacityAndClearsOmittedFields(t *testing.T) {
	t.Parallel()

	first := validDocument()
	first.Nodes[0].Style.Border = BorderRounded
	first.Edges[0].Bends = []PinnedBend{{
		Point:    Point{X: 10, Y: 10},
		Incoming: DirectionEast,
		Outgoing: DirectionSouth,
	}}
	data, err := Marshal(first)
	require.NoError(t, err)
	var dst Document
	require.NoError(t, UnmarshalInto(data, &dst))
	nodes := &dst.Nodes[0]
	nodeCapacity := cap(dst.Nodes)
	portCapacity := cap(nodes.Ports)

	second := validDocument()
	second.ID = first.ID
	data, err = Marshal(second)
	require.NoError(t, err)
	require.NoError(t, UnmarshalInto(data, &dst))
	require.Equal(t, nodeCapacity, cap(dst.Nodes))
	require.Equal(t, portCapacity, cap(dst.Nodes[0].Ports))
	require.Equal(t, NodeStyle{}, dst.Nodes[0].Style)
	require.Empty(t, dst.Edges[0].Bends)
}

func TestDocumentValidation(t *testing.T) {
	t.Parallel()

	const unknownStyle = "future"
	tests := []struct {
		name   string
		mutate func(*Document)
		want   string
	}{
		{
			name: "unknown side",
			mutate: func(doc *Document) {
				doc.Ports[0].Side = Side("diagonal")
			},
			want: "unknown side",
		},
		{
			name: "unknown border style",
			mutate: func(doc *Document) {
				doc.Nodes[0].Style.Border = unknownStyle
			},
			want: "unknown border",
		},
		{
			name: "unknown node stroke",
			mutate: func(doc *Document) {
				doc.Nodes[0].Style.Stroke = unknownStyle
			},
			want: "unknown stroke",
		},
		{
			name: "unknown node padding",
			mutate: func(doc *Document) {
				doc.Nodes[0].Style.Padding = unknownStyle
			},
			want: "unknown padding",
		},
		{
			name: "unknown arrow style",
			mutate: func(doc *Document) {
				doc.Edges[0].Style.PortBArrow = unknownStyle
			},
			want: "unknown arrow",
		},
		{
			name: "unknown edge stroke",
			mutate: func(doc *Document) {
				doc.Edges[0].Style.Stroke = unknownStyle
			},
			want: "unknown stroke",
		},
		{
			name: "unknown node port",
			mutate: func(doc *Document) {
				doc.Nodes[0].Ports[0] = 9
			},
			want: "unknown port",
		},
		{
			name: "duplicate port owner",
			mutate: func(doc *Document) {
				doc.Nodes[1].Ports[0] = 0
			},
			want: "multiple nodes",
		},
		{
			name: "unowned port",
			mutate: func(doc *Document) {
				doc.Nodes[1].Ports = nil
			},
			want: "no owning node",
		},
		{
			name: "unknown edge port",
			mutate: func(doc *Document) {
				doc.Edges[0].PortB = 9
			},
			want: "unknown port",
		},
		{
			name: "self edge",
			mutate: func(doc *Document) {
				doc.Edges[0].PortB = doc.Edges[0].PortA
			},
			want: "to itself",
		},
		{
			name: "duplicate edge",
			mutate: func(doc *Document) {
				doc.Edges = append(doc.Edges, Edge{PortA: 1, PortB: 0})
			},
			want: "duplicates ports",
		},
		{
			name: "incomplete layers",
			mutate: func(doc *Document) {
				doc.Layers = doc.Layers[:2]
			},
			want: "contains 2 layers, want 3",
		},
		{
			name: "unknown layer kind",
			mutate: func(doc *Document) {
				doc.Layers[0].Kind = "port"
			},
			want: "unknown kind",
		},
		{
			name: "unknown layer ID",
			mutate: func(doc *Document) {
				doc.Layers[0].ID = 9
			},
			want: "references unknown node",
		},
		{
			name: "duplicate layer",
			mutate: func(doc *Document) {
				doc.Layers[1] = doc.Layers[0]
			},
			want: "duplicates node",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			doc := validDocument()
			test.mutate(&doc)
			_, err := doc.Convert()
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestDocumentRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	doc := validDocument()
	doc.Version++
	_, err := doc.Convert()
	require.ErrorIs(t, err, ErrUnsupportedVersion)

	_, err = Unmarshal([]byte(`{"version":5,"future_field":true}`))
	require.ErrorIs(t, err, ErrUnsupportedVersion)
}

func validDocument() Document {
	return Document{
		Version: CurrentVersion,
		ID:      uuid.New(),
		Nodes: []Node{
			{
				Label:  "source",
				Origin: Point{X: 2, Y: 3},
				Ports:  []uint32{0},
			},
			{
				Label:  "sink",
				Origin: Point{X: 20, Y: 3},
				Ports:  []uint32{1},
			},
		},
		Ports: []Port{
			{Side: SideRight, Offset: 0.5},
			{Side: SideLeft, Offset: 0.5},
		},
		Edges: []Edge{{PortA: 0, PortB: 1}},
		Layers: []Layer{
			{Kind: LayerNode, ID: 0},
			{Kind: LayerNode, ID: 1},
			{Kind: LayerEdge, ID: 0},
		},
		Options: Options{
			Router: Router{
				Costs: Costs{
					Step:         10,
					SharedStep:   2,
					Bend:         5,
					Crossing:     15,
					EndpointStep: 40,
				},
				ReroutePasses: 1,
			},
		},
	}
}

func generatedDocument(minNodes int) *rapid.Generator[Document] {
	return rapid.Custom(func(t *rapid.T) Document {
		nodeCount := rapid.IntRange(minNodes, 6).Draw(t, "node count")
		label := rapid.Custom(func(t *rapid.T) string {
			lineCount := rapid.IntRange(1, 3).Draw(t, "line count")
			lines := rapid.SliceOfN(
				rapid.StringMatching(`[a-z]{0,8}`),
				lineCount,
				lineCount,
			).Draw(t, "lines")
			return strings.Join(lines, "\n")
		})
		labels := rapid.SliceOfN(
			label,
			nodeCount,
			nodeCount,
		).Draw(t, "labels")
		portCounts := rapid.SliceOfN(
			rapid.IntRange(1, 6),
			nodeCount,
			nodeCount,
		).Draw(t, "port counts")
		origins := rapid.SliceOfN(
			rapid.Uint32Range(0, 100),
			nodeCount*2,
			nodeCount*2,
		).Draw(t, "origins")

		portCount := 0
		for _, count := range portCounts {
			portCount += count
		}
		sides := rapid.SliceOfN(
			rapid.SampledFrom([]Side{SideTop, SideRight, SideBottom, SideLeft}),
			portCount,
			portCount,
		).Draw(t, "port sides")
		offsets := rapid.SliceOfN(
			rapid.Float32Range(0, 1),
			portCount,
			portCount,
		).Draw(t, "port offsets")

		doc := Document{
			Version: CurrentVersion,
			ID:      uuid.New(),
			Nodes:   make([]Node, 0, nodeCount),
			Ports:   make([]Port, 0, portCount),
			Edges:   make([]Edge, 0),
			Options: Options{
				Router: Router{
					Costs: Costs{
						Step:       rapid.Uint32Range(0, 100).Draw(t, "step cost"),
						SharedStep: rapid.Uint32Range(0, 100).Draw(t, "shared step cost"),
						Bend:       rapid.Uint32Range(0, 100).Draw(t, "bend cost"),
						Crossing:   rapid.Uint32Range(0, 100).Draw(t, "crossing cost"),
						EndpointStep: rapid.Uint32Range(0, 100).
							Draw(t, "endpoint step cost"),
					},
					ReroutePasses: rapid.Uint8Range(0, 4).Draw(t, "reroute passes"),
				},
			},
		}
		nextPort := 0
		for nodeID := range nodeCount {
			padding := rapid.SampledFrom([]PaddingLevel{
				"",
				PaddingNone,
				PaddingExtra,
			}).Draw(t, "node padding")
			node := Node{
				Label: labels[nodeID],
				Origin: Point{
					X: origins[nodeID*2],
					Y: origins[nodeID*2+1],
				},
				Ports: make([]uint32, 0, portCounts[nodeID]),
				Style: NodeStyle{
					Border: rapid.SampledFrom([]BorderStyle{
						"",
						BorderRounded,
						BorderDouble,
						BorderNone,
					}).Draw(t, "node border"),
					Stroke: rapid.SampledFrom([]StrokeStyle{
						"",
						StrokeDashed,
					}).Draw(t, "node stroke"),
					Padding: padding,
				},
			}
			if rapid.Bool().Draw(t, "explicit size") {
				cells := documentPaddingCells(padding)
				node.Size = Size{
					Width: rapid.Uint32Range(
						uint32(cells.Left)+uint32(cells.Right)+2,
						uint32(cells.Left)+uint32(cells.Right)+24,
					).Draw(t, "node width"),
					Height: rapid.Uint32Range(
						uint32(cells.Top)+uint32(cells.Bottom)+2,
						uint32(cells.Top)+uint32(cells.Bottom)+12,
					).Draw(t, "node height"),
				}
			}
			for range portCounts[nodeID] {
				node.Ports = append(node.Ports, uint32(nextPort))
				doc.Ports = append(doc.Ports, Port{
					Side:   sides[nextPort],
					Offset: offsets[nextPort],
				})
				nextPort++
			}
			doc.Nodes = append(doc.Nodes, node)
		}
		if portCount < 2 {
			for nodeID := range doc.Nodes {
				doc.Layers = append(doc.Layers, Layer{
					Kind: LayerNode,
					ID:   uint32(nodeID),
				})
			}
			return doc
		}

		edge := rapid.Custom(func(t *rapid.T) Edge {
			portA := rapid.Uint32Range(0, uint32(portCount-1)).Draw(t, "port a")
			delta := rapid.Uint32Range(1, uint32(portCount-1)).Draw(t, "port delta")
			portB := (portA + delta) % uint32(portCount)
			return Edge{
				PortA: min(portA, portB),
				PortB: max(portA, portB),
			}
		})
		maxEdges := min(portCount*(portCount-1)/2, 12)
		doc.Edges = append(
			doc.Edges,
			rapid.SliceOfNDistinct(edge, 0, maxEdges, func(edge Edge) uint64 {
				return uint64(edge.PortA)<<32 | uint64(edge.PortB)
			}).
				Draw(t, "edges")...,
		)
		for i := range doc.Edges {
			doc.Edges[i].Style = EdgeStyle{
				PortAArrow: rapid.SampledFrom([]ArrowStyle{
					"",
					ArrowOpen,
					ArrowFilled,
					ArrowCircle,
					ArrowCircleBullet,
				}).Draw(t, "port A arrow"),
				PortBArrow: rapid.SampledFrom([]ArrowStyle{
					"",
					ArrowOpen,
					ArrowFilled,
					ArrowCircle,
					ArrowCircleBullet,
				}).Draw(t, "port B arrow"),
				Stroke: rapid.SampledFrom([]StrokeStyle{
					"",
					StrokeDashed,
				}).Draw(t, "edge stroke"),
			}
		}
		layers := make([]Layer, 0, len(doc.Nodes)+len(doc.Edges))
		for nodeID := range doc.Nodes {
			layers = append(layers, Layer{Kind: LayerNode, ID: uint32(nodeID)})
		}
		for edgeID := range doc.Edges {
			layers = append(layers, Layer{Kind: LayerEdge, ID: uint32(edgeID)})
		}
		doc.Layers = rapid.Permutation(layers).Draw(t, "layers")
		return doc
	})
}

func documentPaddingCells(padding PaddingLevel) layout.Padding {
	switch padding {
	case PaddingNone:
		return layout.PaddingNone.Cells()
	case PaddingExtra:
		return layout.PaddingExtra.Cells()
	default:
		return layout.PaddingDefault.Cells()
	}
}
