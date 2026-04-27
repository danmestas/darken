#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="${REPO}/.scion/skills-staging"

rm -rf "${STAGE}/sme"

bash "${REPO}/scripts/stage-skills.sh" sme

for skill in ousterhout hipp; do
  if [[ ! -d "${STAGE}/sme/${skill}" ]]; then
    echo "FAIL: ${skill} not staged for sme" >&2; exit 1
  fi
  if [[ -L "${STAGE}/sme/${skill}" ]]; then
    echo "FAIL: ${skill} is a symlink (must be a copy)" >&2; exit 1
  fi
done
echo "PASS"
