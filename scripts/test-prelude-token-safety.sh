#!/usr/bin/env bash
# Asserts that prelude scripts use credential-header clone (not AUTH_URL) so
# GITHUB_TOKEN is not persisted in the workspace .git/config after cloning.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

FAIL=0

# Test 1: Static analysis -- no AUTH_URL pattern in any prelude.
for image in claude codex gemini pi; do
  prelude="${ROOT}/images/${image}/darkish-prelude.sh"
  if grep -q "AUTH_URL=" "${prelude}"; then
    echo "FAIL: ${image}/darkish-prelude.sh still uses AUTH_URL pattern" >&2
    FAIL=1
  else
    echo "ok: ${image}/darkish-prelude.sh does not use AUTH_URL"
  fi
done

# Test 2: Functional -- git clone with http.extraheader does not persist token in .git/config.
WORKDIR="$(mktemp -d)"
trap "rm -rf ${WORKDIR}" EXIT

ORIGIN="${WORKDIR}/origin"
CLONE_DIR="${WORKDIR}/clone"
FAKE_TOKEN="testtokenSECRETXYZ987"

git init --bare "${ORIGIN}" -q
# Clone using the credential-header approach (mirrors the new prelude logic).
B64_CREDS="$(printf 'x-access-token:%s' "${FAKE_TOKEN}" | base64 | tr -d '\n')"
git -c "http.extraheader=Authorization: Basic ${B64_CREDS}" \
    clone "${ORIGIN}" "${CLONE_DIR}" -q 2>/dev/null

if grep -rq "${FAKE_TOKEN}" "${CLONE_DIR}/.git/config" 2>/dev/null; then
  echo "FAIL: .git/config contains GITHUB_TOKEN material" >&2
  FAIL=1
else
  echo "ok: .git/config does not contain token material"
fi

[[ ${FAIL} -eq 0 ]] && echo "PASS" || exit 1
