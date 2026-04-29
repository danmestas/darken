#!/usr/bin/env bash
# darkish-prelude.sh — runs before sciontool init in the darkish-claude container.
#
# Two responsibilities:
#   1. Pre-accept Claude Code's folder-trust dialog for the workspace path
#      this agent will operate in. Without this, claude blocks on a TUI prompt
#      that --dangerously-skip-permissions does not cover.
#   2. If a Claude OAuth credentials file is mounted at ~/.claude/.credentials.json,
#      derive CLAUDE_CODE_OAUTH_TOKEN from it and unset ANTHROPIC_API_KEY so OAuth
#      is preferred. Scion's claude harness does not support auth-file mode
#      natively (pkg/harness/claude_code.go:57), so we handle it here.
#
# After both setups, exec the original scion-claude entrypoint.

set -euo pipefail

# --- 1. Trust the workspace ---------------------------------------------------

WORKSPACE_PATH="/repo-root/.scion/agents/${SCION_AGENT_NAME:-unknown}/workspace"

mkdir -p "${HOME}/.claude"

if [[ -f "${HOME}/.claude.json" ]]; then
  EXISTING="$(cat "${HOME}/.claude.json")"
else
  EXISTING='{}'
fi

# Merge: set projects.<path>.hasTrustDialogAccepted = true.
# Use jq if present (we install it in this image); fall back to a minimal
# python json shim if jq is somehow missing.
if command -v jq >/dev/null 2>&1; then
  echo "${EXISTING}" | jq --arg p "${WORKSPACE_PATH}" '
    .projects = (.projects // {}) |
    .projects[$p] = ((.projects[$p] // {}) + {"hasTrustDialogAccepted": true})
  ' > "${HOME}/.claude.json.tmp"
  mv "${HOME}/.claude.json.tmp" "${HOME}/.claude.json"
elif command -v python3 >/dev/null 2>&1; then
  python3 - <<PY
import json, os, sys
p = "${WORKSPACE_PATH}"
data = json.loads('''${EXISTING}''')
data.setdefault("projects", {}).setdefault(p, {})["hasTrustDialogAccepted"] = True
with open(os.path.expanduser("~/.claude.json"), "w") as f:
    json.dump(data, f)
PY
else
  echo "darkish-prelude: WARNING — no jq or python3 available; trust state not written" >&2
fi

# --- 2. Claude OAuth from mounted credentials file ----------------------------

CREDS_FILE="${HOME}/.claude/.credentials.json"

if [[ -f "${CREDS_FILE}" ]]; then
  # The keychain blob from Claude Code on macOS is JSON shaped like:
  #   { "claudeAiOauth": { "accessToken": "...", "refreshToken": "...", "expiresAt": ... } }
  # Some installs may store flatter shapes; try both.
  if command -v jq >/dev/null 2>&1; then
    TOKEN="$(jq -r '
      .claudeAiOauth.accessToken
      // .accessToken
      // .oauth_token
      // empty
    ' "${CREDS_FILE}")"

    if [[ -n "${TOKEN}" ]]; then
      export CLAUDE_CODE_OAUTH_TOKEN="${TOKEN}"
      # Prefer OAuth over API key when both are present.
      unset ANTHROPIC_API_KEY
      echo "darkish-prelude: OAuth token loaded from ${CREDS_FILE}" >&2
    else
      echo "darkish-prelude: WARNING — credentials file present but no token field found" >&2
    fi
  fi
fi

# --- 3. Pre-clone workspace (work around sciontool go-git Mac FUSE bug) ------
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
  AUTH_URL="https://x-access-token:${GITHUB_TOKEN}@${SCION_GIT_CLONE_URL#https://}"
  if git clone --depth "${SCION_GIT_DEPTH:-1}" -b "${SCION_GIT_BRANCH:-main}" "${AUTH_URL}" /workspace 2>&1 | sed 's/x-access-token:[^@]*@/x-access-token:***@/g' >&2; then
    echo "darkish-prelude: pre-clone complete" >&2
  else
    echo "darkish-prelude: pre-clone FAILED — sciontool will likely fail too" >&2
  fi
fi

# --- 4. Heartbeat URL fix (work around scion broker SCION_HUB_URL = localhost) ---
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

# --- 5. Hand off to scion ----------------------------------------------------

# Resolve sciontool from PATH; the scion-claude image installs it but the
# absolute path varies by release.
exec sciontool init -- "$@"
