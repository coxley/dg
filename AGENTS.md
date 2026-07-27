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

- create and place nodes with multiline Unicode labels
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
- serialize live graph data, placement, and options to versioned JSON
- group edits into undoable interactions
- restore persistent undo and redo history after reopening a saved document
- auto-size multiline labels or retain explicit node dimensions
- wrap and visually clip labels inside explicit dimensions without losing text
- resize nodes from the nearest corner with right-drag
- order nodes and edges in persistent back-to-front layers
- occlude lower geometry and labels using raster-cell ownership

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
New nodes receive ports at offsets `.5`, `.25`, and `.75` on every side. Stored
order defines connection priority.

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
`Layout.explicitSizes` stores optional fixed outer dimensions by node ID.
`Layout.drawOrder` stores live node and edge hits from back to front. Creation
appends objects to the front; deletion removes their entries.
Auto-sized nodes derive their dimensions from their full labels.
`LabelLine` stores `uint32` byte offsets and display width. `AppendLabelLines`
preserves explicit newlines and adds Unicode-aware wrapping when given a width.

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
err = geo.SetNodeSize(source, layout.Size{Width: 20, Height: 8})
err = geo.AutoSizeNode(source)
err = geo.PlaceNode(sink, layout.NewPoint(20, 3))
err = geo.DeleteEdge(edge)
err = geo.Build()
```

`layout.History` is optional. Attach it through `WithHistory`:

```go
history, err := layout.NewHistory(
	layout.WithHistoryLimit(256),
)
geo, err := layout.New(layout.WithHistory(history))
```

Transactions group live changes into one interaction. `Commit` keeps the final
state. `Cancel` restores the initial state. `Interrupt` commits the latest
applied state, which prevents focus loss or shutdown from leaving unrecorded
changes.

Undo, redo, and cancellation rebuild the layout. Replay failures restore the
layout and history cursor to their prior state. Closed transaction tokens return
`ErrTransactionClosed` and cannot affect a newer transaction.

`History.Store(path)` marks a saved checkpoint and writes history to a cache.
`History.Restore(path)` attaches matching cached history without changing the
saved document's visible state. Unsaved changes after the checkpoint return as
a redo tail. Cache writes use a 100 millisecond debounce and atomic replacement.
Cache files use gzip compression. Readers also accept the earlier plain JSON
format. Cache failures never block diagram editing or saving.

`Layout.Hits(point)` yields the topmost visible object. At a visible node
endpoint it also yields the usable port and incident edges so endpoint dragging
remains possible. It checks compact edge segments, so its cost does not depend
on rendered edge length.

`Layout.PreviewRoute` treats a grid point as a temporary port and chooses its
approach side by route cost. `PreviewRouteWithoutEdge` omits a relocated edge
from occupancy. Both methods reuse router scratch and leave graph and history
state unchanged.

`Layout.Selection()` owns index-aligned node and edge selection sets. It
supports individual replacement and toggling, area intersection, iteration,
whole-layout selection, and connected-component expansion. `ir.Components`
provides the reusable union-find index; rebuilding it after graph mutations
keeps edge deletion semantics correct.

### `document`

`document.Document` is the versioned persisted schema. It stores live semantic
objects, node origins, optional fixed node sizes, padding, and router
configuration. Its layer list stores nodes and edges from back to front. Export
compacts runtime tombstones and remaps port and layer references; import
reconstructs independent runtime slices. Documents without layers derive
node-then-edge creation order.

Routes, geometry, free lists, and reusable scratch are derived state and are not
persisted. `document.Marshal` writes indented JSON. `document.Unmarshal`
strictly decodes and validates version 1. `Document.Layout` creates an editable
layout and accepts layout options such as `WithHistory`.

### `render`

`layout.Rasterize` converts rectangles and routes into aligned directional
connectivity and ownership slices. Later layers replace lower connectivity at
unrelated crossings and across complete node rectangles. Common-endpoint edges
still merge shared connectivity. `render.Unicode` maps visible connectivity to
box-drawing glyphs and only places label graphemes whose cells remain owned by
their node. Fixed-size nodes wrap to their inner width and clip visually at
their inner height without changing the stored label.

### `internal/tui`

The Bubble Tea model keeps terminal-only state:

- cursor and viewport origins
- the reusable hit-selection buffer
- navigation, node movement, live label editing, and edge connection modes
- cached rendered text, document bounds, raster scratch, and terminal cursor

Bubble Tea messages mutate `Layout` synchronously. Every node movement and label
keystroke calls `Build` and refreshes the cached frame. Editing has a
grapheme-aware multiline caret. Enter inserts a newline; Ctrl-Enter and Escape
commit the current label as one undoable interaction. Label and save-path
editing use whitespace and slash as word boundaries. They support Ctrl-A and
Ctrl-E for the line bounds, Alt-B to move to the previous word, Ctrl-W to delete
the previous word, and Ctrl-U to delete to the line start.

The TUI also supports:

- ANSI highlighting for selected node outlines and edges
- port-only highlighting while creating or reconnecting an edge
- node creation at the cursor
- `l` line tool with direct port-to-port mouse dragging and a live orthogonal
  preview routed around node obstacles
- direct endpoint reconnection by dragging an edge within three cells of its
  connected port
- reconnection previews omit the original edge until the drag commits or
  cancels
- selected edges take mouse-hit priority over overlapping nodes and ports
- edge endpoint reassignment without changing the edge ID
- mouse hit selection and overlapping-hit cycling
- left-drag node movement
- right-drag resizing from the nearest corner as one history interaction
- click-dragging a node commits an active label edit
- mouse-wheel viewport panning
- terminal cursor visibility only while editing text
- double-click restoration of a fixed-size node to content-derived dimensions
- Ctrl-S saving with an inline path prompt for new diagrams
- filesystem path completion in the save prompt
- `u` or Ctrl-Z undo; Ctrl-R, Ctrl-Y, or Ctrl-Shift-Z redo
- one history interaction per committed label edit, move, drag, connection, or
  deletion
- committed final placement when focus loss or shutdown interrupts a drag
- focus-colored area selection by dragging from an empty cell
- Ctrl-click toggling for non-contiguous node and edge selection
- Ctrl-A expansion through selected connected components, followed by
  whole-document selection
- grouped movement and deletion as one history interaction
- `[` and `]` move the focused object backward or forward one layer
- Shift-`[` and Shift-`]` send the focused object to the back or front

The model accepts pasted multiline labels and rejects carriage returns before
they reach the layout.

Run the example editor with:

```sh
go run ./cmd/dg
go run ./cmd/dg path/to/diagram.json
```

Ctrl-S saves directly when the editor opened a document from disk. New
diagrams prompt for a path on the first save; Tab completes existing filesystem
paths.

## Settled design decisions

- `Point` and `Size` use `uint32`.
- `Padding` uses `uint8`.
- rectangles use half-open bounds.
- node dimensions are exactly label size plus padding and borders. Layout does
  not add parity padding; odd-width nodes retain their true center cell.
- the first port stored on each side is always connectable. Later ports become
  connectable in stored order when one boundary cell separates them from both
  corners and every earlier connectable port. Availability is offset-agnostic,
  so future custom ports use the same rule.
- default padding is one horizontal cell and zero vertical cells.
- labels preserve explicit newlines and use terminal display width, including
  wide graphemes.
- nodes auto-size to the widest explicit line and total line count by default.
- fixed outer dimensions remain authoritative across label edits. Their labels
  wrap to the available width and clip to the available height.
- fixed-size labels retain their full source text when clipped.
- ports contain an `Anchor` on the node boundary and an `Exit` outside it.
- an `Exit` equals its `Anchor` when unsigned coordinates cannot represent the
  outward neighbor.
- the router is configuration passed to `layout.New`.
- routing costs include step, shared step, bend, crossing, and endpoint-step
  costs.
- endpoint steps add 40 by default while ordinary steps cost 10. This prefers
  exterior detours without making overlapping endpoints unroutable.
- route sharing requires a common endpoint and begins only where that endpoint
  is nearer than both distinct endpoints. This keeps shared trunks near their
  logical destination instead of visually joining source nodes.
- unrelated edges may cross but may not share a segment or touch an endpoint.
- rerouting replaces an entire route. Comments mark where more local rerouting
  would need different logic.
- the router uses a concrete heap and `cmp.Compare`.
- `Layout` owns reusable router scratch. `Router` remains copyable configuration.
- `render.Encoder` owns reusable raster and label-placement scratch.
- obstacle access uses `Layout.Obstacles()` instead of a stored obstacle slice.
- rasterization decides glyph connectivity after layout.
- node tombstones retain an empty port buffer; zero-value edges represent
  deleted entries.
- deleted ports use an invalid owner because a zero-value port is valid.
- IDs remain stable while live and may be reused after deletion.
- free lists fill tombstones before slices grow.
- deletion does not compact slices.
- history retains 256 interactions by default and accepts a custom limit.
- cached history uses the SHA-256 digest of the normalized absolute document
  path.
- macOS stores cached history under
  `os.UserCacheDir()/org.coxley.dg/history`.
- Linux and Windows store cached history under
  `os.UserCacheDir()/dg/history`.
- anonymous diagrams do not write history until their first save.
- draw order contains each live node and edge exactly once.
- new objects start at the front of draw order.
- edge endpoint cells remain node-owned so ports and incident edges stay
  selectable.
- unrelated edge crossings show only the upper edge; common-endpoint routes
  retain merged connectivity.
- endpoint edges may route through overlapping endpoint rectangles. Raster
  ownership hides the covered route cells.

## Current performance

Results from an Apple M4 Max on July 27, 2026:

```text
BenchmarkLayoutBuild             39.9 µs/op   0 B/op   0 allocs/op
BenchmarkLayoutMoveAndBuild      43.7 µs/op   0 B/op   0 allocs/op
BenchmarkLayoutEditLabelAndBuild 23.6 µs/op   0 B/op   0 allocs/op
BenchmarkLayoutDeleteAndCreateNode
                                  114 ns/op   0 B/op   0 allocs/op
BenchmarkLayoutDeleteAndConnectEdge
                                  7.7 ns/op   0 B/op   0 allocs/op
BenchmarkLayoutHits/node         32.6 ns/op   0 B/op   0 allocs/op
BenchmarkLayoutHits/edge         30.5 ns/op   0 B/op   0 allocs/op
BenchmarkLayoutHits/miss         31.2 ns/op   0 B/op   0 allocs/op
BenchmarkPreviewRoute             0.8 ms/op   0 B/op   0 allocs/op
BenchmarkEncoderEncode           2.3 µs/op   0 B/op   0 allocs/op
BenchmarkModelMoveAndView        3.68 µs/op   2304 B/op   1 allocs/op
BenchmarkAppendLabelLines        2.89 µs/op      0 B/op   0 allocs/op
```

These benchmarks use a small three-node, two-edge diagram. `Layout.Hits` scans
nodes, ports, and compact edge segments. Add a spatial index only if larger
interactive diagrams show that this scan matters.

The TUI benchmark uses a one-node document and covers a Bubble Tea key update,
node movement, layout build, rasterization, viewport composition, and view
creation. Its only steady-state allocation converts the completed byte buffer
to the frame string required by Bubble Tea.

## Recommended next work

The next phase should make object creation, focus, styling, and export feel like
one editor rather than separate engine demonstrations.

### 1. Add rectangle creation and node focus

Add an explicit rectangle tool on `r`:

- click-drag creates an explicitly sized node with an empty label
- `e` edits the label after creation
- the rectangle uses the same fixed-size wrapping and clipping rules
- creation, sizing, and interruption form one history interaction

Change Tab navigation to cycle focused nodes in draw order. Arrow keys should
move the focused node. Preserve a separate way to cycle overlapping hits when
needed.

### 2. Add inherited node and edge styles

Pressing `b` on a focused node should cycle its border style. Each new node
inherits the most recently selected border style.

Support a no-border style for text-only labels. Borderless nodes still need a
logical rectangle for obstacle avoidance and ports along its invisible
boundary.

Lines need independent arrow styles at both ends. Each new line inherits the
most recent origin and destination arrow styles. Leave about one cell between
an arrowhead and the node border so the endpoint remains readable.

Increase port contrast while the line tool is active. Test red and green
against the current theme instead of relying on the existing blue highlight.

### 3. Add clipboard export

Ctrl-C should copy the selected rendered cells to the system clipboard. This
replaces the current Ctrl-C quit binding, so choose another quit shortcut.

Two successive Ctrl-C presses should open an export prompt. The prompt should
default to line comments and also offer a Markdown code block. Line-comment
export prefixes every copied row.

Store the preferred comment prefix, such as `// ` or `# `, in a user
preferences file under the platform cache directory. Keep preferences separate
from per-document history.

### 4. Complete text layout

Add horizontal alignment, vertical alignment, and justification after the
object and style model can persist those choices.

### 5. Add reusable custom shapes

Support custom shapes composed from multiple boxes and lines. Rasterize their
components as one object so internal overlaps retain their intended
connectivity instead of gaining the gap cells used when independent layered
objects occlude each other.

For example, a reusable database shape could render as:

```text
┌─────────────┐
│  cassandra  ├┐
└┬────────────┘│
 └─────────────┘
```

Each shape definition needs to identify:

- the component that owns the label
- which components scale vertically for multiline labels, and in what
  proportions
- which components scale horizontally for wider labels, and in what
  proportions

Persist custom shape definitions under the platform cache directory so users
can quickly reuse designs across documents.

## Known limits

- coordinates cannot be negative
- node and edge styles do not exist
- nodes always draw borders
- edges have no arrowheads
- layer commands currently reorder one selected object rather than a selected
  group
- moving a connected node still rolls back when an edge cannot route around
  unrelated obstacles
- Tab cycles overlapping hits rather than focused nodes
- line-tool ports need stronger contrast
- Ctrl-C quits instead of copying
- every `Build` routes every edge
- hit testing scans all geometry
- public geometry slices rely on callers not mutating them
- several methods assume valid IDs and may panic on invalid indices

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
