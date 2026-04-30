#!/usr/bin/env bash
# Regression test for Bug 17: darkish-prelude.sh must write a Claude Code
# Stop hook so SessionStop events route to the operator via scion message.
#
# Tests:
#   1. Static: prelude references settings.json hook wiring.
#   2. Functional: running the prelude (with sciontool stubbed out) writes
#      ~/.claude/settings.json with a Stop hook whose command calls scion.
#   3. Functional: executing the hook script fires scion message --to.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PRELUDE="${ROOT}/images/claude/darkish-prelude.sh"

FAIL=0

# ---------------------------------------------------------------------------
# Test 1: static analysis
# ---------------------------------------------------------------------------
if ! grep -q "settings.json\|hooks" "${PRELUDE}"; then
  echo "FAIL T1: darkish-prelude.sh does not reference settings.json or hooks" >&2
  FAIL=1
else
  echo "ok T1: prelude references hook configuration"
fi

# ---------------------------------------------------------------------------
# Test 2: prelude writes settings.json with Stop hook when executed
# ---------------------------------------------------------------------------
TMPDIR_WORK="$(mktemp -d)"
trap 'rm -rf "${TMPDIR_WORK}"' EXIT

FAKE_HOME="${TMPDIR_WORK}/home"
FAKE_BIN="${TMPDIR_WORK}/bin"
SCION_LOG="${TMPDIR_WORK}/scion-calls.log"

mkdir -p "${FAKE_HOME}/.claude" "${FAKE_BIN}"

# sciontool stub: exit immediately instead of initialising a workspace.
cat > "${FAKE_BIN}/sciontool" << 'STUB'
#!/usr/bin/env bash
exit 0
STUB
chmod +x "${FAKE_BIN}/sciontool"

# scion stub: record call arguments.
cat > "${FAKE_BIN}/scion" << STUB
#!/usr/bin/env bash
echo "\$@" >> "${SCION_LOG}"
exit 0
STUB
chmod +x "${FAKE_BIN}/scion"

RC=0
HOME="${FAKE_HOME}" \
  PATH="${FAKE_BIN}:${PATH}" \
  SCION_AGENT_NAME="test-harness-17" \
  DARKEN_HOOK_RECIPIENT="user:test-operator" \
  SCION_HUB_URL="" \
  SCION_HUB_ENDPOINT="" \
  DARKEN_HUB_ENDPOINT="" \
  SCION_GIT_CLONE_URL="" \
  bash "${PRELUDE}" 2>/dev/null || RC=$?

SETTINGS="${FAKE_HOME}/.claude/settings.json"

if [[ ! -f "${SETTINGS}" ]]; then
  echo "FAIL T2a: settings.json not written by prelude (exit ${RC})" >&2
  FAIL=1
else
  echo "ok T2a: settings.json written"
  if ! jq -e '.hooks.Stop | length > 0' "${SETTINGS}" >/dev/null 2>&1; then
    echo "FAIL T2b: Stop hook not present in settings.json" >&2
    cat "${SETTINGS}" >&2
    FAIL=1
  else
    echo "ok T2b: Stop hook present in settings.json"
  fi
fi

# ---------------------------------------------------------------------------
# Test 3: hook script calls scion message --to when executed
# ---------------------------------------------------------------------------
if [[ -f "${SETTINGS}" ]]; then
  HOOK_CMD="$(jq -r '.hooks.Stop[0].hooks[0].command // empty' "${SETTINGS}" 2>/dev/null || true)"
  if [[ -z "${HOOK_CMD}" ]]; then
    echo "FAIL T3: cannot extract hook command from settings.json" >&2
    FAIL=1
  elif [[ ! -x "${HOOK_CMD}" ]]; then
    echo "FAIL T3: hook command ${HOOK_CMD} is not executable" >&2
    FAIL=1
  else
    HOME="${FAKE_HOME}" \
      PATH="${FAKE_BIN}:${PATH}" \
      SCION_AGENT_NAME="test-harness-17" \
      DARKEN_HOOK_RECIPIENT="user:test-operator" \
      bash "${HOOK_CMD}" < /dev/null 2>/dev/null || true

    if [[ ! -f "${SCION_LOG}" ]]; then
      echo "FAIL T3: scion was not called by the hook script" >&2
      FAIL=1
    elif ! grep -q "test-operator" "${SCION_LOG}"; then
      echo "FAIL T3: scion call did not reference the configured recipient" >&2
      cat "${SCION_LOG}" >&2
      FAIL=1
    else
      echo "ok T3: hook script called scion message with correct recipient"
    fi
  fi
fi

[[ "${FAIL}" -eq 0 ]] && echo "PASS" || exit 1
