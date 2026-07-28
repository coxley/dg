# Preferences package

## Responsibility

`preferences` owns the editable preferences form, numeric inputs, form
navigation, and the current form value. `Model` implements `tea.Model`.

## Boundaries

- The parent supplies the initial value, dimensions, and styles.
- The parent applies router changes to the layout, owns edit transactions, and
  decides when to persist a completed value.
- Commands from Huh and numeric children return as `UpdateMsg`; route those
  messages back to `Model.Update`.
- `Completed` distinguishes Save from Cancel after the form finishes.
- The form uses its natural content height when space permits. `SetHeight`
  constrains only terminals or explicitly resized modal bodies.

## Verification

Run `go test ./internal/tui/preferences`.
