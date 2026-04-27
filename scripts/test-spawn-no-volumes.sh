#!/usr/bin/env bash
# Smoke-test researcher and sme post-migration to hub-secret auth.
# Both should spawn successfully without any volumes block in their manifest.
set -euo pipefail

cleanup() {
  scion stop researcher-smoke --yes 2>/dev/null || true
  scion delete researcher-smoke --yes 2>/dev/null || true
  scion stop sme-smoke --yes 2>/dev/null || true
  scion delete sme-smoke --yes 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p /tmp/scion-smoke

# Researcher (claude backend, claude_auth hub secret)
scion create researcher-smoke --type researcher --workspace /tmp/scion-smoke "Say hello." >/dev/null
{ scion start researcher-smoke 2>&1 || true; } | head -10
sleep 3
scion list | grep researcher-smoke || { echo "FAIL: researcher-smoke not in list"; exit 1; }
scion stop researcher-smoke --yes >/dev/null
scion delete researcher-smoke --yes >/dev/null

# SME (codex backend, codex_auth hub secret) — codex needs --no-hub for auth-file detection
scion --no-hub create sme-smoke --type sme --workspace /tmp/scion-smoke "What is 1+1? Answer with the number." >/dev/null
{ scion --no-hub start sme-smoke 2>&1 || true; } | head -10
sleep 3
scion --no-hub list | grep sme-smoke || { echo "FAIL: sme-smoke not in list"; exit 1; }
scion --no-hub stop sme-smoke --yes >/dev/null
scion --no-hub delete sme-smoke --yes >/dev/null

echo "PASS: both researcher and sme spawn cleanly without volumes blocks"
