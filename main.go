package main

import (
	"fmt"
	"log"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
)

func main() {
	var geo layout.Layout
	geo.Padding = layout.Padding{Left: 1, Right: 1}

	sink := geo.NewNodeAt("sinks", layout.NewPoint(6, 6))
	geo.ConnectNodes(
		geo.NewNodeAt("foo", layout.NewPoint(4, 0)),
		ir.Bottom,
		ir.Top,
		sink,
	)
	geo.ConnectNodes(
		geo.NewNodeAt("bar", layout.NewPoint(10, 0)),
		ir.Bottom,
		ir.Top,
		sink,
	)
	if err := geo.Build(); err != nil {
		log.Fatal(err)
	}
	drawing, err := render.Unicode(&geo)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(drawing)
}
