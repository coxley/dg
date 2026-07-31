package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/history"
	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/internal/tui"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	canvasstore "github.com/coxley/dg/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	store, err := settings.DefaultStore()
	if err != nil {
		return fmt.Errorf("configure settings: %w", err)
	}
	snapshot, err := store.Load()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	preferred := snapshot.SaveDirectory
	if preferred == "" {
		preferred = "."
	}
	canvases, err := canvasstore.New(preferred)
	if err != nil {
		return fmt.Errorf("configure canvas store: %w", err)
	}
	geo, doc, entry, err := initialCanvas(args, snapshot, canvases)
	if err != nil {
		return err
	}
	undo, err := history.New(geo, history.WithStore(canvases.History()))
	if err != nil {
		return fmt.Errorf("configure history: %w", err)
	}
	if _, err := undo.Restore(doc); err != nil {
		log.Printf("restore undo history: %v", err)
	}
	defer func() {
		if undo.Dirty() {
			if err := undo.Flush(); err != nil {
				log.Printf("flush undo history: %v", err)
			}
		}
	}()
	if err := tui.Run(
		geo,
		tui.WithDocument(doc),
		tui.WithHistory(undo),
		tui.WithCanvasStore(canvases, entry),
		tui.WithSettings(snapshot, store),
	); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}

func initialCanvas(
	args []string,
	snapshot settings.Snapshot,
	canvases *canvasstore.Store,
) (*layout.Layout, document.Document, canvasstore.Entry, error) {
	switch len(args) {
	case 0:
		entries, err := canvases.List()
		if err != nil {
			return nil, document.Document{}, canvasstore.Entry{}, fmt.Errorf("list canvases: %w", err)
		}
		if len(entries) != 0 {
			entry := slices.MaxFunc(entries, func(a, b canvasstore.Entry) int {
				return a.Modified.Compare(b.Modified)
			})
			doc, err := canvases.Load(entry)
			if err != nil {
				return nil, document.Document{}, canvasstore.Entry{}, fmt.Errorf("load recent canvas: %w", err)
			}
			geo, err := doc.Convert()
			if err != nil {
				return nil, document.Document{}, canvasstore.Entry{}, fmt.Errorf("convert recent canvas: %w", err)
			}
			return geo, doc, entry, nil
		}
		geo, err := exampleLayoutWithSettings(snapshot)
		if err != nil {
			return nil, document.Document{}, canvasstore.Entry{}, err
		}
		doc := document.New(geo)
		entry, err := canvases.CreateDraft(doc)
		if err != nil {
			return nil, document.Document{}, canvasstore.Entry{}, fmt.Errorf("create initial draft: %w", err)
		}
		return geo, doc, entry, nil
	case 1:
		entry, doc, err := canvases.Import(args[0])
		if err != nil {
			return nil, document.Document{}, canvasstore.Entry{}, fmt.Errorf("import diagram %q: %w", args[0], err)
		}
		geo, err := doc.Convert()
		if err != nil {
			return nil, document.Document{}, canvasstore.Entry{}, fmt.Errorf("load diagram %q: %w", args[0], err)
		}
		return geo, doc, entry, nil
	default:
		return nil, document.Document{}, canvasstore.Entry{}, errors.New("usage: dg [path]")
	}
}

func exampleLayout() (*layout.Layout, error) {
	return exampleLayoutWithSettings(settings.Snapshot{})
}

func exampleLayoutWithSettings(snapshot settings.Snapshot) (*layout.Layout, error) {
	var options []layout.Option
	if snapshot.ApplyToFuture {
		options = append(options, layout.WithRouter(snapshot.Router))
	}
	geo, err := layout.New(options...)
	if err != nil {
		return nil, err
	}
	sink, err := geo.NewNodeAt("sinks", layout.NewPoint(7, 6))
	if err != nil {
		return nil, err
	}
	foo, err := geo.NewNodeAt("foo", layout.NewPoint(4, 0))
	if err != nil {
		return nil, err
	}
	geo.ConnectNodes(foo, ir.Bottom, ir.Top, sink)
	bar, err := geo.NewNodeAt("bar", layout.NewPoint(12, 0))
	if err != nil {
		return nil, err
	}
	geo.ConnectNodes(bar, ir.Bottom, ir.Top, sink)
	return geo, nil
}
