# Live tint and edge attachments

## Goal

Add two focused improvements:

1. Follow terminal background changes and switch between the configured light
   and dark tints.
2. Let nodes attach implicitly to routed edges without allowing failed routing
   to corrupt history.

## Agreed behavior

### Live tint

- Keep the startup OSC 11 background query.
- Enable DEC mode 2031 for palette-change notifications.
- Treat a mode-2031 notification as a reason to query OSC 11 again. Select the
  tint from the actual terminal background, not the notification itself.
- Query OSC 11 again when the terminal regains focus as a fallback.
- Disable mode 2031 on exit.
- Do not poll.

### Transactions and undo

- Every committed editor state must route successfully.
- If a drag release or interruption cannot build, roll back the transaction
  and report that the placement was rejected.
- A rejected placement must not prevent later undo or redo.
- Repair this transaction boundary before enabling focus reports, because live
  blur events add another way to interrupt a drag.

### Edge attachments

- Attachment is implicit during the normal node-drag interaction.
- Do not add an Attach mode, toolbar entry, keybinding, or contextual-help
  command.
- While a node hovers over an eligible edge, highlight that edge. Do not draw a
  separate attachment-point marker; the hovering node shows the intended
  position.
- Preserve the existing drag preview and movement behavior. Change the
  attachment relationship and reroute only on release.
- Releasing an unattached node over an eligible edge attaches it there.
- Releasing an attached node over another position or edge updates or transfers
  the attachment.
- Releasing an attached node away from an eligible edge detaches it and keeps
  its released absolute position.
- After its host edge reroutes, an attached node remains at the same relative
  position along the edge until detached.
- Selecting an edge selects the edge and its attachments. Selecting that edge
  again leaves only the edge selected.
- Deleting the expanded selection deletes the edge and its attachments.
- Deleting only the edge detaches its nodes. Keep each node in place when that
  position remains legal; otherwise move it to the closest legal area.
- Preserve an attachment when duplicating or structurally exporting only when
  both the node and its host edge are included. Otherwise the copied node is
  detached.
- Evolve the development document schema in place. Do not increment its version
  or add migrations before the format is stable.
- The root theme owns a named semantic style for the candidate-edge highlight.
  Tests must not assert concrete theme colors, borders, padding, or dimensions.

Layer-based routing is not part of this work. A node's layer must not implicitly
decide whether an edge may pass through it.

## Implementation questions

Resolve these from the layout invariants, focused tests, and benchmarks before
settling the engine design:

- How to encode a stable relative position along a routed edge and align the
  node to it.
- How attachments interact with shared route segments and edge occlusion.
- How multiple attachments and routing dependencies avoid collisions or
  impossible cycles.
- How to find the closest legal detached position deterministically without
  making deletion too expensive.
- Which selection commands, beyond the agreed selection and deletion behavior,
  should operate on the edge alone or on its attachments too.

Collect any remaining user-visible choices from these questions and present
them together. Do not silently turn implementation proposals into interaction
decisions.

## Phase 1: protect transaction boundaries

- In `internal/tui`, commit a rigid pointer move only after
  `BuildSelection` succeeds.
- Validate an active rigid move before an interruption commits it.
- On release or interruption failure, cancel the transaction, restore the last
  committed geometry, rerender it, and retain a useful placement error.
- Keep non-rigid movement's existing per-step validation.
- Add the reported regression: make two invalid drops, then verify undo still
  works.
- Add a generated model invariant: every completed or interrupted interaction
  leaves a buildable layout, and advertised undo or redo actions succeed.

## Phase 2: follow terminal background changes

- Enable DEC mode 2031 before the initial OSC 11 request.
- Forward palette-change notifications into a fresh OSC 11 query.
- Request OSC 11 on terminal focus.
- Keep `tea.BackgroundColorMsg` as the only input that selects the light or dark
  tint.
- Enable focus reporting and preserve existing blur interaction behavior.
- Disable mode 2031 whenever the program exits normally or with an error.

Tests verify command ordering, notification and focus queries, confirmed
background selection, focus reporting, and cleanup. Style assertions use
semantic identities or injected sentinel styles rather than theme defaults.

## Phase 3: define the layout attachment contract

- Add layout-owned attachment storage and the minimum attach, update, transfer,
  detach, and query operations.
- Keep attachment geometry out of `ir.Graph`.
- Include attachments in clone, history, undo/redo, ID reuse, node and edge
  deletion, and whole-layout translation.
- Preserve attachments during duplication only under the both-objects rule.
- Make building and attachment mutations atomic: failure restores the prior
  buildable state.
- Settle the open representation, routing, shared-segment, collision, and
  relocation questions with focused examples and benchmarks.

Property tests cover mutation sequences, clone independence, tombstoned ID
reuse, duplication, deletion, and complete undo/redo traversal.

## Phase 4: persist and integrate the interaction

Persistence:

- Add optional attachment data to the current document schema without changing
  its version.
- Remap attachment references during compact export and validate them on
  import.
- Add readable JSON and generated round-trip coverage.

TUI:

- Detect the candidate edge from the retained canvas routes during a normal
  node drag.
- Render only the root-owned candidate-edge highlight.
- On release, ask the layout to attach, update, transfer, or detach as required.
- Implement the two agreed edge-selection states and both deletion paths.
- Keep attachment out of the toolbar, keymap, and contextual help.

## Verification

Run focused tests while implementing each package boundary:

```sh
GOCACHE=/private/tmp/dg-codex-go-build \
  go test ./layout ./document ./internal/tui -count=1
```

Then run the repository gates:

```sh
GOCACHE=/private/tmp/dg-codex-go-build go test ./...
GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...
GOCACHE=/private/tmp/dg-codex-go-build go vet ./...
GOCACHE=/private/tmp/dg-codex-go-build \
  GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache \
  golangci-lint run --path-mode abs
```

Use headless-terminal checks at `100x30`, `80x16`, and `80x12` to verify:

- live light/dark changes, focus fallback, and cleanup;
- implicit attach, reposition, transfer, and detach behavior;
- candidate highlighting without a separate attachment marker;
- both edge-selection deletion paths; and
- failed drops leave the layout buildable and undoable.
