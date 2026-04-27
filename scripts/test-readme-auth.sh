#!/usr/bin/env bash
# Verify images/README.md documents the hub-secret auth model and no
# longer references the legacy ~/.scion-credentials path.
set -euo pipefail
README="$(dirname "$0")/../images/README.md"

if grep -q "scion-credentials" "${README}"; then
  echo "FAIL: README references legacy ~/.scion-credentials path" >&2; exit 1
fi

for tok in "hub secret" "claude_auth" "codex_auth"; do
  if ! grep -q "${tok}" "${README}"; then
    echo "FAIL: README does not document '${tok}'" >&2; exit 1
  fi
done

echo "PASS"
