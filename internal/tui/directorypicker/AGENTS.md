# Directory picker package

## Responsibility

`directorypicker` owns bounded filesystem navigation, dimensions, selection,
and theme selection.

## Boundaries

- Callers own the selected path, open/close policy, and surrounding form.
- Do not add ordinary fields, actions, layout, or traversal to this package.
- Show only non-hidden directories. Files and hidden entries do not
  participate in rendering or navigation.

## Verification

Run `go test ./internal/tui/directorypicker`.
