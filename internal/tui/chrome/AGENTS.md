# Chrome package guide

## Responsibility

`chrome` owns reusable terminal UI mechanics: intrinsic measurement,
arrangement, retained layout plans, menu, form, and text-input geometry, panes,
finite viewports, deterministic cell transitions, workspace surfaces, and
diagnostics used by the chrome lab.

## Boundaries

- Do not import `internal/tui`, `canvas`, `layout`, `render`, `document`, or
  application command and scope types.
- Accept application-owned semantic IDs, content, styles, and policy as data.
- Use one arranged plan for rendering, hit testing, clipping, and diagnostics.
- Keep arranged IDs stable across resize and reflow; reject duplicates.
- Treat ANSI-rendered text only as leaf content, never as recovered component
  geometry.
- Forms expose semantic field, button, spacer, and button-list IDs through one
  retained plan. Applications own declarations and value mapping.
- Form controls share one left-aligned value column derived from the widest
  rendered field label.
- Number and select fields share directional bracket focus and activation
  feedback.
- A form may declare a default action. Enter on a field submits that action
  unless the field owns Enter activation; Enter on a button submits the
  focused button.
- Text inputs edit grapheme clusters, clip by display cell, and keep typing,
  paste, pointer, and accessible value paths within the form declaration.
- Keep keyboard traversal and accessible execution paths aligned with the same
  declared control order.
- Surface plans retain both full content placement and the terminal-clipped
  pointer rectangle. Drawers translate content while docks move the shared
  canvas boundary.
- Workspace transitions retarget and reverse from their current integer cell.
  Disabled motion snaps to the same final placement.
- Keep the canvas's unbounded document viewport outside this package.

## Performance

Retain plans and reusable render data. Invalidation must be explicit. Do not
allocate a new declarative tree for every unchanged frame.

## Verification

Use table and generated tests for geometry invariants, display-cell behavior,
overflow, and resize-before-view input. Use `testing/synctest` for future timed
behavior.
