#!/usr/bin/env bash
set -euo pipefail
D="$(dirname "$0")/../.scion/templates/planner-t1"
for f in scion-agent.yaml agents.md system-prompt.md; do
  [[ -f "${D}/${f}" ]] || { echo "FAIL: ${D}/${f} missing"; exit 1; }
done
grep -q "claude-sonnet-4-6" "${D}/scion-agent.yaml" || { echo "FAIL: wrong model"; exit 1; }
grep -q "max_turns: 15" "${D}/scion-agent.yaml" || { echo "FAIL: wrong max_turns"; exit 1; }
echo "PASS"
