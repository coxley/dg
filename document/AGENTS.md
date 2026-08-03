# Document package guide

## Responsibility

`document` defines the versioned JSON schema and maps it to and from
`layout.Layout`. It is a persistence boundary, not a runtime model.

## Persisted data

`Document` stores:

- a required UUIDv4 identity
- live nodes, ordered ports, and edges
- node origins and optional explicit sizes
- node border, stroke, and text alignment
- edge stroke and endpoint arrows
- ordered pinned edge bends
- landmark-relative edge attachments
- node padding and router configuration
- back-to-front layers

The zero value of optional style fields represents the runtime default.
`CurrentVersion` is the only supported schema version.

## Conversion rules

`New` exports a Layout with a fresh identity. `Update` preserves identity while
reusing document capacity. Export compacts runtime tombstones and remaps every
node, port, edge, and layer reference.

`Unmarshal` decodes once and validates the complete document. `UnmarshalInto`
reuses top-level and nested slice capacity; callers must treat its destination
as undefined after an error.

`Convert` creates an independent Layout. `ConvertInto` atomically replaces an
existing Layout through its retained staging state, preserving the Layout
pointer and change callback. Import validates enum values, offsets, IDs, and
layers and clears transient selection on success.

Persist routing constraints, but not computed routes, raster cells, selection,
free lists, geometry scratch, history, or frontend state. Layout rebuilds
derived values.

Schema structs intentionally mirror JSON rather than runtime storage. Keep
conversion explicit so schema evolution does not leak into engine types.

## Compatibility

Version 3 is the only accepted schema. Reject every other version with
`ErrUnsupportedVersion`; the project does not retain legacy readers yet.

When adding runtime capabilities:

- persist them only when users expect them to survive save and load
- define stable string enums instead of numeric implementation values
- add round-trip and invalid-input tests
- verify tombstone compaction and layer remapping

## Areas for improvement

Future schema work may include custom shapes, portless endpoints, document
metadata, and migration between versions. Persistent undo history remains a
separate gzip cache.

## Verification

Use property tests with entropy across every node, port, edge, label, style,
layer, and router setting. Retain a small set of readable JSON examples for
schema shape, unsupported versions, and validation failures.
