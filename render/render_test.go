package render

import (
	"testing"

	"github.com/coxley/dg/ir"
	"github.com/coxley/dg/layout"
)

func TestUnicode(t *testing.T) {
	t.Parallel()

	var geo layout.Layout
	geo.Padding = layout.Padding{Left: 1, Right: 1}
	source := geo.NewNode("source")
	sink := geo.NewNodeAt("sink", layout.Point{X: 18, Y: 6})
	geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, sink)
	if err := geo.Build(); err != nil {
		t.Fatal(err)
	}
	got, err := Unicode(&geo)
	if err != nil {
		t.Fatal(err)
	}
	want := "" +
		"┌────────┐                \n" +
		"│ source ├───────┐        \n" +
		"└────────┘       │        \n" +
		"                 │        \n" +
		"                 │        \n" +
		"                 │        \n" +
		"                 │┌──────┐\n" +
		"                 └┤ sink │\n" +
		"                  └──────┘\n"
	if got != want {
		t.Errorf("Unicode() =\n%s\nwant:\n%s", got, want)
	}
}

func TestUnicodeWideLabel(t *testing.T) {
	t.Parallel()

	var geo layout.Layout
	geo.Padding = layout.Padding{Left: 1, Right: 1}
	geo.NewNode("A界")
	if err := geo.Build(); err != nil {
		t.Fatal(err)
	}
	got, err := Unicode(&geo)
	if err != nil {
		t.Fatal(err)
	}
	want := "┌─────┐\n│ A界 │\n└─────┘\n"
	if got != want {
		t.Errorf("Unicode() =\n%s\nwant:\n%s", got, want)
	}
}
