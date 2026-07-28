# Document package guide

## Responsibility

`document` defines the versioned JSON schema and maps it to and from
`layout.Layout`. It is a persistence boundary, not a runtime model.

## Persisted data

`Document` stores:

- live nodes, ordered ports, and edges
- node origins and optional explicit sizes
- node border, stroke, and text alignment
- edge stroke and endpoint arrows
- symmetric padding and router configuration
- back-to-front layers

The zero value of optional style fields represents the runtime default.
`CurrentVersion` is the only supported schema version.

## Conversion rules

Export compacts runtime tombstones and remaps every node, port, edge, and layer
reference. Import recreates independent runtime slices and validates enum
values, offsets, IDs, and layers.

Do not persist routes, raster cells, selection, free lists, geometry scratch,
history, or frontend state. Layout rebuilds those values.

Schema structs intentionally mirror JSON rather than runtime storage. Keep
conversion explicit so schema evolution does not leak into engine types.

## Compatibility

Never reinterpret an existing field silently. Add a new document version when
a change cannot preserve old meaning. Keep old readers deterministic: reject
unsupported versions with `ErrUnsupportedVersion`.

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
