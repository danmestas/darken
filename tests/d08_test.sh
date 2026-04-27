#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
P="${ROOT}/.design/pipeline-mechanics.md"
O="${ROOT}/.scion/templates/orchestrator/agents.md"
for tok in "planner-t1" "planner-t2" "planner-t3" "planner-t4" "darwin"; do
  grep -q "${tok}" "${P}" || { echo "FAIL: ${tok} missing from pipeline-mechanics"; exit 1; }
  grep -q "${tok}" "${O}" || { echo "FAIL: ${tok} missing from orchestrator/agents.md"; exit 1; }
done
echo "PASS"
