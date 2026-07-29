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
  ht send --wait-text "scenario: focus" --timeout 5s "$session" "7" >/dev/null
  ht send --wait-text "profile: mac" --timeout 5s "$session" "p" >/dev/null
  printf -v paste '\e[200~pasted\e[201~'
  ht send --raw --wait-text "text: pasted" --timeout 5s "$session" "$paste" >/dev/null
  ht send --wait-text "scenario: surfaces" --timeout 5s "$session" "<Right>" >/dev/null
  ht send --wait-text "legacy modal adapter: open" --timeout 5s "$session" "m" >/dev/null
  printf -v modal_press '\e[<0;30;8M'
  printf -v modal_release '\e[<0;30;8m'
  ht send --raw --wait-text "pointer-capture: modal" --timeout 5s \
    "$session" "$modal_press" >/dev/null
  ht send --raw --wait-text "pointer-capture: none" --timeout 5s \
    "$session" "$modal_release" >/dev/null
  printf -v outside_press '\e[<0;2;2M'
  ht send --raw --wait-text "legacy modal adapter: closed" --timeout 5s \
    "$session" "$outside_press" >/dev/null
  ht send --wait-text "scenario: forms" --timeout 5s "$session" "9" >/dev/null
  ht send --wait-text "router-step: 11" --timeout 5s "$session" "<Right>" >/dev/null
  ht send --wait-idle 100ms --timeout 5s "$session" "<Down>" >/dev/null
  ht send --wait-text "key-profile: mac" --timeout 5s "$session" "<Right>" >/dev/null
  ht send --wait-idle 100ms --timeout 5s "$session" "<Down>" >/dev/null
  ht send --wait-text "NESTED DIRECTORY PICKER" --timeout 5s "$session" "<Enter>" >/dev/null
  ht send --wait-text "Save directory" --timeout 5s "$session" "q" >/dev/null
  ht send --wait-idle 100ms --timeout 5s "$session" "<Down>" >/dev/null
  ht send --wait-text "file-name: q" --timeout 5s "$session" "q" >/dev/null
  printf -v form_paste '\e[200~uick.json\n\e[201~'
  ht send --raw --wait-text "file-name: quick.json" --timeout 5s \
    "$session" "$form_paste" >/dev/null
  ht send --wait-text "scenario: dialogs" --timeout 5s "$session" "0" >/dev/null
  ht send --wait-text "dialog: dialog.save" --timeout 5s "$session" "s" >/dev/null
  ht send --wait-text "dialog: closed" --timeout 5s "$session" "<Esc>" >/dev/null
  ht send --wait-text "dialog: dialog.notice" --timeout 5s "$session" "n" >/dev/null
  ht send --raw --wait-text "dialog: dialog.notice" --timeout 5s \
    "$session" "$outside_press" >/dev/null
  ht send --wait-text "dialog: closed" --timeout 5s "$session" "<Esc>" >/dev/null
  ht send --wait-text "dialog: dialog.confirm" --timeout 5s "$session" "c" >/dev/null
  ht send --wait-text "scenario: sidebar" --timeout 5s "$session" "<Right>" >/dev/null
  ht send --wait-text "sidebar-position: 24" --timeout 5s "$session" "o" >/dev/null
  ht send --wait-text "sidebar-target: 18" --timeout 5s "$session" "t" >/dev/null
  ht wait --text "sidebar-position: 18" --timeout 5s "$session" >/dev/null
  ht send --wait-text "sidebar-placement: drawer" --timeout 5s "$session" "p" >/dev/null
  ht send --wait-text "motion-enabled: false" --timeout 5s "$session" "m" >/dev/null
  ht send --wait-text "sidebar-position: 0" --timeout 5s "$session" "r" >/dev/null
  ht send --wait-text "sidebar-position: 18" --timeout 5s "$session" "r" >/dev/null
  ht send --wait-text "sidebar-position: 0" --timeout 5s "$session" "<Esc>" >/dev/null

  snapshot="$(ht view --json "$session")"
  grep -q "\"cols\": ${size%x*}" <<<"$snapshot"
  grep -q "\"rows\": ${size#*x}" <<<"$snapshot"
  grep -q "scenario: sidebar" <<<"$snapshot"
  grep -q "density: compact" <<<"$snapshot"
  grep -q "sidebar-placement: drawer" <<<"$snapshot"
  grep -q "motion-enabled: false" <<<"$snapshot"

  ht stop "$session" >/dev/null
  ht remove "$session" >/dev/null
done

failed_session=""
rmdir "$failure_dir"
trap - ERR
echo "chrome-lab smoke passed"
