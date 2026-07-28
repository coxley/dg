# TUI package guide

## Responsibility

`internal/tui` is a Bubble Tea v2 frontend. It translates input into
`layout.Layout` mutations and renders retained `render.Frame` values. Engine
semantics must remain outside this package.

## Architecture

The root `Model` owns editor coordination:

- viewport, cursor, active tool, focus, and modal state
- mouse, drag, resize, label-edit, and reconnect interactions
- save, preference transactions, document selection export, and notices

Composable presentation models live in sub-packages:

- `canvas` owns canvas styles, encoders, retained frames, and row indexes.
- `clipboard` owns copy debounce, export formatting and form state, terminal
  capability probing, and fallback writes.
- `nav` owns floating tool navigation styles, geometry, hover, and activation.
- `modal` owns modal and tab styles, sizing, full-screen fallback, movement,
  and pointer hit testing.
- `numinput` owns bounded numeric stepping and directional feedback; its Huh
  adapter keeps form traversal separate.
- `preferences` owns the Huh preferences form, its numeric children, sizing,
  navigation, and editable value.

The root configures these models and translates their semantic `tea.Msg`
values into editor actions. Component interactions cross boundaries through
`tea.Msg` and `tea.Cmd`; do not reach into child state from root.

Bubbles help renders shortcuts. Save, export, and preferences use `huh/v2`.

The sparse canvas renderer remains custom. Generic viewports do not model
document coordinates, occlusion, preview ownership, or its hot-path needs.

## Interaction rules

- the canvas covers the terminal and the centered toolbar floats above it
- cursor display is limited to label editing
- left drag moves objects or creates rectangles and lines
- right drag resizes from the nearest corner
- double-click restores automatic node sizing
- Alt-drag duplicates selected nodes and their internal edges
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

Node-only duplicate previews layer over committed frames without routing.
Rigid committed moves skip routing when static edges cannot be affected.
Keep preview geometry separate from committed geometry.

Each component defines its own `Styles`; root `Theme` configures them. Derive
component dimensions from border and padding geometry. Cache Lip
Gloss-rendered navigation and highlight spans. Rendering every cell through
`lipgloss.Style` causes large allocation and latency regressions.

Profile drag changes with the existing TUI benchmarks and `go tool pprof`.

## Settings and clipboard

The settings modal overlays the diagram. Left-dragging empty modal cells moves
it; right-dragging resizes from the nearest corner. Tab and Shift-Tab switch
Shortcuts and Preferences. Esc, `q`, or an outside click closes it.

The preferences model reports editable values; root applies router changes
live and owns the layout history transaction. Cancel, close, outside click, or
lost focus restores the original values. Numeric fields change only with Left
and Right or `h` and `l`, and briefly highlight the pressed arrow. The
directory field opens `huh.NewFilePicker` as a temporary subview. Save applies
the current form; Save as Defaults also enables the persisted router for new
diagrams. The form renders at its natural height when the terminal allows it
and follows explicit modal resizing. Settings tabs share the larger tab's
default size when both fit.

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
