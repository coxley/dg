# Floating navigation package

## Responsibility

`nav` owns the floating tool selector's styles, geometry, hover state, active
tool, render cache, and mouse hit testing.

## Boundaries

- Tool IDs are local to this package; root `tui` maps them to editor modes.
- State changes cross the root boundary through `tea.Msg` and `tea.Cmd`.
- Geometry derives from `Styles.Container`; do not duplicate border or padding
  constants in the root package.
- `Lines` supports the canvas's cell-aware overlay path without coupling nav to
  canvas internals.

## Verification

Run `go test ./internal/tui/nav` and the root TUI drag benchmark after changing
rendering or geometry.
