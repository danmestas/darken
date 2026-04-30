#!/usr/bin/env bash
# Regression test for Bug 27: make sync-embed-data must not wipe skills
# that are listed in internal/substrate/skills.manifest.txt but are NOT
# present in .claude/skills/ (i.e., vendored skills like hipp, ousterhout).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="${ROOT}/internal/substrate/skills.manifest.txt"

TMPDIR_WORK="$(mktemp -d)"
trap 'rm -rf "${TMPDIR_WORK}"' EXIT

# Create a fake SUBSTRATE_DATA pre-seeded with vendored skills.
FAKE_DATA="${TMPDIR_WORK}/data"
mkdir -p "${FAKE_DATA}/skills/hipp" "${FAKE_DATA}/skills/ousterhout"
echo "# hipp stub" > "${FAKE_DATA}/skills/hipp/SKILL.md"
echo "# ousterhout stub" > "${FAKE_DATA}/skills/ousterhout/SKILL.md"

# Run sync-embed-data against the fake data dir.
make -C "${ROOT}" sync-embed-data SUBSTRATE_DATA="${FAKE_DATA}" -s 2>&1

# Assert vendored skills survived.
FAILED=0
while IFS= read -r skill || [[ -n "${skill}" ]]; do
  [[ -z "${skill}" ]] && continue
  case "${skill}" in \#*) continue ;; esac
  # Skip skills that come from .claude/skills/ (they are not vendored).
  if [[ -d "${ROOT}/.claude/skills/${skill}" ]]; then
    continue
  fi
  # This skill is vendored; it must still be present after sync-embed-data.
  if [[ ! -d "${FAKE_DATA}/skills/${skill}" ]]; then
    echo "FAIL: vendored skill ${skill} was wiped by sync-embed-data" >&2
    FAILED=1
  fi
done < "${MANIFEST}"

if [[ "${FAILED}" -ne 0 ]]; then
  exit 1
fi

echo "PASS: sync-embed-data preserved all vendored skills"
