package main

import (
	"fmt"

	"github.com/coxley/dg/ir"
)

func main() {
	var g ir.Graph

	foo := g.NewNode("foo")
	bar := g.NewNode("bar")
	baz := g.NewNode("baz")
	g.ConnectNodes(foo, ir.Bottom, ir.Top, bar)
	g.ConnectNodes(foo, ir.Bottom, ir.Top, baz)
	g.ConnectNodes(bar, ir.RightSide, ir.LeftSide, baz)

	fmt.Println(g.String())
}
