# TUI package guide

## Responsibility

`internal/tui` is a Bubble Tea v2 frontend. It translates input into
`layout.Layout` mutations and renders retained `render.Frame` values. Engine
semantics must remain outside this package.

## Architecture

`Model` owns frontend state:

- viewport, cursor, active tool, focus, and modal state
- mouse, drag, resize, label-edit, and reconnect interactions
- committed and preview frames
- reusable encoders, highlights, row spans, and view buffers
- save, preferences, clipboard, and notice state

`Theme` in `theme.go` owns terminal-facing colors and styles. Lip Gloss layers
and a compositor place modals over the canvas. Bubbles help renders shortcuts.
Tabs use small local state based on the official Bubble Tea example.
Save, export, and preferences use `huh/v2`.

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

Cache Lip Gloss-rendered toolbar and highlight spans. Rendering every cell
through `lipgloss.Style` causes large allocation and latency regressions.
Profile drag changes with the existing TUI benchmarks and `go tool pprof`.

## Settings and clipboard

The settings modal overlays the diagram and can move by dragging its top
border. Tab and Shift-Tab switch Shortcuts and Preferences. Esc, `q`, or an
outside click closes it.

Router preferences apply live. Cancel, close, outside click, or lost focus
restores their original values. Numeric fields change only with Left and Right
and briefly highlight the pressed arrow. The directory field uses
`huh.NewFilePicker`.

Copy uses Super-C or Ctrl-C. The first copy waits 100 ms. A second copy in that
window cancels the provisional write and opens Export. Stale timers must never
write after another interaction.

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
