# TUI package guide

## Responsibility

`internal/tui` is a Bubble Tea v2 frontend. It translates input into
`layout.Layout` mutations and renders retained `render.Frame` values. Engine
semantics must remain outside this package.

## Architecture

The root `Model` owns editor coordination:

- viewport, cursor, active tool, focus, workspace surfaces, and dialog state
- mouse, drag, resize, edge-bend, label-edit, and reconnect interactions
- save, preference transactions, document selection export, and notices
- active Store entry, catalog reconciliation, canvas switching, and autosave

Composable presentation models live in sub-packages:

- `canvas` owns canvas styles, encoders, retained frames, and row indexes.
- `chrome` owns declarative layout, forms, text input, focus, surfaces, panes,
  viewports, and cell-aware transitions.
- `clipboard` owns copy debounce, export formatting and chrome form state,
  native multi-format writes, and terminal fallback probing.
- `directorypicker` owns bounded directory-only filesystem navigation.
- `nav` owns floating tool navigation styles, geometry, hover, and activation.
- `modal` owns modal and tab styles, sizing, full-screen fallback, movement,
  and pointer hit testing.
- `preferences` owns preference field declarations, value projection, and
  persistence-facing actions.

The root configures these models and translates their semantic `tea.Msg`
values into editor actions. A retained dialog controller owns active dialog
identity, shell geometry, pointer capture, and body routing. Stateful dialog
bodies own forms, pickers, focus, and drafts. Component interactions cross
boundaries through `tea.Msg` and `tea.Cmd`; do not reach into child state from
root.

Bubbles help renders shortcuts. Save, Export, and Preferences use chrome forms.
`directorypicker` provides the only filesystem-navigation surface.

The sparse canvas renderer remains custom. Generic viewports do not model
document coordinates, occlusion, preview ownership, or its hot-path needs.

## Interaction rules

- the canvas covers the terminal and the terminal-centered toolbar floats
  above it; dock motion never shifts the toolbar
- cursor display is limited to label editing
- left drag moves objects or creates rectangles and lines
- rectangle and line tools remain active after successful creation so repeated
  drags create repeated objects; q returns either tool to Cursor before a
  subsequent q quits
- the line tool always waits for a fresh source drag; selecting a port or edge
  before activation does not seed a connection or reconnect
- a line press and release snap to the closest usable port within two Manhattan
  cells
- right drag resizes from the nearest corner
- right drag on a node body resizes from the nearest corner; otherwise, right
  drag within three Manhattan cells of a visible edge corner pins and moves
  that bend along one dominant axis; selected edges win ambiguous bend hits
- double-click restores automatic node sizing
- double-clicking an edge clears all of its pinned bends
- Alt-drag duplicates selected nodes and their internal edges
- Ctrl constrains selection moves and Alt-drag duplication to the dominant
  horizontal or vertical axis
- Ctrl-click toggles non-contiguous selection
- Ctrl-A expands to connected components, then selects everything
- edge endpoint dragging begins only after pointer movement
- selected edges win ambiguous hit priority near their ports
- previews use the engine's routing and raster APIs
- one completed interaction produces one history transaction
- interruption commits the last visible placement

Selection may move partly outside the viewport while a visible part remains.
When movement would cross unsigned coordinate zero, rebase geometry and the
viewport together.

## Performance

Connection and bend previews reuse a retained frame with the edited edge
removed, then rasterize only the draft route over it. Node-only duplicate
previews layer over committed frames without routing.
Rigid committed moves skip routing when static edges cannot be affected.
Keep preview geometry separate from committed geometry.

Completed history changes schedule a 500 ms autosave. Switching synchronously
persists a dirty document and flushes dirty history before reusing the same
Document and Layout storage for the destination. Each canvas restores its own
UUID-keyed history. Clean switches skip document and history writes.

Ctrl-N replaces the active state with a pristine memory-only draft. The first
semantic mutation or an explicit name materializes its Store entry. Once
materialized, undoing or deleting all content still autosaves the empty document.

External modifications to the active canvas prompt before replacement. Loading
one records a whole-document undo boundary; keeping local content asks Store to
preserve the external bytes as a backup. External deletion can recreate the
named record or keep the rendered document as the active draft. A replacement
with another UUID preserves the old canvas as a draft and opens the replacement
without an undo boundary between identities. Repeated invalidations update the
pending conflict to the latest filesystem state.

The owner event loop serializes autosave, switching, conflict resolution, and
flush requests. `RequestFlush` acknowledges after both the active document and
dirty history finish writing. Normal quit and the first SIGINT or SIGTERM use
that boundary; a second signal exits immediately. Bubble Tea restores terminal
state after a panic, then bounded cleanup flushes for at most two seconds and
re-panics the retained value.

Each component defines its own `Styles`; root `Theme` configures them. Derive
component dimensions from border and padding geometry. Cache Lip
Gloss-rendered navigation and highlight spans. Rendering every cell through
`lipgloss.Style` causes large allocation and latency regressions.

Profile drag changes with the existing TUI benchmarks and `go tool pprof`.
`BenchmarkModelConnectionPreviewHighWater` sends Bubble Tea click and motion
messages through the complete link-drag update and view path for fresh, active
stress, and post-deletion layouts.
`BenchmarkModelHorizontalScrollHighWater` measures motion and view generation
near the far edge of the 200-cluster layout.
`BenchmarkModelSelectionHighWater` measures selecting and clearing every
cluster. `BenchmarkModelDragAllHighWater` measures rigid motion of the complete
layout.

## Dialogs, settings, sidebar, and clipboard

Name Canvas, Export, Preferences, Confirmation, and Notice declare distinct
workspace surfaces through one dialog controller. Left-dragging empty dialog cells moves a
floating shell; right-dragging resizes it. Fit alone selects floating or
full-screen placement. One retained dialog plan supplies rendering and local
pointer coordinates. Back and outside-click behavior comes from each
declaration.

The preferences model reports an editable draft; root previews router, shortcut,
and semantic tint changes and owns the layout history transaction. Cancel,
close, outside click, or lost focus restores the session baseline. Settings
persistence succeeds before root commits the layout-backed preview and promotes
the draft. Numeric fields change only with Left and Right or `h` and `l`, and
briefly highlight the pressed arrow. Independent dark and light tint selectors
follow the terminal background without painting the canvas. The directory field
opens the bounded filesystem adapter. Save applies the current form; Save as
Defaults also enables the persisted router for new diagrams. Growing form
spacers anchor actions to the body bottom.

Enter submits the primary action from ordinary form fields: Save in
Preferences, Copy in Export, and Name Canvas in the naming form. Fields with
their own Enter action take precedence, including the preferences directory
picker. Enter on an explicitly focused button submits that button. The naming
form right-justifies compact text values opposite their field labels.

The sidebar uses an application-declared Pane with separate Canvases and Drafts
tabs. Each tab owns half of the header content width and supports pointer and
keyboard focus and activation. Header, inactive, hovered, focused, and active
tab styles remain independent. Every list row reserves a two-cell focus prefix,
and the active canvas remains styled independently from keyboard focus.
Canvases groups root records and collapsible one-level sections with independent
normal and focused styles; Drafts shows newest-first modification times and a
separately styled clear action. Width follows the
widest known content with a 30-cell minimum and remains stable across tabs and
collapse. It docks only while preserving 48 canvas cells and otherwise becomes
a drawer. One workspace transition owns the boundary and canvas origin. Back
leaves a docked sidebar visible but returns keyboard focus to the canvas; Back
or an outside click dismisses a drawer. Ctrl-B opens, focuses, refocuses, or
closes it.

New sessions open the sidebar without taking canvas focus. Any pointer click
inside the sidebar focuses it, including its header, empty body space, and
scrollbar. The floating navigation remains terminal-anchored while a dock
opens or closes. It stays on the canvas fast path when fully contained and
uses its compositor layer only where a dock overlaps it.

Dragging a named canvas onto a section header or one of its canvases moves it
to that section. Dropping it on the Canvases header or unused list space moves
it to the preferred directory root. A click without pointer motion opens the
canvas on release. Drafts do not participate in section dragging.

Backspace or Delete on a named canvas moves it to Drafts without changing its
document identity. The active canvas flushes before demotion and remains open
as that draft. Delete permanently removes an inactive draft; the active draft
remains protected. Clear Drafts preserves the active draft.

Root renders the selected cells and pairs that text with a portable layout
fragment. Native clipboard writes publish both formats atomically. External
applications consume the rendered text; paste into dg validates and inserts
the fragment. Same-canvas paste offsets the fragment like duplication, while
cross-canvas paste roots it at the cursor. Edge-only selections remain
text-only because portable fragments require at least one node. Repeated paste
of the same fragment advances by the fragment width and the duplication gap.
Export changes the plain-text wrapper without discarding the fragment.

Copy uses Super-C or Ctrl-C. The first copy waits 175 ms. A second copy in that
window cancels the provisional write and opens Export. Passive all-motion
mouse events and standalone modifier-key events do not cancel this window.
Stale timers must never write after another interaction.

The first actual clipboard write prefers the internal native backend. It
publishes plain text and `application/vnd.dg.fragment` in one clipboard
replacement. If native initialization or writing fails, a 100 ms terminal
probe selects the OSC52 text-only fallback. Structural paste stays unavailable
when only OSC52 works. Advertise Super-C only after a keyboard enhancement
message.

## Keymap

- `r`: rectangle tool
- `l`: line tool
- `e`: edit label
- `b`: cycle borders
- `-`: toggle dashed stroke
- `a` and `A`: cycle endpoint arrows
- `t` and `T`: cycle text alignment
- arrow keys: move selection
- Tab and Shift-Tab: cycle node focus
- Backspace or Delete: delete selection
- `d`: duplicate
- `[` and `]`: move one layer
- `{` and `}`: send to back or front
- `u` or Ctrl-Z: undo
- Ctrl-R or Ctrl-Y: redo
- Ctrl-S: name a draft or report/save named-canvas autosave
- Ctrl-N: create and switch to a durable blank draft
- `?`: help and preferences
- Ctrl-B: open or close sidebar
- `q`: quit

## Areas for improvement

- frontend integration for engine-level snapping and guide previews
- selection-wide layer commands
- copy and duplication of portless lines
- preferences for comment style and future custom shapes
- more mouse coverage in headless-terminal
- additional no-route drag cases

## Verification

Use model tests for state transitions and history boundaries. Use
`testing/synctest` for ticks, notices, clipboard probes, and debounces. Keep
assertions outside `b.Loop`.

Run the package benchmarks after changes to view rendering, highlighting,
dragging, previews, or routing decisions:

```sh
GOCACHE=/private/tmp/dg-codex-go-build \
  go test ./internal/tui -run '^$' -bench 'BenchmarkModel' -benchmem
```

Use the root headless-terminal procedure for visual changes.
