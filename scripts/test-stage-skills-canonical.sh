#!/usr/bin/env bash
# Verifies that stage-skills.sh resolves the canonical skills source
# from DARKEN_SKILLS_CANONICAL when set, not from the hardcoded path.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT

# Build a minimal fake canonical source with one skill.
FAKE_CANONICAL="${TMPDIR}/skills"
mkdir -p "${FAKE_CANONICAL}/test-skill-c3"
echo "# test-skill-c3" > "${FAKE_CANONICAL}/test-skill-c3/SKILL.md"

# Provide a minimal harness template dir that declares test-skill-c3.
FAKE_HARNESS="c3-harness"
FAKE_TEMPLATES="${TMPDIR}/templates/${FAKE_HARNESS}"
mkdir -p "${FAKE_TEMPLATES}"
cat > "${FAKE_TEMPLATES}/scion-agent.yaml" <<YAML
skills:
  - test-skill-c3
YAML

STAGE_DIR="${ROOT}/.scion/skills-staging/${FAKE_HARNESS}"
rm -rf "${STAGE_DIR}"

DARKEN_SKILLS_CANONICAL="${FAKE_CANONICAL}" \
DARKEN_TEMPLATES_DIR="${TMPDIR}/templates" \
  bash "${ROOT}/scripts/stage-skills.sh" "${FAKE_HARNESS}"

if [[ ! -f "${STAGE_DIR}/test-skill-c3/SKILL.md" ]]; then
  echo "FAIL: skill not staged from DARKEN_SKILLS_CANONICAL" >&2
  rm -rf "${STAGE_DIR}"
  exit 1
fi

rm -rf "${STAGE_DIR}"
echo "PASS: stage-skills.sh respected DARKEN_SKILLS_CANONICAL"
