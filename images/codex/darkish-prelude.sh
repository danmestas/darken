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

# --- 2. spec-kit (planner-t4 only) ------------------------------------------
#
# planner-t4 is the only harness that needs the github/spec-kit CLI.
# Installing it per-harness keeps the codex image small for the other
# codex-backed roles (verifier, reviewer, sme, darwin).
#
# Install paths tried in order; first to succeed wins. Idempotent:
# skips entirely when `specify` is already on PATH.

if [[ "${SCION_TEMPLATE_NAME:-}" == "planner-t4" ]]; then
  if ! command -v specify >/dev/null 2>&1; then
    echo "darkish-prelude: installing spec-kit for planner-t4..." >&2
    if npm install -g @github/spec-kit 2>/dev/null; then
      echo "darkish-prelude: spec-kit via npm OK" >&2
    else
      TARBALL_URL="https://github.com/github/spec-kit/releases/latest/download/spec-kit-linux-x64.tar.gz"
      if curl -fsSL "${TARBALL_URL}" -o /tmp/spec-kit.tgz; then
        mkdir -p /opt/spec-kit
        tar -xzf /tmp/spec-kit.tgz -C /opt/spec-kit
        ln -sf /opt/spec-kit/specify /usr/local/bin/specify
        echo "darkish-prelude: spec-kit via tarball OK" >&2
      else
        echo "darkish-prelude: WARNING — spec-kit install failed; planner-t4 will exit early" >&2
      fi
    fi
  fi
fi

# --- 3. Hand off to scion ----------------------------------------------------

exec sciontool init -- "$@"
