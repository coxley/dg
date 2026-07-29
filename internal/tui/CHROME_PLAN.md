# TUI Chrome Architecture and Migration Plan

- Status: complete
- Scope: non-canvas UI under `internal/tui`
- Current phase: Phase 10 complete; later feature sweeps not started
- Last updated: 2026-07-28

This document is the execution record for making the TUI chrome declarative.
Update the phase checklist, decision log, changed-file ledger, and verification
ledger as work progresses. Do not leave completed or deferred work represented
only in a PR description or chat.

Use `Not started`, `In progress`, `Blocked`, or `Complete` in the phase table.
When a phase becomes blocked, record the exact condition beside its checklist
before working around it.

## Outcome

Adding a field, button, menu, dialog, or sidebar section should primarily be a
declarative change. Application code supplies content, commands, bindings, and
business behavior. Shared chrome code computes geometry, manages focus and input
precedence, renders interaction states, and adapts to terminal constraints.

The migration must:

- eliminate parent-owned border, padding, width, and height arithmetic;
- use one layout result for rendering, focus order, pointer hit testing, clipping,
  scrolling, and diagnostics;
- centralize visual state and responsive density;
- make active keybindings inspectable and testable;
- preserve existing editor semantics and history boundaries;
- keep the canvas renderer and document geometry independent of chrome layout;
- introduce one canvas host rectangle for screen placement without replacing the
  canvas's unbounded document viewport;
- remain incrementally reviewable, with each phase removing the old logic it
  supersedes.

## Non-goals

- Build a general graph-layout system for the canvas.
- Replace Bubble Tea or Lip Gloss.
- Provide arbitrary user-defined key rebinding in the initial migration.
- Support multiple independently scrolling bodies in one pane. Nest panes when
  that need becomes concrete.
- Commit a broad matrix of image goldens. Prefer structural assertions and
  failure artifacts until experience demonstrates a specific need.
- Implement all future sidebar content as part of the foundational migration.

## References

- `AGENTS.md` and `internal/tui/AGENTS.md` define the project and frontend
  boundaries.
- The
  [headless-terminal skill](https://github.com/montanaflynn/headless-terminal/blob/main/skills/headless-terminal/SKILL.md)
  defines PTY synchronization, capture, and cleanup practices for the chrome
  lab.

## Current Failure Modes

The existing package split establishes useful ownership, but components exchange
rendered strings instead of layout contracts.

- `internal/tui/modal.go` reconstructs modal body dimensions from theme frame
  sizes and previously rendered overlay state.
- `internal/tui/modal.go` coordinates preferences height, active tab height,
  toolbar avoidance, and modal framing in the root model.
- `internal/tui/preferences/preferences.go` measures natural height by rendering
  the Huh form and locates action buttons by searching stripped ANSI output.
- `internal/tui/model.go` repeats input precedence across message-type,
  modal-type, and component-type switches.
- `internal/tui/keymap.go`, `internal/tui/preferences.go`,
  `internal/tui/preferences/preferences.go`, and
  `internal/tui/numinput` each interpret physical keys.
- `internal/tui/theme.go` assembles related focus, active, hover, and button
  styles independently, so state changes can alter geometry or drift visually.
- `internal/tui/nav/nav.go` owns useful geometry, but its tool declarations and
  placement are fixed and cannot directly support additional menus.
- `internal/tui/view.go` and `internal/tui/nav/nav.go` duplicate toolbar
  placement, while modal avoidance reconstructs the toolbar's top edge.
- `internal/tui/view.go` mutates navigation state while rendering.
- Canvas composition, mouse conversion, cursor visibility, and the one-row
  status footer assume a canvas rooted at `(0, 0)` with full terminal width.

Recent TUI history contains consecutive fixes for preference sizing, action
placement, row alignment, clicks, navigation, and modal sizing. The plan treats
those as API-design failures rather than isolated test omissions.

## Dependency Direction

`internal/tui/chrome` owns reusable mechanics:

- intrinsic measurement and arrangement;
- boxes, rows, columns, panes, viewports, and scrollbars;
- focus scopes and focus restoration;
- key profiles, binding resolution, collision detection, and help projection;
- surface placement, z-order, pointer capture, and modal blocking;
- deterministic cell-aware transitions;
- layout and interaction diagnostics.

Application and component packages own policy:

- command and scope identifiers;
- binding declarations and help labels;
- preference schemas and persistence;
- sidebar contents and object operations;
- canvas commands and editor modes;
- transaction, save, export, clipboard, and document behavior.

Chrome must not import application packages. Application code configures chrome
through public data structures and receives semantic command messages through
Bubble Tea. Adding an application command must not require editing chrome.

Allowed chrome dependencies are Bubble Tea, Lip Gloss, ANSI/display-width
utilities, and the standard library. Chrome must not import `internal/tui`,
`canvas`, `layout`, `render`, `document`, or application command and scope
types.

The root application should converge on:

- `internal/tui/bindings.go` for application-owned command IDs, scope IDs, and
  binding declarations;
- `internal/tui/workspace.go` for surface declarations, canvas host placement,
  and screen-to-canvas coordinate transforms.

## Settled Contracts

### Measurement and arrangement

Every arranged element has computed minimum and preferred sizes. Measurement
receives constraints because wrapping makes height dependent on assigned width.
The exact Go API may change during Phase 2, but it must preserve this flow:

```text
environment + constraints
          ↓
       measure
  minimum + preferred
          ↓
       arrange
  rectangles + overflow
          ↓
 render / focus / hit test / diagnostics
```

Per-axis declarations use semantic policies:

- size: hug content or fill available space;
- shrink: never or down to computed minimum;
- horizontal overflow: wrap, clip, or scroll;
- vertical overflow: clip or scroll;
- scrollbar: never, automatic, or always.

Linear containers additionally declare:

- main-axis grow and shrink weights;
- fixed or density-derived gaps;
- explicit spacers that absorb remaining space;
- start, center, end, and space-between justification;
- start, center, end, stretch, and baseline-compatible cross-axis alignment.

Allocation begins from each child's preferred size, distributes surplus by grow
weight, and distributes deficit by shrink weight without crossing computed
minimums. Deterministic remainder allocation proceeds in stable child order.
Preference rows must be expressible as a shrinking label plus a right-aligned
control without component-specific width arithmetic.

Minimum sizes come from content and children, not duplicated constants in parent
models. When the physical terminal is smaller than a component's minimum, the
result must use an explicit emergency overflow policy rather than produce
negative or out-of-bounds geometry.

Rendered ANSI output is not a source of component geometry. Text measurement at
leaf boundaries may use ANSI-aware display-cell width.

Arranged elements need stable semantic identities across resize and reflow.
Focus restoration, scroll anchoring, diagnostics, and pointer capture must not
depend on slice position or render order remaining unchanged.

### Layout lifecycle

Each model retains a versioned arranged plan. Environment, content, style
geometry, visibility, and layout-relevant state changes invalidate it.

- `ensureLayout` computes the latest plan before rendering or processing
  geometry-dependent input.
- `View` consumes the plan and must not mutate component state.
- Pointer hit testing, focus traversal, clipping, and diagnostics consume that
  exact plan rather than reconstructing geometry.
- Content-only visual state may reuse a plan when its semantic style variants
  have equal geometry.
- Duplicate arranged IDs are diagnostic errors.

Tests must change terminal size, content, and style, then send pointer input
before another `View`; the hit target must match the newly arranged layout.
Retained plans and reusable scratch should keep invalidation explicit without
adding per-frame tree allocation.

### Pane and viewport

A pane owns one shared box and three slots:

```text
header: optional, sticky
body:   one independently scrollable viewport
footer: optional, sticky
```

Header and footer consume their measured height. The body receives the remaining
height, owns scroll offsets and clipping, and optionally reserves scrollbar
cells. Nest panes for multiple independently scrolling regions.

Automatic horizontal and vertical scrollbars must converge deterministically
when reserving either bar changes wrapping or the other axis's overflow. Test
the complete two-axis fixed-point matrix.

### Density and responsiveness

Density is one global environment value: regular or compact. Components consume
density-aware theme tokens rather than checking terminal size independently.

Structural changes remain fit-driven:

- a floating modal becomes full-screen when its measured layout cannot fit;
- a sidebar docks only when its minimum width plus the minimum usable canvas
  width can fit;
- scrollbars appear when arranged content exceeds its viewport.

The exact density threshold, compact insets, and minimum usable canvas size must
be selected from chrome-lab evidence, then centralized.

### Visual states

Themes expose semantic tokens for normal, hovered, focused, active, selected,
disabled, and error states. State variants must have equal border, padding,
margin, and resulting cell geometry unless a component explicitly declares a
layout-changing transition.

Application Theme owns colors, glyphs, and default semantic roles. Global
border, focus, spacing, density, and motion tokens should derive component
styles. Chrome accepts those tokens and enforces equal geometry; it does not
choose application appearance. Component packages may add roles but must not
clone the core state construction.

### Focus

Interactive surfaces form focus scopes. The arranged component tree supplies
the ordered enabled focus targets.

- Opening a sidebar or modal saves the previously focused target and moves focus
  into the new scope.
- Reopening restores the last valid target in that scope, otherwise the first
  enabled target.
- Tab and Shift-Tab traverse the active scope.
- Closing restores saved focus, falling back to the canvas.
- A docked sidebar's Back command returns focus to the canvas while leaving the
  sidebar visible.
- A compact overlay sidebar's Back command dismisses the sidebar and restores
  canvas focus.
- Clicking the canvas follows the same placement-specific behavior.

Hidden and disabled controls do not participate in focus order.
Stable focus IDs survive insertion, removal, reordering, resize, density
changes, and dock/drawer placement changes. Tests must cover restoring focus
after those operations.

### Commands and bindings

Chrome owns normalized chords, Auto/Mac/Standard profiles, scope precedence,
resolution, shadowing, collision checks, and help generation. Application code
declares scopes, semantic command IDs, chords, labels, and handlers.

`Primary` resolves to Command in Mac mode and Control in Standard mode. Auto
selects the platform default, and users can explicitly choose Mac mode when
working over SSH.

Dispatch proceeds from the focused scope through containing surfaces, workspace,
and global scopes. The resolver emits an opaque semantic command message; the
owning application or component handles it in `Update`.

Text entry has precedence for typable characters. Explicit modifier commands
may resolve first, but unmodified, shifted, composed, and pasted text must not
bubble to parent scopes. A plain `q` types `q` in a text field.

The input environment includes terminal keyboard capabilities. Binding
projection advertises Command/Super chords only after Bubble Tea reports the
required enhancement. Paste remains an explicit text-input path. Preserve the
clipboard debounce rules: passive all-motion events and standalone modifier
events do not cancel a pending copy, while other interactions do.

The binding registry must generate the merged effective binding list used by
Help and by collision/matrix tests. Shadowed chords appear once with the action
that will actually execute.

### Surfaces and Help

Chrome provides placement, z-order, focus, capture, and blocking mechanisms.
Application declarations choose anchors, priority, dismiss-on-outside behavior,
Back behavior, focus-on-open behavior, and dock-versus-overlay policy.

The workspace distinguishes:

- dock regions that reduce the main canvas rectangle;
- interactive panels that participate in keyboard focus;
- passive inspectors that observe focus but do not take keyboard focus;
- floating menus;
- compact overlay drawers;
- dialogs and modals that block lower surfaces.

Pointer dispatch follows current arranged rectangles and z-order. An active
pointer gesture retains capture until release or cancellation.

Floating surfaces declare anchors relative to terminal, workspace, canvas, or a
dock region. Movable and resizable surfaces retain a requested rectangle
separately from the terminal-clamped arranged rectangle, so shrinking and then
growing the terminal does not discard the user's requested size.

Help becomes a passive contextual inspector:

- visible at the bottom-right by default;
- movable, resizable, scrollable, and hideable;
- toggled by `?`;
- pointer-interactive without joining the keyboard focus stack;
- always displays the merged effective bindings for the underlying active
  context.

Preferences becomes a separate modal opened by `Primary+,`.

### Canvas host

The root workspace owns an invisible screen-space allocation for the canvas.
The canvas host is not a rendered Box or surface: it adds no border, padding,
background, or other visual boundary. Chrome does not own the canvas's
unbounded document viewport or sparse renderer.

One transform maps between screen cells and canvas-host cells; the existing
document viewport mapping remains inside `internal/tui`. All of these must use
the canvas host:

- canvas render width, height, origin, and clipping;
- screen-to-document pointer conversion;
- keyboard-driven cursor visibility;
- label-edit cursor screen placement;
- floating-surface anchoring relative to the canvas.

Objects may remain partly outside the visible host and move back into view.
Preserve the current viewport rebasing and off-screen drag behavior so the
canvas continues to feel spatially unbounded.

During docked sidebar motion, tests must verify screen-to-document-to-screen
round trips at every integer boundary.

### Sidebar and motion

The sidebar uses the same pane content in two placements:

- regular and sufficiently wide: docked, reducing the canvas rectangle;
- compact or constrained: animated overlay drawer, leaving the canvas fixed.

One workspace-owned transition controls the docked sidebar boundary and canvas
rectangle. Opening uses ease-out; closing uses ease-in. Compact placement
animates only the drawer position.

Transitions are deterministic and cell-aware:

- render only when an interpolated integer cell position changes;
- retarget from the current position after terminal, density, or content changes;
- reverse from the current position when toggled mid-transition;
- remove closing surfaces from focus and key routing immediately;
- use the current visible rectangle for pointer hit testing;
- permit global motion disabling without changing final layout.

Canvas document coordinates and viewport origin remain stable during a docked
transition. Only the canvas screen rectangle and clipping change; sidebar motion
must not trigger graph layout or routing.

## Target Application Shape

The public chrome API should make root configuration resemble data, not event
plumbing. Names below are illustrative:

```go
bindings := chrome.Bindings{
    {
        Scope:   scopeCanvas,
        Chords:  chrome.Keys("q"),
        Command: commandQuit,
        Label:   "quit",
    },
    {
        Scope:   scopePreferences,
        Chords:  chrome.Keys("q", "esc"),
        Command: commandClosePreferences,
        Label:   "back",
    },
    {
        Scope:   scopeGlobal,
        Chords:  chrome.Primary(","),
        Command: commandOpenPreferences,
        Label:   "preferences",
    },
}
```

Preference fields, menu items, actions, tabs, and sidebar sections follow the
same dependency direction: application-owned declarations configure reusable
chrome behavior.

## Verification Strategy

### Pure invariants

Generated and table-driven tests cover terminal sizes, densities, content
lengths, style geometry, and overflow combinations. Assert that:

- every rectangle is non-negative and contained by its parent;
- arranged IDs remain stable across resize and reflow;
- docked regions and canvas exactly tile the workspace;
- overlays never change the canvas rectangle;
- rendered lines fit their assigned width;
- ANSI styles, grapheme clusters, and wide cells clip only at valid display-cell
  boundaries;
- minimum sizes hold unless the physical parent is smaller;
- declared overflow and scrollbar policies determine the result;
- focused controls remain visible in their viewport;
- visual state variants retain equal geometry;
- animations move monotonically, reverse safely, retarget after resize, and end
  at exact bounds.

### Interaction scenarios

A reusable in-process harness drives Bubble Tea messages, window changes, key
profiles, pointer input, and deterministic animation time. It exposes semantic
state: focus, scope stack, effective commands, rectangles, scroll offsets,
pointer capture, and animation targets.

Required scenarios include:

- opening the sidebar focuses it;
- Back returns from a docked sidebar without hiding it;
- Back dismisses a compact drawer;
- typing `q` in text input inserts text;
- paste enters the focused text control without reaching parent scopes;
- keyboard-enhancement changes update effective Command/Super bindings;
- pending copy survives passive motion and standalone modifier events;
- Preferences changes Help to the preference binding context;
- a nested picker shadows preference commands correctly;
- resizing across the dock/drawer boundary preserves content and focus;
- reversing an in-flight transition does not jump;
- an outside click respects modal and pointer-capture precedence.

Use `testing/synctest`; do not add unwrapped sleeps.

### Chrome lab and headless terminal

Add an internal interactive executable:

```sh
go run ./internal/tui/cmd/chrome-lab
```

Its initial phase composes representative menus, boxes, panes, viewports,
densities, and overflow states. Later phases extend the same program with forms,
surfaces, key scopes, and animation as those capabilities become available. A
diagnostics pane exposes terminal size, density, focus, scopes, rectangles,
scroll offsets, pointer capture, animation state, and effective bindings.

The headless-terminal runner:

- launches named sessions;
- combines input with `--wait-text` or an explicit `--wait-idle DURATION` rather
  than sleeping;
- sends SGR mouse input through `ht send --raw`;
- uses `ht view --json` for terminal size, cursor, and text assertions;
- exposes stable labeled diagnostic rows or an explicit lab dump mode for
  application-semantic assertions;
- writes JSON and PNGs under a `mktemp -d` failure directory and reports that
  path for human diagnosis;
- always stops and removes sessions through cleanup handling.

Headless scenarios remain focused end-to-end checks. Broad state and size
coverage stays in-process. Revisit committed text or image goldens only when a
specific visual regression cannot be expressed reliably as an invariant.

### Phase gates

During iteration, run the narrow package tests. At each integration gate, run:

```sh
GOCACHE=/private/tmp/dg-codex-go-build go test ./...
GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...
GOCACHE=/private/tmp/dg-codex-go-build go vet ./...
GOCACHE=/private/tmp/dg-codex-go-build \
  GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache \
  golangci-lint run --path-mode abs
```

Run root headless-terminal checks at `100x30`, `80x16`, and `80x12`. Run the
existing TUI benchmarks after changes to root rendering, highlighting, pointer
dispatch, or canvas clipping:

```sh
GOCACHE=/private/tmp/dg-codex-go-build \
  go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem
```

Add focused benchmarks for chrome measure, arrange, and render allocation before
root adoption. Re-run those and the root View benchmark after Phases 6, 7, and
9. Chrome must not render each canvas cell through Lip Gloss or rebuild an
unchanged layout tree every frame.

## Migration Phases

Only one phase should be in progress at a time. Complete its narrow tests,
review, and integration gate before beginning the next phase.

| Phase | Status | Review gate |
| --- | --- | --- |
| 0. Architecture record | Complete | Decisions and boundaries recorded |
| 1. Baseline characterization | Complete | Baseline tests are green |
| 2. Geometry and menu | Complete | Existing nav uses chrome geometry |
| 3. Pane and viewport | Complete | Overflow matrix and resize invariants pass |
| 4. Chrome lab | Complete | Interactive and initial `ht` scenarios pass |
| 5. Commands and focus | Complete | Effective binding matrix matches dispatch |
| 6. Surfaces and contextual Help | Complete | One z-order/input router owns active surfaces |
| 7. Declarative Preferences | Complete | No rendered-text geometry recovery remains |
| 8. Existing modal migration | Complete | Root modal-type input switches are removed |
| 9. Adaptive sidebar and motion | Complete | Dock/drawer transition invariants pass |
| 10. Remaining forms and cleanup | Complete | Superseded paths are audited and minimized |

### Phase 1: baseline characterization

- [x] Resolve the existing copy-help naming mismatch: the test expects
  `super+c / ctrl+c`, while the model renders `cmd+c / ctrl+c`.
- [x] Run all current TUI and command tests before production changes.
- [x] Add characterization only where an upcoming migration lacks coverage;
  avoid duplicating existing model tests.
- [x] Record current root view/drag benchmark results.
- [x] Confirm current headless output at `100x30`, `80x16`, and `80x12`.

Do not mix baseline repairs with chrome package introduction.

### Phase 2: geometry and data-driven menu

- [x] Add `internal/tui/chrome/AGENTS.md` with the settled boundaries.
- [x] Define environment, size, rectangle, inset, constraints, metrics, and
  arrangement result types.
- [x] Define versioned layout invalidation, retained plans, stable arranged IDs,
  and duplicate-ID diagnostics.
- [x] Implement the minimum Box, Row, Column, Text, and Menu behavior required
  to replace current navigation geometry.
- [x] Implement deterministic grow, shrink-to-minimum, gap, spacer,
  justification, and cross-axis alignment.
- [x] Make menu items application-supplied data with stable semantic IDs.
- [x] Use the arrangement result for rendering and pointer hit testing.
- [x] Migrate `internal/tui/nav` without changing visible behavior.
- [x] Remove root assumptions that can be answered by arranged nav geometry.
- [x] Add generated layout invariants and state-geometry tests.
- [x] Select one ANSI/grapheme/display-cell measurement and wrapping path; cover
  wide glyphs, combining marks, styled content, multiline text, empty lines,
  and truncation.
- [x] Verify geometry-dependent input refreshes stale layout before `View`.
- [x] Compare the existing root drag/view benchmark before and after migration.

Deliver this phase as three reviewable slices where practical:

1. chrome contracts and tests;
2. a nav adapter using those contracts;
3. root adoption and removal of superseded placement logic.

Do not add scrolling, forms, surfaces, or animation in this phase.

### Phase 3: pane, viewport, and scrolling

- [x] Implement Pane with optional sticky header and footer.
- [x] Implement one body Viewport with clipping and scroll offsets.
- [x] Add horizontal and vertical never, automatic, and always scrollbar
  policies with two-axis convergence.
- [x] Define `Reveal(Rect)` mechanics and pointer-coordinate translation through
  a viewport; connect Reveal to focus only after focus scopes exist.
- [x] Cover wrapping/scrollbar remeasurement and constrained terminal cases.
- [x] Verify nested panes can express two independent scroll regions without a
  special multi-body API.

### Phase 4: chrome lab

- [x] Add `internal/tui/cmd/chrome-lab` without importing editor business logic.
- [x] Add scenario selection for layout, pane, viewport, menu, density, and
  overflow combinations.
- [x] Add an on-screen diagnostics pane.
- [x] Add a headless-terminal runner with named sessions and guaranteed cleanup.
- [x] Exercise keyboard, resize, wheel, click, drag, and raw SGR mouse input.
- [x] Capture JSON for assertions and PNG only on failure.
- [x] Keep initial scenarios limited to capabilities completed through Phase 3.
- [x] Document one interactive command and one automated smoke command.

### Phase 5: semantic commands and focus scopes

- [x] Define opaque scope and command IDs, normalized chords, and profile-aware
  chord declarations.
- [x] Implement active-scope resolution, shadowing, and collision diagnostics.
- [x] Emit semantic command messages instead of invoking application callbacks.
- [x] Build focus registration and traversal from arranged elements.
- [x] Implement focus save, restore, and last-valid-target behavior.
- [x] Guarantee text-entry precedence for typable input.
- [x] Move application binding declarations to a centralized
  `internal/tui/bindings.go`.
- [x] Implement and test Auto, Mac, and Standard profile resolution without
  changing the current Preferences form.
- [x] Generate effective-binding data for Help and tests from the resolver.
- [x] Preserve paste, keyboard-enhancement, Keystroke-based copy detection, and
  clipboard debounce cancellation semantics.
- [x] Connect focused elements to Viewport Reveal.
- [x] Extend the chrome lab and `ht` smoke with focus, profile, text, paste, and
  effective-binding scenarios.

### Phase 6: surfaces and contextual Help

- [x] Implement workspace dock, floating, passive, drawer, and modal roles.
- [x] Implement z-order, modal blocking, outside-click policy, and pointer capture.
- [x] Keep placement, dismissal, Back, focus-on-open, and priority as
  application-supplied declarations.
- [x] Define terminal/workspace/canvas/dock anchors and retain requested surface
  rectangles separately from clamped arranged rectangles.
- [x] Introduce one canvas host rectangle and screen-to-canvas transform while
  preserving current full-screen placement.
- [x] Keep the host visually transparent: no border, padding, background, or
  other rendered frame.
- [x] Migrate canvas render clipping, pointer translation, keyboard visibility,
  label cursor placement, and canvas-relative anchors to that host.
- [x] Keep the sparse canvas renderer and unbounded document viewport outside
  chrome's finite Viewport abstraction.
- [x] Move the existing nav into the surface manager as its first floating
  consumer.
- [x] Adapt the existing Preferences shell as a chrome modal containing the
  legacy Huh content; do not run independent legacy and chrome modal routers.
- [x] Express the status line as a workspace footer rather than a hard-coded
  subtraction in canvas sizing.
- [x] Separate Help from Preferences.
- [x] Render Help as a passive bottom-right inspector with Pane/Viewport.
- [x] Support moving, resizing, scrolling, hiding, and `?` toggling without
  stealing keyboard focus.
- [x] Open Preferences through `Primary+,`.
- [x] Ensure Help reflects canvas, modal, form, and nested component contexts.
- [x] Replace root message-precedence branches covered by surface dispatch.
- [x] Extend the chrome lab and `ht` smoke with z-order, pointer capture, legacy
  modal adapter, and passive Help scenarios.

### Phase 7: declarative Preferences

- [x] Define application-owned field and action declarations.
- [x] Implement the minimal Number, Select, Directory, ActionBar, and Spacer
  controls required by current preferences.
- [x] Anchor actions through layout growth rather than computed top padding.
- [x] Preserve live router changes, transaction ownership, Save, Save as
  Defaults, Cancel, and nested directory-picker behavior.
- [x] Retain a temporary Huh file-picker adapter only if replacing it would
  materially enlarge this phase.
- [x] Add the persisted Auto, Mac, and Standard profile selector.
- [x] Remove `NaturalHeight`, `blockOrigin`, rendered action lookup, and
  preference-specific frame arithmetic.
- [x] Verify adding a representative field changes only the preference
  declaration and value mapping.
- [x] Preserve the accessible labels and keyboard traversal provided by the
  current fields.
- [x] Extend the chrome lab and `ht` smoke with form, nested picker, and live
  preference-context scenarios.

### Phase 8: existing modal migration

- [x] Express the current Save, Export, and Notice surfaces through the surface
  manager.
- [x] Provide a reusable confirmation-dialog shell without adding unsaved-close
  business behavior in this phase.
- [x] Make floating/full-screen selection fit-driven.
- [x] Preserve movable and resizable modal behavior.
- [x] Route pointer and keyboard messages through active surface scopes.
- [x] Remove the root modal enum branches that duplicate surface behavior.
- [x] Keep application transactions and persistence in `internal/tui`.
- [x] Extend the chrome lab and `ht` smoke with current dialog lifecycle
  scenarios.

### Phase 9: adaptive sidebar and coordinated motion

- [x] Add a sidebar Pane with application-provided header, body, and footer.
- [x] Dock when sidebar minimum plus canvas minimum fits.
- [x] Use an overlay drawer when constrained or compact.
- [x] Focus the sidebar when explicitly opened.
- [x] Preserve docked visibility when Back returns focus to the canvas.
- [x] Dismiss compact placement on Back or outside click.
- [x] Implement workspace-owned cell-aware transition progress.
- [x] Coordinate docked sidebar width and canvas origin from one boundary.
- [x] Keep the canvas fixed during compact drawer animation.
- [x] Verify render clipping, pointer translation, keyboard visibility, cursor
  placement, and canvas-relative anchors throughout docked motion.
- [x] Preserve partially off-screen dragging, viewport rebasing, and moving
  objects back into the visible host.
- [x] Verify screen-to-document coordinate round trips at every animated cell
  boundary.
- [x] Test resize retargeting, transition reversal, motion disabling, pointer
  bounds, and focus restoration.
- [x] Extend the chrome lab and `ht` smoke with docked push, compact overlay,
  reversal, resize-retarget, and disabled-motion scenarios.

Initial sidebar contents may be a representative list. Recent diagrams, undo
history, layers, and object-tree behavior remain application feature phases.

### Phase 10: remaining forms and cleanup

- [x] Migrate Save and Export away from Huh where chrome controls suffice.
- [x] Implement a chrome TextInput before migrating Save.
- [x] Either replace the file picker or document a bounded adapter and the
  remaining reason for its Huh dependency.
- [x] Audit `flex`, `modal`, `nav`, `numinput`, and `preferences`; delete only
  packages whose responsibilities are fully superseded. Retain app-content
  packages that still express clean ownership.
- [x] Search all Huh imports and obsolete component-message wrappers; remove
  `charm.land/huh/v2` from `go.mod` and `go.sum` only when no bounded adapter
  remains.
- [x] Remove obsolete theme adapters and consolidate semantic tokens.
- [x] Preserve accessible labels, keyboard traversal, and accessible execution
  paths before removing Huh-backed controls.
- [x] Re-run full tests, race, vet, lint, benchmarks, and all headless sizes.
- [x] Update `internal/tui/AGENTS.md` to describe the final architecture.
- [x] Audit public chrome declarations for unused configurability and delete it.

## Later Feature Sweeps

Track these separately after the foundation proves stable:

- [ ] Additional floating action menus.
- [ ] Recent saved and unsaved diagram browser.
- [ ] Undo-history sidebar content and navigation.
- [ ] Layers and object-ID tree.
- [ ] Unsaved-close confirmation dialog.
- [ ] Persisted Help visibility, position, and size.
- [ ] Persisted sidebar visibility, width, selected section, and focus target.
- [ ] User-configurable motion disabling or reduced-motion detection.
- [ ] Arbitrary user key rebinding, import/export, and conflict UI.
- [ ] Accessibility behavior for custom form controls.
- [ ] Optional committed screen goldens if structural tests miss recurring visual
  defects.

## Risk Register

- Unstable arranged IDs can restore focus or scroll anchors to the wrong element
  after reflow.
- A legacy modal router running beside the surface manager can produce competing
  z-order, pointer-capture, and outside-click decisions.
- Wrapping and two-axis automatic scrollbars can oscillate without a convergent
  layout rule.
- ANSI styling, grapheme clusters, and wide cells can make measured, clipped,
  and rendered widths disagree.
- Enhanced-key events can blur the boundary between typable text and
  Command/Super shortcuts.
- Pointer capture can become invalid while animation or terminal resize changes
  arranged bounds.
- A general layout tree can add allocation and View latency unless plans and
  scratch storage are retained.
- Temporary adapters can become permanent unless each phase records what old
  arithmetic, routing, or dependencies it removed.

## Implementation Choices to Resolve with Evidence

These are not architectural reopenings. Record the selected values in the
decision log when the relevant phase supplies evidence.

- Concrete Go representation for layout elements: concrete tree, small
  interfaces at component boundaries, or a hybrid.
- Regular/compact density threshold and compact spacing tokens.
- Minimum usable canvas width for automatic sidebar placement.
- Scrollbar glyphs, track behavior, and whether automatic bars reserve space or
  overlay content.
- Motion duration, tick cadence, easing formula, and default motion setting.
- Focus indication when a visible docked sidebar does not own keyboard focus.
- Whether Help position and size persistence belongs in the initial surface
  phase or a later feature sweep.
- Whether the Huh file picker is adapted temporarily or replaced during Phase 7.
- Whether headless-terminal smoke runs in default CI, an integration target, or
  both.

## Decision Log

| Date | Decision | Evidence |
| --- | --- | --- |
| 2026-07-28 | Build an app-owned chrome toolkit and migrate incrementally away from Huh. | Existing extracted components still exchange rendered strings and require root geometry workarounds. |
| 2026-07-28 | Use intrinsic minimum/preferred measurement and semantic axis policies. | Width-dependent wrapping and content-derived minimums cannot be represented safely by fixed constants. |
| 2026-07-28 | Use one global regular/compact density mode. | Component-local breakpoints would recreate visual and sizing drift. |
| 2026-07-28 | Model sticky content as Pane slots around one Viewport. | Keeps scrolling explicit and permits multiple regions through composition. |
| 2026-07-28 | Keep application commands and binding declarations outside chrome. | Business changes must not require edits to generic package internals. |
| 2026-07-28 | Emit semantic command messages instead of storing behavior callbacks in binding data. | Preserves Bubble Tea ownership and component-local testing. |
| 2026-07-28 | Retain one versioned layout plan for render, input, focus, and diagnostics. | Reconstructing geometry during View or click handling permits stale or divergent hit targets. |
| 2026-07-28 | Keep placement policy in application declarations and generic z-order/capture mechanics in chrome. | Help anchors, outside-click dismissal, and dock/drawer choices are application behavior. |
| 2026-07-28 | Make Help a passive contextual inspector and Preferences an independent modal. | Help must observe current command resolution without changing focus context. |
| 2026-07-28 | Use adaptive docked/sidebar drawer placement. | Large terminals benefit from persistent navigation; constrained terminals must preserve canvas space. |
| 2026-07-28 | Place the existing sparse canvas through one invisible screen allocation without replacing its document viewport. | Docked chrome must shift screen placement without coupling chrome to canvas semantics. |
| 2026-07-28 | Keep the canvas host visually transparent. | A rendered border or frame would undermine the canvas's intentionally unbounded feel. |
| 2026-07-28 | Animate a workspace-owned boundary in docked mode and only the drawer in compact mode. | One transition keeps canvas/sidebar geometry coordinated and avoids unnecessary graph work. |
| 2026-07-28 | Use deterministic in-process tests plus focused headless-terminal scenarios. | Structural assertions provide broad coverage; a real PTY catches input-decoding and terminal-composition defects. |
| 2026-07-28 | Advertise the enhanced copy chord as `super+c`, matching Bubble Tea's chord name. | The binding, `Keystroke`, TUI guide, and existing characterization test all use Super; only the help label used `cmd`. |
| 2026-07-28 | Add no further Phase 1 characterization tests. | Existing tests cover nav geometry and activation, root toolbar placement and hover, resize-before-view behavior, constrained terminals, canvas composition, and cursor placement. |
| 2026-07-28 | Represent Phase 2 layout as a concrete `Node` tree with retained immutable `Plan` results. | The initial consumers need a small fixed set of mechanics; concrete nodes avoid interface dispatch and keep duplicate-ID validation and stable traversal explicit. |
| 2026-07-28 | Use `charmbracelet/x/ansi` for display width, wrapping, and truncation at text leaves. | It is already a direct dependency and preserves ANSI sequences, grapheme boundaries, combining marks, and wide cells consistently with existing TUI code. |
| 2026-07-28 | Clip below intrinsic minimum only as an emergency when the physical parent cannot contain minimums. | This keeps every arranged rectangle inside its parent while preserving minimum sizes whenever the terminal can satisfy them. |
| 2026-07-28 | Reserve scrollbar cells and converge monotonically from required bars to automatic bars. | Reserving one bar can induce overflow on the other axis; monotonic addition reaches the least stable arrangement without oscillation. |
| 2026-07-28 | Use `█` thumbs with `│` and `─` tracks for the initial viewport. | These single-cell glyphs remain legible at constrained sizes and make thumb bounds deterministic; the chrome lab can provide evidence for later theme tokens. |
| 2026-07-28 | Keep the headless chrome-lab smoke as an explicit integration command for now. | The smoke depends on the external `ht` daemon; deterministic model tests remain in the default Go suite while CI placement stays open for operational evidence. |
| 2026-07-28 | Test resize through `WindowSizeMsg` plus separate real PTYs at each required size. | The installed `ht` version has no live resize command; this combination verifies in-process reflow and real terminal composition without sleeps. |
| 2026-07-28 | Resolve commands to values and keep application behavior in `Update`. | Semantic messages preserve Bubble Tea ownership while allowing effective bindings and dispatch to share one registry. |
| 2026-07-28 | Retain requested surface rectangles and derive clamped rectangles from terminal, workspace, canvas, or dock anchors. | Moving and resizing must survive constrained terminals without losing application placement policy. |
| 2026-07-28 | Keep Help session-local and adapt the existing Huh Preferences body behind one chrome modal surface. | The foundational phase needs contextual inspection and one input router; persistence and declarative fields belong to later listed phases. |
| 2026-07-28 | Capture the transparent canvas host during pointer gestures. | Canvas drags must continue beneath higher floating surfaces after they start, while idle pointer input still follows z-order. |
| 2026-07-28 | Recompute root surfaces only for terminal, Help, or modal state changes. | Unconditional arrangement doubled interactive latency; retained invalidation restores the root View path to its Phase 2 benchmark envelope. |
| 2026-07-28 | Represent forms as application declarations backed by one retained `chrome.FormPlan`. | Semantic field, spacer, action-bar, and action IDs now drive rendering, input, focus, accessibility, and diagnostics without recovering geometry from rendered text. |
| 2026-07-28 | Keep Huh only as a bounded Preferences directory-picker adapter through Phase 7. | Replacing filesystem navigation would materially enlarge the preference migration; all ordinary fields, actions, sizing, and traversal now use chrome controls. |
| 2026-07-28 | Persist Auto, Mac, and Standard key profiles and apply edits live. | The resolver updates with the form value, Cancel restores the original projection, and Save writes the selected profile. |
| 2026-07-28 | Use one application-owned dialog-spec table keyed by distinct workspace surface IDs. | Preferences, Save, Export, and Notice now declare context, scopes, sizing, dismissal, content routing, and close behavior once; root no longer maintains a parallel modal enum or type switches. |
| 2026-07-28 | Select floating versus fullscreen placement solely from available fit. | Content floats when its shell fits below avoided rows and fills the terminal otherwise; the same rule covers every current dialog. |
| 2026-07-28 | Build confirmation as a semantic modal body without close policy. | The reusable body owns prompt layout, accessible text, and confirm/cancel action IDs while future unsaved-document decisions remain application behavior. |
| 2026-07-28 | Use a 26-cell preferred sidebar, 24-cell minimum, 48-cell minimum canvas, and compact placement at 80 columns or fewer. | The required PTY sizes then exercise docked push at `100x30` and compact overlay at both 80-column sizes while preserving a usable canvas. |
| 2026-07-28 | Advance workspace transitions every 16 ms with quadratic ease-out when opening and quadratic ease-in when closing. | Each update skips unchanged interpolation frames and exposes only a new integer cell; reversal and resize retarget from the current visible extent. |
| 2026-07-28 | Retain full surface content placement separately from its clipped pointer rectangle. | Drawers can translate a stable pane partly outside the terminal while input and rendering use the same visible bounds. |
| 2026-07-28 | Indicate sidebar focus only on its selected item. | Back can leave a docked sidebar visible without suggesting that its commands still own keyboard input. |
| 2026-07-28 | Keep Huh only behind `directorypicker` as the filesystem-navigation adapter. | Chrome forms now own every ordinary Save, Export, and Preferences control; replacing directory traversal remains a separate bounded change. |
| 2026-07-28 | Edit TextInput values as grapheme clusters and clip them by display cells. | Combining marks and wide glyphs then share one caret, paste, pointer, and rendering model without leaking Unicode arithmetic into applications. |
| 2026-07-28 | Delete `flex` and `numinput`, but retain `modal`, `nav`, and `preferences`. | Chrome fully supersedes the former layout and numeric-control mechanics; the retained packages still own distinct application content or presentation policy. |
| 2026-07-28 | Route concrete child messages separately in root Update. | Grouping message types forced unrelated mouse events to escape; concrete cases preserve semantic routing and the Phase 9 allocation envelope. |

## Changed-File Ledger

Update this table after each completed phase or reviewable slice.

| Phase | Files | Purpose |
| --- | --- | --- |
| 0 | `internal/tui/CHROME_PLAN.md` | Record architecture, migration gates, deferred sweeps, and verification strategy. |
| 1 | `internal/tui/keymap.go`, `internal/tui/CHROME_PLAN.md` | Align enhanced copy help with the registered Super chord and record the baseline. |
| 2 | `internal/tui/chrome/AGENTS.md`, `internal/tui/chrome/{geometry,layout,menu,text}.go`, `internal/tui/chrome/{layout,menu}_test.go`, `internal/tui/nav/{nav.go,nav_test.go}`, `internal/tui/{model,view,modal}.go`, `internal/tui/CHROME_PLAN.md` | Add retained chrome geometry and menu mechanics, migrate navigation and root placement, and remove duplicated toolbar geometry. |
| 3 | `internal/tui/chrome/{pane,viewport}.go`, `internal/tui/chrome/{pane,viewport}_test.go`, `internal/tui/CHROME_PLAN.md` | Add sticky panes, finite scrolling viewports, convergent reserved scrollbars, reveal, pointer translation, and nested independent scroll regions. |
| 4 | `internal/tui/cmd/chrome-lab/{main.go,main_test.go,README.md,smoke.sh}`, `internal/tui/CHROME_PLAN.md` | Add the interactive chrome lab, stable diagnostics, responsive scenarios, and failure-artifact PTY smoke runner. |
| 5 | `internal/tui/chrome/{command,focus}.go`, matching tests, `internal/tui/bindings.go`, `internal/tui/model.go`, chrome-lab files, `internal/tui/CHROME_PLAN.md` | Add semantic command/profile resolution, focus scopes and restoration, root declarations, and focus/profile/paste lab scenarios. |
| 6 | `internal/tui/chrome/{surface.go,surface_test.go}`, `internal/tui/{bindings,help,modal,model,model_test,mouse,preferences,theme,view,workspace}.go`, `internal/tui/cmd/chrome-lab/{main.go,main_test.go,smoke.sh}`, `internal/tui/CHROME_PLAN.md` | Add one surface/workspace router, transparent canvas host and footer, contextual passive Help, legacy Preferences adaptation, and surface lab scenarios. |
| 7 | `internal/tui/chrome/{AGENTS.md,form.go,form_test.go}`, `internal/tui/preferences/{AGENTS.md,preferences.go,preferences_test.go}`, deleted `internal/tui/preferences/{actions.go,row.go}`, `internal/tui/{modal.go,model_test.go,preferences.go,theme.go}`, `internal/tui/cmd/chrome-lab/{README.md,main.go,main_test.go,smoke.sh}`, `internal/tui/CHROME_PLAN.md` | Add retained declarative forms, migrate Preferences and persisted key profiles, bound the remaining Huh picker, remove rendered-text geometry recovery, and add form/picker lab coverage. |
| 8 | `internal/tui/modal/{AGENTS.md,confirmation.go,confirmation_test.go,modal.go,modal_test.go}`, `internal/tui/{bindings,clipboard_test,modal,model,model_test,preferences,save,view,workspace}.go`, `internal/tui/cmd/chrome-lab/{README.md,main.go,main_test.go,smoke.sh}`, `internal/tui/CHROME_PLAN.md` | Replace the modal enum with declarative dialog specs and distinct surfaces, add fit-driven placement and a reusable confirmation body, preserve application transactions, and cover current dialog lifecycles. |
| 9 | `internal/tui/chrome/{AGENTS.md,motion.go,motion_test.go,surface.go,surface_test.go}`, `internal/tui/{bindings,model,sidebar,sidebar_test,theme,view,workspace}.go`, `internal/tui/cmd/chrome-lab/{README.md,main.go,main_test.go,smoke.sh}`, `internal/tui/CHROME_PLAN.md` | Add workspace-owned cell transitions, animated dock/drawer geometry, an application-declared sidebar Pane, placement-aware focus and dismissal, and deterministic lab/root PTY coverage. |
| 10 | `go.mod`, `internal/tui/{AGENTS.md,bindings.go,clipboard_test.go,modal.go,model.go,model_test.go,preferences.go,save.go,theme.go}`, `internal/tui/chrome/{AGENTS.md,command_test.go,form.go,form_test.go,textinput.go,textinput_test.go}`, `internal/tui/clipboard/{AGENTS.md,clipboard.go,clipboard_test.go}`, `internal/tui/directorypicker/{AGENTS.md,directorypicker.go,directorypicker_test.go}`, `internal/tui/preferences/{AGENTS.md,preferences.go,preferences_test.go}`, `internal/tui/cmd/chrome-lab/{main.go,main_test.go,smoke.sh}`, deleted `internal/tui/flex/*`, deleted `internal/tui/numinput/*`, `internal/tui/CHROME_PLAN.md` | Add grapheme- and cell-aware text input, migrate Save and Export to declarative forms, isolate Huh filesystem navigation, remove superseded helpers and theme adapters, and record the final chrome architecture. |

## Verification Ledger

Record exact commands and results. Do not replace failed commands with a later
passing command without preserving the investigated failure.

| Date | Phase | Command | Result |
| --- | --- | --- | --- |
| 2026-07-28 | 0 | Documentation inspection only | Plan creation; no production code changed. |
| 2026-07-28 | 0 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/... ./cmd/dg` | Failed: `TestKeyboardEnhancementsAdvertiseSuperCopy` expects `super+c / ctrl+c`; model renders `cmd+c / ctrl+c`. Subpackages and `cmd/dg` passed. Resolve in Phase 1. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/... ./cmd/dg` (sandboxed) | Blocked before compilation: Go could not create the module cache under `/Users/codey.oxley/go`. Re-ran with module-cache access. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/... ./cmd/dg` (pre-change) | Failed only `TestKeyboardEnhancementsAdvertiseSuperCopy`: expected `super+c / ctrl+c`, got `cmd+c / ctrl+c`; all TUI subpackages and `cmd/dg` passed. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui -run '^TestKeyboardEnhancementsAdvertiseSuperCopy$' -count=1` | Passed. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/... ./cmd/dg` | Passed all TUI and command packages. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Apple M4 Max: AltDrag 46,249 ns/op, 10,920 B/op, 9 allocs/op; MoveCommittedDuplicate 47,742 ns/op, 10,937 B/op, 11 allocs/op; MoveAndView 5,049 ns/op, 4,680 B/op, 8 allocs/op. |
| 2026-07-28 | 1 | `ht run --size 100x30 --name dg-phase1-100x30 env GOCACHE=/private/tmp/dg-codex-go-build go run ./cmd/dg`; `ht view --json dg-phase1-100x30` | Passed: 100 columns by 30 rows; diagram, centered toolbar, and status row visible; cursor hidden at row 30, column 33. Session stopped and removed. |
| 2026-07-28 | 1 | `ht run --size 80x16 --name dg-phase1-80x16 env GOCACHE=/private/tmp/dg-codex-go-build go run ./cmd/dg`; `ht view --json dg-phase1-80x16` | Passed: 80 columns by 16 rows; diagram, centered toolbar, and status row visible; cursor hidden at row 16, column 33. Session stopped and removed. |
| 2026-07-28 | 1 | `ht run --size 80x12 --name dg-phase1-80x12 env GOCACHE=/private/tmp/dg-codex-go-build go run ./cmd/dg`; `ht view --json dg-phase1-80x12` | Passed: 80 columns by 12 rows; diagram, centered toolbar, and status row visible; cursor hidden at row 12, column 33. Session stopped and removed. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./...` | Passed all packages. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...` | Passed all packages. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build go vet ./...` | Passed. |
| 2026-07-28 | 1 | `GOCACHE=/private/tmp/dg-codex-go-build GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache golangci-lint run --path-mode abs` | Passed with 0 issues. |
| 2026-07-28 | 2 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/chrome ./internal/tui/nav ./internal/tui -count=1` | Passed chrome invariants, nav adapter, and root integration tests. |
| 2026-07-28 | 2 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/chrome -run '^$' -bench . -benchmem -count=1` | Apple M4 Max: Measure 4,639 ns/op, 3,952 B/op, 36 allocs/op; Arrange 6,167 ns/op, 4,448 B/op, 39 allocs/op; MenuRender 8,105 ns/op, 2,144 B/op, 89 allocs/op. |
| 2026-07-28 | 2 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Apple M4 Max: AltDrag 47,952 ns/op, 10,921 B/op, 9 allocs/op; MoveCommittedDuplicate 48,554 ns/op, 10,936 B/op, 11 allocs/op; MoveAndView 4,758 ns/op, 4,680 B/op, 8 allocs/op. Root View allocations are unchanged from Phase 1. |
| 2026-07-28 | 2 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./...` | Passed all packages. |
| 2026-07-28 | 2 | `GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...` | Passed all packages. |
| 2026-07-28 | 2 | `GOCACHE=/private/tmp/dg-codex-go-build go vet ./...` | Passed. |
| 2026-07-28 | 2 | `GOCACHE=/private/tmp/dg-codex-go-build GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache golangci-lint run --path-mode abs` | Passed with 0 issues. |
| 2026-07-28 | 2 | `ht run` and `ht view --json` at `100x30`, `80x16`, and `80x12` | Passed: diagram, centered toolbar, status row, and hidden cursor matched Phase 1 at every size. All sessions stopped and removed. |
| 2026-07-28 | 2 | `dd-gopls check ./internal/tui/chrome/... ./internal/tui/nav/... ./internal/tui/...` | Did not run diagnostics: `check` accepts file paths, not package patterns. Re-ran against each changed Go file. |
| 2026-07-28 | 2 | `dd-gopls check <changed-go-file>` for all nine changed Go files | Passed with no diagnostics. |
| 2026-07-28 | 3 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/chrome -count=1` | Passed pane, viewport, overflow matrix, convergence, reveal, pointer, constrained-size, and nesting tests. |
| 2026-07-28 | 3 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/chrome -run '^$' -bench 'BenchmarkViewportArrange' -benchmem -count=1` | Apple M4 Max: 2,469 ns/op, 1,068 B/op, 31 allocs/op. |
| 2026-07-28 | 3 | Full test, race, vet, and lint gate | Tests, race, and vet passed; lint failed with four `goconst` findings in tests and one `ineffassign` in viewport convergence. Fixed all five findings and re-ran the complete gate. |
| 2026-07-28 | 3 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./...` | Passed all packages after lint fixes. |
| 2026-07-28 | 3 | `GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...` | Passed all packages after lint fixes. |
| 2026-07-28 | 3 | `GOCACHE=/private/tmp/dg-codex-go-build go vet ./...` | Passed after lint fixes. |
| 2026-07-28 | 3 | `GOCACHE=/private/tmp/dg-codex-go-build GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache golangci-lint run --path-mode abs` | Passed with 0 issues after fixes. |
| 2026-07-28 | 3 | `ht run` and `ht view --json` at `100x30`, `80x16`, and `80x12` | Passed: exact dimensions, toolbar, status, and hidden cursor confirmed. All sessions stopped and removed. |
| 2026-07-28 | 3 | `dd-gopls check <phase-3-go-file>` for all four Phase 3 Go files | Passed with no diagnostics. |
| 2026-07-28 | 4 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/cmd/chrome-lab -count=1` | Passed interaction and resize-before-view tests. |
| 2026-07-28 | 4 | `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed at `100x30`, `80x16`, and `80x12`; exercised keyboard, wheel, click, drag, and raw SGR input with wait-coupled sends and cleaned all sessions. |
| 2026-07-28 | 4 | Full test, race, vet, and lint gate | Tests, race, and vet passed; lint failed on three repeated scenario-name literals. Replaced them with semantic constants and re-ran the complete gate and smoke. |
| 2026-07-28 | 4 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./...` | Passed all packages after lint fixes. |
| 2026-07-28 | 4 | `GOCACHE=/private/tmp/dg-codex-go-build go test -race ./...` | Passed all packages after lint fixes. |
| 2026-07-28 | 4 | `GOCACHE=/private/tmp/dg-codex-go-build go vet ./...` | Passed after lint fixes. |
| 2026-07-28 | 4 | `GOCACHE=/private/tmp/dg-codex-go-build GOLANGCI_LINT_CACHE=/private/tmp/dg-codex-golangci-cache golangci-lint run --path-mode abs` | Passed with 0 issues after fixes. |
| 2026-07-28 | 4 | `./internal/tui/cmd/chrome-lab/smoke.sh` (post-fix) | Passed all three sizes. |
| 2026-07-28 | 4 | Root `ht run` and `ht view --json` at `100x30`, `80x16`, and `80x12` | Passed: toolbar and status visible, cursor hidden. All sessions stopped and removed. |
| 2026-07-28 | 4 | `dd-gopls check internal/tui/cmd/chrome-lab/{main.go,main_test.go}` | Passed with no diagnostics. |
| 2026-07-28 | 4 | `bash -n internal/tui/cmd/chrome-lab/smoke.sh` | Passed. |
| 2026-07-28 | 5 | Narrow chrome/root tests | Initially failed because global Save intercepted modal input, then because it bypassed existing-path save behavior. Restricted canvas scopes while modals are active and routed semantic Save through `requestSave`; passed. |
| 2026-07-28 | 5 | Full test, race, vet, and lint gate | Tests, race, and vet passed; lint found repeated test fixtures in two runs. Consolidated semantic fixture constants and re-ran. |
| 2026-07-28 | 5 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./...`; `go test -race ./...`; `go vet ./...`; `golangci-lint run --path-mode abs` | Passed all packages with 0 lint issues. |
| 2026-07-28 | 5 | `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed focus, profile, bracketed paste, pointer, and all prior scenarios at all three sizes. |
| 2026-07-28 | 6 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/chrome ./internal/tui -count=1` | Passed the initial surface/workspace and root canvas-host slice. Phase 6 remains incomplete and has not run its integration gate. |
| 2026-07-28 | 6 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/... ./cmd/dg -count=1` | Passed surface, canvas-host, Help, legacy Preferences, lab, and all existing TUI/command tests. |
| 2026-07-28 | 6 | Initial `go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Exposed unconditional surface arrangement: AltDrag 106,737 ns/op and 126 allocs/op; MoveAndView 52,366 ns/op and 127 allocs/op. Added retained invalidation and removed unchanged-frame plan copies. |
| 2026-07-28 | 6 | Final `go test ./internal/tui/chrome -run '^$' -bench . -benchmem -count=1`; `go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Apple M4 Max: Measure 4,598 ns/op, Arrange 6,098 ns/op, MenuRender 8,267 ns/op, ViewportArrange 2,414 ns/op. Root AltDrag 45,911 ns/op, 10,920 B/op, 9 allocs/op; MoveCommittedDuplicate 46,193 ns/op, 10,985 B/op, 13 allocs/op; MoveAndView 4,772 ns/op, 4,728 B/op, 10 allocs/op. |
| 2026-07-28 | 6 | First full test, race, vet, and lint gate | Tests, race, and vet passed. Lint reported two complexity thresholds, one repeated lab surface ID, one switch simplification, and two superseded helpers. Extracted focused helpers and removed obsolete Help/height paths. |
| 2026-07-28 | 6 | Sandboxed `dd-gopls check <all changed Go files>` and `./internal/tui/cmd/chrome-lab/smoke.sh` | Both were environment-blocked: `dd-gopls` could not write normal Go caches and `ht` could not reach its daemon socket. Re-ran with the required local cache/socket access. |
| 2026-07-28 | 6 | `dd-gopls check <all changed Go files>` | Passed with no diagnostics. |
| 2026-07-28 | 6 | First unsandboxed `./internal/tui/cmd/chrome-lab/smoke.sh` | Failed at the new surface scenario because `8` was correctly typed into the focused text field. Switched scenarios with `<Right>` to preserve text-entry precedence. |
| 2026-07-28 | 6 | Final `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed all three sizes with surface z-order, modal capture/dismissal, passive Help, focus, paste, pointer, and prior scenarios; all sessions cleaned up. |
| 2026-07-28 | 6 | First root `ht` check at `100x30`, `80x16`, and `80x12` | Screen assertions passed, but the harness left passing JSON snapshots in its temporary directory and failed cleanup. Removed the snapshots and reran without pass artifacts. |
| 2026-07-28 | 6 | Final root `ht run`, `ht wait`, and `ht view --json` at `100x30`, `80x16`, and `80x12` | Passed exact dimensions, toolbar, status, and hidden-cursor assertions; all sessions stopped and removed. |
| 2026-07-28 | 6 | Final `go test ./...`; `go test -race ./...`; `go vet ./...`; `golangci-lint run --path-mode abs` | Passed all packages with 0 lint issues. |
| 2026-07-28 | 7 | Initial narrow chrome, Preferences, and root tests | Compilation exposed all remaining tests coupled to removed Huh fields and geometry. After replacing those tests, root picker tests still assumed Huh traversal commands and mouse actions were asynchronous. Rewrote them against semantic IDs and made pointer activation synchronous at the Preferences boundary. |
| 2026-07-28 | 7 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/chrome ./internal/tui/preferences ./internal/tui/cmd/chrome-lab ./internal/tui` | Passed declarative form, Preferences, lab, and root integration tests. |
| 2026-07-28 | 7 | First `./internal/tui/cmd/chrome-lab/smoke.sh` | All form interactions reached the final state at `80x12`; the final snapshot failed only because a redundant click-count diagnostic moved below the constrained viewport. Removed that final assertion while retaining the wait-coupled click checks. |
| 2026-07-28 | 7 | Final `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed form traversal, live profile context, nested picker, surfaces, focus, paste, pointer, and prior scenarios at all three sizes; all sessions cleaned up. |
| 2026-07-28 | 7 | Root `ht` interaction sweep at `100x30`, `80x16`, and `80x12` | Passed exact dimensions, enhanced-key Preferences open, nested picker open/close, live Mac profile selection, Cancel restoration path, toolbar/status composition, and hidden cursor; all sessions stopped and removed. |
| 2026-07-28 | 7 | `go test ./internal/tui/chrome -run '^$' -bench . -benchmem -count=1`; `go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Apple M4 Max: Measure 5,364 ns/op, Arrange 6,545 ns/op, MenuRender 8,309 ns/op, ViewportArrange 2,450 ns/op. Root AltDrag 48,308 ns/op, 10,921 B/op, 9 allocs/op; MoveCommittedDuplicate 47,765 ns/op, 10,952 B/op, 12 allocs/op; MoveAndView 5,418 ns/op, 4,696 B/op, 9 allocs/op. |
| 2026-07-28 | 7 | First full test, race, vet, and lint gate | Tests, race, and vet passed. Lint reported four repeated literals and one lab complexity threshold. Consolidated semantic constants and extracted lab key handling. |
| 2026-07-28 | 7 | `dd-gopls check <all changed Phase 7 Go files>` | Passed with no diagnostics. |
| 2026-07-28 | 7 | Final `go test ./...`; `go test -race ./...`; `go vet ./...`; `golangci-lint run --path-mode abs` | Passed all packages with 0 lint issues after the lint fixes. |
| 2026-07-28 | 7 | Post-fix `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed all three terminal sizes after extracting lab key handling. |
| 2026-07-28 | 8 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/modal ./internal/tui/cmd/chrome-lab ./internal/tui` | Passed confirmation, fit selection, distinct surface, scope, movement, resizing, lifecycle, and root integration tests. |
| 2026-07-28 | 8 | First dialog-lifecycle `./internal/tui/cmd/chrome-lab/smoke.sh` | Reached the notice state correctly; the harness then expected Notice to remain open after Escape. Corrected the wait target to distinguish outside-click preservation from Back dismissal. |
| 2026-07-28 | 8 | Final `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed Save, Export, Notice, confirmation, outside-click policy, Back, forms, surfaces, and all prior scenarios at all three sizes. |
| 2026-07-28 | 8 | First root Save/Export `ht` sweep | Save opened correctly, but the harness expected `File name` while the active bounded file picker intentionally showed only `Directory`. Removed the stale assertion. |
| 2026-07-28 | 8 | Final root `ht` Save/Export sweep at `100x30`, `80x16`, and `80x12` | Passed exact dimensions, Save open/Back, selected-node double-copy Export open/Back, option content, fit, and hidden cursor; all sessions stopped and removed. |
| 2026-07-28 | 8 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Apple M4 Max: AltDrag 45,959 ns/op, 10,920 B/op, 9 allocs/op; MoveCommittedDuplicate 46,094 ns/op, 10,985 B/op, 13 allocs/op; MoveAndView 4,774 ns/op, 4,728 B/op, 10 allocs/op. |
| 2026-07-28 | 8 | First full test, race, vet, and lint gate | Tests, race, and vet passed. Lint reported repeated semantic strings, two preallocations, and one switch simplification. Reused typed IDs, preallocated retained slices, and simplified the command branch. |
| 2026-07-28 | 8 | Final `go test ./...`; `go test -race ./...`; `go vet ./...`; `golangci-lint run --path-mode abs` | Passed all packages with 0 lint issues. |
| 2026-07-28 | 8 | `dd-gopls check <all changed Phase 8 Go files>` | Passed with no diagnostics. |
| 2026-07-28 | 9 | `GOCACHE=/private/tmp/dg-codex-go-build go test ./internal/tui/chrome ./internal/tui/cmd/chrome-lab ./internal/tui -count=1` | Passed cell easing, retarget, reversal, disabled motion, dock/drawer geometry, every-boundary coordinate round trips, focus restoration, clipping, pointer, resize, existing off-screen drag/rebase behavior, lab, and root integration tests. |
| 2026-07-28 | 9 | First sidebar `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed `100x30` and `80x16`; `80x12` reached the correct drawer state but the motion-enabled diagnostic was below the constrained viewport. Moved required transition fields into the visible diagnostic prefix. |
| 2026-07-28 | 9 | Final `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed docked push, compact overlay, retarget, reversal controls, disabled motion, Back, and all prior scenarios at all three sizes; all sessions cleaned up. |
| 2026-07-28 | 9 | Root sidebar `ht` sweep at `100x30`, `80x16`, and `80x12` | Passed settled rendering, exact dimensions, hidden cursor, docked Back persistence at `100x30`, compact Back/outside dismissal at both 80-column sizes, and clean session removal. |
| 2026-07-28 | 9 | `go test ./internal/tui/chrome -run '^$' -bench . -benchmem -count=1`; `go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Apple M4 Max: Measure 4,584 ns/op, Arrange 6,390 ns/op, MenuRender 8,176 ns/op, ViewportArrange 2,433 ns/op. Root AltDrag 49,169 ns/op, 10,921 B/op, 9 allocs/op; MoveCommittedDuplicate 47,979 ns/op, 10,984 B/op, 13 allocs/op; MoveAndView 4,975 ns/op, 4,728 B/op, 10 allocs/op. |
| 2026-07-28 | 9 | First `golangci-lint run --path-mode abs` | Reported one repeated lab Back chord and root `Update` complexity 34. Reused one semantic chord constant and extracted workspace invalidation policy. |
| 2026-07-28 | 9 | Post-fix narrow tests and `golangci-lint run --path-mode abs` | Passed affected chrome, lab, and root packages with 0 lint issues. |
| 2026-07-28 | 9 | Final `go test ./...`; `go test -race ./...`; `go vet ./...`; `golangci-lint run --path-mode abs` | Passed all packages with 0 lint issues. |
| 2026-07-28 | 9 | `dd-gopls check <all changed Phase 9 Go files>` | Passed with no diagnostics. |
| 2026-07-28 | 10 | Narrow chrome, directory-picker, clipboard, Preferences, root, and command tests | Passed TextInput grapheme editing, display-cell clipping, paste sanitization, pointer caret placement, form traversal/accessibility, bounded picker lifecycle, Save/Export routing, and existing TUI integration behavior. |
| 2026-07-28 | 10 | Sandboxed `go mod tidy` | Failed while resolving module metadata because sandboxed network access could not reach the module proxy. Re-ran with module-cache/network access. |
| 2026-07-28 | 10 | `go mod tidy` | Passed; removed the unused direct `golang.org/x/exp` requirement and retained Huh because `directorypicker` remains the bounded adapter. |
| 2026-07-28 | 10 | First `golangci-lint run --path-mode abs` | Reported one incomplete `FieldKind` switch, repeated semantic literals, and root `Update` complexity 31. Completed the switch, consolidated constants, and extracted child routing. |
| 2026-07-28 | 10 | First root Save/Export `ht` matrix | Save and Export rendered correctly, but the harness clicked a toolbar/status cell and ambiguously matched copy guidance. Corrected the raw node coordinate to row 2, column 7 and required the exact `selected  nodes 1` status. |
| 2026-07-28 | 10 | Root Save/Export `ht` matrix at `100x30`, `80x16`, and `80x12` | Passed Save fields/actions, nested-picker visibility and Back restoration, text editing and paste, cancel-without-write, node selection, double-copy Export, option changes, exact dimensions, and hidden cursor; all sessions stopped and removed. |
| 2026-07-28 | 10 | Initial `go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Added one 32-byte AltDrag allocation. An allocation probe isolated a grouped child-message type switch; concrete routing restored the prior hot path. |
| 2026-07-28 | 10 | Final `go test ./internal/tui/chrome -run '^$' -bench . -benchmem -count=1`; `go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem -count=1` | Apple M4 Max: Measure 4,924 ns/op, Arrange 6,360 ns/op, MenuRender 8,276 ns/op, ViewportArrange 2,432 ns/op. Root AltDrag 47,101 ns/op, 10,921 B/op, 9 allocs/op; MoveCommittedDuplicate 46,487 ns/op, 10,952 B/op, 12 allocs/op; MoveAndView 4,982 ns/op, 4,696 B/op, 9 allocs/op. Allocation counts match or improve on Phase 9. |
| 2026-07-28 | 10 | First `dd-gopls check <changed files>` | Failed because zsh preserved the newline-delimited path list as one argument. Re-ran with a path array containing every existing changed or untracked Go file. |
| 2026-07-28 | 10 | `dd-gopls check <all changed Phase 10 Go files>` | Passed with no diagnostics. |
| 2026-07-28 | 10 | Final `go test ./...`; `go test -race ./...`; `go vet ./...`; `golangci-lint run --path-mode abs` | Passed all packages with 0 lint issues. |
| 2026-07-28 | 10 | `./internal/tui/cmd/chrome-lab/smoke.sh` | Passed TextInput typing/paste, forms, bounded picker, dialogs, sidebar, and all prior scenarios at `100x30`, `80x16`, and `80x12`; all sessions cleaned up. |
| 2026-07-28 | 10 | First final root `ht` harness attempts | The harness waited for non-rendered modal titles and sent enhanced Ctrl-C instead of the terminal's C0 copy byte. Inspected the live PTY, switched assertions to retained form content, and used the decoded copy sequence. |
| 2026-07-28 | 10 | Final root `ht` Save/Export sweep at `100x30`, `80x16`, and `80x12` | Passed Save open, nested picker open/Back, form restoration, Cancel, exact node selection, double-copy Export, option input, exact dimensions, and hidden cursor; all sessions stopped and removed. |
| 2026-07-28 | 10 | Public declaration and dependency audit with `rg` across chrome structs, setters, obsolete wrappers, deleted packages, and Huh imports | Every remaining declaration supports application use, retained diagnostics, or testable policy. No obsolete component wrappers remain; only `directorypicker` imports Huh. |
