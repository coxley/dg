# Canvas Store Implementation Plan

Status: accepted for implementation

This plan introduces named canvases, durable drafts, fast canvas switching,
autosave, external-change handling, and disposable per-canvas undo persistence.
It describes planned behavior. Package guides must describe only behavior that
has landed.

## Product model

Users work with canvas names, not file paths. Named canvases happen to be
stored as `.dg` files below a preferred directory. Unnamed canvases are drafts
that survive application restarts.

- Every active canvas autosaves after 500 ms without a completed change.
- Switching, normal quit, SIGINT, and SIGTERM synchronously flush dirty state.
- A top-level panic performs a bounded best-effort flush and then re-panics.
- SIGKILL and power-loss durability are out of scope.
- Ctrl-S opens **Name Canvas** for a draft and reports **Autosaved** for a
  named canvas.
- Ordinary canvas switches do not participate in undo or redo.
- Undo and redo always operate on the active canvas.

## Package boundaries

Dependencies flow in one direction:

```text
document -> layout
history  -> layout, document
store    -> document
tui      -> layout, document, history, store
cmd/dg   -> tui, store
```

More precisely:

- `layout` owns editable state, geometry, reversible mutation descriptions,
  and atomic state replacement. It does not import `document` or `history`.
- `document` imports `layout` and owns the portable JSON schema plus layout
  conversion.
- `history` imports `layout` and `document`. It owns transactions, undo/redo,
  external-reload boundaries, cache encoding, and cache dirtiness.
- `store` imports `document`. It owns canvas records, gzip encoding, files,
  drafts, catalog reconciliation, revision checks, and compressed warm data.
  It provides blob storage satisfying the small cache interface declared by
  `history`; `store` does not import `history`.
- `internal/tui` owns the active layout, active history, autosave scheduling,
  canvas switching, prompts, and presentation.
- `cmd/dg` owns process startup, signal handling, and final cleanup.

`document` does not own history. A shared `.dg` never bundles undo data.

## Document format and identity

- `.dg` is one gzip member containing JSON for `document.Document`.
- The next schema version is version 2 and requires a UUIDv4.
- Generate new IDs with `github.com/google/uuid.New`, not `uuid.NewUUID`.
- Importing or discovering a `.dg` preserves its UUID.
- Naming a draft and renaming or moving a named canvas preserve its UUID.
- Creating an independent canvas duplication generates a new UUID.
- Copies and backups may contain the same UUID. This is valid.
- Version 1 and uncompressed legacy files are rejected. The project is in
  development; do not add format readers or public compatibility wrappers.
- The filename is the authoritative display name. Do not duplicate the title
  in `document.Document`.

The document API changes in place:

```go
func document.New(source *layout.Layout) document.Document
func (*document.Document) Update(source *layout.Layout)
func (*document.Document) Convert(options ...layout.Option) (*layout.Layout, error)
func (*document.Document) ConvertInto(dst *layout.Layout, options ...layout.Option) error
```

Remove `document.FromLayout`, `Document.Layout`, and layout-taking marshal
helpers. `New` creates a fresh UUID. `Update` snapshots an edited Layout while
preserving the document UUID and reusing document slice capacity; autosave,
naming, renaming, and moving use this path. `ConvertInto` preserves the
destination pointer, retained capacities, and installed layout change
callback. It clears transient selection and leaves the destination unchanged
when validation or geometry construction fails.

## Reversible layout changes and history

`layout.Layout` exposes one concrete reversible change stream. A successful
semantic mutation invokes an installed callback with an immutable,
value-owned `layout.Change`. `Change` and `layout.Snapshot` keep their fields
private and own their JSON codecs. Builds, previews, and selection-only
changes do not emit history changes. Each successful semantic mutation emits
one reversible change. A failed atomic mutation emits none; a composite API
may emit earlier successful changes only when its existing contract permits
partial success. Test every currently atomic composite error path for unchanged
state and zero callbacks.

The engine surface is intentionally narrow:

```go
type layout.Change struct { /* opaque */ }
type layout.Snapshot struct { /* opaque */ }
type layout.ChangeCallback func(layout.Change)
type layout.ReplayDirection uint8

func (*layout.Layout) SetChangeCallback(layout.ChangeCallback) error
func (*layout.Layout) Replay([]layout.Change, layout.ReplayDirection) error
func (*layout.Layout) Snapshot() layout.Snapshot
func (*layout.Layout) Restore(layout.Snapshot) error
func layout.CoalesceChanges([]layout.Change, layout.Change) ([]layout.Change, bool)
```

`SetChangeCallback` permits one observer. Installing a second non-nil callback
returns an attachment error; passing nil detaches the current callback.
`history.New` installs its callback on an existing Layout and fails if another
observer already owns it.

`Replay` suppresses callbacks, applies one complete history entry, builds once,
and restores its prior snapshot on failure. Exact node and edge slot recovery
remains private to `layout`, preserving tombstones and stable IDs. Change
construction gives the callback value-owned slice payloads, cloning once at
the producer when necessary.

`history.History` installs the callback and retains the `*layout.Layout`:

- transactions aggregate emitted changes;
- replay ignores callback events rather than recording them again;
- stale transactions fail after interruption or layout replacement;
- ordinary entries retain the existing 256-interaction limit;
- converting another document into the same layout keeps History attached;
- resetting for an ordinary canvas switch replaces History contents without
  replacing the History pointer.

History owns layout-replacement sequencing:

```go
func (*history.History) Reset(replace func() error) error
func (*history.History) Reload(replace func() error) error
```

`Reset` performs an ordinary canvas replacement and, only after success,
invalidates transactions and replaces the history baseline. `Reload` performs
the same atomic replacement but inserts the whole-document undo boundary.
Callers must not sequence `ConvertInto` and history clearing independently.

An accepted external reload creates one whole-document boundary among normal
entries. It must support this exact chain:

1. Load the modified document.
2. Make a normal edit.
3. Undo the edit to reach the modified document.
4. Undo the reload to reach the original document.
5. Redo the reload to reach the modified document.
6. Redo the edit to reach the modified document plus the edit.

Keep at most one external-reload boundary per canvas. In-memory undoability is
required. Persist the boundary only if it does not materially complicate the
cache representation; losing cached history may lose reload undo but never
document data.

Before accepting a second reload, discard the redo branch and every entry
through the prior reload boundary. Retain only the compatible ordinary suffix
on the currently rendered side, then append the new boundary. If reload
boundaries remain memory-only, cache only ordinary history on the current side
of the boundary; never serialize entries whose baseline belongs to the other
document state.

A replacement at the same filename containing another UUID represents a
different canvas, not an editable revision of the active one. Preserve the old
active canvas as a draft and reconcile the replacement as the named canvas.
Do not create an undo boundary spanning two document identities.

## History cache

History remains disposable under the platform cache directory.

- Cache filename/key: document UUID only.
- Cache header guard: CRC32 IEEE over canonical document JSON.
- The guard is an accidental mismatch/corruption check, not a security claim.
- Replace both current SHA-256 history digests with CRC32.
- Bump the cache version; do not retain cache compatibility.
- A guard mismatch discards cached history instead of applying it.
- Duplicate UUID files may overwrite the same cache file; the guard prevents
  applying history to divergent content.
- Identical copies can intentionally share compatible cached history. Cache
  isolation is best-effort; divergent copies lose or replace cached history
  rather than receiving incompatible entries.
- `History.Dirty` compares current cache generation with the last successful
  flush generation.
- Lifecycle boundaries use:

  ```go
  if history.Dirty() {
      history.Flush()
  }
  ```

Clean switches must not serialize, compress, or write history again.

## Store contract

`store` is a public, synchronous, record-oriented package. It accepts and
returns `document.Document` values. Synchronous encoding gives Store ownership
of submitted mutable slices before the call returns. Watch notifications are
asynchronous and expose errors and closure explicitly.

`Entry` directly identifies a record; there is no opaque `EntryRef`:

- named identity: `(Section, Name)`;
- draft identity: UUID;
- `Name` excludes `.dg`;
- `Section` is empty for root canvases and drafts;
- entries also expose document UUID, modification time, and an observed
  revision token used for compare-and-swap writes.

Store operations return an updated Entry after writes, naming, renames, and
moves. A removed or stale location returns `ErrEntryNotFound`. A conflicting
name in the same section returns `ErrEntryExists`; the TUI requires another
name. Identical names in different sections are valid.

Names and sections are single path components. Reject separators, absolute
paths, `.`, `..`, and deeper paths.

All writes use a temporary file and atomic rename. Before replacement, compare
the current raw-file revision with the Entry revision. A mismatch returns an
external-modification result instead of overwriting unseen changes. Serialize
autosave, switching, conflict resolution, and shutdown writes through one TUI
coordinator.

## Named canvases and sections

The preferred save directory contains root canvases and at most one level of
sections:

```text
Architecture.dg
Databases.dg
Interviews/Candidate 1.dg
Interviews/Candidate 2.dg
RFCs/Proposal 1.dg
```

Reconciliation uses these explicit patterns:

```go
fs.Glob(os.DirFS(preferred), "*.dg")
fs.Glob(os.DirFS(preferred), "*/*.dg")
```

Ignore deeper `.dg` files. Only sections containing `.dg` files appear.

`dg /outside/path/file.dg` imports the document into Drafts, preserves its
UUID, and never changes the source file.

## Drafts

Drafts live in durable application state, never the OS cache directory.
Resolve the platform state location separately from history cache storage.
Draft filenames may use UUIDs because users never manage them directly.

- Drafts survive restart.
- Draft rows sort newest first and display only contextual modification time.
- Users may delete one draft or choose **Clear Drafts...**.
- Bulk clearing always excludes the active draft.
- Confirm the exact count, for example **Delete 15 canvases?**.
- Naming writes the named document before deleting the draft.
- A small promotion journal recovers a crash between those operations without
  losing the draft or repeatedly presenting both records.

## Compressed warm cache

Store retains recent encoded records in an LRU:

- at most 5 entries;
- at most `16 << 20` compressed bytes total;
- count compressed document, history, and external-reload snapshots;
- exclude catalog metadata and indexes;
- do not admit one entry larger than the byte budget;
- admit only immutable, successfully encoded clean blobs; dirty state remains
  owned by the TUI until Store accepts it;
- keep the active decoded layout outside the compressed-byte budget.

Measure cold and warm switching. Reusing `Layout` capacity is a performance
goal, not a promise of literally zero allocations.

## Watching and reconciliation

Use `github.com/fsnotify/fsnotify` as an invalidation signal, not the catalog
source of truth.

- Watch the preferred root and each direct subdirectory.
- Add and remove direct-subdirectory watches as directories change.
- Debounce event bursts into a full glob reconciliation.
- Reconcile at startup, after watcher overflow/error, and when the application
  regains focus.
- Suppress conflicts from Store's own successfully committed revision.
- Coalesce repeated external changes while a prompt is open to the latest raw
  revision.
- Inactive changes refresh catalog metadata and invalidate warm content.

## External changes and backups

An externally modified active canvas prompts:

> `<title> has been externally modified; load it?`

If accepted, load it through the single external-reload history boundary.

If declined:

1. Under Store's write lock, choose `title.bak.dg`, then `title.bak1.dg`, and
   so on without racing another writer.
2. Rename the exact raw external bytes; do not decode and re-encode them.
3. Atomically restore the active local document at the original path.

Backup suffixes are not reserved. A user-created `.bak` name naturally gets a
backup pill, and Store selects the next available generated suffix.

An externally deleted active canvas prompts:

> `<title> was externally deleted; restore it?`

Accepting recreates the named document atomically. Declining preserves the
local document as the active draft.

## Sidebar

The sidebar has separate **Canvases** and **Drafts** tabs.

Canvases shows root entries followed by collapsible, non-empty sections.
Drafts shows modification times, individual deletion, and bulk clear. Focus
contains actionable visible rows only and repairs itself after collapse,
deletion, reconciliation, or tab changes.

Width is content-driven:

- minimum width: 30 cells;
- measure tabs, every known entry, section indentation, backup pills, and
  action rows so collapse does not resize the canvas;
- preserve at least 48 canvas cells when docked;
- use drawer placement when the desired width cannot dock;
- clamp only when content exceeds the terminal itself;
- retarget width through the existing workspace animation.

## Implementation phases

### Phase 1: history package boundary

Status: complete.

- Add the reversible `layout.Change` callback and replay surface.
- Move history transactions and tests into public `history`.
- Remove `Layout.history`, `WithHistory`, `Layout.History`, and layout-owned
  cache APIs without wrappers.
- Migrate TUI, command, document, and tests to explicit `*history.History`.
- Keep the existing path-keyed cache behavior in `history`; encode Layout's
  runtime snapshot and change values directly and bump the disposable cache
  version instead of maintaining parallel wire structs.
- Add `history/AGENTS.md` and update the root/layout guides to describe the
  package boundary only after the extraction works.
- Gate on cache store/restore round trips in addition to transaction replay.
- Keep the repository green at the phase gate.

### Phase 2: reusable layout conversion

Status: complete.

- Add atomic Layout state replacement while preserving callbacks and capacity.
- Add document schema v2 UUIDs.
- Cut over to `New`, `Convert`, and `ConvertInto`.
- Add equivalence, rollback, reuse, and allocation benchmarks.

### Phase 3: UUID history cache

Status: complete.

- Introduce UUID keys, CRC32 guards, cache version bump, and `Dirty`.
- Add the one external-reload boundary.
- Verify history isolation, mismatch rejection, and the full undo/redo chain.
- Verify repeated reload replacement and memory-only boundary cache filtering.

### Phase 4: Store core

Status: complete.

- Add `.dg` codec, Entry CRUD, revision-checked atomic writes, naming, moves,
  backups, drafts, promotion recovery, and history blob storage.
- Add the compressed LRU and scan/switch benchmarks.
- Return independently owned documents with no aliases into cached values.
- Reject concatenated gzip members and decompressed JSON larger than
  `64 << 20` bytes.
- Test race-free create/name collisions as well as backup suffix allocation.
- Add `store/AGENTS.md` describing only implemented behavior.

### Phase 5: catalog watching

- Add the two glob patterns, one-level sections, fsnotify invalidation, and
  reconciliation.
- Test newly created/deleted directories, event bursts, watcher errors,
  corrupt records, self-write suppression, and external-write races.

### Phase 6: TUI switching and autosave

- Replace path-centric state with an active Entry.
- Add reusable-layout switching and independent per-canvas history restore.
- Add 500 ms autosave and deterministic lifecycle flushes.
- Import external CLI paths into Drafts.

### Phase 7: sidebar and naming

- Refactor flat sidebar items into semantic tab, section, record, and action
  rows.
- Add content-driven sizing, Canvases/Drafts, naming, collapse, deletion, and
  bulk-clear confirmation.
- Preserve dock, drawer, keyboard, mouse, focus, and viewport behavior.
- Test docking at `desiredWidth + 48`, drawer placement one cell below, wide
  Unicode names, terminal-width overflow, stable width across tabs/collapse,
  reconciliation during width animation, focus repair, and 1,000-row cost.

### Phase 8: external-conflict workflow

- Add modification/deletion prompts, raw backup generation, reload undo, and
  draft preservation.
- Test malformed replacements, repeated events, different-UUID replacement as
  a new named canvas, backup allocation races, and failures between backup and
  restore.

### Phase 9: lifecycle and final integration

- Expose a TUI flush request that marshals onto the owner event loop and
  acknowledges completion after the serialized write coordinator drains.
- Wire normal quit, switch, SIGINT, and SIGTERM through that request. A second
  signal exits immediately.
- Give panic cleanup two seconds to join an in-flight write and flush; report
  cleanup errors without replacing the original panic, then re-panic.
- Update root and package architecture guides to the behavior now present.
- Run the complete verification gate.

## Verification gates

Each phase receives focused tests and an independent review before proceeding.
The final gate runs:

```sh
GOCACHE=/private/tmp/dg-codex-go-build go test ./...
GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...
GOCACHE=/private/tmp/dg-codex-go-build go vet ./...
GOCACHE=/private/tmp/dg-codex-go-build \
  GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache \
  golangci-lint run --path-mode abs
```

Performance gates include:

- `Convert` versus alternating and steady-state `ConvertInto`, with `-benchmem`;
- Store gzip encode/decode and catalogs with 10, 100, and 1,000 entries;
- cold and warm canvas switching on the existing 200-cluster fixture;
- autosave encoding and compression latency;
- all existing `BenchmarkModel` and layout stress/high-water benchmarks;
- CPU and memory profiles when a benchmark regresses.

Visual gates use headless-terminal at `100x30`, `80x16`, and `80x12`, covering
sidebar tabs and sizing, naming, draft deletion, conflict prompts, focus,
cursor visibility, and canvas alignment.
