#!/usr/bin/env bash
# darkish-prelude.sh (codex variant) — runs before sciontool init in
# the darkish-codex container.
#
# Codex is simpler than Claude: scion natively auto-detects
# ~/.codex/auth.json on the host and mounts it into the container at
# the standard location, so there is no OAuth shim required here.
# This prelude only handles trust-state injection (and any future
# codex-specific setup).
#
# TODO: Codex CLI's first-encounter trust mechanism is not yet
# verified by us. The placeholder below pre-creates ~/.codex with
# a minimal config in case codex reads from there. Validate against
# real codex behavior on first use; update as needed.

set -euo pipefail

# --- 1. Trust the workspace --------------------------------------------------

WORKSPACE_PATH="/repo-root/.scion/agents/${SCION_AGENT_NAME:-unknown}/workspace"

mkdir -p "${HOME}/.codex"

# Placeholder trust mechanism. Codex CLI's actual config schema needs
# verification — this is a starting point modeled on Claude's pattern.
# When we first run a codex harness end-to-end, observe what dialog it
# shows and update this section.
if [[ ! -f "${HOME}/.codex/trusted.json" ]]; then
  cat > "${HOME}/.codex/trusted.json" <<JSON
{
  "trusted_directories": ["${WORKSPACE_PATH}"]
}
JSON
fi

# --- 2. Hand off to scion ----------------------------------------------------

exec sciontool init -- "$@"
