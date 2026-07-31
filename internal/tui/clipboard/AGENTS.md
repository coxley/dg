# Clipboard package

## Responsibility

`clipboard` owns copy debouncing, native multi-format clipboard access,
terminal fallback probing, export formatting, and the declarative chrome
export form. `native` is an internalized MIT-licensed clipboard backend with
atomic multi-format writes.
`Model` implements `tea.Model`.

## Boundaries

- The parent renders the selected diagram and sends its text and optional
  portable fragment with `RequestCopy`.
- Native writes atomically replace the clipboard with plain text and the
  fragment MIME type. OSC52 fallback writes plain text only.
- `ReadPaste` returns structural data only when its CRC32 envelope matches the
  terminal's pasted text.
- `OpenExportMsg`, `CloseExportMsg`, `CopiedMsg`, and `ErrorMsg` report
  application-level outcomes to the parent.
- `UpdateMsg` routes timers, native results, and chrome form commands back to
  the clipboard model.
- The parent owns modal placement and success/error presentation.

## Verification

Use `testing/synctest` for debounce and probe timing. Run
`go test ./internal/tui/clipboard`.
