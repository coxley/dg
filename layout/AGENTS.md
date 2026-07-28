# Layout package guide

## Responsibility

`layout` owns editable IR, terminal-cell geometry, orthogonal routing, raster
ownership, selection, layers, and undo history. Frontends should mutate a
diagram through `Layout`, not through a copied `ir.Graph`.

## Geometry

- `Point` and `Size` use `uint32`
- `Padding` uses `uint8`
- `Rect` uses half-open bounds `[Min, Max)`
- default padding is one horizontal cell and zero vertical cells
- `Node` stores its rectangle and label origin
- `Port.Anchor` is the boundary cell
- `Port.Exit` is the first outward cell
- `Port.Exit == Port.Anchor` when unsigned coordinates cannot represent the
  neighbor
- `Edge.Points` stores orthogonal route vertices
- `LabelLine` uses `uint32` byte offsets

Automatic nodes grow to their label. Once a caller sets an explicit size, it
remains authoritative until `AutoSizeNode`. Explicit nodes wrap and visually
clip labels without losing source text.

The first port on a side is always usable. Later ports become usable only when
the side can keep one cell between the candidate, corners, and earlier usable
ports. Apply this rule to every offset, including user-defined ports.

## Storage and mutation

`Layout.Nodes`, `Layout.Ports`, and `Layout.Edges` align with the corresponding
IR IDs. Origins, explicit sizes, styles, and selection membership also use
index-aligned slices.

Mutations update semantic and derived state in place. `Build` resolves labels
and routes edges. `WithGraph` clones external initial IR. Exported geometry
slices are read-only by contract.

Deletion preserves tombstones. Draw order contains each live node and edge
exactly once, from back to front. New objects start at the front.

## Routing

`Router` is copyable configuration. `Layout` owns reusable route scratch and a
concrete allocation-free heap.

Default costs provide these reference points:

- `Step = 10`: ordinary path length
- `SharedStep = 2`: common-endpoint shared segments
- `Bend = 5`: direction changes
- `Crossing = 15`: unrelated edge crossings
- `EndpointStep = 40`: travel through source or destination rectangles
- `ReroutePasses = 1`: one extra whole-edge reroute pass

Lower shared-step cost favors trunks. Higher bend, crossing, or endpoint cost
favors straighter, separated, exterior routes.

Only edges with a common endpoint may share segments. Sharing starts where the
common endpoint is nearer than both distinct endpoints. Unrelated edges may
cross, but they may not share a segment or touch another edge's endpoint.

Smart-arrow routes prefer two straight cells before a bend. Keep that clearance
when it does not lengthen the route. Permit shorter clearance when the extra
cell would force a much worse route.

`PreviewRoute` treats a point as a roaming port.
`PreviewRouteWithoutEdge` omits a relocated edge from occupancy. Preview
methods reuse scratch and do not mutate committed geometry or history.

## Raster, hits, selection, and layers

Raster cells store directional connectivity and object ownership. Complete
front nodes occlude unrelated lower geometry. Collinear node boundaries may
merge. Common-endpoint edges retain joined connectivity.

`Layout.Hits` returns visible objects in interaction priority order. It checks
compact edge segments rather than every rendered edge cell.

`Selection` stores node and edge membership. It supports replacement,
toggle, area intersection, component expansion, and whole-document selection.
`DuplicateSelection` copies selected nodes and edges whose two endpoint nodes
are copied.

Rigid selection moves skip routing when static edges cannot be affected.
`BuildSelection` routes only affected geometry. Add safe no-route cases before
optimizing the full router.

## History

`History` records engine mutations as bounded interactions:

- `Commit` records the final state
- `Cancel` restores the initial state
- `Interrupt` commits the latest visible state

Stale transactions return `ErrTransactionClosed`. History retains 256 entries
by default.

Persistent history is gzip-compressed and stored separately from the document.
Cache writes use a 100 ms debounce and atomic replacement. The cache key is the
SHA-256 digest of the normalized absolute document path. Anonymous diagrams do
not persist history until first save. Cache failures never block document
editing or saving. `HistoryStore` makes cache storage replaceable for tests;
use it with `fstest.MapFS` under `testing/synctest`.

## Performance

Reuse `routeScratch`, `draftPorts`, grid buffers, and history storage. Keep
router configuration separate from scratch so callers can replace costs
cheaply. Use `b.Loop`, `go tool pprof`, and allocation benchmarks for routing
and interactive builds.

## Areas for improvement

Alignment snapping belongs here so every frontend can use it. A first
`Snapper` should:

- compare unselected node left, center, right, top, middle, and bottom anchors
- represent centers in doubled integers to avoid floats
- evaluate X and Y independently
- acquire within one cell and release after two cells
- return adjusted signed movement plus guide descriptors
- keep multi-selection geometry rigid
- let Shift bypass snapping in the frontend
- reuse candidate and guide buffers

Start with linear candidate scans. Add sorted indices or a spatial index only
after benchmarks.

Other future work includes portless lines, boxless ports, selection-wide layer
commands, custom compound shapes, and more no-route build cases.

## Verification

Use property tests for geometry bounds, port usability, transaction drift,
undo and redo, layer invariants, selection, and snapping. Keep focused examples
for route aesthetics and raster junctions. Benchmark layout builds, routing,
components, hits, and history cache behavior.
