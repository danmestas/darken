#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
P="${ROOT}/images/codex/darkish-prelude.sh"
grep -q "SCION_TEMPLATE_NAME" "${P}" || { echo "FAIL: prelude does not branch on SCION_TEMPLATE_NAME"; exit 1; }
grep -q "planner-t4" "${P}" || { echo "FAIL: prelude does not detect planner-t4"; exit 1; }
grep -q "spec-kit" "${P}" || { echo "FAIL: prelude does not install spec-kit"; exit 1; }
echo "PASS"
