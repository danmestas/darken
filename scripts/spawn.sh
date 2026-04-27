#!/usr/bin/env bash
# spawn.sh — stage credentials + skills, then start a Scion harness.
#
# Usage:
#   scripts/spawn.sh <agent-name> --type <harness> [--no-stage] [scion-start-args...]
#
# --no-stage skips both stage-creds.sh and stage-skills.sh (faster spawn
# when nothing has changed since the last run).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <agent-name> --type <harness> [--no-stage] [scion-start-args...]" >&2
  exit 2
fi

AGENT_NAME="$1"; shift

STAGE=true
HARNESS=""
ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-stage) STAGE=false; shift ;;
    --type)     HARNESS="$2"; ARGS+=("$1" "$2"); shift 2 ;;
    *)          ARGS+=("$1"); shift ;;
  esac
done

if [[ -z "${HARNESS}" ]]; then
  echo "spawn: --type <harness> is required" >&2; exit 2
fi

if ${STAGE}; then
  "${ROOT}/scripts/stage-creds.sh" all || true
  "${ROOT}/scripts/stage-skills.sh" "${HARNESS}" || true
fi

exec scion start "${AGENT_NAME}" "${ARGS[@]}"
