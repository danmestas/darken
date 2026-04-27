#!/usr/bin/env bash
# Integration smoke: actually run stage-creds.sh and verify the hub has
# at least claude_auth + codex_auth (the two backends that always have
# operator credentials present in this dev environment).
set -euo pipefail

bash "$(dirname "$0")/stage-creds.sh" all

for name in claude_auth codex_auth; do
  if ! scion hub secret list 2>/dev/null | grep -q "${name}"; then
    echo "FAIL: ${name} not in hub secret list" >&2; exit 1
  fi
done
echo "PASS"
