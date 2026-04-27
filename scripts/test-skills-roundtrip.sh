#!/usr/bin/env bash
# Validates each manifest's skills field is well-formed and parses via
# the same YAML reader stage-skills.sh uses internally.
#
# Note: scion's `templates show --format json` does NOT expose the
# `skills` field (it's not part of scion's config schema). We parse
# the YAML directly instead.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"

read_skills() {
  local manifest="$1"
  python3 - <<PY
import re, sys
with open("${manifest}") as f:
    in_skills = False
    skills = []
    for line in f:
        line = line.rstrip("\n")
        if line.startswith("skills:"):
            in_skills = True
            continue
        if in_skills:
            m = re.match(r"^\s*-\s*(.+?)\s*$", line)
            if m:
                skills.append(m.group(1))
                continue
            if line and not line.startswith(" ") and not line.startswith("\t"):
                in_skills = False
print("\n".join(skills))
PY
}

for h in orchestrator designer planner tdd-implementer verifier reviewer sme; do
  manifest="${REPO}/.scion/templates/${h}/scion-agent.yaml"
  skills="$(read_skills "${manifest}")"
  if [[ -z "${skills}" ]]; then
    echo "FAIL: ${h} has no skills declared in YAML" >&2; exit 1
  fi
  while IFS= read -r ref; do
    if ! [[ "${ref}" =~ ^[a-z0-9-]+/[a-z0-9-]+/skills/[a-z0-9-]+$ ]] && \
       ! [[ "${ref}" =~ ^[a-z0-9-]+$ ]]; then
      echo "FAIL: ${h} skill ref '${ref}' not in expected APM-style format" >&2; exit 1
    fi
  done <<< "${skills}"
done

echo "PASS: 7 manifests with valid skill refs"
