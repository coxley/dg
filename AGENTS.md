# Project guide

## Goal

`dg` is a Monodraw-like diagram engine and interactive terminal editor in Go.
It supports programmatic construction and point-and-click editing.

This is a freeform editor, not a global graph layout system. Callers place
nodes. The engine resolves cell geometry, routes orthogonal edges, and renders
the result.

## Constraints

- keep layout grid-based and independent of final Unicode glyphs
- keep semantic, geometric, rendering, persistence, and frontend concerns
  separate
- prefer small structs, slice indices as IDs, aligned slices, and reusable
  scratch
- preserve stable live IDs; reuse tombstoned slots
- benchmark interactive paths before adding bookkeeping or abstractions
- use concrete algorithms when interfaces add allocation or indirection
- route application-level IR mutations through `layout.Layout`
- keep changes inside one package when its public contract permits
- read the nearest package `AGENTS.md` before changing that package

## Current capabilities

The engine supports multiline labels, automatic and explicit node sizes,
custom ports, orthogonal routing, route sharing, arrows, border and stroke
styles, layers, occlusion, hit testing, selection, routed previews, persistent
undo history, and versioned JSON documents.

The Bubble Tea frontend supports mouse editing, rectangle and line tools,
resizing, duplication, label editing, layering, style changes, undo and redo,
saving, clipboard export, preferences, and live previews.

## Packages

- `cmd/dg`: starts the example editor or opens a saved diagram
- `document`: maps layouts to and from the versioned JSON schema
- `ir`: stores semantic objects in `ir.Graph`
- `layout`: composes `ir.Graph` with geometry, routing, raster ownership,
  selection, layers, and change history
- `render`: encodes layout raster data as Unicode terminal frames
- `internal/tui`: implements the Bubble Tea interactive editor
- `internal/tui/canvas`: retains canvas frames, encoders, and drawing styles
- `internal/tui/clipboard`: owns copy debounce, export, and clipboard backends
- `internal/tui/flex`: allocates ANSI-aware horizontal terminal rows
- `internal/tui/modal`: renders movable or full-screen modal shells and tabs
- `internal/tui/nav`: renders and handles the floating tool navigation
- `internal/tui/numinput`: implements bounded arrow-key numeric controls
- `internal/tui/preferences`: owns the editable preferences form

Each package has a local `AGENTS.md` with its decisions and constraints.

## Change boundaries

Prefer one package per change and commit. If work crosses a boundary:

1. Define and test the smallest required contract in the owning package.
2. Implement the package-local behavior without frontend assumptions.
3. Integrate the caller in a separate change.

Do not duplicate engine behavior in `internal/tui` to avoid changing
`layout`. Do not extend `ir` with geometry or persistence concerns.

## Headless terminal verification

Use
[`montanaflynn/headless-terminal`](https://github.com/montanaflynn/headless-terminal)
to inspect actual terminal output:

```sh
ht run --size 100x30 --name dg-smoke \
  env GOCACHE=/private/tmp/dg-codex-go-build go run ./cmd/dg
ht send dg-smoke '?'
ht view dg-smoke
ht view --format png --output /private/tmp/dg-smoke.png dg-smoke
ht stop dg-smoke
ht remove dg-smoke
```

Also check `80x16` and `80x12`. Validate overlays, focus, cursor visibility,
cell alignment, and interaction state. Use `ht send --help` for supported
mouse input.

## Next work

1. Add engine-level alignment snapping with guides and hysteresis.
2. Replace cell-local dashed glyphs with phase-aware segment dashing.
3. Apply layer commands to whole selections.
4. Add portless lines or boxless ports.
5. Add reusable custom shapes composed from multiple primitives.
6. Continue finding safe cases that can skip routing or reuse raster output.

## Verification

Use `require` for test assertions. Direct benchmark failures are allowed
inside `b.Loop` to avoid boxing. Keep sleeps inside `testing/synctest`.

```sh
GOCACHE=/private/tmp/dg-codex-go-build go test ./...
GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...
GOCACHE=/private/tmp/dg-codex-go-build go vet ./...
GOCACHE=/private/tmp/dg-codex-go-build \
  GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache \
  golangci-lint run --path-mode abs
```
