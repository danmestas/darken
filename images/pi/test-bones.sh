#!/usr/bin/env bash
# Smoke-test that the universal baseline is present in darkish-pi.
#
# NOTE (A-03 mirrors A-01 / A-02): bones is currently 2 binaries
# (agent-init, agent-tasks) pre-built on the host. mgrep omitted
# (paid; context-mode replaces). Pi auth is OPENROUTER_API_KEY env var.
set -euo pipefail

IMG="${1:-local/darkish-pi:latest}"

REQUIRED_BIN=(
  agent-init agent-tasks
  jq rg fzf gh
)

for b in "${REQUIRED_BIN[@]}"; do
  if ! docker run --rm --entrypoint /bin/sh "${IMG}" -c "command -v ${b} >/dev/null"; then
    echo "FAIL: ${b} not on PATH in ${IMG}" >&2
    exit 1
  fi
done

echo "PASS: all baseline binaries present in ${IMG}"
