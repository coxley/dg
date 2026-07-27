package main

import (
	"fmt"
	"log"
	"time"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
	"github.com/coxley/dg/render"
)

func main() {
	geo, err := layout.New()
	if err != nil {
		log.Fatal(err)
	}

	sink := must2(geo.NewNodeAt("sinks", layout.NewPoint(6, 6)))
	foo := must2(geo.NewNodeAt("foo", layout.NewPoint(4, 0)))
	geo.ConnectNodes(
		foo,
		ir.Bottom,
		ir.Top,
		sink,
	)
	bar := must2(geo.NewNodeAt("bar", layout.NewPoint(15, 0)))
	geo.ConnectNodes(
		bar,
		ir.Bottom,
		ir.Top,
		sink,
	)
	must(geo.Build())
	fmt.Print(must2(render.Unicode(geo)))

	time.Sleep(time.Millisecond * 500)
	must(geo.SetNodeLabel(foo, "fooooo"))
	must(geo.Build())
	fmt.Print(must2(render.Unicode(geo)))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func must2[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
