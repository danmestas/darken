#!/usr/bin/env bash
set -euo pipefail
GI="$(dirname "$0")/../.gitignore"
for path in ".scion/skills-staging" ".scion/darwin-recommendations"; do
  if ! grep -qE "^${path}/?$" "${GI}"; then
    echo "FAIL: ${path} not gitignored" >&2; exit 1
  fi
done
echo "PASS"
