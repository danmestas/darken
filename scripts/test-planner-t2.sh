#!/usr/bin/env bash
set -euo pipefail
D="$(dirname "$0")/../.scion/templates/planner-t2"
for f in scion-agent.yaml agents.md system-prompt.md; do
  [[ -f "${D}/${f}" ]] || { echo "FAIL: ${D}/${f} missing"; exit 1; }
done
grep -q "claude-opus-4-7" "${D}/scion-agent.yaml" || { echo "FAIL: wrong model"; exit 1; }
grep -q "max_turns: 30" "${D}/scion-agent.yaml" || { echo "FAIL: wrong max_turns"; exit 1; }
grep -q 'max_duration: "1h"' "${D}/scion-agent.yaml" || { echo "FAIL: wrong max_duration"; exit 1; }
echo "PASS"
