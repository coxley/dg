# IR package guide

## Responsibility

`ir` stores diagram semantics. It does not know positions, dimensions, routes,
styles, layers, history, or terminal glyphs.

## Core data

- `Graph.Nodes`, `Graph.Ports`, and `Graph.Edges` use their slice indices as
  IDs
- `Node` stores a label and ordered port IDs
- `Port` stores its node ID, side, and normalized offset in `[0, 1]`
- `Edge` stores two port IDs and is undirected
- `Components` stores reusable union-find parents and ranks

## Invariants

- deletion leaves tombstones and per-type free lists reuse their IDs
- live IDs do not shift when another object is deleted
- node deletion removes its ports and incident edges
- duplicate port pairs return the existing edge ID
- a port cannot connect to itself
- `Node.Empty`, `Port.Empty`, and `Edge.Empty` define tombstone semantics
- graph mutation must keep every node's ordered port list consistent

`NewNode` creates ports at offsets `.5`, `.25`, and `.75` on each side.
Stored order defines connection preference. Use `NewNodeWithPorts` for custom
definitions.

`Components.Build` reuses storage. Rebuild it after graph mutations before
querying connected components.

## Performance

Keep IDs as array indices. Reuse tombstoned node port capacity with
`slices.Grow`. Do not add maps or compaction until measurements justify them.
If compaction becomes necessary, compact tombstones in one bulk pass and
return an explicit ID remap.

## Areas for improvement

- portless edges require a new semantic endpoint representation
- boxless ports may need a graph-owned anchor object
- valid zero-value nodes would require explicit tombstone membership
- large graphs may justify a duplicate-edge index, but current lookup is
  linear

## Verification

Property tests should cover arbitrary create, delete, reconnect, and ID reuse
sequences. Benchmark `Components.Build` and mutation-heavy slot reuse.
