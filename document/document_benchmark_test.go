package document

import (
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

var benchmarkConvertedLayout *layout.Layout

func BenchmarkDocumentConversion(b *testing.B) {
	first := benchmarkDocument(b, 0)
	second := benchmarkDocument(b, 1)

	b.Run("Convert", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			geo, err := first.Convert()
			if err != nil {
				b.Fatal(err)
			}
			if err := geo.Build(); err != nil {
				b.Fatal(err)
			}
			benchmarkConvertedLayout = geo
		}
	})

	b.Run("ConvertIntoSteady", func(b *testing.B) {
		geo, err := layout.New()
		require.NoError(b, err)
		require.NoError(b, first.ConvertInto(geo))
		require.NoError(b, geo.Build())
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if err := first.ConvertInto(geo); err != nil {
				b.Fatal(err)
			}
			if err := geo.Build(); err != nil {
				b.Fatal(err)
			}
		}
		benchmarkConvertedLayout = geo
	})

	b.Run("ConvertIntoAlternating", func(b *testing.B) {
		geo, err := layout.New()
		require.NoError(b, err)
		documents := [...]Document{first, second}
		for _, doc := range documents {
			require.NoError(b, doc.ConvertInto(geo))
			require.NoError(b, geo.Build())
		}
		iteration := 0
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if err := documents[iteration&1].ConvertInto(geo); err != nil {
				b.Fatal(err)
			}
			if err := geo.Build(); err != nil {
				b.Fatal(err)
			}
			iteration++
		}
		benchmarkConvertedLayout = geo
	})
}

func benchmarkDocument(b *testing.B, offset uint32) Document {
	b.Helper()
	geo, err := layout.New()
	require.NoError(b, err)
	for cluster := range uint32(200) {
		x := offset + cluster%20*24
		y := cluster / 20 * 12
		sink, err := geo.NewNodeAt("sinks", layout.NewPoint(x+8, y+7))
		require.NoError(b, err)
		foo, err := geo.NewNodeAt("foo", layout.NewPoint(x+2, y))
		require.NoError(b, err)
		bar, err := geo.NewNodeAt("bar", layout.NewPoint(x+14, y))
		require.NoError(b, err)
		geo.ConnectNodes(foo, ir.Bottom, ir.Top, sink)
		geo.ConnectNodes(bar, ir.Bottom, ir.Top, sink)
	}
	require.NoError(b, geo.Build())
	return New(geo)
}
