# Store package guide

## Responsibility

`store` persists named canvases, durable drafts, and disposable history blobs.
It owns gzip encoding, filesystem revisions, atomic replacement, draft
promotion recovery, and the compressed warm cache. It imports `document` and
does not import `layout`, `history`, or frontend packages.

## Records

Named entries use `(Section, Name)` identity below the preferred directory.
Draft entries use their document UUID below the application state directory.
Names and sections contain one path component; sections are optional and only
one level deep. `Entry.Revision` guards replacement, movement, and deletion
against unseen external writes.

`.dg` files contain exactly one gzip member with versioned document JSON.
Decode rejects additional members and JSON larger than `64 << 20` bytes.
Writes create a same-directory temporary file before link or rename. Creation
never replaces an existing name.

Catalog discovery atomically rewrites supported legacy documents in place and
returns the resulting revision. Imported files migrate only in the new draft;
Store never changes the external source. Migration writes use the same revision
check, warm-cache replacement, and self-authored watcher marker as ordinary
Store writes.

Conflict resolution reads the current path without trusting a stale revision.
Keeping local content hard-links the current raw bytes to the first available
`.bak`, `.bak1`, and later name, removes the original, then atomically recreates
it from the local document. A failed recreation leaves the backup intact.
Deleted named records can only be restored without replacing a path that has
reappeared. Draft preservation replaces the UUID-keyed durable draft.

Draft naming writes the named record before deleting the draft. A promotion
journal removes a duplicate draft after restart when the named write completed.
If the named write did not complete, recovery preserves the draft.
Demotion writes the draft before deleting the named record. A demotion journal
finishes removing an unchanged named record after restart when the draft write
completed.
`Import` copies a compressed external document into Drafts while preserving its
UUID and leaving the source untouched.

## Warm data and history

The warm cache retains immutable compressed values with both a five-entry and
`16 << 20`-byte limit by default. It does not admit a value larger than the
byte budget. Store returns independently decoded documents, so callers never
share mutable slices with cached data or another load.

`History` returns a name-keyed blob adapter compatible with the cache interface
declared by package `history`. Blob data lives below the disposable cache
directory, not beside portable canvas files.

## Catalog

Catalog scans use `*.dg` and `*/*.dg` against the preferred directory plus the
durable drafts directory. `Watch` observes the preferred root, direct section
directories, and drafts with fsnotify. Events only invalidate state; a
debounced full scan remains authoritative. `Reconcile` provides the same scan
for focus recovery. Catalog events distinguish Store-authored revisions from
external changes and report watcher, scan, and closure states explicitly.
Store-authored revision markers remain valid across repeated reconciliations
and clear only after the path presents a different revision.

## Verification

Test revision conflicts, name collisions, draft promotion recovery, one-level
scanning, watcher bursts and dynamic sections, gzip bounds, returned-value
ownership, LRU eviction, and concurrent creation. Benchmark both warm and cold
large-document loads and catalog scans.
