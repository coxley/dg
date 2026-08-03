# Layout package guide

## Responsibility

`layout` owns editable IR, terminal-cell geometry, orthogonal routing, raster
ownership, selection, layers, and reversible mutation descriptions. Frontends
should mutate a diagram through `Layout`, not through a copied `ir.Graph`.

## Geometry

- `Point` and `Size` use `uint32`
- `Padding` uses `uint8`
- `Rect` uses half-open bounds `[Min, Max)`
- node padding cycles among none, one horizontal cell, and one extra cell on
  every side
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
IR IDs. Origins, explicit sizes, styles including padding, pinned edge bends, and selection
membership also use index-aligned slices.

Mutations update semantic and derived state in place. `Build` resolves labels
and routes edges. `WithGraph` clones external initial IR. Exported geometry
slices are read-only by contract.

Deletion preserves tombstones. Draw order contains each live node and edge
exactly once, from back to front. New objects start at the front.

## Attachments

`Layout` owns attachment geometry; `ir.Graph` remains unaware of it.
`Layout.attachments` aligns with node IDs, and the zero `Attachment` means the
node is detached. Do not add a parallel presence slice or presence bit.

`Attachment` contains:

- `NodeID` and `EdgeID`, which reference live layout objects
- `Position`, a `uint16` normalized distance along the host's complete
  Manhattan route
- `Anchor`, the attached edge cell's offset from the node origin

Valid positions lie strictly between zero and `math.MaxUint16`. Endpoint
positions would overlap a connecting node; rejecting them also makes the zero
value unambiguously detached, including for node and edge ID zero. `Anchor`
preserves the exact drag alignment. Node dimensions alone could only derive a
canonical alignment such as centering and would make off-center drops jump.

`AttachNode` derives `Position` and `Anchor` from a routed cell inside the node.
`SetAttachment` updates one relationship, while `SetAttachments` restores a
batch before the first route. Both build atomically. `DetachNode` removes the
relationship and keeps the current absolute origin.

Attachment builds alternate routing and attached-node placement until geometry
stabilizes. The host edge ignores its attached nodes as obstacles; every other
edge still routes around them. A node cannot attach to an incident edge.
Dependency cycles that do not stabilize within the bounded build passes fail,
and reusable rollback snapshots restore the previous buildable geometry.

Clone, change replay, translation, deletion, and tombstone reuse must preserve
these invariants. Deleting a host edge detaches its nodes in place.
Deleting a node also removes attachments hosted by its incident edges.
Duplication copies a relationship only when both the attached node and host
edge are copied.

`Fragment` owns a portable selected subgraph. `CopySelection` includes edges
only when both endpoint nodes are selected and applies the same rule to
attachments. `Paste` roots the fragment bounds at a destination point,
selects the new objects, and leaves route computation to the next build.

Selecting an edge first selects its attached nodes too. Selecting the same edge
again leaves only the edge selected. Raster ownership remains authoritative:
an edge passing under an attached node must not claim or highlight the node's
label cells.

## Routing

`Router` is copyable configuration. `Layout` owns reusable route scratch and a
concrete allocation-free heap.

### Route pipeline

`Build`, `BuildSelection`, and preview routing share the same search machinery.
They differ in which committed routes seed occupancy and which results they
commit.

Routing proceeds in this order:

1. Reset retained scratch, build the node obstacle index, and mark ports that
   need smart-arrow clearance.
2. Seed route occupancy. A full build starts empty and adds edges in ID order.
   A selection build first adds unrelated committed edges. A preview adds every
   committed edge except its explicit exclusion.
3. Split a constrained edge at each ordered `PinnedBend`. Virtual ports force
   the specified incoming and outgoing unit segments at every pinned point.
4. Derive endpoint-node exemptions and finite search bounds for each part.
5. Run A* from `Port.Exit` to `Port.Exit`. Each state contains a point and the
   arrival direction because bend cost depends on direction.
6. Reject moves that violate arrow clearance, coordinate bounds, node
   obstacles, sharing rules, or edge-touch rules.
7. Score legal moves by step, sharing, bend, crossing, and endpoint-node costs.
8. Reconstruct each winning route part as a cell-by-cell path, join the parts
   at their pinned points, and add the complete route to
   occupancy.
9. During a full build, align earlier common-port routes with trunks introduced
   by later siblings, then reconsider crossing edges for at most
   `ReroutePasses`.
10. Compact committed routes to endpoints and bend vertices in `Edge.Points`.

Occupancy always uses expanded, cell-by-cell paths. Committed routes remain
compact. Expand a committed route before adding it to occupancy, including
routes preserved by `BuildSelection`.

Full builds route live edges in stable ID order. The queue breaks equal
priorities by crossing count and insertion order. These rules make route shapes
deterministic, so heuristic changes can affect visible geometry even when they
preserve total cost.

### Search costs and heuristic

Default costs provide these reference points:

- `Step = 10`: ordinary path length
- `SharedStep = 2`: common-endpoint shared segments
- `Bend = 5`: direction changes
- `Crossing = 15`: unrelated edge crossings
- `EndpointStep = 40`: travel through source or destination rectangles
- `ReroutePasses = 1`: align shared branches and run one crossing reroute pass

Lower shared-step cost favors trunks. Higher bend, crossing, or endpoint cost
favors straighter, separated, exterior routes.

Only edges with a common endpoint may share segments. Sharing starts where the
common endpoint is nearer than both distinct endpoints. Unrelated edges may
cross, but they may not share a segment or touch another edge's endpoint.
After the initial ID-ordered pass, the router aligns an earlier sibling's first
branch with a later sibling's branch when moving the segment preserves route
legality and score. This local refinement produces one clean junction without
running A* again.

The A* heuristic is Manhattan distance multiplied by the cheapest step the
current route can take:

- use `Step` when no occupied edge shares either exact endpoint port
- use `min(Step, SharedStep)` when sharing is possible

This is a lower bound. It excludes bend, crossing, endpoint, and obstacle
penalties. Add a term only when it remains a lower bound for every route-specific
exemption and occupancy state. Also run route and renderer snapshots because a
stronger admissible heuristic can change equal-cost route shapes.

Landmark or differential distances must use a graph relaxation that every edge
may traverse. Node obstacles are route-specific because endpoints and host-edge
attachments receive exemptions. Only explore landmarks when queue and
search-map work dominates after the current bounds and minimum-step checks.

Smart-arrow routes prefer two straight cells before a bend. Keep that clearance
when it does not lengthen the route. Permit shorter clearance when the extra
cell would force a much worse route. The router first searches with full
clearance. It then searches with relaxed clearance when needed and keeps the
lower-scoring valid result.

`PreviewRoute` treats a point as a roaming port.
`PreviewRouteWithoutEdge` omits a relocated edge from occupancy. Preview
methods reuse scratch and do not mutate committed geometry or history. A
coordinate index resolves a cursor on a usable port without scanning the
ID-aligned port slice; overlapping ports retain the lowest live ID.

`PinnedBend` is a hard, ordered route constraint. Its incoming and outgoing
directions must be perpendicular. `PreviewPinnedBends` checks a draft without
mutating the edge. Pins translate with the complete layout and with duplicated
or rigidly moved components. Moving only one endpoint keeps pins fixed; reject
the operation when no route can satisfy them. Clearing all pins restores fully
automatic routing.

### Performance controls and fallbacks

Use the existing controls before adding another index or cache:

- `BuildSelection` limits routing to selected edges, edges incident to selected
  nodes, and exact-port siblings needed to preserve shared branch geometry
- `BuildSelection` preserves a selected internal route when it remains clear
  and legal against static occupancy
- rigid selection moves skip routing when static geometry cannot be affected
- `PreviewRouteWithoutEdge` excludes the edge being relocated
- `ReroutePasses` bounds optional shared-branch and crossing-improvement work
- `routeScratch` retains maps, slices, paths, and the concrete heap across
  builds
- the obstacle index groups ordinary nodes into 16 by 16 cell buckets
- nodes that cover more than 64 buckets stay in a direct-scan list, which
  prevents unusually large nodes from expanding the index without bound
- route occupancy tracks active edge IDs so the heuristic only assumes
  `SharedStep` when exact-port sharing is possible
- the usable-port coordinate index keeps preview destination lookup independent
  of live and tombstoned port counts

Profile `BenchmarkLayoutStress` before changing these controls. Its 200
three-node clusters exercise preview and commit, selection movement,
attachment, label editing, CPU, allocations, and retained live bytes.
`BenchmarkLayoutHighWaterConnect` compares a fresh two-node layout with the
same scene after deleting a 600-node high-water workload.

Use profile children to choose the next change:

- high `blockedForRoute` or `nodeBlocksRoute` means obstacle lookup dominates;
  inspect bucket density, large-node count, and node dimensions
- high `stepCostFor` or occupancy map access means dense edge interaction,
  sharing, touching, or crossing checks dominate
- high route queue or search-map time means A* visits too many states; inspect
  bounds and the route-aware minimum step before adding a stronger heuristic
- high `addCompact` or `appendExpandedPath` means partial builds spend time
  expanding preserved routes; measure the live-byte cost before caching
  expanded paths
- high `routeBounds` means the per-edge obstacle bounds scan has become visible
- new allocations usually mean retained scratch stopped covering a hot path;
  check both `B/op` and live bytes before growing scratch further

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

## Reversible changes

Layout emits an opaque, value-owned `Change` after each successful semantic
mutation. One callback may attach at a time. Builds, previews, and selection
changes emit nothing.

Each internal change stores one shared before/after state shape. Change and
Snapshot JSON encode those runtime values directly; validation remains
separate from representation.

`Replay` applies a complete change entry forward or backward, builds once, and
restores its prior `Snapshot` on failure. Exact-slot restoration remains
private so node, port, and edge tombstones retain stable IDs. `Restore`
atomically replaces semantic state from an opaque Snapshot. Replay and restore
suppress change callbacks.

The `history` package owns transactions, undo/redo cursors, coalescing use, and
cache persistence. Layout must not import it.

`Replace` configures a retained staging Layout and swaps it into the receiver
only after configuration succeeds. It preserves the receiver pointer and
change callback, clears selection, and keeps the previous state as the next
staging buffer.

## Performance

Reuse `routeScratch`, `draftPorts`, grid buffers, and change storage. Keep
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
change replay, layer invariants, selection, and snapping. Keep focused examples
for route aesthetics and raster junctions. Benchmark layout builds, routing,
components, hits, and change callback behavior.
