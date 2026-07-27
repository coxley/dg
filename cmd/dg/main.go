package main

import (
	"log"

	"github.com/coxley/dg/internal/tui"
	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
)

func main() {
	geo, err := exampleLayout()
	if err != nil {
		log.Fatal(err)
	}
	if err := tui.Run(geo); err != nil {
		log.Fatal(err)
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
