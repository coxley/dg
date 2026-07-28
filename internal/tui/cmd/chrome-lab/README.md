# Chrome lab

Run the interactive lab from the repository root:

```sh
go run ./internal/tui/cmd/chrome-lab
```

Select scenarios with `1`–`6` or Tab, toggle density with `d`, scroll with the
arrow keys or mouse wheel, and drag with the left mouse button. Use
`--scenario layout|pane|viewport|menu|density|overflow` to choose the initial
scenario.

Run the real-PTY smoke:

```sh
./internal/tui/cmd/chrome-lab/smoke.sh
```

The smoke checks `100x30`, `80x16`, and `80x12`, drives keyboard and raw SGR
mouse input, and removes every named session. On failure it reports a temporary
directory containing JSON and PNG diagnostics.
