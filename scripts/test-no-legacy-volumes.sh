#!/usr/bin/env bash
# Verify no manifest references the legacy ~/.scion-credentials path.
# After B-01's hub-secret push, claude_auth supersedes that mount.
set -euo pipefail
cd "$(dirname "$0")/../.scion/templates"
fail=0
for manifest in */scion-agent.yaml; do
  if grep -q "scion-credentials" "${manifest}"; then
    echo "FAIL: ${manifest} still references ~/.scion-credentials" >&2
    fail=1
  fi
done
if [[ ${fail} -eq 0 ]]; then echo "PASS: no legacy volume mounts"; fi
exit ${fail}
