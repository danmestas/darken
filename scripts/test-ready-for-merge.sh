#!/usr/bin/env bash
# Ready-for-merge gate. Confirms:
#   - tree is clean (no uncommitted/untracked changes)
#   - HEAD is a real commit
#   - all earlier phase tests pass
#   - go test passes
#
# Operator runs this manually before tagging v0.2.0-rc1. The plan
# explicitly does NOT auto-tag in CI per constitution §V.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

if git status --porcelain | grep -q .; then
  echo "FAIL: dirty tree"
  git status --porcelain
  exit 1
fi

git rev-parse HEAD >/dev/null

for t in \
  scripts/test-stage-creds.sh \
  scripts/test-no-legacy-volumes.sh \
  scripts/test-stage-skills.sh \
  scripts/test-spec-kit-prelude.sh \
  scripts/test-planner-t4-prompt.sh \
  scripts/test-docs-sync.sh \
  scripts/test-bootstrap-wrapper.sh \
; do
  if [[ -x "${t}" ]]; then
    bash "${t}" >/dev/null
  fi
done

go test ./... >/dev/null

echo "PASS"
