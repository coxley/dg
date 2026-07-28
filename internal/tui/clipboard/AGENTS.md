# Clipboard package

## Responsibility

`clipboard` owns copy debouncing, terminal clipboard capability probing,
fallback writes, export formatting, and the export form. `Model` implements
`tea.Model`.

## Boundaries

- The parent renders the selected diagram and sends its text with
  `RequestCopy`.
- `OpenExportMsg`, `CloseExportMsg`, `CopiedMsg`, and `ErrorMsg` report
  application-level outcomes to the parent.
- `UpdateMsg` routes timers, fallback results, and Huh commands back to the
  clipboard model.
- The parent owns modal placement and success/error presentation.

## Verification

Use `testing/synctest` for debounce and probe timing. Run
`go test ./internal/tui/clipboard`.
