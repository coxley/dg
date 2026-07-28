# Numeric input package

## Responsibility

`numinput` owns a bounded unsigned numeric input controlled by Left and Right.
Its `Model` implements `tea.Model`; `Field` adapts that model to `huh.Field`.

## Boundaries

- Callers own the bound string value.
- Callers provide all visual styles through `Styles`.
- Flash-expiration messages return through the parent update loop. Parents
  route them to `Field.HandleFlash` before updating a containing form.
- Navigation bindings belong to the `Field` adapter because they participate
  in Huh form traversal.

## Verification

Run `go test ./internal/tui/numinput`.
