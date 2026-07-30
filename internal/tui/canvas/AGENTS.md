# Canvas package

## Responsibility

`canvas` owns terminal-facing drawing styles, reusable render encoders, retained
base and preview frames, and row indexes used by viewport rendering.

## Boundaries

- Root `tui` owns editor interactions, selections, history, and layout
  mutations.
- `canvas` may depend on `layout` and `render`, but it must not reproduce their
  routing or raster semantics.
- Frame IDs describe presentation roles only.
- Keep retained frame and encoder allocations reusable across renders.

## Verification

Run `go test ./internal/tui/canvas` and the root TUI move/duplicate benchmarks
after changing frame retention or rendering.

`BenchmarkModelRenderHighWater` separates active node count, active raster
bounds, and retained allocations after a large layout shrinks.
