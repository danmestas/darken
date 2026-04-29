#!/usr/bin/env bash
# darkish-prelude.sh (gemini variant) — runs before sciontool init in
# the darkish-gemini container.
#
# Gemini auth: scion auto-detects ~/.gemini/oauth_creds.json on the
# broker host (pkg/harness/auth.go) for OAuth, or accepts GEMINI_API_KEY
# env var for key-based auth. The user can fund either path:
#   - Free tier: GEMINI_API_KEY from https://aistudio.google.com/apikey
#   - Vertex AI: GOOGLE_APPLICATION_CREDENTIALS for service account
#   - OAuth (gemini-cli): ~/.gemini/oauth_creds.json
#
# TODO: Gemini CLI's first-encounter trust mechanism is not yet
# verified. Likely candidates:
#   - ~/.gemini/settings.json with a trust block
#   - A per-project file
# Validate against real gemini-cli behavior on first use.

set -euo pipefail

# --- 1. Trust state (placeholder) -------------------------------------------
# (Add Gemini-specific trust handling here when verified.)

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
