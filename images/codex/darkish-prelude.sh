#!/usr/bin/env bash
# darkish-prelude.sh (codex variant) — runs before sciontool init in
# the darkish-codex container.
#
# Codex auth is handled by scion natively: pkg/harness/codex.go reads
# ~/.codex/auth.json from the host and mounts it into the container at
# /home/scion/.codex/auth.json. No OAuth shim is required.
#
# This prelude only handles trust-state injection. Codex CLI tracks
# per-project trust in ~/.codex/config.toml as:
#
#     [projects."<absolute-path>"]
#     trust_level = "trusted"
#
# If the workspace path lacks such a block, codex will prompt on first
# encounter — blocking the harness on a TUI dialog. We append the block
# at start-up so the prompt never fires.

set -euo pipefail

# --- 1. Trust the workspace --------------------------------------------------

WORKSPACE_PATH="/repo-root/.scion/agents/${SCION_AGENT_NAME:-unknown}/workspace"
CONFIG="${HOME}/.codex/config.toml"

mkdir -p "${HOME}/.codex"

if [[ -f "${CONFIG}" ]] && grep -qE "^\[projects\.\"${WORKSPACE_PATH//\//\\/}\"\]" "${CONFIG}"; then
  : # Trust block already present.
else
  {
    echo ""
    echo "[projects.\"${WORKSPACE_PATH}\"]"
    echo "trust_level = \"trusted\""
  } >> "${CONFIG}"
fi

# --- 2. Hand off to scion ----------------------------------------------------

exec sciontool init -- "$@"
