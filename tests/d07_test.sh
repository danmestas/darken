#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
R="${ROOT}/.design/harness-roster.md"
for h in orchestrator researcher designer planner-t1 planner-t2 planner-t3 planner-t4 \
         tdd-implementer verifier reviewer sme admin darwin; do
  grep -q "\`${h}\`" "${R}" || { echo "FAIL: ${h} not in roster"; exit 1; }
done
echo "PASS"
