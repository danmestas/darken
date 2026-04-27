#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for h in verifier reviewer; do
  f="${ROOT}/.scion/templates/${h}/scion-agent.yaml"
  grep -q "default_harness_config: codex" "${f}" \
    || { echo "FAIL: ${h} not on codex"; exit 1; }
  grep -q "image: local/darkish-codex:latest" "${f}" \
    || { echo "FAIL: ${h} image wrong"; exit 1; }
  grep -q "model: gpt-5.5" "${f}" \
    || { echo "FAIL: ${h} model wrong"; exit 1; }
done
echo "PASS"
