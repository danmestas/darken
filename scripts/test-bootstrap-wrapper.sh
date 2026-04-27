#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
S="${ROOT}/scripts/bootstrap.sh"
[[ -x "${S}" ]] || { echo "FAIL: bootstrap.sh missing/non-exec"; exit 1; }
grep -q "darkish bootstrap" "${S}" || { echo "FAIL: bootstrap.sh does not call darkish"; exit 1; }
echo "PASS"
