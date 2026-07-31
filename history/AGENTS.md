# History package guide

## Responsibility

`history` records reversible `layout.Layout` mutations as bounded undo and redo
interactions. It owns transactions, cursor state, coalescing policy, and the
disposable on-disk history cache. It does not own document files or geometry.

## Layout boundary

`History` attaches to one existing Layout through its sole change callback.
The Layout emits opaque, value-owned `layout.Change` values after successful
semantic mutations. Layout retains exact-slot replay and snapshot restoration
so history never depends on private graph, tombstone, or geometry fields.

Only one callback may attach to a Layout. `New` returns `ErrAttached` when the
Layout already has one. `History.Layout` returns the attached pointer for
ownership checks; it does not transfer History to another Layout.

`Reset` wraps whole-layout replacement. It suppresses emitted changes, restores
the prior snapshot on failure, and clears entries only after success. Callers
must not replace an attached Layout and clear History as separate operations.

## Transactions

- `Begin` commits an interrupted transaction and starts a new interaction.
- `Commit` records the final state as one entry.
- `Cancel` replays the active entry backward and records nothing.
- `Interrupt` commits the latest visible state.
- stale transactions return `ErrTransactionClosed`.

History retains 256 entries by default. `WithLimit` changes that bound.
Coalescing belongs to Layout because `layout.Change` remains opaque.

## Cache

Persistent history remains separate from the document. The current cache:

- uses the SHA-256 digest of the normalized absolute document path as its key;
- stores gzip-compressed version 4 JSON;
- serializes Layout's runtime snapshot and change values without parallel
  cache-only representations;
- validates the saved semantic Layout digest before restore;
- writes after a 100 ms debounce and uses atomic replacement;
- treats missing, malformed, or incompatible cache data as disposable;
- never blocks document editing or saving after an asynchronous failure.

`Store` abstracts cache bytes for tests. Use `testing/synctest` for debounce and
stale-write behavior. UUID keys and CRC32 guards belong to the later Store
migration; do not describe them here until implemented.

## Verification

Test transaction closure, coalescing, exact-ID node and edge replay, attachment
and layer restoration, failed replay rollback, cache branch invalidation,
debounced writes, stale generations, and cache corruption. Keep layout-private
state assertions in `layout`; exercise public undo behavior here.
