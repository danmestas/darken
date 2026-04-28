#!/usr/bin/env bash
# Smoke-test that the universal baseline is present in darkish-claude.
#
# bones is the unified CLI from agent-infra (replaces the prior
# agent-init + agent-tasks binaries with subcommands). The smoke test
# confirms `bones` is on PATH and `bones --help` runs cleanly inside
# the image.
set -euo pipefail

IMG="${1:-local/darkish-gemini:latest}"

REQUIRED_BIN=(
  bones
  jq rg fzf gh
)

for b in "${REQUIRED_BIN[@]}"; do
  if ! docker run --rm --entrypoint /bin/sh "${IMG}" -c "command -v ${b} >/dev/null"; then
    echo "FAIL: ${b} not on PATH in ${IMG}" >&2
    exit 1
  fi
done

# bones --help must run cleanly (catches truncated/corrupted binary).
if ! docker run --rm --entrypoint /bin/sh "${IMG}" -c "bones --help >/dev/null 2>&1"; then
  echo "FAIL: bones --help failed in ${IMG}" >&2
  exit 1
fi

echo "PASS: all baseline binaries present in ${IMG}"
