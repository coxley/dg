The project is now at the point where the **next concrete step** is to build a terminal-aware geometry layer that turns your IR into cell-based rectangles, ports, and orthogonal routes, but stops short of choosing final Unicode glyphs. [yworks](https://www.yworks.com/pages/drawing-orthogonal-diagrams)

## Project context

You’re building a Monodraw-like diagram engine in Go, with a custom IR that models:
- nodes with ports,
- ports attached to sides with relative offsets,
- undirected edges between ports,
- and later, routing/rendering on a character grid. [sciencedirect](https://www.sciencedirect.com/science/article/abs/pii/S1045926X13000943)

The goal is **not** a Graphviz-style global layout system. Instead, it’s a freeform editor and renderer where flow can go in any direction, while the engine helps with clean orthogonal routing and sensible port placement. [docs.yfiles](https://docs.yfiles.com/yfiles/doc/developers-guide/orthogonal_edge_router.html)

## Where the IR stands

Your current IR is already in a good place:
- `Node` owns a list of ports.
- `Port` stores `Node`, `Side`, and `Offset` in the normalized range `[0.0, 1.0]`.
- `Edge` connects two ports.
- The graph is intentionally undirected and duplicate port-pairs are rejected, which makes it behave like a simple graph over ports. [en.wikipedia](https://en.wikipedia.org/wiki/Multigraph)

That means the model now captures the important semantic information needed for orthogonal diagrams: side constraints and relative positions on node boundaries. [publikationen.uni-tuebingen](https://publikationen.uni-tuebingen.de/xmlui/bitstream/handle/10900/49366/pdf/diss.pdf?isAllowed=y&sequence=1)

## What the geometry layer should do

The geometry layer should be **cell-aware**, because terminal box drawing depends on monospaced character cells and adjacency-aligned line art. But it should not yet decide the actual Unicode rune used for each junction; that is a final rasterization concern. [en.wikipedia](https://en.wikipedia.org/wiki/Box-drawing_characters)

Its job is to resolve:
- node label size into a rectangle,
- ports into concrete points on the rectangle boundary,
- edges into orthogonal routes on a grid,
- and occupied space into a rasterizable cell structure. [users.monash](https://users.monash.edu/~mwybrow/papers/marriott-diagrams-2014.pdf)

## Recommended next milestone

The best next milestone is:

**Take two manually placed nodes and render one orthogonally routed edge between them on a Unicode grid.** [people.eng.unimelb.edu](https://people.eng.unimelb.edu.au/pstuckey/papers/gd09.pdf)

That forces you to build the full pipeline in miniature:
1. measure text,
2. assign a rectangle,
3. resolve ports to anchor points,
4. route a path,
5. rasterize box drawing characters. [link.springer](https://link.springer.com/article/10.1007/s00454-023-00593-y)

## Suggested package split

A clean split for the project is:

- `ir/` — your current semantic model.
- `layout/` — cell geometry, port resolution, routing.
- `render/` — convert occupancy/segments into Unicode box drawing.

That mirrors the common graph-drawing separation between abstract graph representation, geometric layout, and final drawing output. [cs.brown](https://cs.brown.edu/people/rtamassi/papers/gdbiblio.pdf)

## What to build first

Build these in order:

1. `MeasureLabel(text)`.
2. `NodeRect` from label size and padding.
3. `ResolvePort(rect, side, offset)`.
4. `RouteOrthogonal(a, b, obstacles)`.
5. `Rasterize(grid)` into box-drawing characters. [en.wikipedia](https://en.wikipedia.org/wiki/Box-drawing_characters)

## The design choice that matters most

The single biggest decision is: **make geometry grid-based, not glyph-based**. Unicode box-drawing characters are just the terminal encoding of orthogonal connectivity, so your layout logic should think in cells, segments, and junction occupancy, not in runes. [unicode](https://www.unicode.org/charts/nameslist/n_2500.html)

That keeps the engine flexible enough to support ASCII fallback later, or even a graphical renderer, without redoing the core geometry.

So the short answer is: your next step is to implement the **layout/cell-geometry layer** with manual placement and simple orthogonal routing, then add rasterization as a separate final pass. [yworks](https://www.yworks.com/pages/drawing-orthogonal-diagrams)
