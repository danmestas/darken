#!/usr/bin/env bash
# bootstrap.sh — thin wrapper around `darkish bootstrap`.
# Operators with the bin/darkish binary built can call either entry
# point; bash users without Go in their PATH use this.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -x "${ROOT}/bin/darkish" ]]; then
  exec "${ROOT}/bin/darkish" bootstrap "$@"
fi
if command -v darkish >/dev/null 2>&1; then
  exec darkish bootstrap "$@"
fi
echo "bootstrap: bin/darkish not built; run 'make darkish' first" >&2
exit 1
