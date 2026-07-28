# Numeric input package

## Responsibility

`numinput` owns a bounded integer input controlled by Left/Right or `h`/`l`.
Its `Model` implements `tea.Model`; `Field` adapts that model to `huh.Field`.

## Boundaries

- Callers own the typed integer value and provide its inclusive maximum.
- Inputs clamp initial values to the range from zero through that maximum.
- Callers provide all visual styles through `Styles`.
- Flash-expiration messages return through the parent update loop. Parents
  route them to `Field.HandleFlash` before updating a containing form.
- Navigation bindings belong to the `Field` adapter because they participate
  in Huh form traversal. Up/Down and `k`/`j` move between fields.

## Verification

Run `go test ./internal/tui/numinput`.
