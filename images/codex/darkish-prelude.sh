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

# --- 5. Operator notification hooks ------------------------------------------
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

# --- 6. Hand off to scion ----------------------------------------------------

# Resolve sciontool from PATH; the scion-claude image installs it but the
# absolute path varies by release.
exec sciontool init -- "$@"
