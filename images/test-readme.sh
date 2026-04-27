#!/usr/bin/env bash
set -euo pipefail
README="$(dirname "$0")/README.md"
for tok in "Universal baseline" "bones" "caveman" "context-mode" "agent-init"; do
  if ! grep -q "${tok}" "${README}"; then
    echo "FAIL: ${tok} not documented in README" >&2; exit 1
  fi
done
echo "PASS"
