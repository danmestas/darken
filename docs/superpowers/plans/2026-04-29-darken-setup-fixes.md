# Darken setup fixes — TDD plan (v0.1.16)

Status: draft (planner-t2)
Date: 2026-04-29
Companion spec: [`2026-04-29-darken-setup-fixes-design.md`](../specs/2026-04-29-darken-setup-fixes-design.md)

## Goal

Make `darken setup` succeed end-to-end on a clean machine — no manual
`/etc/hosts` edits, no cross-repo clone of `agent-config`, no
cross-grove template gaps, no print-before-success false positives,
no daemon-down-but-doctor-OK lies, and orchestrator skills that bias
toward subharness dispatch.

## Architecture (per spec §3, condensed)

1. Bundle canonical skills via `go:embed`, fed by `make vendor-skills`
   from a build-time canonical source.
2. Replace per-template hardcoded Hub URL with a substrate knob
   (`${DARKEN_HUB_ENDPOINT}`) applied by a centralized `scion-cmd`
   helper at spawn time. Pair with a doctor check (no auto-sudo).
3. Rewrite `orchestrator-mode` and `subagent-to-subharness` skills
   so subharness dispatch is the default when any of the 14 roles
   fits the task. `Agent` is a documented fallback.

## Task ordering

Tasks are grouped into three waves. Within a wave, tasks are
independent; between waves there are real dependencies (later waves
depend on earlier waves' helpers).

- **Wave A (unblock the rest):** centralized `scion-cmd` helper,
  canonical roles constant, manifest substitution. (Bugs #5, #6
  mechanism.)
- **Wave B (the actual bugfixes):** #1 bones rerun, #3 stage-creds
  ordering, #4 daemon liveness, #7 prelude propagation + sniff-test,
  #8 broker provide, #9 doctor /etc/hosts, #10 templates upload.
- **Wave C (substrate + skills):** #2 bundling pipeline, #11 skill
  rewrite. These touch the most files; saved for last.

Each task: failing test first, implementation, commit. No code in
this plan — only file paths, test names, and assertions.

---

## Wave A — Centralized helpers (bugs #5, #6 mechanism)

### Task A1 — `canonicalRoles` constant

**Files touched:**
- new: `cmd/darken/roles.go`
- new: `cmd/darken/roles_test.go`

**Failing test (`roles_test.go`):**
- `TestCanonicalRoles_Has14_AndMatchesSpec` — asserts the slice
  contains exactly these 14 names in alpha order:
  `admin, base, darwin, designer, orchestrator, planner-t1,
  planner-t2, planner-t3, planner-t4, researcher, reviewer, sme,
  tdd-implementer, verifier`.
- `TestCanonicalRoles_TemplatesExistOnDisk` — for each role, asserts
  `.scion/templates/<role>/scion-agent.yaml` exists. Catches
  manifest deletions / typos.

**Implementation:**
- Declare `var canonicalRoles = []string{…}` in `cmd/darken/roles.go`.
- No behavior wired yet; later tasks consume it.

**Commit:** `cmd/darken: introduce canonicalRoles constant (14 roles)`

---

### Task A2 — Spawn posArgs regression test

**Files touched:**
- existing: `cmd/darken/spawn_test.go`
- (no production change — pinning the v0.1.15 inline fix)

**Failing test (write first; should pass against v0.1.15 head, but
becomes the regression guard for the upcoming refactor):**
- `TestRunSpawn_PosArgsForwardedAsTaskString` — invokes
  `runSpawn([]string{"agentname", "--type", "researcher", "do the
  thing", "with multiple words"})` against a stubbed `scion`
  binary on PATH. Asserts the stubbed scion received the trailing
  positional words as a single task argument, not split by flag
  parsing or `--`.
- `TestRunSpawn_FailsWithoutType` — pre-existing safety net
  (verify it still passes after the refactor).

**Implementation:** none yet. Test pins existing behavior. The task
that introduces `scion-cmd` (A3) must keep this green.

**Commit:** `cmd/darken: regression test for spawn posArgs forwarding`

---

### Task A3 — `scion-cmd` helper

**Files touched:**
- new: `cmd/darken/scion_cmd.go`
- new: `cmd/darken/scion_cmd_test.go`

**Failing tests (`scion_cmd_test.go`):**
- `TestScionCmd_SetsHubEndpointFromEnv` — when
  `DARKEN_HUB_ENDPOINT=http://example:9090` is set, the resulting
  `*exec.Cmd`'s env contains `SCION_HUB_ENDPOINT=http://example:9090`.
- `TestScionCmd_DefaultsHubEndpoint` — with
  `DARKEN_HUB_ENDPOINT` unset, `SCION_HUB_ENDPOINT` defaults to
  `http://host.docker.internal:8080` (preserves v0.1.15 behavior).
- `TestScionCmd_PropagatesRepoRoot` — `DARKEN_REPO_ROOT` is set
  per `scriptEnv()`'s existing pattern.
- `TestScionCmd_ArgsForwardedVerbatim` — `scionCmd(["spawn",
  "name", "--type", "researcher", "do thing"])`'s `Args` matches
  exactly the prefix `[scion, spawn, name, --type, researcher,
  do thing]`.

**Implementation:**
- Add `scionCmd(args []string) *exec.Cmd` that wraps
  `exec.Command("scion", args...)` with env set per the tests above.
- Re-export `scriptEnv()` if needed (or inline the env logic and
  unify in a follow-up).

**Commit:** `cmd/darken: add scionCmd helper centralizing scion invocations`

---

### Task A4 — Migrate doctor + bootstrap call sites to `scionCmd`

**Files touched:**
- existing: `cmd/darken/doctor.go` (4 call sites: `checkScion`,
  `checkScionServer`, `checkHubSecrets`, harness-secret list)
- existing: `cmd/darken/bootstrap.go` (`ensureScionServer`)
- existing: `cmd/darken/spawn.go` (the spawn invocation)
- existing: tests for each (golden behavior pinned)

**Failing test (per migration):** the existing tests for each
function should keep passing. Add one assertion per migration that
`scionCmd` is the construction path (e.g. via a small interface
seam: a package-level `scionCmdFn` var defaulting to `scionCmd`,
overridden in tests to record args).

**Implementation:**
- Replace `exec.Command("scion", …)` with `scionCmd([…])` in each
  call site.
- Run all existing tests; nothing else should change semantically.

**Commit (one per file or one combined, implementer's call):**
`cmd/darken: route scion invocations through scionCmd helper`

---

### Task A5 — Manifest `${DARKEN_HUB_ENDPOINT}` substitution

**Files touched:**
- existing: `.scion/templates/*/scion-agent.yaml` (all 14 — replace
  hardcoded URL with literal `${DARKEN_HUB_ENDPOINT}`)
- existing: `internal/substrate/data/.scion/templates/*/scion-agent.yaml`
  (mirror)
- new or extend: `cmd/darken/manifest.go` (substitution helper)
- existing: `cmd/darken/manifest_test.go`

**Failing tests:**
- `TestManifest_SubstitutesHubEndpoint` — given a manifest body
  containing `endpoint: ${DARKEN_HUB_ENDPOINT}`, with env
  `DARKEN_HUB_ENDPOINT=http://example:9090`, returns body with
  `endpoint: http://example:9090`.
- `TestManifest_DefaultsHubEndpoint` — env unset → substitutes
  `http://host.docker.internal:8080`.
- `TestManifest_RestrictsExpansionToDarkenPrefix` — body containing
  `$HOME` is left untouched (only `${DARKEN_*}` substitutes).
- `TestManifest_AllTemplatesUseSubstitution` — sweeps all 14
  templates, asserts each contains `${DARKEN_HUB_ENDPOINT}` and
  no literal `host.docker.internal` string.

**Implementation:**
- Edit each of the 28 manifest files (project + embedded copies).
- Add `expandManifest(body string) string` using `os.Expand` with
  a custom mapper that ignores anything not matching `^DARKEN_`.
- Wire `expandManifest` into the path that materializes a manifest
  for spawn (likely upstream of `scion templates show` /
  `scion spawn`'s manifest pickup — implementer decides exact
  insertion point during implementation; see `cmd/darken/spawn.go`
  + `cmd/darken/manifest.go`).

**Commit:**
1. `templates: replace hardcoded hub.endpoint with ${DARKEN_HUB_ENDPOINT}`
2. `cmd/darken: expandManifest substitutes DARKEN_* env vars only`
3. `cmd/darken: wire manifest expansion into spawn path`

(One-task-multi-commit is fine; keep the test green at each.)

---

## Wave B — Bugfixes

### Task B1 — Bug #1: `bones init` rerun no-ops

**Files touched:**
- existing: `cmd/darken/init.go` (`runBonesInit`)
- existing: `cmd/darken/init_test.go`

**Failing test (`init_test.go`):**
- `TestRunBonesInit_AlreadyInitializedIsNoOp` — stub `bones` on PATH
  to print `workspace already initialized` to stderr and exit 1.
  Asserts `runBonesInit` returns nil and stdout contains
  `bones init: workspace already initialized (skipping)`.
- `TestRunBonesInit_RealFailureStillReturnsError` — stub `bones` to
  exit 1 with stderr `bones: io error`. Asserts non-nil return.

**Implementation:**
- Capture combined output instead of inheriting.
- Match `workspace already initialized` substring; if matched,
  print the skip notice and return nil. Else preserve current
  behavior.
- Existing soft-fail-then-continue caller stays unchanged.

**Commit:** `cmd/darken: bones init rerun is a clean no-op`

---

### Task B2 — Bug #3: `stage-creds.sh` print-after-push

**Files touched:**
- existing: `scripts/stage-creds.sh`
- existing: `internal/substrate/data/scripts/stage-creds.sh`
- new: `scripts/test-stage-creds-push-ordering.sh`
  (or extend existing `scripts/test-stage-creds.sh`)

**Failing test (`scripts/test-stage-creds-push-ordering.sh`):**
- Stub `scion` on PATH that exits 1 (simulate failed PUT).
- Run `stage-creds.sh claude` against a fixture keychain blob.
- Assert: stdout does NOT contain `pushed (file →`.
- Assert: exit non-zero.
- Stub `scion` to exit 0; rerun; assert stdout DOES contain the
  `pushed` line.

**Implementation:**
- In `push_file_secret` and `push_env_secret`, restructure:
  ```
  if scion hub secret set …; then
    echo "stage-creds: ${name} pushed (…)"
  else
    return 1
  fi
  ```
  No bare `echo` after `set -e` letting the success line print
  before the call. Identical change in both copies (project + embedded).

**Commit:**
1. `scripts/stage-creds: print success only after scion PUT succeeds`
2. `internal/substrate/data: mirror stage-creds.sh ordering fix`

(Embed-drift test `scripts/test-embed-drift.sh` should already
catch divergence between the two copies.)

---

### Task B3 — Bug #4: doctor daemon liveness

**Files touched:**
- existing: `cmd/darken/doctor.go` (`checkScionServer`)
- existing: `cmd/darken/doctor_test.go`

**Failing test (`doctor_test.go`):**
- `TestCheckScionServer_FailsOnDaemonNotRunning` — stub `scion` to
  print `Daemon: not running\n` to stdout and exit 0. Assert
  `checkScionServer()` returns non-nil error mentioning "daemon".
- `TestCheckScionServer_OkOnDaemonRunning` — stub prints
  `Daemon: running\n`, exits 0. Assert nil error.
- `TestCheckScionServer_AmbiguousIsWarn` — stub prints output with
  no `Daemon:` line, exits 0. Assert non-nil error (treat ambiguous
  as fail; the spec calls this WARN but unit-test-wise we promote
  to fail until the doctor framework supports WARN return values).

**Implementation:**
- Capture combined output.
- Scan for `Daemon: not running`, `Daemon: stopped`, `Daemon: running`.
- Return error or nil per match.
- Include raw output in the error message for ambiguous case.

**Commit:** `cmd/darken/doctor: parse Daemon: line to detect dead scion server`

---

### Task B4 — Bug #7a: propagate prelude pre-clone to codex/gemini/pi

**Files touched:**
- existing: `images/codex/darkish-prelude.sh` (the v0.1.15 claude
  patch is the reference)
- existing: `images/gemini/darkish-prelude.sh`
- existing: `images/pi/darkish-prelude.sh`
- new or existing: `images/test-prelude-symmetry.sh`

**Failing test (`images/test-prelude-symmetry.sh`):**
- Diff the four `darkish-prelude.sh` files against each other,
  ignoring backend-specific blocks (delimited by
  `# backend-specific:start` / `# backend-specific:end`). Assert
  the pre-clone workaround block is present and identical across
  all four.

**Implementation:**
- Copy the v0.1.15 claude pre-clone block to the other three,
  wrapped in `# darkish-prelude:pre-clone-workaround:start /
  :end` markers.
- Ensure the test diffs the marker blocks, not the whole file.

**Commit:** `images: propagate sciontool pre-clone workaround to codex, gemini, pi preludes`

---

### Task B5 — Bug #7b: doctor sciontool sniff-test

**Files touched:**
- existing: `cmd/darken/doctor.go`
- existing: `cmd/darken/doctor_test.go`

**Failing test:**
- `TestDoctor_SciontoolSniff_GatedByFlag` — invoking
  `runDoctor([]string{})` does NOT run the sniff. Invoking
  `runDoctor([]string{"--check=sciontool"})` does.
- `TestDoctor_SciontoolSniff_DetectsEmptyStderrFailure` — stub
  `docker run` to exit non-zero with empty stderr. Assert reported
  as the FS-incompatibility pattern with the prelude-workaround
  remediation hint.

**Implementation:**
- Add a `--check=sciontool` flag dispatch path in `runDoctor`.
- Implementation runs the smallest available darkish image with
  `sciontool init /tmp/test_grove`. Captures combined output.
- On non-zero exit + empty stderr → FS-incompat (known pattern).
- On non-zero exit + non-empty stderr → unrelated failure, report
  the stderr as-is.

**Commit:** `cmd/darken/doctor: add --check=sciontool sniff for FS+go-git incompatibility`

---

### Task B6 — Bug #8: bootstrap runs `scion broker provide`

**Files touched:**
- existing: `cmd/darken/bootstrap.go`
- existing: `cmd/darken/bootstrap_test.go`

**Failing test (`bootstrap_test.go`):**
- `TestEnsureBrokerProvide_CallsScionBrokerProvide` — stub `scion`
  on PATH that records its argv. Asserts `ensureBrokerProvide()`
  invoked `scion broker provide`.
- `TestEnsureBrokerProvide_PropagatesError` — stub exits 1; assert
  non-nil return that surfaces the broker output.

**Implementation:**
- New step `{"broker provides current grove", ensureBrokerProvide}`
  inserted in `runBootstrap`'s steps slice between
  `ensureScionServer` and `ensureImages`.
- `ensureBrokerProvide()` shells via `scionCmd([]string{"broker",
  "provide"})`. (Grove identity assumed implicit; if the
  implementer discovers scion needs an explicit grove arg, surface
  it via the existing repo-root resolver and document.)

**Commit:** `cmd/darken/bootstrap: provide current grove to broker`

---

### Task B7 — Bug #9: doctor detects missing /etc/hosts entry

**Files touched:**
- existing: `cmd/darken/doctor.go`
- existing: `cmd/darken/doctor_test.go`

**Failing test:**
- `TestDoctor_HostDockerInternal_GatedByEndpoint` — when
  `DARKEN_HUB_ENDPOINT=http://my-broker:9090`, the check is SKIPped.
- `TestDoctor_HostDockerInternal_DetectsResolverFailure` — stub the
  in-container probe to exit 2 (name not found). Assert FAIL with
  remediation showing both the `/etc/hosts` one-liner AND the
  `DARKEN_HUB_ENDPOINT` knob alternative.
- `TestDoctor_HostDockerInternal_OkWhenResolves` — probe succeeds
  → OK.

**Implementation:**
- New check function `checkHostDockerInternal()` in `doctor.go`.
- Skip when `DARKEN_HUB_ENDPOINT` is set and doesn't contain
  `host.docker.internal`.
- Probe via `docker run --rm <smallest-darkish-image> getent hosts
  host.docker.internal` (image guaranteed present after
  `ensureImages`).
- Add to `doctorBroad`'s checks slice after `checkImages` (so
  image-missing is caught first; otherwise the probe spuriously
  fails on image-not-built).

**Commit:** `cmd/darken/doctor: detect host.docker.internal resolution failure`

---

### Task B8 — Bug #10: setup uploads all 14 templates at user scope

**Files touched:**
- existing: `cmd/darken/setup.go`
- new: `cmd/darken/setup_uploads.go` (or inline; implementer's call)
- existing: `cmd/darken/setup_test.go`

**Failing test (`setup_test.go`):**
- `TestRunSetup_UploadsAll14TemplatesAtUserScope` — stub `scion`
  on PATH recording argvs. Run `runSetup([]string{})`. Assert at
  least 14 invocations of `scion templates upload <name> --scope
  user`, one per role in `canonicalRoles`. Ordering not asserted.
- `TestRunSetup_TemplateUploadIdempotent` — stub `scion templates
  upload` to exit 0 with body `template already exists` — assert
  setup still returns nil.
- `TestRunSetup_TemplateUploadHardError` — stub exits 1 with
  unrelated error; assert setup returns non-nil.

**Implementation:**
- After `runBootstrap` completes successfully, iterate
  `canonicalRoles` (from Task A1). For each, invoke
  `scionCmd(["templates", "upload", role, "--scope", "user"])`.
- Treat "already exists" (or scion's idempotent-no-op signal) as
  success. Treat other non-zero as fail-fast (with the role name
  in the error).

**Commit:** `cmd/darken/setup: upload all 14 templates to Hub at user scope`

---

## Wave C — Substrate + skills

### Task C1 — `make vendor-skills` pipeline

**Files touched:**
- new: `internal/substrate/skills.manifest.txt`
- existing: `Makefile` (or new `vendor-skills.mk` included from it)
- new: `scripts/test-vendor-skills.sh`
- existing: `scripts/test-embed-drift.sh` (extend to cover skills tree)

**Failing test (`scripts/test-vendor-skills.sh`):**
- Setup: create a tmpdir representing a fake `agent-config` with
  `skills/hipp/SKILL.md` and `skills/ousterhout/SKILL.md`.
- Set `DARKEN_SKILLS_SOURCE=$tmpdir`.
- Run `make vendor-skills`.
- Assert `internal/substrate/data/skills/hipp/SKILL.md` exists and
  matches the source.
- Assert: when a manifest entry is missing from the source tree,
  `make vendor-skills` exits non-zero with the missing skill name.
- Assert: re-running is idempotent (no diff on second run).

**Implementation:**
- `internal/substrate/skills.manifest.txt`: list one skill per
  line, with optional `:in-tree` suffix to mark those sourced from
  the embedded data directory itself (`superpowers`, `spec-kit`).
  Default source is `${DARKEN_SKILLS_SOURCE:-$HOME/projects/agent-config/skills}/<name>/`.
- `make vendor-skills` target: bash loop reads the manifest, copies
  each non-`:in-tree` skill into `internal/substrate/data/skills/<name>/`
  via `rsync -a --delete`. Hard-fails on missing source.
- `make darken` declares `vendor-skills` as a prereq.
- Extend `scripts/test-embed-drift.sh` to also assert the skills
  tree under `internal/substrate/data/skills/` matches what would
  be produced by `make vendor-skills` against the manifest (using
  the operator's actual canon directory; CI can pin a fixture).

**Commit:**
1. `internal/substrate: declare skills.manifest.txt with vendored roster`
2. `Makefile: add vendor-skills target gated on skills.manifest.txt`
3. `scripts: test-vendor-skills smoke + extended embed-drift coverage`

---

### Task C2 — Author MV `superpowers` + `spec-kit` skill shells

**Files touched:**
- new: `internal/substrate/data/skills/superpowers/SKILL.md`
- new: `internal/substrate/data/skills/spec-kit/SKILL.md`
- (manifest declares both `:in-tree` from C1)

**Failing test:**
- `TestEmbeddedSkillsHaveSuperpowersAndSpecKit` (Go test in
  `internal/substrate/embed_test.go` if it exists, else new):
  asserts `embed.FS.ReadFile("data/skills/superpowers/SKILL.md")`
  and `…/spec-kit/SKILL.md` both succeed and have non-empty
  bodies starting with valid SKILL.md frontmatter (`---\nname:`).

**Implementation:**
- Author minimum-viable `SKILL.md` shells for both. Frontmatter
  block + a 2-paragraph "what this skill is, what it isn't" body
  that maps to the planner-t3 / planner-t4 system-prompts'
  expectations. Full content is an explicit follow-on; the goal
  here is "manifest references resolve" so planner-t3 / planner-t4
  no longer reference missing skills.

**Commit:** `internal/substrate: minimum-viable superpowers and spec-kit SKILL shells`

---

### Task C3 — Replace `CANONICAL` hardcode in stage-skills

**Files touched:**
- existing: `scripts/stage-skills.sh`
- existing: `internal/substrate/data/scripts/stage-skills.sh`
- existing: `cmd/darken/script_runner.go` (extend `scriptEnv()`)
- existing: `cmd/darken/script_runner_test.go`
- existing: `scripts/test-stage-skills.sh` (extend)

**Failing test:**
- `TestScriptEnv_InjectsSkillsCanonical` (Go) — asserts
  `scriptEnv()` includes `DARKEN_SKILLS_CANONICAL=<path>`.
- `test-stage-skills.sh` extension — set
  `DARKEN_SKILLS_CANONICAL=$tmpdir/fake-canon`, populate the fake
  canon, run stage-skills, assert it materialized from the fake
  canon (not from the operator's `~/projects/agent-config`).

**Implementation:**
- In `scripts/stage-skills.sh`: replace
  `CANONICAL="${HOME}/projects/agent-config/skills"` with
  `CANONICAL="${DARKEN_SKILLS_CANONICAL:-${HOME}/projects/agent-config/skills}"`.
  Mirror in the embedded copy.
- In `script_runner.go`'s `scriptEnv()`:
  - In dev mode (binary run from a working tree): set
    `DARKEN_SKILLS_CANONICAL=$DARKEN_REPO_ROOT/internal/substrate/data/skills`
    if that path exists.
  - In release mode (no working tree): extract the embedded
    skills tree to a tmp dir per-invocation, set
    `DARKEN_SKILLS_CANONICAL` to the tmpdir, schedule cleanup.
  - Operator override (env var pre-set) wins.

**Commit:**
1. `scripts/stage-skills: read CANONICAL from DARKEN_SKILLS_CANONICAL env`
2. `cmd/darken/script_runner: inject DARKEN_SKILLS_CANONICAL from substrate`

---

### Task C4 — Bug #11: rewrite orchestrator-mode + subagent-to-subharness

**Files touched:**
- existing: `.claude/skills/orchestrator-mode/SKILL.md`
- existing: `internal/substrate/data/skills/orchestrator-mode/SKILL.md`
- existing: `.claude/skills/subagent-to-subharness/SKILL.md`
- existing: `internal/substrate/data/skills/subagent-to-subharness/SKILL.md`
- existing: `cmd/darken/doctor_test.go` (drift check should stay green)

**Failing tests:**
- `TestOrchestratorSkill_BiasesTowardSubharness` (new Go test in
  `internal/substrate/skills_content_test.go`):
  - `embed.FS` read of `data/skills/orchestrator-mode/SKILL.md`
    contains the literal phrase
    `dispatch a subharness for any task that fits one of the 14 roles`.
  - Contains the literal phrase
    `Agent tool is the fallback`.
- `TestSubagentToSubharness_ListsAllRoles` — `embed.FS` read of
  the mapping skill's body contains each of the 14 role names from
  `canonicalRoles`.
- `TestSubstrateDriftStaysGreen` — `checkSubstrateDrift()` returns
  the OK string after the rewrite (project + embedded copies must
  match byte-for-byte).

**Implementation (per spec §3.3):**
- Rewrite the decision tree in `subagent-to-subharness/SKILL.md`'s
  "Decision tree" section so the first branch is "Does the task
  fit a role? → Yes: spawn. No: stay inline / Agent."
- Add the four exception clauses (substrate-down, no-role-fit,
  operator-override, already-spawned) verbatim per spec.
- Add the role-fit trigger phrase set to the mapping table (one
  extra column per row showing trigger phrases).
- In `orchestrator-mode/SKILL.md`'s Step 2 routing classifier:
  insert the role-fit precheck before the six-axis scoring. If a
  role matches, skip the axes and dispatch.
- Mirror identical edits to both project and embedded copies.

**Commit:**
1. `skills/subagent-to-subharness: bias decision tree toward subharness`
2. `skills/orchestrator-mode: role-fit precheck before routing axes`
3. `internal/substrate/data: mirror skill rewrites`

---

### Task C5 — End-to-end smoke

**Files touched:**
- existing: `scripts/test-end-to-end.sh`

**Failing test (extension):**
- New phase: after `darken setup` completes on a fixture clean
  grove, assert each of the following in order:
  1. `scion server status` reports daemon running.
  2. `scion broker list` (or equivalent) shows the current grove
     provided.
  3. `scion templates list --scope user` includes all 14 roles.
  4. `darken doctor` exits 0 with no FAIL lines.
  5. `darken spawn smoke-1 --type researcher "say hi"` exits 0
     and the agent reaches a non-`Starting` state within
     `DARKEN_SPAWN_READY_TIMEOUT`.
- Optionally: run from a *different* grove (fixture clean dir
  with no templates) and assert step 5 still works (proves the
  user-scope upload is doing what bug #10 needed).

**Implementation:** sequencing of existing commands; no new
production code.

**Commit:** `scripts: end-to-end test covers v0.1.16 setup invariants`

---

## Acceptance criteria

The plan is complete when:

1. `make darken && bin/darken doctor` exits 0 on a fresh laptop
   after `brew install` (no `agent-config` clone).
2. `bin/darken setup` in a fresh grove succeeds with no manual
   `/etc/hosts` edits when `DARKEN_HUB_ENDPOINT` is unset and the
   operator's machine already has `host.docker.internal`
   resolvable; OR with `DARKEN_HUB_ENDPOINT=…` set, no host edits
   needed.
3. `bin/darken spawn …` works from a *different* grove than the
   one where `darken setup` ran (templates uploaded at user scope
   per #10).
4. `scripts/test-stage-creds-push-ordering.sh` proves no
   "pushed" line escapes when scion is unhealthy.
5. `bin/darken doctor` reports FAIL when the daemon is down (vs.
   the v0.1.15 OK lie).
6. `bin/darken doctor --check=sciontool` correctly identifies the
   FS+go-git incompatibility pattern.
7. `planner-t3` / `planner-t4` spawn cleanly with all declared
   skills resolving from the substrate (no
   `agent-config/skills/superpowers` not-found errors).
8. The orchestrator-mode + subagent-to-subharness skills' rewritten
   decision tree leads with role-fit; an operator reading the
   skills can name the four exception clauses.
9. The drift check in `darken doctor` reports OK for both rewritten
   skills (project + embedded copies match).
10. The end-to-end script (Task C5) runs green in CI.

## Test discipline reminder

Every task above lists the failing test FIRST. The implementer
writes the test, watches it fail with the expected error, then
writes the minimal implementation to make it pass, then commits.
No skipping the failing-test step. No combining multiple tasks
into a mega-commit.

## Open questions deferred (NOT blocking implementation)

- **Grove identity for `scion broker provide`** — implementer
  discovers during B6. If scion needs explicit grove arg, surface
  via repo-root resolver and document; if implicit, no change
  beyond the spawn invocation.
- **Sciontool upstream issue** — operator files outside this plan;
  the in-repo deliverable is the doctor sniff-test.
- **`superpowers` / `spec-kit` full content** — C2 ships shells
  good enough to resolve the manifest references; full skills are
  follow-on work tracked separately.
- **Doctor WARN return value** — the current framework returns
  `error` only (binary OK/FAIL). True WARN support is a follow-on;
  for now ambiguous-daemon-output is treated as FAIL per Task B3.
