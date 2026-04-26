#!/usr/bin/env bash
# spawn.sh — stage credentials, then start a Scion harness.
#
# Wraps two operator-side actions into one command:
#   1. scripts/stage-creds.sh all   (refresh OAuth files from keychain)
#   2. scion start <args>            (spawn the harness)
#
# Use this instead of bare `scion start` so you never spawn an agent with
# stale credentials. Re-run any time host tokens roll.
#
# Usage:
#   scripts/spawn.sh <agent-name> [scion-start-args...]
#
# Examples:
#   scripts/spawn.sh smoke-test --type researcher --workspace /tmp/scion-smoke "task..."
#   scripts/spawn.sh worker-1 --type tdd-implementer --workspace /path/to/feature

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <agent-name> [scion-start-args...]" >&2
  exit 2
fi

# Refresh staged credentials. Soft-fail if either claude or codex creds
# are unavailable; the harness manifest decides which file is mounted.
"${ROOT}/scripts/stage-creds.sh" all || true

exec scion start "$@"
