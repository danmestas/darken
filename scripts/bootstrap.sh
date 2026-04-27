#!/usr/bin/env bash
# bootstrap.sh — thin wrapper around `darken bootstrap`.
# Operators with the bin/darken binary built can call either entry
# point; bash users without Go in their PATH use this.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -x "${ROOT}/bin/darken" ]]; then
  exec "${ROOT}/bin/darken" bootstrap "$@"
fi
if command -v darken >/dev/null 2>&1; then
  exec darken bootstrap "$@"
fi
echo "bootstrap: bin/darken not built; run 'make darken' first" >&2
exit 1
