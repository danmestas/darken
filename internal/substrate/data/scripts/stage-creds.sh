#!/usr/bin/env bash
# stage-creds.sh — push every backend's auth into scion's hub secret store.
#
# Idempotent: re-running with the same source state is a no-op.
# Soft-fails per backend (a missing keychain entry skips that backend
# only; other backends still get staged).
#
# Hub secret targets (per spec §6.2):
#   claude  → /home/scion/.claude/.credentials.json   (file type)
#   codex   → /home/scion/.codex/auth.json            (file type)
#   pi      → OPENROUTER_API_KEY                       (env type)
#   gemini  → /home/scion/.gemini/oauth_creds.json     (file type, OAuth)
#             OR GEMINI_API_KEY (env type, API-key path)
#
# Usage:
#   scripts/stage-creds.sh              # all backends
#   scripts/stage-creds.sh claude       # one backend

set -euo pipefail

WHAT="${1:-all}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

scion_present() {
  command -v scion >/dev/null 2>&1
}

push_file_secret() {
  local name="$1" target="$2" src="$3"
  if ! scion_present; then
    echo "stage-creds: scion CLI not on PATH; cannot push ${name}" >&2
    return 1
  fi
  # Guard the success echo explicitly: set -e is suppressed inside functions
  # called from an "|| true" context (POSIX), so check the exit code directly.
  if scion hub secret set --type file --target "${target}" "${name}" "@${src}" >/dev/null; then
    echo "stage-creds: ${name} pushed (file -> ${target})"
  else
    echo "stage-creds: ${name} push FAILED" >&2
    return 1
  fi
}

push_env_secret() {
  local name="$1" value="$2"
  if ! scion_present; then
    echo "stage-creds: scion CLI not on PATH; cannot push ${name}" >&2
    return 1
  fi
  printf '%s' "${value}" > "${TMP_DIR}/${name}"
  # Guard the success echo explicitly: set -e is suppressed inside functions
  # called from an "|| true" context (POSIX), so check the exit code directly.
  if scion hub secret set --type env --target "${name}" "${name}" "@${TMP_DIR}/${name}" >/dev/null; then
    rm -f "${TMP_DIR}/${name}"
    echo "stage-creds: ${name} pushed (env)"
  else
    rm -f "${TMP_DIR}/${name}"
    echo "stage-creds: ${name} push FAILED" >&2
    return 1
  fi
}

stage_claude() {
  if ! command -v security >/dev/null 2>&1; then
    echo "stage-creds: WARNING — security CLI unavailable (non-macOS host); skipping claude." >&2
    return 1
  fi
  local blob="${TMP_DIR}/claude.json"
  if ! security find-generic-password -s "Claude Code-credentials" -w > "${blob}" 2>/dev/null; then
    echo "stage-creds: WARNING — Keychain entry 'Claude Code-credentials' not found; skipping claude." >&2
    return 1
  fi
  chmod 600 "${blob}"
  push_file_secret claude_auth "/home/scion/.claude/.credentials.json" "${blob}"
}

stage_codex() {
  local src="${HOME}/.codex/auth.json"
  if [[ ! -f "${src}" ]]; then
    echo "stage-creds: WARNING — ${src} not found; skipping codex." >&2
    return 1
  fi
  push_file_secret codex_auth "/home/scion/.codex/auth.json" "${src}"
}

stage_pi() {
  if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
    echo "stage-creds: WARNING — OPENROUTER_API_KEY not set; skipping pi." >&2
    return 1
  fi
  push_env_secret OPENROUTER_API_KEY "${OPENROUTER_API_KEY}"
}

stage_gemini() {
  if [[ -f "${HOME}/.gemini/oauth_creds.json" ]]; then
    push_file_secret gemini_auth "/home/scion/.gemini/oauth_creds.json" "${HOME}/.gemini/oauth_creds.json"
    return 0
  fi
  if [[ -n "${GEMINI_API_KEY:-}" ]]; then
    push_env_secret GEMINI_API_KEY "${GEMINI_API_KEY}"
    return 0
  fi
  echo "stage-creds: WARNING — neither ~/.gemini/oauth_creds.json nor GEMINI_API_KEY found; skipping gemini." >&2
  return 1
}

case "${WHAT}" in
  claude) stage_claude ;;
  codex)  stage_codex ;;
  pi)     stage_pi ;;
  gemini) stage_gemini ;;
  all)
    stage_claude || true
    stage_codex  || true
    stage_pi     || true
    stage_gemini || true
    ;;
  *)
    echo "Usage: $0 [claude|codex|pi|gemini|all]" >&2
    exit 2
    ;;
esac
