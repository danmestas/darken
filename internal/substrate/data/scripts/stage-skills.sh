#!/usr/bin/env bash
# stage-skills.sh — materialize a harness's role skills into the
# staging directory mounted by its scion-agent.yaml.
#
# Idempotent. Re-runs are safe; staging is rebuilt from the manifest.
#
# Modes:
#   stage-skills.sh <harness>                  # rebuild
#   stage-skills.sh <harness> --add <skill>    # mutate manifest + rebuild
#   stage-skills.sh <harness> --remove <skill> # mutate manifest + rebuild
#   stage-skills.sh <harness> --diff           # canonical-vs-staged diff
#
# Resolution rule (APM-style refs):
#   "danmestas/agent-skills/skills/<name>"  → ~/projects/agent-skills/skills/<name>
#   "<name>"                                 → ~/projects/agent-skills/skills/<name>
# External org refs ("<other-org>/<repo>/skills/<name>") not yet
# supported; fail loudly with a TODO message.

set -euo pipefail

# Resolve repo root: env var (set by darken when script is extracted from
# the embedded substrate to a tmp file) wins over BASH_SOURCE-relative.
# Direct invocation (`bash scripts/stage-skills.sh`) still works via the
# BASH_SOURCE fallback.
REPO="${DARKEN_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
# DARKEN_SKILLS_CANONICAL is injected by the darken scion-cmd helper.
# Direct invocations fall back to the standard agent-config layout.
CANONICAL="${DARKEN_SKILLS_CANONICAL:-${HOME}/projects/agent-config/skills}"

usage() {
  cat <<EOF >&2
Usage: $0 <harness> [--add <skill> | --remove <skill> | --diff]
EOF
  exit 2
}

if [[ $# -lt 1 ]]; then usage; fi
HARNESS="$1"; shift

MODE="rebuild"
TARGET_SKILL=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --add)    MODE="add";    TARGET_SKILL="$2"; shift 2 ;;
    --remove) MODE="remove"; TARGET_SKILL="$2"; shift 2 ;;
    --diff)   MODE="diff";   shift ;;
    *) usage ;;
  esac
done

# DARKEN_TEMPLATES_DIR is set by `darken bootstrap` to a tmpdir holding
# the embedded substrate templates when the operator's project doesn't
# carry its own .scion/templates/. Direct invocations (e.g. operator
# running `bash scripts/stage-skills.sh researcher` from the darken
# source repo) fall back to ${REPO}/.scion/templates which is the
# original behavior.
TEMPLATES_DIR="${DARKEN_TEMPLATES_DIR:-${REPO}/.scion/templates}"
MANIFEST_DIR="${TEMPLATES_DIR}/${HARNESS}"
if [[ ! -d "${MANIFEST_DIR}" ]]; then
  echo "stage-skills: harness '${HARNESS}' not found at ${MANIFEST_DIR}" >&2
  exit 1
fi

STAGE_DIR="${REPO}/.scion/skills-staging/${HARNESS}"

resolve_ref() {
  local ref="$1"
  case "${ref}" in
    danmestas/agent-skills/skills/*)
      echo "${CANONICAL}/${ref##*/}"
      ;;
    */*/skills/*)
      echo "stage-skills: external skill refs not yet supported: ${ref}" >&2
      return 1
      ;;
    *)
      echo "${CANONICAL}/${ref}"
      ;;
  esac
}

read_skills_from_manifest() {
  local manifest="${MANIFEST_DIR}/scion-agent.yaml"
  if [[ ! -f "${manifest}" ]]; then
    echo "stage-skills: manifest not found at ${manifest}" >&2
    return 1
  fi
  # Use python3 to parse YAML (available on macOS without extra deps).
  # Falls back gracefully if skills key is absent.
  python3 - "${manifest}" <<'PYEOF'
import sys, json
try:
    import yaml
    with open(sys.argv[1]) as f:
        data = yaml.safe_load(f)
    skills = data.get('skills') or []
    for s in skills:
        print(s)
except ImportError:
    # No PyYAML — fall back to grep-based extraction
    import re
    in_skills = False
    with open(sys.argv[1]) as f:
        for line in f:
            if re.match(r'^skills\s*:', line):
                in_skills = True
                continue
            if in_skills:
                m = re.match(r'^\s+-\s+(.+)', line)
                if m:
                    print(m.group(1).strip())
                elif re.match(r'^\S', line):
                    break
PYEOF
}

do_rebuild() {
  # Use a per-process temp dir so parallel invocations never share a
  # destination during cp.  Copy work happens outside the lock.
  # The publish pair (rm -rf + mv) is serialized with a lock directory;
  # mkdir is atomic on POSIX, so only one process enters the critical
  # section at a time.
  local stage_tmp="${STAGE_DIR}.tmp.$$"
  local lock_dir="${STAGE_DIR}.lock"
  rm -rf "${stage_tmp}"
  mkdir -p "${stage_tmp}"
  local refs
  refs="$(read_skills_from_manifest || true)"
  if [[ -z "${refs}" ]]; then
    echo "stage-skills: no role skills declared for ${HARNESS}" >&2
    rm -rf "${stage_tmp}"
    return 0
  fi
  while IFS= read -r ref; do
    [[ -z "${ref}" ]] && continue
    local src dest name
    src="$(resolve_ref "${ref}")"
    name="${ref##*/}"
    dest="${stage_tmp}/${name}"
    if [[ ! -d "${src}" ]]; then
      echo "stage-skills: source skill missing at ${src}" >&2
      rm -rf "${stage_tmp}"
      return 1
    fi
    cp -R "${src}" "${dest}"
    echo "stage-skills: copied ${name} → ${STAGE_DIR}/${name}"
  done <<< "${refs}"
  # Acquire publish lock (spin-wait up to ~10 s).
  local i=0
  while ! mkdir "${lock_dir}" 2>/dev/null; do
    if [[ $((i++)) -ge 200 ]]; then
      echo "stage-skills: timed out waiting for publish lock at ${lock_dir}" >&2
      rm -rf "${stage_tmp}"
      return 1
    fi
    sleep 0.05
  done
  # Critical section: remove old staging dir then atomically rename tmp into place.
  rm -rf "${STAGE_DIR}"
  mv "${stage_tmp}" "${STAGE_DIR}"
  # Release lock.
  rm -rf "${lock_dir}"
}

do_diff() {
  local refs
  refs="$(read_skills_from_manifest || true)"
  while IFS= read -r ref; do
    [[ -z "${ref}" ]] && continue
    local name src staged
    name="${ref##*/}"
    src="$(resolve_ref "${ref}")"
    staged="${STAGE_DIR}/${name}"
    if [[ ! -d "${staged}" ]]; then
      echo "drift: ${name} declared but not staged"
      continue
    fi
    if ! diff -qr "${src}" "${staged}" >/dev/null 2>&1; then
      echo "drift: ${name} differs between canonical and staged"
      diff -r "${src}" "${staged}" || true
    else
      echo "in-sync: ${name}"
    fi
  done <<< "${refs}"
}

do_mutate_manifest() {
  local op="$1" skill="$2"
  local f="${MANIFEST_DIR}/scion-agent.yaml"
  case "${op}" in
    add)
      if grep -q "  - ${skill}\$" "${f}"; then
        echo "stage-skills: ${skill} already declared"
        return 0
      fi
      if ! grep -q "^skills:" "${f}"; then
        printf '\nskills:\n  - %s\n' "${skill}" >> "${f}"
      else
        awk -v s="  - ${skill}" '
          /^skills:/ { print; print s; next }
          { print }
        ' "${f}" > "${f}.tmp" && mv "${f}.tmp" "${f}"
      fi
      ;;
    remove)
      grep -v "  - ${skill}\$" "${f}" > "${f}.tmp" && mv "${f}.tmp" "${f}"
      ;;
  esac
}

case "${MODE}" in
  rebuild) do_rebuild ;;
  diff)    do_diff ;;
  add)     do_mutate_manifest add "${TARGET_SKILL}"; do_rebuild ;;
  remove)  do_mutate_manifest remove "${TARGET_SKILL}"; do_rebuild ;;
esac
