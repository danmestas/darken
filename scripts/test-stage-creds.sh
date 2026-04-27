#!/usr/bin/env bash
# Verifies stage-creds.sh has sections for all four backends and pushes
# each as a hub secret (not as a file under ~/.scion-credentials/).
set -euo pipefail

SCRIPT="$(dirname "$0")/stage-creds.sh"

for backend in claude codex pi gemini; do
  if ! grep -q "stage_${backend}" "${SCRIPT}"; then
    echo "FAIL: stage_${backend} function missing" >&2; exit 1
  fi
done

if ! grep -q "scion hub secret set" "${SCRIPT}"; then
  echo "FAIL: stage-creds.sh does not push to hub" >&2; exit 1
fi

if grep -q '\${HOME}/.scion-credentials' "${SCRIPT}"; then
  echo "FAIL: stage-creds.sh still writes to ~/.scion-credentials (legacy path)" >&2
  exit 1
fi

echo "PASS"
