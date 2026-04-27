#!/usr/bin/env bash
# Smoke-test that the universal baseline is present in darkish-codex.
#
# NOTE (A-02 mirrors A-01): bones is currently 2 binaries (agent-init,
# agent-tasks) pre-built on the host. mgrep is intentionally omitted
# (paid product; operator uses context-mode for search). Update list
# when more agent-infra cmds land or paid tools are added.
set -euo pipefail

IMG="${1:-local/darkish-codex:latest}"

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
