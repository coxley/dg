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
- `Completed` reports Save, Save as Defaults, or Cancel.
- The form uses its natural content height when space permits. `SetHeight`
  constrains only terminals or explicitly resized modal bodies.
- Preference rows justify titles against the left edge and controls against
  the right edge of the current modal body width.
- Wheel input moves form focus without activating fields. Arrow keys and
  `h`/`j`/`k`/`l` provide equivalent navigation.
- The directory picker stays collapsed until Right, `l`, or Enter opens its
  zoomed subview. Escape closes that subview before it closes the modal.

## Verification

Run `go test ./internal/tui/preferences`.
