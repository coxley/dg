package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/history"
	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/internal/tui"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
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
	geo, doc, path, err := initialLayout(args, snapshot)
	if err != nil {
		return err
	}
	undo, err := history.New(geo)
	if err != nil {
		return fmt.Errorf("configure history: %w", err)
	}
	if path != "" {
		if _, err := undo.Restore(path); err != nil {
			log.Printf("restore undo history: %v", err)
		}
	}
	defer func() {
		if err := undo.Flush(); err != nil {
			log.Printf("flush undo history: %v", err)
		}
	}()
	if err := tui.Run(
		geo,
		path,
		tui.WithDocument(doc),
		tui.WithHistory(undo),
		tui.WithSettings(snapshot, store),
	); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}

func initialLayout(
	args []string,
	snapshot settings.Snapshot,
) (*layout.Layout, document.Document, string, error) {
	switch len(args) {
	case 0:
		geo, err := exampleLayoutWithSettings(snapshot)
		if err != nil {
			return nil, document.Document{}, "", err
		}
		return geo, document.New(geo), "", nil
	case 1:
		data, err := os.ReadFile(args[0]) //nolint:gosec // The CLI argument intentionally selects an arbitrary diagram.
		if err != nil {
			return nil, document.Document{}, "", fmt.Errorf("read diagram %q: %w", args[0], err)
		}
		doc, err := document.Unmarshal(data)
		if err != nil {
			return nil, document.Document{}, "", fmt.Errorf("decode diagram %q: %w", args[0], err)
		}
		geo, err := doc.Convert()
		if err != nil {
			return nil, document.Document{}, "", fmt.Errorf("load diagram %q: %w", args[0], err)
		}
		return geo, doc, args[0], nil
	default:
		return nil, document.Document{}, "", errors.New("usage: dg [path]")
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
