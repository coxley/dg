# Chrome package guide

## Responsibility

`chrome` owns reusable terminal UI mechanics: intrinsic measurement,
arrangement, retained layout plans, menu and form geometry, panes, finite
viewports, and diagnostics used by the chrome lab.

## Boundaries

- Do not import `internal/tui`, `canvas`, `layout`, `render`, `document`, or
  application command and scope types.
- Accept application-owned semantic IDs, content, styles, and policy as data.
- Use one arranged plan for rendering, hit testing, clipping, and diagnostics.
- Keep arranged IDs stable across resize and reflow; reject duplicates.
- Treat ANSI-rendered text only as leaf content, never as recovered component
  geometry.
- Forms expose semantic field, action, spacer, and action-bar IDs through one
  retained plan. Applications own declarations and value mapping.
- Keep keyboard traversal and accessible execution paths aligned with the same
  declared control order.
- Keep the canvas's unbounded document viewport outside this package.

## Performance

Retain plans and reusable render data. Invalidation must be explicit. Do not
allocate a new declarative tree for every unchanged frame.

## Verification

Use table and generated tests for geometry invariants, display-cell behavior,
overflow, and resize-before-view input. Use `testing/synctest` for future timed
behavior.
