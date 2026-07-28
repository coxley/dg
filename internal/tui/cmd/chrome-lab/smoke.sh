#!/usr/bin/env bash
set -euo pipefail

repo="$(git rev-parse --show-toplevel)"
failure_dir="$(mktemp -d)"
sessions=()
failed_session=""

cleanup() {
  for session in "${sessions[@]}"; do
    ht stop "$session" >/dev/null 2>&1 || true
    ht remove "$session" >/dev/null 2>&1 || true
  done
}

capture_failure() {
  if [[ -n "$failed_session" ]]; then
    ht view --json "$failed_session" >"$failure_dir/$failed_session.json" || true
    ht view --format png --output "$failure_dir/$failed_session.png" "$failed_session" || true
  fi
  echo "chrome-lab smoke failed; artifacts: $failure_dir" >&2
}

trap cleanup EXIT
trap capture_failure ERR

for size in 100x30 80x16 80x12; do
  session="dg-chrome-lab-${size}-$$"
  failed_session="$session"
  sessions+=("$session")
  ht run --size "$size" --cwd "$repo" --name "$session" \
    env GOCACHE=/private/tmp/dg-codex-go-build \
    go run ./internal/tui/cmd/chrome-lab >/dev/null
  ht wait --text "CHROME LAB" --timeout 10s "$session" >/dev/null

  ht send --wait-text "scenario: pane" --timeout 5s "$session" "2" >/dev/null
  ht send --wait-text "density: compact" --timeout 5s "$session" "d" >/dev/null
  ht send --wait-idle 100ms --timeout 5s "$session" "<Down>" >/dev/null

  printf -v wheel '\e[<65;10;8M'
  printf -v press '\e[<0;10;8M'
  printf -v drag '\e[<32;14;9M'
  printf -v release '\e[<0;14;9m'
  ht send --raw --wait-idle 100ms --timeout 5s "$session" "$wheel" >/dev/null
  ht send --raw --wait-text "pointer-capture: scenario" --timeout 5s \
    "$session" "$press" >/dev/null
  ht send --raw --wait-idle 100ms --timeout 5s "$session" "$drag" >/dev/null
  ht send --raw --wait-text "pointer-capture: none" --timeout 5s \
    "$session" "$release" >/dev/null

  snapshot="$(ht view --json "$session")"
  grep -q "\"cols\": ${size%x*}" <<<"$snapshot"
  grep -q "\"rows\": ${size#*x}" <<<"$snapshot"
  grep -q "scenario: pane" <<<"$snapshot"
  grep -q "density: compact" <<<"$snapshot"
  grep -q "events: click=1" <<<"$snapshot"

  ht stop "$session" >/dev/null
  ht remove "$session" >/dev/null
done

failed_session=""
rmdir "$failure_dir"
trap - ERR
echo "chrome-lab smoke passed"
