#!/usr/bin/env bash
set -euo pipefail
D="$(dirname "$0")/../.scion/templates/darwin"
for f in scion-agent.yaml agents.md system-prompt.md; do
  [[ -f "${D}/${f}" ]] || { echo "FAIL: ${D}/${f} missing"; exit 1; }
done
grep -q "default_harness_config: codex" "${D}/scion-agent.yaml" || { echo "FAIL: wrong harness_config"; exit 1; }
grep -q "gpt-5.5" "${D}/scion-agent.yaml" || { echo "FAIL: wrong model"; exit 1; }
grep -q "max_turns: 50" "${D}/scion-agent.yaml" || { echo "FAIL: wrong max_turns"; exit 1; }
grep -q 'max_duration: "4h"' "${D}/scion-agent.yaml" || { echo "FAIL: wrong max_duration"; exit 1; }
echo "PASS"
