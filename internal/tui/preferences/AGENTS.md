# Preferences package

## Responsibility

`preferences` owns application field/action declarations, editable preference
values, and the bounded directory-picker adapter. `Model` implements
`tea.Model`; `chrome.Form` owns generic form geometry and traversal.

## Boundaries

- The parent supplies the initial value, dimensions, and styles.
- The parent applies router changes to the layout, owns edit transactions, and
  decides when to persist a completed value.
- Commands from chrome and the picker return as `UpdateMsg`; route those
  messages back to `Model.Update`.
- `Completed` reports Save, Save as Defaults, or Cancel.
- A zero height hugs declared content. Positive heights constrain the form and
  let its retained plan reveal the focused control.
- Preference rows justify titles against the left edge and controls against
  the right edge of the current modal body width.
- A growing declarative spacer anchors actions to the body bottom-left.
- Wheel input moves form focus without activating fields. Arrow keys and
  `h`/`j`/`k`/`l` provide equivalent navigation.
- The directory picker stays collapsed until Right, `l`, or Enter opens its
  zoomed subview. Escape or `q` closes that subview before it closes the modal.
- Huh remains only inside the bounded directory-picker adapter. Do not add Huh
  field geometry or traversal back to the preference declaration.

## Verification

Run `go test ./internal/tui/preferences`.
