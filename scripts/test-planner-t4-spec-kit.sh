#!/usr/bin/env bash
# Live smoke test for planner-t4 + spec-kit prelude.
#
# Prerequisites:
#   - bin/darkish built (`make darkish`)
#   - darkish-codex image built with F-01's prelude (`make -C images codex`)
#   - scion server running, hub secrets staged (codex_auth)
#
# Behavior: spawns a planner-t4 with a trivial spec task, polls until
# completion, then asserts that the agent invoked `specify`. Cleans up
# even on failure.
set -euo pipefail
cleanup() {
  scion stop pt4-smoke --yes 2>/dev/null || true
  scion delete pt4-smoke --yes 2>/dev/null || true
}
trap cleanup EXIT

bash scripts/spawn.sh pt4-smoke --type planner-t4 \
  "Create a one-line spec for a 'reverse a string' utility. Use spec-kit."

for _ in $(seq 1 120); do
  state="$(scion list | awk -v n=pt4-smoke '$1==n {print $2}')"
  [[ "${state}" == "completed" ]] && break
  sleep 5
done

out="$(scion look pt4-smoke 2>&1)"
if ! echo "${out}" | grep -qi "specify"; then
  echo "FAIL: planner-t4 did not invoke specify CLI" >&2
  exit 1
fi
echo "PASS"
