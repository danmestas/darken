#!/usr/bin/env bash
# End-to-end smoke for the bootstrap → spawn flow.
#
# Prerequisites:
#   - Docker daemon running
#   - scion installed and on PATH
#   - hub secrets `claude_auth` and `codex_auth` already pushed
#     (or codex auth.json present at ~/.codex/auth.json so
#     stage-creds.sh can push it)
#
# Behavior: copies the repo into a tmp dir, runs `make darkish` to
# build the binary, then exercises bootstrap → doctor → two spawns.
# Cleans up the tmp dir on exit.
set -euo pipefail
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
cp -R . "${TMP}/factory"
cd "${TMP}/factory"
make darkish
bin/darkish bootstrap
bin/darkish doctor
bin/darkish spawn smoke-r --type researcher "echo hi"
bin/darkish spawn smoke-s --type sme --backend codex "what is 1+1?"
echo "PASS"
