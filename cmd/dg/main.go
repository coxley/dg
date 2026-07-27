package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/coxley/dg/document"
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
	geo, path, err := initialLayout(args)
	if err != nil {
		return err
	}
	if err := tui.Run(geo, path); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}

func initialLayout(args []string) (*layout.Layout, string, error) {
	switch len(args) {
	case 0:
		geo, err := exampleLayout()
		return geo, "", err
	case 1:
		data, err := os.ReadFile(args[0]) //nolint:gosec // The CLI argument intentionally selects an arbitrary diagram.
		if err != nil {
			return nil, "", fmt.Errorf("read diagram %q: %w", args[0], err)
		}
		doc, err := document.Unmarshal(data)
		if err != nil {
			return nil, "", fmt.Errorf("decode diagram %q: %w", args[0], err)
		}
		geo, err := doc.Layout()
		if err != nil {
			return nil, "", fmt.Errorf("load diagram %q: %w", args[0], err)
		}
		return geo, args[0], nil
	default:
		return nil, "", errors.New("usage: dg [path]")
	}
}

func exampleLayout() (*layout.Layout, error) {
	geo, err := layout.New()
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
