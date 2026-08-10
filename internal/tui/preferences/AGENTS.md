# Preferences package

## Responsibility

`preferences` owns application field/action declarations and editable
preference values. `Model` implements `tea.Model`; `chrome.Form` owns generic
form geometry and traversal, and `directorypicker` owns filesystem navigation.

## Boundaries

- The parent supplies the initial value, dimensions, and styles.
- The parent applies router changes to the layout, owns edit transactions, and
  decides when to persist a completed value.
- Commands from chrome and the picker return as `UpdateMsg`; route those
  messages back to `Model.Update`.
- `Completed` reports Save or Cancel.
- A zero height hugs declared content. Positive heights constrain the form and
  let its retained plan reveal the focused control.
- Preference rows align titles against the left edge and controls against one
  shared value-column edge.
- The application supplies independent dark and light semantic tint options;
  the form stores only their selected IDs.
- The background selector stores whether the parent should leave the terminal
  background visible or paint the viewport with the active tint.
- A growing declarative spacer anchors actions to the body bottom-left.
- Wheel input moves form focus without activating fields. Arrow keys and
  `h`/`j`/`k`/`l` provide equivalent navigation.
- The directory picker stays collapsed until Right, `l`, or Enter opens its
  zoomed subview. Escape or `q` closes that subview before it closes the modal.
- Enter on any other preference field submits Save. Focused actions still
  submit their own Save or Cancel behavior.
- Do not add filesystem-picker or generic form geometry back to the preference
  declaration.

## Verification

Run `go test ./internal/tui/preferences`.
