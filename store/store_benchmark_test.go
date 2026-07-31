package store

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
)

func BenchmarkStoreSwitch(b *testing.B) {
	store := benchmarkStore(b)
	entry, err := store.Create("", "Large", benchmarkDocument(b))
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Warm", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := store.Load(entry); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("WarmInto", func(b *testing.B) {
		var doc document.Document
		if err := store.LoadInto(entry, &doc); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if err := store.LoadInto(entry, &doc); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Cold", func(b *testing.B) {
		path := store.namedPath(entry.Section, entry.Name)
		key := warmKey(path, entry.Revision)
		b.ReportAllocs()
		for b.Loop() {
			store.mu.Lock()
			store.warm.remove(key)
			store.mu.Unlock()
			if _, err := store.Load(entry); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkStoreList(b *testing.B) {
	store := benchmarkStore(b)
	doc := testDocument(b, "node")
	for i := range 200 {
		section := ""
		if i >= 100 {
			section = "Section"
		}
		if _, err := store.Create(section, fmt.Sprintf("Canvas %03d", i), doc); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.List(); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkStore(b *testing.B) *Store {
	b.Helper()
	root := b.TempDir()
	store, err := New(
		filepath.Join(root, "canvases"),
		WithStateDir(filepath.Join(root, "state")),
		WithCacheDir(filepath.Join(root, "cache")),
	)
	if err != nil {
		b.Fatal(err)
	}
	return store
}

func benchmarkDocument(b *testing.B) document.Document {
	b.Helper()
	geo, err := layout.New()
	if err != nil {
		b.Fatal(err)
	}
	for cluster := range uint32(200) {
		x := 20 * (cluster % 20)
		y := 12 * (cluster / 20)
		sink, err := geo.NewNodeAt("sinks", layout.NewPoint(x+7, y+6))
		if err != nil {
			b.Fatal(err)
		}
		foo, err := geo.NewNodeAt("foo", layout.NewPoint(x+4, y))
		if err != nil {
			b.Fatal(err)
		}
		bar, err := geo.NewNodeAt("bar", layout.NewPoint(x+12, y))
		if err != nil {
			b.Fatal(err)
		}
		geo.ConnectNodes(foo, ir.Bottom, ir.Top, sink)
		geo.ConnectNodes(bar, ir.Bottom, ir.Top, sink)
	}
	return document.New(geo)
}
