#!/usr/bin/env bash
# Smoke-test that sme spawns and observes /home/scion/skills/role/ contents.
# Codex requires --no-hub for OAuth file auto-detection from broker host.
set -euo pipefail

cleanup() {
  scion --no-hub stop sme-skills-smoke --yes 2>/dev/null || true
  scion --no-hub delete sme-skills-smoke --yes 2>/dev/null || true
}
trap cleanup EXIT

REPO="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p /tmp/scion-smoke

bash "${REPO}/scripts/stage-skills.sh" sme

scion --no-hub create sme-skills-smoke --type sme --workspace /tmp/scion-smoke \
  "List file names directly under /home/scion/skills/ and tell me what role-specific skills you have. Answer in two short lines." >/dev/null

scion --no-hub start sme-skills-smoke 2>&1 | head -5

# Wait up to 90s for terminal state
for _ in $(seq 1 45); do
  state="$(scion --no-hub list 2>/dev/null | awk -v n=sme-skills-smoke '$1==n {print $7}')"
  if [[ "${state}" == "completed" || "${state}" == "stopped" ]]; then
    break
  fi
  sleep 2
done

out="$(scion --no-hub look sme-skills-smoke --plain 2>&1 | tail -50)"
echo "--- agent transcript tail ---"
echo "${out}"
echo "--- end ---"

# At minimum codex should mention skills/role/ contents OR caveman OR ousterhout/hipp
if ! echo "${out}" | grep -qiE "ousterhout|hipp|caveman|role/|skills/"; then
  echo "FAIL: sme transcript did not reference any skill or skills directory" >&2
  exit 1
fi

echo "PASS: sme observed staged skills"
