#!/usr/bin/env bash
# Smoke-test that the universal baseline is present in darkish-claude.
#
# NOTE (A-01): The original spec listed 14 agent-infra binaries plus mgrep.
# The current agent-infra repo only ships agent-init and agent-tasks;
# the remaining 12 cmds (assert, autoclaim, chat, compactanthropic, dispatch,
# fossil, holds, jskv, presence, tasks, testutil, workspace) are not yet
# present upstream. mgrep-code-search is a private repo; both are omitted
# from REQUIRED_BIN until they land. Update this list when the cmds exist.
set -euo pipefail

IMG="${1:-local/darkish-claude:latest}"

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
