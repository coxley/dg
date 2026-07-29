# Modal package

## Responsibility

`modal` owns the movable and resizable dialog shell, container and tab styles,
cell geometry, positioning, pointer state, outside-click detection, optional
tabs, and the reusable declarative confirmation body.

## Boundaries

- The TUI dialog controller provides body content and preferred dimensions
  through `Configure`.
- The controller and workspace surfaces own application identity, scope,
  dismissal policy, and body routing.
- Tab IDs carry identity only; this package has no knowledge of help,
  preferences, save, or export semantics.
- State transitions cross component boundaries through `tea.Msg` and
  `tea.Cmd`. Use `SwitchTab` and `Close` rather than mutating the model from a
  parent.
- Modal placement reserves `avoidTop` rows for higher-priority floating UI.
- Left-drag moves from empty cells. Right-drag resizes from the nearest corner.
- Resize preserves the pointer-to-corner offset and clamps to the terminal.
- Resized content must fit the reported body dimensions so the outer border
  remains present in the composed terminal frame.
- Natural content floats whenever it fits below avoided rows; otherwise the
  shell fills the terminal.
- `Confirmation` emits semantic chrome action IDs and contains no close or
  unsaved-document policy.

## Verification

Run `go test ./internal/tui/modal` and root headless-terminal checks at
`100x30`, `80x16`, and `80x12`.
