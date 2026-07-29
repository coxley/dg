# Directory picker package

## Responsibility

`directorypicker` is the single bounded adapter around Huh filesystem
navigation. It owns picker initialization, dimensions, theme selection, and
child-command routing.

## Boundaries

- Callers own the selected path, open/close policy, and surrounding form.
- Do not add ordinary fields, actions, layout, or traversal to this package.
- Huh imports outside this package indicate a boundary regression.

## Verification

Run `go test ./internal/tui/directorypicker`.
