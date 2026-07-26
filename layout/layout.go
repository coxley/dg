// Package layout [...]
package layout

import "github.com/coxley/dg/ir"

type Point struct {
	X, Y int32
}

type Node struct {
	Point Point
	W, H  int32
}

type Port struct {
	Point Point
}

type Edge struct {
	Points []Point
}

type Layout struct {
	g     *ir.Graph
	Nodes []Node
	Ports []Port
	Edges []Edge
}
