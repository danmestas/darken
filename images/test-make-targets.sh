#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
for target in tools-only-claude tools-only-codex tools-only-pi tools-only-gemini \
              prelude-only-claude prelude-only-codex prelude-only-pi prelude-only-gemini \
              tools-only-all; do
  if ! make -n ${target} >/dev/null 2>&1; then
    echo "FAIL: make target ${target} missing" >&2; exit 1
  fi
done
echo "PASS: fast-rebuild targets present"
