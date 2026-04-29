#!/usr/bin/env bash
# Verifies that every skill listed in internal/substrate/skills.manifest.txt
# has a corresponding directory in internal/substrate/data/skills/.
# Run as a CI check after any manifest or data/skills change.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

MANIFEST="${ROOT}/internal/substrate/skills.manifest.txt"
if [[ ! -f "${MANIFEST}" ]]; then
  echo "FAIL: internal/substrate/skills.manifest.txt not found" >&2; exit 1
fi

FAIL=0
while IFS= read -r skill || [[ -n "${skill}" ]]; do
  [[ -z "${skill}" || "${skill}" == \#* ]] && continue
  dir="${ROOT}/internal/substrate/data/skills/${skill}"
  if [[ ! -d "${dir}" ]]; then
    echo "FAIL: skill '${skill}' declared in manifest but missing from data/skills/" >&2
    FAIL=1
  fi
done < "${MANIFEST}"

[[ ${FAIL} -eq 0 ]] && echo "PASS" || exit 1
