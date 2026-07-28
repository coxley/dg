# Command package guide

## Responsibility

`cmd/dg` wires persistence, history, layout defaults, and the TUI into the
executable. Keep engine and interaction logic in their owning packages.

## Behavior

```sh
go run ./cmd/dg
go run ./cmd/dg path/to/diagram.json
```

No argument opens the example diagram. One argument loads a document, restores
its cached history, and opens the editor. More arguments return usage.

The command flushes history on exit. History cache failures are logged and
must not block document editing or saving. New diagrams use persisted router
preferences when the user enabled them.

## Constraints

- keep startup and shutdown explicit
- wrap errors with the operation and path
- do not add frontend state to this package
- do not add document conversion or layout mutation logic here
- preserve the optional single-path command interface unless adding a real
  flag parser

## Verification

Test argument handling, file errors, history attachment, and saved document
loading. Use headless-terminal for executable smoke tests.
