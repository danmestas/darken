#!/usr/bin/env bash
# Regression test for Bug 18 + F3: concurrent stage-skills.sh invocations
# must both succeed without cp errors (dest-exists race) AND must produce
# exactly the expected top-level staging contents with no leftover temp or
# lock dirs.
#
# Usage: test-stage-skills-concurrency.sh [N]
#   N defaults to 10.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

N="${1:-10}"

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

STAGING_BASE="${ROOT}/.scion/skills-staging"
STAGE_DIR="${STAGING_BASE}/${FAKE_HARNESS}"
rm -rf "${STAGE_DIR}" "${STAGE_DIR}.lock" "${STAGE_DIR}".tmp.*

# Launch N parallel invocations.
PIDS=()
for i in $(seq 1 "${N}"); do
  DARKEN_SKILLS_CANONICAL="${FAKE_CANONICAL}" \
  DARKEN_TEMPLATES_DIR="${TMPDIR_WORK}/templates" \
    bash "${ROOT}/scripts/stage-skills.sh" "${FAKE_HARNESS}" \
    > "${TMPDIR_WORK}/out${i}.txt" 2>&1 &
  PIDS+=($!)
done

FAILED=0
for i in $(seq 1 "${N}"); do
  RC=0
  wait "${PIDS[$((i-1))]}" || RC=$?
  if [[ "${RC}" -ne 0 ]]; then
    echo "FAIL: invocation ${i} exited ${RC}" >&2
    cat "${TMPDIR_WORK}/out${i}.txt" >&2
    FAILED=1
  fi
done
if [[ "${FAILED}" -ne 0 ]]; then
  rm -rf "${STAGE_DIR}" "${STAGE_DIR}.lock" "${STAGE_DIR}".tmp.*
  exit 1
fi

# --- Assertion 1: expected skills are present ---
for skill in alpha-skill beta-skill; do
  if [[ ! -f "${STAGE_DIR}/${skill}/SKILL.md" ]]; then
    echo "FAIL: ${skill}/SKILL.md missing from staging dir after ${N} parallel runs" >&2
    rm -rf "${STAGE_DIR}" "${STAGE_DIR}.lock" "${STAGE_DIR}".tmp.*
    exit 1
  fi
done

# --- Assertion 2: exact top-level entries (no nested tmp dirs, no extra dirs) ---
EXTRA=()
while IFS= read -r -d '' entry; do
  name="$(basename "${entry}")"
  case "${name}" in
    alpha-skill|beta-skill) ;;   # expected
    *) EXTRA+=("${name}") ;;     # unexpected — catches nested .tmp.<pid> dirs
  esac
done < <(find "${STAGE_DIR}" -mindepth 1 -maxdepth 1 -print0 2>/dev/null | sort -z)

if [[ "${#EXTRA[@]}" -gt 0 ]]; then
  echo "FAIL: unexpected top-level entries in staging dir after ${N} parallel runs:" >&2
  printf '  %s\n' "${EXTRA[@]}" >&2
  ls -la "${STAGE_DIR}" >&2
  rm -rf "${STAGE_DIR}" "${STAGE_DIR}.lock" "${STAGE_DIR}".tmp.*
  exit 1
fi

# --- Assertion 3: no leftover .tmp.* or .lock dirs beside the staging dir ---
LEFTOVER=()
while IFS= read -r -d '' entry; do
  LEFTOVER+=("${entry}")
done < <(find "${STAGING_BASE}" -maxdepth 1 \
           \( -name "${FAKE_HARNESS}.tmp.*" -o -name "${FAKE_HARNESS}.lock" \) \
           -print0 2>/dev/null || true)

if [[ "${#LEFTOVER[@]}" -gt 0 ]]; then
  echo "FAIL: leftover tmp/lock dirs after ${N} parallel runs:" >&2
  printf '  %s\n' "${LEFTOVER[@]}" >&2
  rm -rf "${STAGE_DIR}" "${STAGE_DIR}.lock" "${STAGE_DIR}".tmp.*
  exit 1
fi

rm -rf "${STAGE_DIR}"
echo "PASS: ${N} parallel stage-skills.sh invocations produced exact staging contents with no temp/lock dirs"
