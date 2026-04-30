#!/usr/bin/env bash
# Regression test for Bug 18: two parallel stage-skills.sh invocations
# must both succeed without cp errors (dest-exists race).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TMPDIR_WORK="$(mktemp -d)"
trap 'rm -rf "${TMPDIR_WORK}"' EXIT

# Build a fake canonical source with two skills.
FAKE_CANONICAL="${TMPDIR_WORK}/skills"
for skill in alpha-skill beta-skill; do
  mkdir -p "${FAKE_CANONICAL}/${skill}"
  echo "# ${skill}" > "${FAKE_CANONICAL}/${skill}/SKILL.md"
done

# Fake harness template that declares both skills.
FAKE_HARNESS="concurrency-test-harness"
FAKE_TEMPLATES="${TMPDIR_WORK}/templates/${FAKE_HARNESS}"
mkdir -p "${FAKE_TEMPLATES}"
cat > "${FAKE_TEMPLATES}/scion-agent.yaml" <<YAML
skills:
  - alpha-skill
  - beta-skill
YAML

STAGE_DIR="${ROOT}/.scion/skills-staging/${FAKE_HARNESS}"
rm -rf "${STAGE_DIR}"

# Launch two parallel invocations and collect exit codes.
DARKEN_SKILLS_CANONICAL="${FAKE_CANONICAL}" \
DARKEN_TEMPLATES_DIR="${TMPDIR_WORK}/templates" \
  bash "${ROOT}/scripts/stage-skills.sh" "${FAKE_HARNESS}" \
  > "${TMPDIR_WORK}/out1.txt" 2>&1 &
PID1=$!

DARKEN_SKILLS_CANONICAL="${FAKE_CANONICAL}" \
DARKEN_TEMPLATES_DIR="${TMPDIR_WORK}/templates" \
  bash "${ROOT}/scripts/stage-skills.sh" "${FAKE_HARNESS}" \
  > "${TMPDIR_WORK}/out2.txt" 2>&1 &
PID2=$!

RC1=0; RC2=0
wait "${PID1}" || RC1=$?
wait "${PID2}" || RC2=$?

if [[ "${RC1}" -ne 0 ]]; then
  echo "FAIL: first invocation exited ${RC1}" >&2
  cat "${TMPDIR_WORK}/out1.txt" >&2
  rm -rf "${STAGE_DIR}"
  exit 1
fi
if [[ "${RC2}" -ne 0 ]]; then
  echo "FAIL: second invocation exited ${RC2}" >&2
  cat "${TMPDIR_WORK}/out2.txt" >&2
  rm -rf "${STAGE_DIR}"
  exit 1
fi

# Verify the staging dir has the expected skills after parallel runs.
for skill in alpha-skill beta-skill; do
  if [[ ! -f "${STAGE_DIR}/${skill}/SKILL.md" ]]; then
    echo "FAIL: ${skill} missing from staging dir after parallel runs" >&2
    rm -rf "${STAGE_DIR}"
    exit 1
  fi
done

rm -rf "${STAGE_DIR}"
echo "PASS: parallel stage-skills.sh invocations both succeeded without cp errors"
