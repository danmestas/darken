#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.scion/templates"

# Harnesses that should have skills + volumes
for h in orchestrator designer planner tdd-implementer verifier reviewer sme; do
  if ! grep -q "^skills:" "${h}/scion-agent.yaml"; then
    echo "FAIL: ${h} missing skills:" >&2; exit 1
  fi
  if ! grep -q "skills-staging/${h}" "${h}/scion-agent.yaml"; then
    echo "FAIL: ${h} missing skills-staging volume" >&2; exit 1
  fi
done

# Harnesses that should NOT have skills (researcher, admin per spec §5.3)
for h in researcher admin; do
  if grep -q "^skills:" "${h}/scion-agent.yaml"; then
    echo "FAIL: ${h} has skills: but should not (spec §5.3)" >&2; exit 1
  fi
done

echo "PASS: 7 harnesses have skills+volumes; 2 correctly have neither"
