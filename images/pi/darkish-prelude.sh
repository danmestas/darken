#!/usr/bin/env bash
# darkish-prelude.sh (pi variant) — runs before sciontool init in
# the darkish-pi container.
#
# Pi uses @mariozechner/pi-coding-agent (separate CLI from Claude Code,
# despite scion-pi's image symlinking `claude` → pi-wrapper.sh). Pi
# auth is via the OPENROUTER_API_KEY env var, which scion injects at
# launch time. No OAuth file mounting required.
#
# TODO: Pi's first-encounter trust mechanism is not yet verified. The
# existing pi-templates in scion-orchestrator run with --non-interactive
# which probably skips any prompt, but if a trust dialog appears at
# startup, add the bypass here. Likely candidates:
#   - ~/.pi/config.json or .toml
#   - A flag in the wrapper script
# Validate against real pi-CLI behavior on first use.

set -euo pipefail

# --- 1. Trust state (placeholder) -------------------------------------------
# (Add Pi-specific trust handling here when verified.)

# --- 2. Heartbeat URL fix (work around scion broker SCION_HUB_URL = localhost) ---
#
# Scion's broker injects SCION_HUB_URL=http://localhost:8080 into the
# container based on its bind address, ignoring per-template hub.endpoint.
# Container localhost is NOT the host on Mac Docker Desktop — heartbeat
# POSTs fail with "connection refused", and Hub eventually reaps the
# agent for missed heartbeats. Override here using the per-template
# DARKEN_HUB_ENDPOINT (set by scion-cmd helper) or fall back to the
# documented default.
if [[ "${SCION_HUB_URL:-}" == http://localhost:8080 || "${SCION_HUB_ENDPOINT:-}" == http://localhost:8080 ]]; then
  HUB_OVERRIDE="${DARKEN_HUB_ENDPOINT:-http://host.docker.internal:8080}"
  export SCION_HUB_URL="${HUB_OVERRIDE}"
  export SCION_HUB_ENDPOINT="${HUB_OVERRIDE}"
  echo "darkish-prelude: SCION_HUB_URL and SCION_HUB_ENDPOINT overridden to ${HUB_OVERRIDE} (broker-localhost workaround)" >&2
fi

# --- 3. Hand off to scion ----------------------------------------------------

# Resolve sciontool from PATH; the scion-claude image installs it but the
# absolute path varies by release.
exec sciontool init -- "$@"
