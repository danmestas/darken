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

# --- 4. Pre-allow .claude/skills writes in Claude Code settings.json ----------
#
# claude --dangerously-skip-permissions does NOT suppress the interactive TUI
# dialog that fires when Claude Code writes to .claude/skills. Skill-editing
# agents block on that dialog until a human presses Y. Fix: merge explicit
# allow entries for .claude/skills into the existing settings.json using jq,
# preserving all other top-level keys, permissions.deny, existing allow rules,
# and hooks. Only missing Darken-owned rules are appended.

CLAUDE_SETTINGS="${HOME}/.claude/settings.json"
mkdir -p "${HOME}/.claude"

if command -v jq >/dev/null 2>&1; then
  if [[ -f "${CLAUDE_SETTINGS}" ]]; then
    _SKILLS_BASE="$(cat "${CLAUDE_SETTINGS}")"
  else
    _SKILLS_BASE='{}'
  fi
  echo "${_SKILLS_BASE}" | jq '
    .permissions       = (.permissions       // {}) |
    .permissions.allow = (.permissions.allow // []) |
    .permissions.deny  = (.permissions.deny  // []) |
    reduce (
      ["Write(**/.claude/skills/**)",
       "Edit(**/.claude/skills/**)",
       "Bash(mkdir -p **/.claude/skills/**)"][]
    ) as $r (
      .;
      if (.permissions.allow | map(select(. == $r)) | length) == 0
      then .permissions.allow += [$r]
      else .
      end
    )
  ' > "${CLAUDE_SETTINGS}.tmp" && mv "${CLAUDE_SETTINGS}.tmp" "${CLAUDE_SETTINGS}"
  echo "darkish-prelude: merged .claude/skills permissions into ${CLAUDE_SETTINGS}" >&2
else
  echo "darkish-prelude: WARNING — no jq; .claude/skills permissions not configured" >&2
fi

# --- 5. Heartbeat URL fix (work around scion broker SCION_HUB_URL = localhost) ---
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

# --- 6. Operator notification hooks ------------------------------------------
#
# Write a Claude Code Stop hook so SessionStop events route to the
# operator via scion message.  The recipient is read from
# DARKEN_HOOK_RECIPIENT (default: user:Development User).
#
# The hook also fires on PreToolUse:AskFollowupQuestion so question
# events are forwarded before the session fully stops.
#
# Implementation: write a self-contained hook script, then merge the
# hook declarations into ~/.claude/settings.json using jq.

DARKEN_HOOKS_DIR="${HOME}/.claude/hooks"
DARKEN_HOOK_SCRIPT="${DARKEN_HOOKS_DIR}/notify-operator.sh"
DARKEN_SETTINGS="${HOME}/.claude/settings.json"

mkdir -p "${DARKEN_HOOKS_DIR}"

cat > "${DARKEN_HOOK_SCRIPT}" << 'HOOKEOF'
#!/usr/bin/env bash
# Darkish Factory subharness: route Stop/AskFollowupQuestion events to operator.
set -uo pipefail
RECIPIENT="${DARKEN_HOOK_RECIPIENT:-user:Development User}"
AGENT="${SCION_AGENT_NAME:-unknown}"
EVENT="${DARKISH_HOOK_EVENT:-Stop}"
scion message --to "${RECIPIENT}" \
  "darkish hook ${EVENT}: agent ${AGENT}" 2>/dev/null || true
HOOKEOF
chmod +x "${DARKEN_HOOK_SCRIPT}"

if command -v jq >/dev/null 2>&1; then
  if [[ -f "${DARKEN_SETTINGS}" ]]; then
    EXISTING_SETTINGS="$(cat "${DARKEN_SETTINGS}")"
  else
    EXISTING_SETTINGS='{}'
  fi
  echo "${EXISTING_SETTINGS}" | jq --arg cmd "${DARKEN_HOOK_SCRIPT}" '
    .hooks = (.hooks // {}) |
    .hooks.Stop = (
      (.hooks.Stop // []) + [{
        "hooks": [{"type": "command", "command": $cmd,
                   "env": {"DARKISH_HOOK_EVENT": "Stop"}}]
      }]
    ) |
    .hooks.PreToolUse = (
      (.hooks.PreToolUse // []) + [{
        "matcher": "AskFollowupQuestion",
        "hooks": [{"type": "command", "command": $cmd,
                   "env": {"DARKISH_HOOK_EVENT": "AskFollowupQuestion"}}]
      }]
    )
  ' > "${DARKEN_SETTINGS}.tmp" && mv "${DARKEN_SETTINGS}.tmp" "${DARKEN_SETTINGS}"
  echo "darkish-prelude: operator notification hooks written to ${DARKEN_SETTINGS}" >&2
else
  echo "darkish-prelude: WARNING -- jq not found, operator notification hooks not configured" >&2
fi

# --- 7. Hand off to scion ----------------------------------------------------

# Resolve sciontool from PATH; the scion-claude image installs it but the
# absolute path varies by release.
exec sciontool init -- "$@"
