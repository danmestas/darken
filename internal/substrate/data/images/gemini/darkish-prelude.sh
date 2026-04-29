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

# --- 2. Pre-clone workspace (work around sciontool go-git Mac FUSE bug) ------
#
# sciontool's built-in `git init` (via go-git) fails silently on Docker
# Desktop's fakeowner FUSE filesystem with empty stderr. The shell C git
# binary works fine. Pre-clone here so sciontool sees "Workspace already
# populated, skipping git clone" and bypasses its broken go-git path.
#
# Only runs in Hub mode (when SCION_GIT_CLONE_URL is set). Local-mode
# worktree spawns get a populated /workspace via host bind-mount.
if [[ -n "${SCION_GIT_CLONE_URL:-}" && -n "${GITHUB_TOKEN:-}" && ! -d /workspace/.git ]]; then
  echo "darkish-prelude: pre-cloning workspace via shell git (sciontool go-git workaround)" >&2
  # Build auth credentials without embedding the token in the clone URL.
  # Passing credentials via -c http.extraheader keeps them out of the
  # persisted /workspace/.git/config.
  _B64_CREDS="$(printf 'x-access-token:%s' "${GITHUB_TOKEN}" | base64 | tr -d '\n')"
  if git \
      -c "http.extraheader=Authorization: Basic ${_B64_CREDS}" \
      clone --depth "${SCION_GIT_DEPTH:-1}" -b "${SCION_GIT_BRANCH:-main}" \
      "${SCION_GIT_CLONE_URL}" /workspace >&2; then
    echo "darkish-prelude: pre-clone complete" >&2
  else
    echo "darkish-prelude: pre-clone FAILED -- sciontool will likely fail too" >&2
  fi
fi

# --- 3. Heartbeat URL fix (work around scion broker SCION_HUB_URL = localhost) ---
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

# --- 4. Hand off to scion ----------------------------------------------------

# Resolve sciontool from PATH; the scion-claude image installs it but the
# absolute path varies by release.
exec sciontool init -- "$@"
