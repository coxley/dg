package document

import (
	"math"
	"strings"
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestRoundTripProperties(t *testing.T) {
	t.Parallel()

	documents := generatedDocument(0)
	rapid.Check(t, func(t *rapid.T) {
		want := documents.Draw(t, "document")
		geo, err := want.Layout()
		require.NoError(t, err)
		require.Equal(t, want, FromLayout(geo))

		first, err := Marshal(geo)
		require.NoError(t, err)
		got, err := Unmarshal(first)
		require.NoError(t, err)
		require.Equal(t, want, got)

		loaded, err := got.Layout()
		require.NoError(t, err)
		second, err := Marshal(loaded)
		require.NoError(t, err)
		require.Equal(t, first, second)
	})
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

		_, err := doc.Layout()
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

	doc := FromLayout(geo)
	require.Equal(t, []string{"source", "sink"}, []string{doc.Nodes[0].Label, doc.Nodes[1].Label})
	require.Len(t, doc.Ports, 24)
	require.Len(t, doc.Edges, 1)
	require.Less(t, doc.Edges[0].PortA, uint32(len(doc.Ports)))
	require.Less(t, doc.Edges[0].PortB, uint32(len(doc.Ports)))

	data, err := Marshal(geo)
	require.NoError(t, err)
	require.Contains(t, string(data), `"version": 1`)
	require.NotContains(t, string(data), "points")
}

func TestUnmarshalVersionOne(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"version": 1,
		"nodes": [],
		"ports": [],
		"edges": [],
		"options": {
			"padding": {"horizontal": 1, "vertical": 0},
			"router": {
				"costs": {"step": 10, "shared_step": 2, "bend": 5, "crossing": 15},
				"reroute_passes": 1
			}
		}
	}`)
	doc, err := Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, CurrentVersion, doc.Version)
	geo, err := doc.Layout()
	require.NoError(t, err)
	require.Equal(t, layout.Padding{Left: 1, Right: 1}, geo.Padding())
	require.Equal(t, layout.DefaultRouter(), geo.Router())
}

func TestRoundTripMultilineLabelAndExplicitSize(t *testing.T) {
	t.Parallel()

	doc := validDocument()
	doc.Nodes[0].Label = "one\ntwo three"
	doc.Nodes[0].Size = Size{Width: 9, Height: 4}

	geo, err := doc.Layout()
	require.NoError(t, err)
	require.Equal(t, doc, FromLayout(geo))

	data, err := Marshal(geo)
	require.NoError(t, err)
	require.Contains(t, string(data), `"size"`)
	decoded, err := Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, doc, decoded)
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
			data: `{"version":1,"nodes":[],"ports":[],"edges":[],"options":{},"unknown":true}`,
			want: "unknown field",
		},
		{
			name: "trailing value",
			data: `{"version":1,"nodes":[],"ports":[],"edges":[],"options":{}} {}`,
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

func TestDocumentValidation(t *testing.T) {
	t.Parallel()

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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			doc := validDocument()
			test.mutate(&doc)
			_, err := doc.Layout()
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestDocumentRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	doc := validDocument()
	doc.Version++
	_, err := doc.Layout()
	require.ErrorIs(t, err, ErrUnsupportedVersion)

	_, err = Unmarshal([]byte(`{"version":2,"future_field":true}`))
	require.ErrorIs(t, err, ErrUnsupportedVersion)
}

func validDocument() Document {
	return Document{
		Version: CurrentVersion,
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
		Options: Options{
			Padding: Padding{Horizontal: 1},
			Router: Router{
				Costs: Costs{
					Step:       10,
					SharedStep: 2,
					Bend:       5,
					Crossing:   15,
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

		horizontalPadding := rapid.Uint8Range(0, 4).Draw(t, "horizontal padding")
		verticalPadding := rapid.Uint8Range(0, 4).Draw(t, "vertical padding")
		doc := Document{
			Version: CurrentVersion,
			Nodes:   make([]Node, 0, nodeCount),
			Ports:   make([]Port, 0, portCount),
			Edges:   make([]Edge, 0),
			Options: Options{
				Padding: Padding{
					Horizontal: horizontalPadding,
					Vertical:   verticalPadding,
				},
				Router: Router{
					Costs: Costs{
						Step:       rapid.Uint32Range(0, 100).Draw(t, "step cost"),
						SharedStep: rapid.Uint32Range(0, 100).Draw(t, "shared step cost"),
						Bend:       rapid.Uint32Range(0, 100).Draw(t, "bend cost"),
						Crossing:   rapid.Uint32Range(0, 100).Draw(t, "crossing cost"),
					},
					ReroutePasses: rapid.Uint8Range(0, 4).Draw(t, "reroute passes"),
				},
			},
		}
		nextPort := 0
		for nodeID := range nodeCount {
			node := Node{
				Label: labels[nodeID],
				Origin: Point{
					X: origins[nodeID*2],
					Y: origins[nodeID*2+1],
				},
				Ports: make([]uint32, 0, portCounts[nodeID]),
			}
			if rapid.Bool().Draw(t, "explicit size") {
				node.Size = Size{
					Width: rapid.Uint32Range(
						2*uint32(horizontalPadding)+2,
						2*uint32(horizontalPadding)+24,
					).Draw(t, "node width"),
					Height: rapid.Uint32Range(
						2*uint32(verticalPadding)+2,
						2*uint32(verticalPadding)+12,
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
			rapid.SliceOfNDistinct(edge, 0, maxEdges, rapid.ID[Edge]).
				Draw(t, "edges")...,
		)
		return doc
	})
}
