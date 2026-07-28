# Flex package

## Responsibility

`flex` allocates terminal cells among single-line, ANSI-styled items and
renders them as horizontal rows.

## Constraints

- Measure display cells rather than bytes or runes.
- Distribute fractional cells deterministically from left to right.
- Keep layout independent of Bubble Tea models and interaction state.
- Keep overflow policy explicit through each item's shrink weight.

## Verification

Run `go test ./internal/tui/flex`.
