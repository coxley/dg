# Command package guide

## Responsibility

`cmd/dg` wires persistence, history, layout defaults, and the TUI into the
executable. Keep engine and interaction logic in their owning packages.

## Behavior

```sh
go run ./cmd/dg
go run ./cmd/dg path/to/diagram.dg
```

No argument opens the most recently modified canvas, or creates a durable draft
from the example diagram when the catalog is empty. One argument imports a
compressed document into Drafts without changing the source. More arguments
return usage.

The TUI serializes document and history writes on its owner event loop. Normal
quit, SIGINT, and SIGTERM flush before exiting; a second signal exits
immediately. Panic cleanup gets two seconds, reports cleanup failures, and then
re-panics the original value. History cache failures never block document
editing or saving. New diagrams use persisted router preferences when the user
enabled them.

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
