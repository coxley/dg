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
- `clipboard` owns copy debounce, export formatting and chrome form state, terminal
  capability probing, and fallback writes.
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

- the canvas covers the terminal and the centered toolbar floats above it
- cursor display is limited to label editing
- left drag moves objects or creates rectangles and lines
- the line tool always waits for a fresh source drag; selecting a port or edge
  before activation does not seed a connection or reconnect
- a line press and release snap to the closest usable port within two Manhattan
  cells
- right drag resizes from the nearest corner
- right drag within three Manhattan cells of a visible edge corner pins and
  moves that bend along one dominant axis; selected edges win ambiguous bend
  hits
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

Save, Export, Preferences, and Notice declare distinct workspace surfaces
through one dialog controller. Left-dragging empty dialog cells moves a
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

The sidebar uses an application-declared Pane. It docks at wide regular sizes
and becomes a compact overlay drawer at 80 columns or fewer. One workspace
transition owns the dock boundary and canvas origin. Back leaves a docked
sidebar visible but returns keyboard focus to the canvas; Back or an outside
click dismisses a drawer. Ctrl-B opens, focuses, refocuses, or closes it.

Root renders the selected cells, then sends that text to the clipboard model.
Copy uses Super-C or Ctrl-C. The first copy waits 300 ms. A second copy in that
window cancels the provisional write and opens Export. Passive all-motion
mouse events and standalone modifier-key events do not cancel this window.
Stale timers must never write after another interaction.

The first actual clipboard write probes terminal OSC52 support for 100 ms.
A response selects `tea.SetClipboard`; timeout selects
`golang.design/x/clipboard`. Advertise Super-C only after a keyboard
enhancement message.

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
- Ctrl-S: save
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
