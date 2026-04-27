#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.scion/templates"
[[ -d planner-t3 ]] || { echo "FAIL: planner-t3/ not present"; exit 1; }
[[ ! -d planner ]] || { echo "FAIL: legacy planner/ still exists"; exit 1; }
grep -q "superpowers" planner-t3/system-prompt.md || { echo "FAIL: planner-t3 prompt does not name superpowers"; exit 1; }
grep -q "danmestas/agent-skills/skills/superpowers" planner-t3/scion-agent.yaml || { echo "FAIL: planner-t3 skills missing superpowers"; exit 1; }
echo "PASS"
