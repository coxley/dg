# Development reload

## Goal

Add `dg dev [path]`, which rebuilds the editor after source changes and replaces
the running child only when compilation succeeds. Successful reloads preserve
the active canvas and semantic TUI state without persisting renderer caches or
partially applied pointer gestures.

## Behavior

- `dg [path]` keeps its current behavior.
- `dg dev [path]` watches Go source, `go.mod`, and `go.sum` below the enclosing
  module root.
- Source invalidations use a 200 ms quiet-period debounce.
- The supervisor builds into a private directory below the user cache.
- An initial build failure returns its diagnostics because no editor is
  running. Later build failures silently leave the current editor untouched.
- A successful build atomically replaces the staged executable and asks the
  child to create a development-session handoff.
- The child exits normally after writing the handoff. The supervisor starts the
  replacement with the same terminal and command arguments.
- The replacement consumes and removes the handoff before rendering.

The build-failure branch contains one comment identifying where diagnostics
could be surfaced later. It must not retain, print, or display compiler output
during the normal reload loop.

## Session boundary

The handoff stores semantic state:

- the current `document.Document`, active Store UUID, and whether autosave is
  still required;
- cursor, viewport, selection, active hit, active tool, and creation styles;
- sidebar tab, focus, scroll, width, and collapsed sections;
- help visibility, placement, size, and scroll;
- an open Preferences form's baseline, draft, focus, and default-router state.

The handoff excludes workspace plans, hits, routes, raster frames, rendered
strings, clipboard data, hover state, notices, confirmations, export state, and
pointer-preview caches. The replacement derives those values normally.

Before capture:

- cancel active pointer transactions and discard their previews;
- commit active label editing;
- close Save, Export, Confirmation, and Notice dialogs;
- for an open Preferences form, capture its values and cancel its live
  transaction before taking the document snapshot.

On restore, reopen Preferences and reapply its draft inside a fresh transaction
so Save and Cancel keep their original meaning.

History remains in its existing compressed cache. Capture updates the reusable
Document from the layout and calls `History.Save` so the cache identity matches
the handoff document. The session file itself is gzip-compressed JSON with an
independent development-only version.

## Process model

The `dg dev` supervisor owns source watching, builds, and child processes. A
private reload marker below its cache workspace coordinates with the child:

1. Supervisor builds the initial executable and starts it.
2. Child watches the reload marker while Bubble Tea runs.
3. Supervisor atomically installs a successful replacement and updates the
   marker.
4. Child serializes the session and exits with a dedicated reload code.
5. Supervisor removes the marker and starts the replacement.

The child mode and cache paths travel through private environment variables;
they are not hidden CLI commands. The supervisor removes its cache workspace
when the session ends.

## Implementation phases

1. Add versioned TUI session capture, gzip encoding, restoration, and focused
   round-trip tests.
2. Add reload-marker lifecycle integration to `tui.Run`.
3. Add root command parsing, source watching, atomic builds, and child restart
   supervision.
4. Add process-level tests for silent failed builds and successful handoff.
5. Run session benchmarks and repository verification.

## Verification

```sh
GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui -count=1
GOCACHE=/private/tmp/dg-codex-go-build go test . -count=1
GOCACHE=/private/tmp/dg-codex-go-build go test ./...
GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...
GOCACHE=/private/tmp/dg-codex-go-build go vet ./...
GOCACHE=/private/tmp/dg-codex-go-build \
  GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache \
  golangci-lint run --path-mode abs
```

Use a headless terminal to confirm that selection, viewport, sidebar, help, and
an open Preferences draft survive a successful rebuild and that a failed build
does not disturb the running editor.
