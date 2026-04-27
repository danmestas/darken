#!/usr/bin/env bash
set -euo pipefail
S="$(dirname "$0")/spawn.sh"
grep -q "stage-creds.sh" "${S}" || { echo "FAIL: spawn.sh skips stage-creds"; exit 1; }
grep -q "stage-skills.sh" "${S}" || { echo "FAIL: spawn.sh skips stage-skills"; exit 1; }
grep -q -- "--no-stage" "${S}" || { echo "FAIL: spawn.sh missing --no-stage flag"; exit 1; }
echo "PASS"
