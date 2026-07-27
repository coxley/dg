package layout

import (
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestRasterizeEdgeIntoMatchesCommittedEdge(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		sourceX := rapid.Uint32Range(1, 10).Draw(t, "source x")
		commonX := rapid.Uint32Range(30, 60).Draw(t, "common x")
		joinX := rapid.Uint32Range(sourceX+1, commonX-1).Draw(t, "join x")
		sourceY := rapid.Uint32Range(12, 30).Draw(t, "source y")
		shared := rapid.Bool().Draw(t, "shared endpoint")

		overlayPortB := uint32(3)
		if shared {
			overlayPortB = 1
		}
		geo := Layout{
			graph: ir.Graph{
				Edges: []ir.Edge{
					{PortA: 0, PortB: 1},
					{PortA: 2, PortB: overlayPortB},
				},
			},
			Ports: []Port{
				{Anchor: NewPoint(sourceX, 10)},
				{Anchor: NewPoint(commonX, 10)},
				{Anchor: NewPoint(sourceX, sourceY)},
				{Anchor: NewPoint(commonX, 10)},
			},
			Edges: []Edge{
				{Points: []Point{
					NewPoint(sourceX, 10),
					NewPoint(commonX, 10),
				}},
				{Points: []Point{
					NewPoint(sourceX, sourceY),
					NewPoint(joinX, sourceY),
					NewPoint(joinX, 10),
					NewPoint(commonX, 10),
				}},
			},
		}

		full, err := RasterizeOwnedInto(nil, nil, &geo)
		require.NoError(t, err)
		base, err := RasterizeWithoutEdgeOwnedInto(nil, nil, &geo, 1)
		require.NoError(t, err)
		overlay, err := RasterizeEdgeInto(
			nil,
			&base,
			&geo,
			RasterEdge{
				Points: geo.Edges[1].Points,
				PortA:  2,
				PortB:  overlayPortB,
			},
		)
		require.NoError(t, err)

		for _, cell := range overlay {
			committed, ok := full.At(cell.Point)
			require.True(t, ok)
			require.Equal(t, committed, cell.Connections, "point %+v", cell.Point)
		}
	})
}

func TestRasterizeEdgeIntoDoesNotJoinFloatingEndpoint(t *testing.T) {
	t.Parallel()

	base, err := NewGrid(Rect{Size: Size{Width: 3, Height: 4}})
	require.NoError(t, err)
	require.NoError(t, base.AddPath([]Point{
		NewPoint(0, 1),
		NewPoint(2, 1),
	}))

	cells, err := RasterizeEdgeInto(
		nil,
		&base,
		&Layout{},
		RasterEdge{
			Points: []Point{
				NewPoint(1, 3),
				NewPoint(1, 1),
			},
			PortA: 0,
			PortB: NoPortID,
		},
	)
	require.NoError(t, err)
	require.Equal(t, RasterCell{
		Point:       NewPoint(1, 1),
		Connections: South,
	}, cells[0])
}
