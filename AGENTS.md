# Project handoff

## Goal

Build a Monodraw-like diagram engine in Go. The engine must support both
programmatic editing and a point-and-click interface.

This is a freeform editor, not a Graphviz-style global layout system. Callers
place nodes. The engine resolves cell geometry, routes orthogonal edges, and
renders the result.

Keep the core geometry grid-based and independent of Unicode glyphs. A future
renderer should be able to target ASCII or graphics without replacing layout.

Prefer data-oriented design:

- keep structs small
- use slice indices as IDs
- keep related geometry in aligned slices
- reuse hot-path memory
- add bookkeeping only after benchmarks justify it

## Current milestone

The first end-to-end milestone is complete. The engine can:

- create and place nodes with single-line Unicode labels
- connect side-constrained ports
- route orthogonal edges around node obstacles
- share routes between edges with a common endpoint
- allow and cost unrelated edge crossings
- reroute crossing edges for one extra pass by default
- rasterize geometry into directional cell connectivity
- render Unicode box-drawing output
- edit labels transactionally
- delete nodes and edges and reuse their IDs
- move nodes and rebuild without steady-state allocations
- query every node, port, and edge rasterized at a grid point
- edit diagrams interactively through a Bubble Tea terminal UI

The example program renders:

```text
┌─────┬─────┐
│ foo │ bar │
└──┬──┴──┬──┘
   │     │
   │     │
   └──┬──┘
  ┌───┴───┐
  │ sinks │
  └───────┘
```

## Package boundaries

### `ir`

`ir.Graph` stores the semantic graph:

- `Node` stores its label and port IDs
- `Port` stores its node ID, side, and normalized offset
- `Edge` stores two port IDs

Edges are undirected. Duplicate port pairs return the existing edge ID.
New nodes currently receive fixed candidate ports on every side.

Deletion leaves tombstones in the public slices. Per-type free lists reuse those
indices before growing storage. Node deletion also removes its ports and
incident edges.

### `layout`

`layout.Layout` owns a cloned graph and index-aligned geometry:

- `Layout.Nodes[nodeID]`
- `Layout.Ports[portID]`
- `Layout.Edges[edgeID]`

Mutations made through `Layout` update node and port geometry immediately.
`Build` routes the edges.

`Layout.draftPorts` is reusable transactional scratch space. Port resolution
finishes there before it commits results to `Layout.Ports`.

The public construction API is:

```go
geo, err := layout.New(
	layout.WithPadding(1, 0),
	layout.WithRouter(layout.DefaultRouter()),
	layout.WithGraph(graph),
)
source, err := geo.NewNodeAt("source", layout.NewPoint(2, 3))
sink, err := geo.NewNode("sink")
edge := geo.ConnectNodes(source, ir.RightSide, ir.LeftSide, sink)
err = geo.SetNodeLabel(source, "renamed")
err = geo.PlaceNode(sink, layout.NewPoint(20, 3))
err = geo.DeleteEdge(edge)
err = geo.Build()
```

`Layout.Hits(point)` yields all overlapping geometry in node, port, then edge
order. It reports ports at their anchor cells. It checks compact edge segments,
so its cost does not depend on rendered edge length.

### `render`

`layout.Rasterize` converts rectangles and routes into directional cell
connectivity. `render.Unicode` maps that connectivity to box-drawing glyphs and
places labels.

The renderer merges connectivity. It does not retain object ownership or
layering information.

### `internal/tui`

The Bubble Tea model keeps terminal-only state:

- cursor and viewport origins
- the reusable hit-selection buffer
- navigation, node movement, and label editing modes
- cached rendered text and document bounds

Bubble Tea messages mutate `Layout` synchronously. Node movement and label
editing call `Build` and refresh the cached frame. The model accepts pasted
single-line labels and rejects pasted newlines before they reach the layout.

Run the example editor with:

```sh
go run ./cmd/dg
```

## Settled design decisions

- `Point` and `Size` use `uint32`.
- `Padding` uses `uint8`.
- rectangles use half-open bounds.
- default padding is one horizontal cell and zero vertical cells.
- labels support one line for the initial milestone.
- label measurement uses terminal display width, including wide graphemes.
- ports contain an `Anchor` on the node boundary and an `Exit` outside it.
- an `Exit` equals its `Anchor` when unsigned coordinates cannot represent the
  outward neighbor.
- the router is configuration passed to `layout.New`.
- routing costs include step, shared step, bend, and crossing costs.
- route sharing requires a common endpoint and applies to the entire route.
- unrelated edges may cross but may not share a segment or touch an endpoint.
- rerouting replaces an entire route. Comments mark where more local rerouting
  would need different logic.
- the router uses a concrete heap and `cmp.Compare`.
- `Layout` owns reusable router scratch. `Router` remains copyable configuration.
- obstacle access uses `Layout.Obstacles()` instead of a stored obstacle slice.
- rasterization decides glyph connectivity after layout.
- node tombstones retain an empty port buffer; zero-value edges represent
  deleted entries.
- deleted ports use an invalid owner because a zero-value port is valid.
- IDs remain stable while live and may be reused after deletion.
- free lists fill tombstones before slices grow.
- deletion does not compact slices.

## Current performance

Results from an Apple M4 Max on July 27, 2026:

```text
BenchmarkLayoutBuild             18.5 µs/op   0 B/op   0 allocs/op
BenchmarkLayoutMoveAndBuild      20.0 µs/op   0 B/op   0 allocs/op
BenchmarkLayoutEditLabelAndBuild 16.4 µs/op   0 B/op   0 allocs/op
BenchmarkLayoutDeleteAndCreateNode
                                  130 ns/op   0 B/op   0 allocs/op
BenchmarkLayoutDeleteAndConnectEdge
                                  7.2 ns/op   0 B/op   0 allocs/op
BenchmarkLayoutHits/node         37.1 ns/op   0 B/op   0 allocs/op
BenchmarkLayoutHits/edge         35.7 ns/op   0 B/op   0 allocs/op
BenchmarkLayoutHits/miss         36.1 ns/op   0 B/op   0 allocs/op
BenchmarkModelMoveAndView         3.1 µs/op   2577 B/op   9 allocs/op
```

These benchmarks use a small three-node, two-edge diagram. `Layout.Hits` scans
nodes, ports, and compact edge segments. Add a spatial index only if larger
interactive diagrams show that this scan matters.

The TUI benchmark uses a one-node document and covers a Bubble Tea key update,
node movement, layout build, rasterization, viewport composition, and view
creation. Its allocations remain outside the layout hot path.

## Recommended next work

The basic TUI is complete. It supports cursor navigation, overlapping-hit
cycling, node movement, label editing, node and edge deletion, viewport
tracking, and resize-aware rendering.

### 1. Define a persisted document format

Do not serialize router scratch or derived routes as authoritative state. Create
a versioned document type that contains:

- semantic graph data
- node origins
- document and router options
- future node and edge styles
- future draw order

`WithGraph` loads semantic graph data but places every node at the origin. A
document loader needs to restore placement as well.

JSON is sufficient for the first format. Keep the persisted schema separate
from runtime structs so internal layout changes do not silently break files.

## Later design work

### Multiline labels

Separate text layout from node rectangle calculation before adding multiline
text. The model will need:

- wrapping width and height constraints
- horizontal alignment
- vertical alignment
- justified lines
- explicit newlines

Keep the current single-line path simple. A text-layout result should provide
measured dimensions and positioned line runs that the renderer can place.

### Borderless nodes

Keep a logical rectangle for obstacle avoidance and port placement even when no
border is drawn. Define whether padding remains part of the obstacle and where
an invisible boundary anchors ports.

### Edge endpoint shapes

Store visual endpoint styles independently for each end of an undirected edge.
Define how arrowheads occupy cells, attach to port anchors, and combine with node
borders before changing the router.

### Layering

Creation order should define the default back-to-front order. Reordering should
not renumber semantic IDs.

A compact direction is a draw-order slice containing object kind and object ID.
Operations can move entries backward, to the back, forward, or to the front.

Layering affects more than rendering:

- `Layout.Hits` must expose or respect back-to-front order
- deletion must remove draw-order entries
- persistence must save draw order
- rasterization must retain ownership instead of only merged connectivity
- unrelated crossings need a rule for which edge appears above the other

Settle whether ports are independent drawable objects before defining this
representation.

## Known limits

- coordinates cannot be negative
- labels cannot contain newlines
- node and edge styles do not exist
- nodes always draw borders
- edges have no arrowheads
- every `Build` routes every edge
- hit testing scans all geometry
- public geometry slices rely on callers not mutating them
- several methods assume valid IDs and may panic on invalid indices
- loaded graphs do not contain node placement
- raster cells lose object ownership

## Verification

Use `github.com/stretchr/testify/require` for test and benchmark assertions.
Inside a `b.Loop` body, use direct benchmark failures to avoid boxing values in
the measured path.

Use a writable Go build cache in the sandbox:

```sh
GOCACHE=/private/tmp/dg-codex-go-build go test ./...
GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...
GOCACHE=/private/tmp/dg-codex-go-build go vet ./...
GOCACHE=/private/tmp/dg-codex-go-build \
  GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache \
  golangci-lint run --path-mode abs
GOCACHE=/private/tmp/dg-codex-go-build \
  go test ./layout -run '^$' \
  -bench 'BenchmarkLayout' -benchmem
```

Before this handoff, tests, race detection, vet, and lint passed.
