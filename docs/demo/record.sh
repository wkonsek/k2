#!/usr/bin/env bash
# Records the README GIFs: docs/demo/record.sh [name ...]
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
if ! command -v vhs >/dev/null; then
  echo "make demo needs vhs: https://github.com/charmbracelet/vhs" >&2
  exit 1
fi
bash docs/demo/setup.sh
names=("$@")
if (( ${#names[@]} == 0 )); then
  for t in docs/demo/*.tape; do
    [[ $t == */settings.tape ]] || names+=("$(basename "$t" .tape)")
  done
fi
for n in "${names[@]}"; do
  vhs "docs/demo/$n.tape"
  if command -v gifsicle >/dev/null; then
    gifsicle -O3 --lossy=40 -b "docs/demo/$n.gif"
  fi
done
