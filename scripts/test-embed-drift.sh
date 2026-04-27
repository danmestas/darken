#!/usr/bin/env bash
# Verifies that internal/substrate/data/ is in sync with the canonical
# substrate sources. Run as a CI check + a pre-commit guard.
#
# How it works: snapshot the data/ tree's content hashes, run the sync
# target, snapshot again. Any diff means the canonical sources have
# been modified without re-running `make sync-embed-data` and
# committing the result.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

snapshot() {
  find internal/substrate/data -type f -exec shasum -a 256 {} \; | sort
}

BEFORE=$(snapshot)
make sync-embed-data >/dev/null 2>&1
AFTER=$(snapshot)

if [[ "${BEFORE}" != "${AFTER}" ]]; then
  echo "FAIL: internal/substrate/data/ is out of sync with canonical sources" >&2
  echo "" >&2
  echo "Run 'make sync-embed-data' and commit the result." >&2
  echo "" >&2
  echo "First few drifted files:" >&2
  diff <(echo "${BEFORE}") <(echo "${AFTER}") | head -20 >&2
  exit 1
fi
echo "PASS"
