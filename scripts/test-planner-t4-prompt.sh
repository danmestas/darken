#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
P="${ROOT}/.scion/templates/planner-t4/system-prompt.md"
for tok in "specify constitution" "specify spec" "specify plan" "specify tasks"; do
  grep -q "${tok}" "${P}" || { echo "FAIL: ${tok} missing from prompt"; exit 1; }
done
echo "PASS"
