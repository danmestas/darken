#!/usr/bin/env bash
# stage-creds.sh — pull OAuth credentials from the macOS Keychain into a
# file the Scion runtime can mount into containers.
#
# Why this exists:
#   Scion's claude harness does not support auth-file mode (it expects
#   ANTHROPIC_API_KEY or Vertex ADC). Claude Code on macOS stores OAuth
#   credentials in the Keychain under "Claude Code-credentials". To run
#   Claude Code inside a Linux container with the user's OAuth session,
#   we extract the keychain blob to a file and bind-mount it.
#
# Usage:
#   scripts/stage-creds.sh           # stages claude (and codex if available)
#   scripts/stage-creds.sh claude    # only claude
#   scripts/stage-creds.sh codex     # only codex
#
# Output:
#   ~/.scion-credentials/claude/.credentials.json   (chmod 600)
#   ~/.scion-credentials/codex/auth.json            (copied from ~/.codex/auth.json if present)
#
# Run this BEFORE `scion start <harness>`. Re-run when the host's keychain
# token is refreshed (Claude Code refreshes silently; the staged file does
# not auto-refresh).
#
# This script never prints credential values. If something goes wrong, it
# exits non-zero and reports the problem without echoing secrets.

set -euo pipefail

WHAT="${1:-all}"
DEST_DIR="${HOME}/.scion-credentials"

stage_claude() {
  local out="${DEST_DIR}/claude/.credentials.json"
  mkdir -p "$(dirname "${out}")"
  chmod 700 "${DEST_DIR}" "$(dirname "${out}")"

  if ! command -v security >/dev/null 2>&1; then
    echo "stage-creds: security CLI not available — only macOS is supported." >&2
    exit 2
  fi

  # -w prints just the password (the JSON blob); -s scopes by service name.
  if security find-generic-password -s "Claude Code-credentials" -w > "${out}" 2>/dev/null; then
    chmod 600 "${out}"
    echo "stage-creds: claude OAuth → ${out}"
  else
    echo "stage-creds: WARNING — Keychain entry 'Claude Code-credentials' not found. Skipping claude." >&2
    rm -f "${out}"
    return 1
  fi
}

stage_codex() {
  local src="${HOME}/.codex/auth.json"
  local out="${DEST_DIR}/codex/auth.json"

  if [[ ! -f "${src}" ]]; then
    echo "stage-creds: WARNING — ${src} not found. Skipping codex." >&2
    return 1
  fi

  mkdir -p "$(dirname "${out}")"
  chmod 700 "${DEST_DIR}" "$(dirname "${out}")"

  cp "${src}" "${out}"
  chmod 600 "${out}"
  echo "stage-creds: codex OAuth → ${out}"
}

case "${WHAT}" in
  claude)
    stage_claude
    ;;
  codex)
    stage_codex
    ;;
  all)
    stage_claude || true
    stage_codex || true
    ;;
  *)
    echo "Usage: $0 [claude|codex|all]" >&2
    exit 2
    ;;
esac

echo ""
echo "Staged credentials live under ${DEST_DIR}/. Mount them into harness containers"
echo "via the 'volumes:' field in each harness's scion-agent.yaml."
