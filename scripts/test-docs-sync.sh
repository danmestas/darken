#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for f in "${ROOT}/images/README.md" "${ROOT}/.design/harness-roster.md" "${ROOT}/.design/pipeline-mechanics.md"; do
  for tok in darwin planner-t1 planner-t2 planner-t3 planner-t4; do
    grep -q "${tok}" "${f}" || { echo "FAIL: ${tok} missing from ${f}"; exit 1; }
  done
done
echo "PASS"
