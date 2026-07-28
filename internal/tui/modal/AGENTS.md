# Modal package

## Responsibility

`modal` owns the movable modal shell, container and tab styles, cell geometry,
positioning, drag state, outside-click detection, and optional tabs.

## Boundaries

- Parent models provide body content and preferred dimensions through
  `Configure`.
- Tab IDs carry identity only; this package has no knowledge of help,
  preferences, save, or export semantics.
- State transitions cross component boundaries through `tea.Msg` and
  `tea.Cmd`. Use `SwitchTab` and `Close` rather than mutating the model from a
  parent.
- Modal placement reserves `avoidTop` rows for higher-priority floating UI.

## Verification

Run `go test ./internal/tui/modal` and root headless-terminal checks at
`100x30`, `80x16`, and `80x12`.
