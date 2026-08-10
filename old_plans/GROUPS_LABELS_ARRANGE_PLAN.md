# Groups, batch labels, and Arrange

## Goal

Add persistent nested groups, sequential label editing for logical selections,
and transient alignment and distribution helpers without turning groups into
rendered containers or live layout policies.

## Agreed behavior

### Group structure

- Groups form a strict tree. A node or subgroup has at most one parent.
- Groups may contain nodes and nested groups, but never edges.
- Every group has at least two immediate children. Deletion that leaves a
  singleton dissolves that group, lifts its remaining child into the parent,
  and repeats the cleanup upward in the same undo transaction.
- `Ctrl-G` and `Cmd-G` group two or more selected sibling items under a new
  parent. Mixed-parent selections are rejected with guidance.
- The same command on one selected group ungroups exactly one level and leaves
  the former immediate children selected.
- Moving individual children never changes membership.
- Grouping and ungrouping preserve draw order and world geometry.

### Selection and interaction

- Clicking visible descendant geometry initially selects its outermost group.
- Clicking geometry inside the sole selected group descends one group level.
  Repeated clicks continue descending.
- Super-click selects the deepest node directly.
- Selection automatically excludes ancestor-descendant pairs. Selecting a
  descendant removes its selected ancestor; selecting an ancestor removes its
  selected descendants. Unrelated selected items remain unchanged.
- Once editing inside a group, ordinary node selection remains inside that
  group until selection clears.
- Selected groups move as rigid items. Drilled-in nodes and subgroups move only
  the explicitly selected branch while retaining membership.
- Group bounds derive from descendant node bounds for selection and movement.
  Empty cells inside those bounds are not hit targets.
- Groups add no raster ownership, routing obstacles, ports, or port anchors.
  Existing ports remain on their original nodes. Current visible geometry
  determines casual port eligibility; grouping never deletes an occluded port.

### Copy, duplicate, and deletion

- Copying or duplicating a selected group preserves its complete nested
  hierarchy.
- Copying drilled-in children copies only those nodes or subgroups and does not
  infer their parent group even when every sibling happens to be selected.
- Existing fragment rules continue to include an edge only when both endpoint
  nodes are copied.
- Deleting a selected group deletes its descendants and incident edges.
- Deleting drilled-in children preserves remaining membership and performs the
  recursive singleton cleanup above.

### Batch label editing

- Pressing `e` with a logical multi-selection or selected group recursively
  collects descendant nodes with existing non-empty labels.
- Explicitly selected empty-label nodes are also excluded. Ordinary
  single-node editing remains the way to add the first label.
- The target sequence is deduplicated and ordered top-to-bottom, then
  left-to-right, using geometry captured when the session starts. Label-driven
  resizing cannot reorder the remaining targets.
- Active input semantics do not change: plain Enter commits a single-line
  label, Shift-Enter creates its first newline, plain Enter adds newlines to a
  multiline label, and Ctrl-Enter or Cmd-Enter commits any label.
- An Enter-based commit advances to the next target and begins a fresh history
  transaction, so every label can be undone independently.
- Escape, tool changes, clicks outside editing, or other interruption commits
  the active label through the existing boundary and stops progression.
- Completing or interrupting the sequence restores the original logical
  selection.

### Arrange flyout

- `Shift-L` toggles a vertical floating Arrange form beside the main
  navigation. It opens on the right and flips left when space requires it, so
  the canvas remains visible.
- The form exposes `Align (h)`, `Align (v)`, and `Distribute` selectors. Each
  starts at a neutral em-dash placeholder rather than a persistent Off value.
- Arrow keys or `h`, `j`, `k`, and `l` move focus and change selectors. Every
  change recomputes a live preview from the geometry captured when the form
  opened, preventing cumulative rounding or placement drift.
- The rows apply top-to-bottom. Distribution therefore takes precedence over
  alignment on the same axis; the preview intentionally permits every
  combination instead of clearing or disabling fields.
- Enter, `Shift-L`, or an outside click commits the preview as one undo
  interaction and closes the form. An outside click continues to its underlying
  canvas or control so the next interaction starts immediately. Escape, loss of
  terminal focus, or loss of a valid multi-selection restores the captured
  geometry and closes it.
- Alignment uses the aggregate selection bounds. Distribution keeps the two
  positional extremes fixed and equalizes gaps between item bounds; it
  requires at least three selected items.
- A selected group participates as one rigid item. Users drill into a group to
  arrange its children.
- Arrange is a TUI convenience, not persisted policy. It computes target
  movement from captured bounds, routes preview mutations through
  `layout.Layout`, and exposes them to history only when Enter commits. Only
  resulting coordinates persist.
- Live flex or container layout remains deferred.

## Phase 1: semantic group hierarchy

- Add stable, tombstone-reused group IDs to `ir.Graph`.
- Store ordered node-or-group members and validate live IDs, unique parentage,
  sibling-only grouping, cycles, and the two-child minimum.
- Implement group, one-level ungroup, parent and ancestry queries, descendant
  traversal, node deletion cleanup, clone, and slot reuse.
- Keep parent lookup allocation-free with aligned reusable storage or bounded
  scans; do not introduce maps without benchmark evidence.
- Add readable table tests plus generated mutation tests for arbitrary nested
  create, delete, group, ungroup, clone, and ID-reuse sequences.

## Phase 2: layout ownership, history, and persistence

- Add group identity to layout selection while preserving the ancestor-
  descendant exclusion invariant.
- Distinguish logical selected items from expanded descendant nodes used by
  geometry, routing, styles, deletion, and movement.
- Derive group bounds without adding raster or hit ownership.
- Add group-aware rigid movement, duplication, fragments, deletion cleanup,
  one-level grouping, and ungrouping through `layout.Layout`.
- Extend reversible layout changes and their JSON codec so group mutations,
  cascading dissolution, undo, redo, and durable history replay remain atomic.
- Increment the document schema, retain the prior wire shape, migrate existing
  documents to no groups, compact tombstoned group IDs on export, and validate
  the complete hierarchy on import.

## Phase 3: group selection and batch labels

- Translate visible node hits into outermost group selection, repeated
  one-level descent, and direct Super-click leaf selection.
- Add `Ctrl-G` and `Cmd-G` to the keymap, preferences, contextual help, and
  collision tests.
- Render derived group bounds as selection affordances without making their
  empty interiors interactive.
- Build the snapshotted non-empty label target sequence from logical selection.
- Advance one history transaction per Enter-based commit and restore logical
  selection on completion or interruption.
- Cover nested click descent, direct leaf selection, antichain normalization,
  rigid movement, preserved child membership, and batch edit interruption in
  model tests.

## Phase 4: Arrange control

- Add a retained three-field form using shared `chrome.Form` geometry and
  root-owned semantic option IDs.
- Place it adjacent to the main floating navigation and flip it within terminal
  bounds without shifting the canvas or primary toolbar.
- Preview align and distribute calculations from captured logical item bounds,
  then commit the resulting ordinary layout movement in one transaction.
- Commit on Enter, `Shift-L`, or outside click; let the outside click continue
  to its underlying target. Restore captured geometry on Escape, focus loss,
  or invalid selection.
- Verify keyboard, mouse, narrow-terminal placement, undo, group rigidity,
  integer remainder distribution, and unsigned-coordinate boundaries.

## Verification

Run focused tests at each boundary:

```sh
GOCACHE=/private/tmp/dg-codex-go-build go test ./ir
GOCACHE=/private/tmp/dg-codex-go-build go test ./layout ./document
GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/...
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

Use headless-terminal checks at `100x30`, `80x16`, and `80x12` for nested
selection descent, group bounds, batch editing progression, Arrange placement,
focus, cursor visibility, and outside-click dismissal.
