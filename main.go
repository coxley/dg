package main

import (
	"fmt"
	"log"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
)

func main() {
	geo, err := layout.New()
	if err != nil {
		log.Fatal(err)
	}

	sink, err := geo.NewNodeAt("sinks", layout.NewPoint(6, 6))
	if err != nil {
		log.Fatal(err)
	}
	foo, err := geo.NewNodeAt("foo", layout.NewPoint(4, 0))
	if err != nil {
		log.Fatal(err)
	}
	geo.ConnectNodes(
		foo,
		ir.Bottom,
		ir.Top,
		sink,
	)
	bar, err := geo.NewNodeAt("bar", layout.NewPoint(10, 0))
	if err != nil {
		log.Fatal(err)
	}
	geo.ConnectNodes(
		bar,
		ir.Bottom,
		ir.Top,
		sink,
	)
	if err := geo.Build(); err != nil {
		log.Fatal(err)
	}
	drawing, err := render.Unicode(geo)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(drawing)
}
