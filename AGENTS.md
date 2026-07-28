# Project handoff

## Goal and constraints

`dg` is a Monodraw-like diagram engine and interactive terminal editor written
in Go. It supports programmatic construction and point-and-click editing.

This is a freeform editor, not a Graphviz-style global layout system. Callers
place nodes. The engine resolves cell geometry, routes orthogonal edges, and
renders the result.

Keep layout grid-based and independent of Unicode glyph selection. Layout owns
geometry and directional connectivity; renderers decide how to encode it.

Prefer data-oriented design:

- keep structs small
- use slice indices as stable IDs
- keep related data in index-aligned slices
- reuse hot-path storage
- benchmark before adding bookkeeping or spatial indices
- use concrete algorithms instead of allocation-heavy interfaces

## Current capabilities

The engine and TUI currently support:

- multiline Unicode labels with grapheme-aware editing
- automatic node sizing from content
- explicit node dimensions with wrapping and visual clipping
- left-drag creation of explicitly sized empty rectangles
- right-drag resizing from the nearest corner
- double-click restoration to automatic sizing
- normalized side ports resolved to usable boundary cells
- orthogonal routing around obstacles
- configurable step, shared-step, bend, crossing, and endpoint costs
- common-endpoint route sharing and one extra reroute pass by default
- smart filled or outline arrows at either edge endpoint
- solid, rounded, double, borderless, and dashed styles
- persistent back-to-front draw order and raster occlusion
- shared borders and common-endpoint T-junction rasterization
- hit testing for visible nodes, ports, and compact edge segments
- engine-owned node and edge selection
- area, non-contiguous, component, and whole-document selection
- transactional edits with persistent undo and redo
- versioned JSON documents
- persistent gzip-compressed history stored separately in the cache directory
- a Bubble Tea editor with mouse support, floating toolbar, settings, saving,
  clipboard export, and live routed previews

The example editor starts with:

```text
┌─────┬─────┐
│ foo │ bar │
└──┬──┴──┬──┘
   │     │
   └──┬──┘
  ┌───┴───┐
  │ sinks │
  └───────┘
```

## Packages

### `ir`

`ir.Graph` stores semantic objects:

- `Node` stores its label and port IDs.
- `Port` stores its node ID, side, and normalized offset.
- `Edge` stores two port IDs.

Edges are undirected. Duplicate port pairs return the existing edge ID.

New nodes receive ports at offsets `.5`, `.25`, and `.75` on every side.
Stored order defines connection priority. The first port on a side is always
usable. Later ports become usable when the boundary has enough cells to keep
one cell between the candidate, corners, and earlier usable ports. This rule is
offset-agnostic so user-defined ports use the same logic.

Deletion leaves tombstones. Per-type free lists reuse tombstoned indices before
growing slices. Node deletion removes its ports and incident edges. Live IDs
remain stable; deleted IDs may be reused.

`ir.Components` owns reusable union-find scratch for connected-component
selection. Rebuild it after graph mutations.

### `layout`

`layout.Layout` owns a cloned graph and index-aligned geometry:

- `Layout.Nodes[nodeID]`
- `Layout.Ports[portID]`
- `Layout.Edges[edgeID]`

Mutations go through `Layout` and update semantic and derived state in place.
`Build` resolves labels and routes edges. `WithGraph` imports initial semantic
data that did not originate through `Layout`.

```go
geo, err := layout.New(
	layout.WithPadding(1, 0),
	layout.WithRouter(layout.DefaultRouter()),
	layout.WithGraph(graph),
	layout.WithHistory(history),
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

Important aligned internal slices include explicit node sizes, node styles,
edge styles, and draw order. `draftPorts`, router state, and raster buffers are
reusable scratch.

`Layout.Selection()` stores index-aligned node and edge membership. It supports
replacement, toggling, area intersection, iteration, connected-component
expansion, and selecting everything. `DuplicateSelection` duplicates selected
nodes and edges whose endpoints are both duplicated. Portless edges are not yet
supported.

Rigid selection moves avoid routing when static edges cannot be affected.
`BuildSelection` routes only geometry affected by the selection. Keep adding
safe no-route cases instead of routing defensively in interactive hot paths.

`Layout.Hits(point)` yields visible objects in interaction priority order.
Visible node endpoints also yield usable ports and incident edges. It checks
compact segments, so edge cost does not depend on rendered length.

`Layout.PreviewRoute` treats a point as a roaming temporary port.
`PreviewRouteWithoutEdge` excludes a relocated edge from occupancy. Styled
variants reserve straight endpoint cells for arrows. Preview operations reuse
router scratch and do not mutate graph, history, or committed geometry.

### Routing

The router uses a concrete allocation-free heap and reusable arrays owned by
`Layout`. `Router` remains copyable configuration.

Default cost reference:

- `Step = 10`: ordinary path length
- `SharedStep = 2`: common-endpoint shared segments; lower values favor trunks
- `Bend = 5`: direction changes; higher values favor straighter paths
- `Crossing = 15`: unrelated edge crossings; higher values favor detours
- `EndpointStep = 40`: travel through endpoint rectangles; higher values favor
  exterior routes without forbidding overlap
- `ReroutePasses = 1`: one additional whole-edge reroute pass

Route sharing requires a common endpoint and starts only where that endpoint is
nearer than both distinct endpoints. This keeps shared trunks near their
logical destination. Unrelated edges may cross but may not share a segment or
touch an endpoint.

Smart-arrow ports prefer two straight cells before a bend. The router keeps
full clearance when it does not lengthen the path, but permits shorter
clearance when the extra cell would force a substantially worse route.

### `history`

History is engine-level rather than TUI-level. A transaction represents one
committed interaction:

- `Commit` records the final applied state.
- `Cancel` restores the initial state.
- `Interrupt` commits the latest visible state.

The last rule prevents focus loss or shutdown from creating unrecorded changes.
Closed or stale tokens return `ErrTransactionClosed`.

History retains 256 interactions by default and accepts a custom limit. Cached
history uses gzip, debounced atomic replacement, and the SHA-256 digest of the
normalized absolute document path. Anonymous diagrams do not cache history
until first save. Cache failures never block editing or document saving.

### `document`

`document.Document` is the versioned persisted schema. It stores:

- live nodes, ports, and edges
- node origins and optional explicit sizes
- node border, stroke, and text alignment
- edge stroke and endpoint arrows
- padding and router configuration
- back-to-front layers

Export compacts runtime tombstones and remaps references. Import recreates
independent runtime slices. Routes, raster cells, free lists, geometry scratch,
and selection are derived and are not persisted.

### `render`

`layout.Rasterize` produces aligned directional-connectivity and ownership
slices. Later layers replace lower unrelated geometry. Common-endpoint edges
retain merged connectivity. Complete node rectangles occlude lower geometry
and labels.

`render.Encoder` reuses raster and label-placement storage. It applies labels,
arrows, border styles, and stroke styles after rasterization.

Dashed strokes currently use Unicode light double-dash glyphs `╌` and `╎`.
These restart their dash pattern inside every cell. Most terminal cells are
taller than wide, so the pair looks denser horizontally than vertically.
Standard triple, quadruple, and heavy dashed box glyphs exaggerate the problem.

The recommended fix is phase-aware rendering:

- retain the same solid directional connectivity and ownership
- render horizontal runs as two `─` cells followed by two spaces
- render vertical runs as one `│` cell followed by one space
- keep corners, bends, crossings, T-junctions, arrows, and ports solid
- fit phase per straight segment so both endpoints remain visible
- choose the pattern for the complete segment, not global `X+Y` parity

The orientation-specific periods compensate for terminal cells being roughly
twice as tall as they are wide. A headless-terminal comparison found horizontal
two-on/two-off plus vertical one-on/one-off the most balanced; one-on/one-off
was too busy horizontally and Unicode triple/quadruple glyphs looked dotted.
Rendering spaces must not change hit testing or ownership.

### `internal/tui`

The Bubble Tea model owns only frontend state: viewport, cursor, active tool,
modal state, cached frames, drag state, and reusable preview buffers.

The canvas covers the entire terminal. A centered rounded toolbar floats above
and occludes it. Selection may move partially beyond the viewport as long as a
visible portion remains recoverable. When unsigned coordinates would cross
zero, the editor rebases geometry and viewport together.

Node-only duplicate previews layer over the committed frame without routing.
Rigid committed moves skip routing when static edges cannot be affected. Keep
preview work separate from committed geometry.

The settings overlay preserves the diagram underneath it:

- `?` opens Shortcuts.
- Tab and Shift-Tab switch Shortcuts and Preferences.
- Esc or `q` closes either settings tab.
- Numeric router fields change only with Left/Right and briefly highlight the
  pressed arrow.
- Preference fields use Up/Down to change fields.
- Preference selects use Up/Down to change fields and Left/Right to change the
  selected value.
- The default directory uses the `huh/v2` file picker.
- Preferences end with explicit Save and Cancel actions.
- Router preferences apply live to the current diagram.
- Cancel, Esc, `q`, an outside click, or lost terminal focus restores the
  router values from before the modal opened.
- “Apply to future diagrams?” controls whether they become defaults.

`Theme` in `internal/tui/theme.go` owns every terminal-facing color and style.
Lip Gloss layers and its compositor place modals over the canvas. The Bubbles
help model renders shortcut columns; local tab state follows Bubble Tea's
official tabs example. The sparse canvas renderer remains custom because its
viewport and preview behavior are diagram-specific. Cache Lip Gloss-rendered
toolbar and highlight spans: invoking the general renderer for every cell
causes a measurable drag regression.

Success notices remain visible for one second or until the next key press.
Errors render in red.

Current editing shortcuts:

| Key | Action |
| --- | --- |
| `r` | rectangle tool |
| `l` | line tool |
| Esc | cursor tool or cancel current interaction |
| `e` | edit selected node label |
| `b` | cycle solid, rounded, double, and borderless borders |
| `-` | toggle dashed stroke on selected nodes and edges |
| `a` / `A` | cycle arrows at either edge endpoint |
| `t` / `T` | cycle horizontal or vertical label alignment |
| Tab / Shift-Tab | cycle node focus |
| arrow keys | move the current node or selection |
| Backspace/Delete | delete selection |
| `d` | duplicate selected nodes and internal edges |
| Alt-drag | duplicate with a live raster preview |
| Ctrl-A | expand to connected components, then select everything |
| Ctrl-click | toggle non-contiguous selection |
| `[` / `]` | move one layer backward or forward |
| Shift-`[` / Shift-`]` | send to back or front |
| `u` / Ctrl-Z | undo |
| Ctrl-R / Ctrl-Y / Ctrl-Shift-Z | redo |
| Ctrl-S | save |
| Super-C or Ctrl-C | copy selection |
| `q` | quit when no modal is open |

Two successive copy commands open an export form. Export supports preferred
line comments (`// `, `# `, or block comments) and Markdown code fences.
Trailing whitespace is removed from copied rows. The first copy waits 100 ms;
a second copy in that window opens Export without writing the provisional
version. On first clipboard use, the editor probes terminal clipboard reads
for 100 ms. A response selects Bubble Tea's OSC52 clipboard commands for the
session; a timeout selects
`golang.design/x/clipboard`. Advertise Super-C only after Bubble Tea reports
keyboard enhancements.

Save, export, and preferences use `huh/v2` forms. New diagrams use a `huh`
file picker on first save. Existing paths save directly. Preferences persist
separately from documents and history.

Run the editor with:

```sh
go run ./cmd/dg
go run ./cmd/dg path/to/diagram.json
```

## Settled geometry decisions

- `Point` and `Size` use `uint32`.
- `Padding` uses `uint8`.
- rectangles use half-open bounds.
- default padding is one horizontal cell and zero vertical cells.
- node dimensions equal content, padding, and border requirements; layout does
  not add parity padding.
- labels preserve explicit newlines and terminal display widths.
- automatic nodes grow to content.
- an explicit size remains authoritative until `AutoSizeNode`.
- explicit nodes wrap and visually clip without losing source text.
- `LabelLine` stores `uint32` byte offsets.
- ports contain a boundary `Anchor` and an outward `Exit`.
- `Exit == Anchor` when unsigned coordinates cannot represent the neighbor.
- rasterization, not routing, chooses final glyphs.
- draw order contains each live node and edge exactly once.
- new objects start at the front.
- edge endpoint cells stay node-owned so ports and incident edges remain
  selectable.
- collinear node boundaries may merge; overlapping interiors occlude.
- stroke is independent of node border shape and edge arrows.

## Snapping design

Snapping should live in `layout`, with the TUI only supplying proposed movement
and drawing returned guides. This keeps behavior reusable by native and web
frontends.

Use an optional reusable `Snapper` with data-oriented scratch:

- build separate X and Y candidate arrays from live, unselected nodes
- use node left/center/right and top/middle/bottom anchors
- represent centers in doubled integer coordinates; this preserves half-cell
  centers without floats
- reject center matches that cannot be reached by whole-cell translation
- snap the selection's outer bounds and move the selection rigidly
- evaluate axes independently
- return a signed adjusted delta plus zero or more guide descriptors
- do not mutate layout while evaluating candidates
- reuse candidate and guide buffers across drag frames

Interaction policy:

- snapping is on by default
- enter a snap within one cell
- retain the latched candidate until raw movement exceeds two cells
- prefer exact edge alignment, then center alignment
- resolve equal candidates deterministically by distance, visual priority, and
  stable object ID
- exclude selected objects from candidates
- initially ignore edges and ports
- hold Shift during drag to bypass snapping temporarily
- render guides in the preview layer
- commit only the final visible snapped placement as one history interaction

The one-cell acquire/two-cell release hysteresis prevents flicker and keeps
always-on snapping from feeling sticky. Future candidate kinds can add equal
spacing, port alignment, and baseline alignment without changing movement or
history APIs.

Property tests should verify:

- results never move farther than the acquisition threshold
- X and Y snapping remain independent
- candidate ordering does not change the result
- multi-selection relative geometry never changes
- bypass returns the exact proposed delta
- interrupted drags commit the final previewed placement
- undo and redo reproduce the same snapped placement

Benchmark no-candidate, many-candidate, and latched-snap drag frames with
`b.Loop`. The initial implementation can scan nodes; add sorted candidate
indices or a spatial index only when benchmarks justify it.

## Headless terminal verification

Use [`montanaflynn/headless-terminal`](https://github.com/montanaflynn/headless-terminal)
to inspect actual Bubble Tea output through a PTY and Ghostty-compatible VT
renderer.

Start a named editor session:

```sh
ht run --size 100x30 --name dg-smoke \
  env GOCACHE=/private/tmp/dg-codex-go-build go run ./cmd/dg
```

Send keys using Vim-style notation:

```sh
ht send dg-smoke '?'
ht send dg-smoke '<tab><down><down>'
ht send dg-smoke '<esc>'
```

Inspect the current terminal:

```sh
ht view dg-smoke
ht view --format ansi dg-smoke
ht view --format png --output /private/tmp/dg-smoke.png dg-smoke
```

For mouse flows, inspect `ht send --help` for the installed version's event
syntax. Use several terminal sizes, especially `100x30`, `80x16`, and a short
height such as `80x12`.

Always clean up named sessions:

```sh
ht stop dg-smoke
ht remove dg-smoke
```

Use headless-terminal to validate cells, focus, overlays, cursor visibility,
and interaction state. Font-specific glyph appearance can still differ from
the user's terminal, so compare Unicode styles in more than one font before
settling rendering choices.

## Next work

Recommended order:

1. Replace static Unicode dashed glyphs with phase-aware segment dashing.
2. Implement the engine-level `Snapper`, guides, hysteresis, TUI integration,
   property tests, and drag benchmarks described above.
3. Apply layer commands to whole selections rather than one focused object.
4. Add portless lines or boxless ports so arbitrary line selections can be
   duplicated.
5. Add reusable custom shapes composed from multiple primitives. Custom shapes
   need a label owner and horizontal/vertical scaling rules. Persist reusable
   definitions in the platform cache directory.
6. Continue identifying interactions that can reuse committed raster output or
   skip routing entirely.

## Known limits

- coordinates cannot be negative; the TUI rebases geometry near zero
- current dashed glyphs have uneven horizontal and vertical rhythm
- double-dashed lines have no faithful Unicode box-drawing representation
- layer commands reorder one object rather than a whole selection
- duplicating selections requires at least one node
- moving connected nodes may fail when no valid route exists
- normal full builds route every edge
- hit testing scans live geometry
- public geometry slices rely on callers not mutating them
- several low-level methods assume valid IDs

## Verification

Use `github.com/stretchr/testify/require` for tests and benchmark setup.
Inside `b.Loop`, use direct benchmark failures to avoid boxing measured values.
Never use `time.Sleep` in tests outside `testing/synctest`.

```sh
GOCACHE=/private/tmp/dg-codex-go-build go test ./...
GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...
GOCACHE=/private/tmp/dg-codex-go-build go vet ./...
GOCACHE=/private/tmp/dg-codex-go-build \
  GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache \
  golangci-lint run --path-mode abs
GOCACHE=/private/tmp/dg-codex-go-build \
  go test ./layout -run '^$' -bench 'BenchmarkLayout' -benchmem
```

Before this handoff, tests, race detection, vet, and lint passed.
