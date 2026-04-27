#!/usr/bin/env bash
set -euo pipefail
APM="$(dirname "$0")/../apm.yml"
for s in ousterhout hipp tigerstyle idiomatic-go norman dx-audit superpowers spec-kit; do
  if ! grep -q "${s}" "${APM}"; then
    echo "FAIL: ${s} missing from apm.yml" >&2; exit 1
  fi
done
echo "PASS"
