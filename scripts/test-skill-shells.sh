#!/usr/bin/env bash
# Verifies minimum-viable superpowers and spec-kit skill shells are
# present in internal/substrate/data/skills/ with substantive content.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

check_skill() {
  local name="$1" required_keyword="$2"
  local path="${ROOT}/internal/substrate/data/skills/${name}/SKILL.md"
  if [[ ! -f "${path}" ]]; then
    echo "FAIL: ${name}/SKILL.md missing" >&2; return 1
  fi
  if grep -qi "placeholder" "${path}"; then
    echo "FAIL: ${name}/SKILL.md is still a placeholder" >&2; return 1
  fi
  if ! grep -qi "${required_keyword}" "${path}"; then
    echo "FAIL: ${name}/SKILL.md missing required keyword '${required_keyword}'" >&2; return 1
  fi
  echo "ok: ${name}"
}

FAIL=0
check_skill superpowers "scaffold"  || FAIL=1
check_skill spec-kit    "spec"      || FAIL=1

[[ ${FAIL} -eq 0 ]] && echo "PASS" || exit 1
