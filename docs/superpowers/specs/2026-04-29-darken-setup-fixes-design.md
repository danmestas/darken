# Darken setup fixes — design (v0.1.16)

Status: draft (planner-t2)
Date: 2026-04-29
Successor of: v0.1.15 (`docs/superpowers/plans/2026-04-28-darken-setup.md`)
Related: `2026-04-28-bones-inter-harness-comms-design.md` (out-of-scope here),
`2026-04-28-roles-bases-template-collapse-design.md` (interacts with §A below)

## 1. Goal

Eliminate the eleven blocking and noise-class failures observed during
the v0.1.15 fresh-repo `darken setup` debugging session. Make
`darken setup` succeed end-to-end on a clean machine — no manual
`/etc/hosts` edits, no cross-grove template gaps, no
cross-repo-clone dependency on the operator's `agent-config`.

## 2. Architecture summary (3 sentences)

`darken setup` becomes the single source of truth for fresh-repo
onboarding: it bundles every canonical skill into the binary via
`go:embed`, registers all 14 templates at user scope on the Hub, and
provides the grove to the broker — no out-of-band manual steps. The
per-template hardcoded Hub endpoint is replaced with a runtime knob
read by a centralized `scion-cmd` helper, removing the brittle
`/etc/hosts` dependency for the common case while keeping the
operator's `host.docker.internal` workflow as the documented default.
The orchestrator-mode skill is rewritten to bias toward subharness
dispatch when any of the 14 roles fits the task — the Anthropic
`Agent` tool becomes an explicit fallback rather than the default for
"light/pure-text" work.

## 3. Three top-line architectural calls

These are the decisions that ripple across the most files. They are
ratified up front so the per-bug tasks below can reference a single
choice rather than re-litigating in each task.

### 3.1 Bundling shape — `go:embed` + vendor pipeline (resolves bug #2)

**Choice:** Bundle canonical skills into the substrate via `go:embed`,
sourced at build time from a configurable canonical directory.

**Rejected alternatives:**

| Option | Why rejected |
|---|---|
| Vendor dir checked into repo | Doubles disk footprint, drift between vendored copy and operator's working canon, non-trivial review noise on every skill bump. |
| Git submodule (`agent-skills` as submodule) | Operator just renamed `agent-skills` → `agent-config`; submodule pinning becomes another moving piece, and submodules + worktrees (which scion uses) have known correctness pitfalls. |
| Clone-on-bootstrap | Network dependency at setup time; `darken setup` already aspires to be local-only after `brew install darken`; would re-introduce the failure mode v0.1.15 was trying to remove. |

**Justification:**

- The substrate resolver already layers `flag → env → user → project →
  embedded`. Embedding extends a pattern that already exists in
  `internal/substrate/data/skills/` (orchestrator-mode and
  subagent-to-subharness ship today via `go:embed`). No new mechanism.
- Embedding makes a `darken` binary version self-contained: a fresh
  laptop with `brew install darken` has every skill it needs without
  any other clone, secret, or network step.
- Operator override survives via the existing resolver layers — power
  users who want a working-tree copy of a skill can drop it in
  `${DARKEN_SUBSTRATE_OVERRIDE}/data/skills/<name>/` and the resolver
  picks it up over the embedded copy, exactly as it does today for
  `orchestrator-mode/SKILL.md` (see `cmd/darken/doctor.go`'s drift
  check).

**Vendor pipeline (build-time):**

- Add `internal/substrate/skills.manifest.txt` — a newline-separated
  list of canonical skill names darken bundles. Initial roster:
  `hipp`, `ousterhout`, `superpowers`, `spec-kit`, plus the existing
  `orchestrator-mode` and `subagent-to-subharness` (already embedded).
- Add `make vendor-skills` target. Reads the manifest. For each
  skill, copies `${DARKEN_SKILLS_SOURCE:-$HOME/projects/agent-config/skills}/<name>/`
  into `internal/substrate/data/skills/<name>/`. Idempotent.
- `make darken` declares `vendor-skills` as a prerequisite.
- CI: `make vendor-skills` + `git diff --exit-code internal/substrate/data/skills`
  catches drift between the manifest and what's checked into the
  embedded tree.

**Missing skills (`superpowers`, `spec-kit`):**

- The brief notes these don't exist in agent-config today.
- `make vendor-skills` will hard-fail on the first missing skill so
  the gap is visible at build time rather than at runtime in a
  containerized planner-t3 / planner-t4.
- Mitigation in this spec: a sub-task in §4 (Bug #2) authors
  minimum-viable `SKILL.md` shells for `superpowers` and `spec-kit`
  in the substrate's embedded tree directly (i.e. checked into
  `internal/substrate/data/skills/superpowers/` and
  `…/spec-kit/`), rather than waiting on agent-config to grow them.
  The manifest can declare these two as "in-tree" (sourced from
  `internal/substrate/data/skills/`) versus the rest as "vendored"
  (sourced from agent-config). One make target, two source roots.

**Tradeoff accepted:** bumping a skill requires a `darken` rebuild +
release. This is desirable: deterministic substrate per binary
version. Drift is now a release-time concern, not a runtime one.

**Stopgap removal:** v0.1.15 inline-patched the `CANONICAL` path in
`scripts/stage-skills.sh` and `internal/substrate/data/scripts/stage-skills.sh`
from `~/projects/agent-skills/skills` to `~/projects/agent-config/skills`.
After this fix, `CANONICAL` resolves from an injected env var
(`DARKEN_SKILLS_CANONICAL`, set by `script_runner.go`) pointing at
either the operator's working canon (dev mode) or a tmpdir extracted
from the embedded tree (release mode). The hardcoded path goes away.

### 3.2 Hub URL strategy — config knob + doctor check, NOT auto-/etc/hosts (resolves bugs #6, #9)

**Choice:** Replace the per-template hardcoded
`http://host.docker.internal:8080` with a substrate-resolved knob
applied at spawn time by a centralized `scion-cmd` helper. Pair with
a `darken doctor` check that flags missing `/etc/hosts` entries and
prints the exact remediation command — but darken does NOT run sudo.

**Rejected alternative (a):** auto-add `/etc/hosts` entry via sudo
prompt in `darken setup`. Reasons:

- A sudo prompt in a wrapper script is a UX trap (looks legitimate;
  trains operators to type sudo passwords into wrapper output).
- Breaks headless / CI / container-of-containers flows.
- macOS `host.docker.internal` is already a Docker Desktop
  convenience name on that platform; on Linux it requires the
  `--add-host` runtime flag or the explicit hosts entry. The solution
  shape differs per OS — better to surface the remediation than try
  to abstract.

**Mechanism (replaces hardcode in 14 templates × 2 trees = 28 files):**

- Templates declare `hub.endpoint: "${DARKEN_HUB_ENDPOINT}"` as a
  literal placeholder string. The scion-cmd helper, before
  invoking `scion spawn`, materializes the template through a tiny
  substitution pass: any `${VAR}` pattern in the materialized
  manifest is replaced with the env value, defaulting to
  `http://host.docker.internal:8080` when unset.
- This does NOT require a YAML library; the existing `scanField`
  approach in `cmd/darken/doctor.go` already shows the project's
  taste for hand-rolled scalar reads. Substitution can be a single
  `os.Expand` call on the materialized manifest text before the
  spawn shells out.
- The default value of `DARKEN_HUB_ENDPOINT` preserves the v0.1.15
  status quo: existing operators with a working `/etc/hosts` entry
  see no behavior change.
- Operators on Linux without `host.docker.internal` set
  `DARKEN_HUB_ENDPOINT=http://172.17.0.1:8080` (the docker0 bridge)
  and stop editing /etc/hosts.
- Operators running darken-in-darken or remote-broker setups override
  to whatever URL their broker advertises.

**Doctor check (#9):**

- New check `host.docker.internal resolvable from container`:
  - probes resolution from inside the smallest local image (e.g.
    `docker run --rm local/darkish-claude:latest getent hosts host.docker.internal`).
  - on FAIL: print exact remediation
    (`echo "$(docker network inspect bridge -f '{{range .IPAM.Config}}{{.Gateway}}{{end}}') host.docker.internal" | sudo tee -a /etc/hosts`)
    AND mention the `DARKEN_HUB_ENDPOINT` knob as the no-sudo
    alternative.
- Check is gated by `DARKEN_HUB_ENDPOINT == "" || endpoint contains 'host.docker.internal'`
  — operators who've moved off the magic name don't get spurious warnings.

**Tradeoff accepted:** operators on a clean Linux machine still need
one manual decision (edit `/etc/hosts` OR set the env var). We
surface the choice rather than make it for them. Sudo-in-wrapper is
worse than one-time documented friction.

### 3.3 Skill bias rule — subharness is the default; `Agent` is the fallback (resolves bug #11)

**Choice:** Rewrite the orchestrator-mode skill's classifier and the
subagent-to-subharness mapping table so the decision tree leads with
"does any of the 14 roles fit?" If yes → spawn subharness, even when
the routing axes say "light/pure-text". The `Agent` tool is reserved
for: (a) substrate genuinely unavailable (doctor fails), (b) no role
matches the task shape, (c) explicit operator override.

**Justification:**

- The Darkish Factory's value prop is cross-vendor + isolated-context
  + auditable dispatch. Using `Agent` for delegate-shaped work
  bypasses every one of those properties — it runs in-process, same
  vendor, no audit log entry, no worktree.
- A "pure-text → inline OR Agent" default trains the orchestrator
  to keep work in-process. The repo's CLAUDE.md is explicit: the
  orchestrator should not implement inline. The skill must reflect
  that, not contradict it.
- Most "pure-text" tasks have a role fit:
  - "produce a brief on X" → researcher
  - "review this diff" → reviewer
  - "answer this focused question" → sme
  - "audit / chronicle activity" → admin
  - "draft a spec" → designer
  - "plan TDD steps" → planner-t1..t4
  The natural-language hook lookup is wide; very few tasks land
  outside the role matrix.

**Rule statement (will appear in both skills verbatim):**

```
Default: dispatch a subharness for any task that fits one of the 14
roles, regardless of routing-axis weight.

Exceptions (when to stay inline or use Agent instead):
  1. Substrate not available (`darken doctor` fails or scion server
     is down). Inline / Agent is forced.
  2. No role fit. Examples: "explain how X flows through this repo"
     (open-ended exploration with no deliverable artifact),
     "summarize the last three commits" (read-only report-back).
     Use Agent (Explore) or stay inline.
  3. Operator explicit override ("just do it", "no spawn", "inline").
  4. Already-spawned: an agent for this task is live (`darken list`
     shows it). Don't double-spawn; resume via `scion send`.

Anti-patterns kept from the previous skill version:
  - Don't spawn yourself (orchestrator role).
  - Don't spawn for trivial 30-second edits (and even then, only in
    operator-direct mode, not orchestrator mode).
```

**Mapping-table changes (subagent-to-subharness skill):**

The "Subagent reflex → Subharness equivalent" table grows two
columns: a **role-fit** trigger phrase set, and a **fallback** column
naming when `Agent` is appropriate. Example row:

```
| "I'll explore the codebase to find X"
| `darken spawn r1 --type researcher "find X; output to docs/research-brief.md"`
| Use `Agent (Explore)` only if X is a one-shot lookup with no artifact (< 30s).
```

The Decision tree's first branch flips from
`Pure-text host work? → Stay inline` to
`Does the task fit a role? → Yes: spawn. No: stay inline.`

**Tradeoff accepted:** subharness dispatch is more expensive (cold
container start, separate hub auth, polling). Some tasks that today
take 30 seconds inline will become 60-second async dispatches. We're
paying that cost for the substrate's value prop. Operator can always
override.

## 4. Per-bug design notes

The plan document at `docs/superpowers/plans/2026-04-29-darken-setup-fixes.md`
breaks each into TDD tasks. This section captures only design
decisions and tradeoffs that go beyond "fix the bug as described".

### Bug #1 — `bones init` rerun noise

**Decision:** detect-and-skip pattern, not `bones join` substitution.

`bones join` joins an existing workspace under a different name; it's
not equivalent to "rerun-as-no-op" semantically. The right response
to "already initialized" is "do nothing, exit 0, print a one-line
notice." `runBonesInit` in `cmd/darken/init.go` already soft-fails
(non-fatal); the change is to treat the specific
`workspace already initialized` exit pattern as a clean no-op and
print `bones init: workspace already initialized (skipping)`.

**Detection:** match on the exact stderr substring
`workspace already initialized` from the bones command's combined
output. Avoids parsing exit codes (which are noisy across bones
versions).

### Bug #3 — `stage-creds.sh` print-before-success

**Decision:** straightforward fix; no design surface.

Move the `echo "stage-creds: ${name} pushed …"` lines to AFTER the
`scion hub secret set` call has succeeded. Bash `set -e` already
aborts the function on a failed `scion` call, so simply reordering
the two lines (with a noop guard `local rc=0; scion ... || rc=$?;
[[ $rc -eq 0 ]] && echo …; return $rc`) is enough. Test asserts the
"pushed" line is absent when scion is mocked to fail.

### Bug #4 — `darken doctor` daemon liveness

**Decision:** parse `Daemon:` line from `scion server status`, OR
probe `/healthz` if the scion server exposes one.

Inspecting the brief, the symptom is: `scion server status` exits 0
but the body says daemon-not-running. So `exec.Command("scion",
"server", "status").Run()` is the wrong check — the exit code is
unreliable. Capture stdout, scan for either:

- a literal `Daemon: not running` (or `Daemon: stopped`) line — fail
- a literal `Daemon: running` line — pass
- ambiguous output (no Daemon: line at all) — WARN with the raw
  output included for debugging

**Tradeoff:** parsing CLI output is fragile across scion versions.
Mitigation: keep the regex narrow (just `Daemon:` line) and add a
unit test with both string forms. If `/healthz` is exposed in a
future scion release, switch.

### Bug #5 — centralized `scion-cmd` helper

**Decision:** new file `cmd/darken/scion_cmd.go` (paired with
`scion_cmd_test.go`). Pure function:

```go
// signature only — implementation in plan
type scionCmdOpts struct { … }
func scionCmd(args []string, opts scionCmdOpts) *exec.Cmd
```

The helper:

- Sets `SCION_HUB_ENDPOINT=${DARKEN_HUB_ENDPOINT}` (the §3.2 knob).
- Sets `DARKEN_REPO_ROOT` (already done in `script_runner.go`).
- Forwards stdout/stderr unless `Captured` is set.
- Threads `posArgs` correctly — fixing the `--`-vs-positional bug
  v0.1.15 inline-patched in `runSpawn`.

Existing call sites migrate one at a time: `runDoctor`'s
`exec.Command("scion", …)` lines, `runBootstrap`'s
`ensureScionServer`, `runSpawn`'s spawn invocation. Migration is
mechanical; each migration commit gets a regression test.

**Regression test for spawn posArgs:** the inline v0.1.15 fix needs
a test that asserts the trailing positional task string is passed
through verbatim (not swallowed by flag parsing). Done as the first
task that touches `runSpawn`, before the migration to
`scion_cmd.go`.

### Bug #7 — prelude pre-clone propagation + sciontool sniff-test

**Decision:** propagate the v0.1.15 claude prelude workaround
verbatim to the other three preludes (`images/codex/`,
`images/gemini/`, `images/pi/`). Add a doctor check that performs
the FS-incompatibility sniff-test using whatever the smallest
locally-available image is.

The brief mentions "file an upstream issue with scion". I leave that
as an out-of-band action item for the operator; the doctor check is
the in-repo deliverable. The sniff-test:

- Skip on host (workaround is image-side; host doctor doesn't
  reproduce the bug).
- Run as: `docker run --rm <smallest-image> sciontool init /tmp/test_grove`
  in a tmpdir. On empty-stderr non-zero exit, FAIL with remediation
  pointing at the prelude pre-clone workaround comment.

**Tradeoff:** the sniff-test runs a container, which is slow (~2-5s)
and requires images already built. Gate it behind a
`--check=sciontool` flag on doctor so the default broad doctor stays
fast; add it to the per-harness doctor pass instead.

### Bug #8 — `scion broker provide`

**Decision:** add a step to `runBootstrap` between `ensureScionServer`
and `ensureImages`:

```
"broker provides current grove" → ensureBrokerProvide
```

`ensureBrokerProvide`: shells out via the new `scion-cmd` helper to
`scion broker provide <grove-id>`. Idempotent on the scion side
(provide-already-provided is a no-op). On error, surface the broker
output and exit 1 — broker-not-providing is fatal for spawn to work
in the current grove.

**Open question to operator (deferred to skill rewrite, not blocking
this plan):** is the grove-id implicit (current dir) or does darken
need to compute/persist it? Plan task assumes implicit; if scion
requires explicit, the implementer adds a `darken init`-time grove
discovery step.

### Bug #10 — upload all 14 templates at user scope

**Decision:** add a step to `runBootstrap` (or `runSetup`,
post-bootstrap) that iterates the 14 known roles and uploads each
via `scion templates upload <name> --scope user`. Idempotent on the
scion side.

Use the canonical roster constant — defined once in a new
`cmd/darken/roles.go` (or extending `script_runner.go`'s pattern):

```go
var canonicalRoles = []string{
  "admin", "base", "darwin", "designer", "orchestrator",
  "planner-t1", "planner-t2", "planner-t3", "planner-t4",
  "researcher", "reviewer", "sme", "tdd-implementer", "verifier",
}
```

The same constant feeds the §3.3 skill rewrite (the role-fit table)
once we surface it in the embedded skills. To be explicit: this
constant is the authoritative source, and the spec/skill rewrites
reference it by-name.

**Tradeoff:** uploads run every `darken bootstrap`. That's fine —
the operation is idempotent and cheap. Alternative (only upload
once, persist a marker) introduces state divergence between the
binary's view of templates and the Hub's; idempotent re-upload is
simpler.

### Bug #11 — orchestrator-mode + subagent-to-subharness rewrite

Already covered in §3.3 above. The plan's task is the actual edit
to both `SKILL.md` files (project copies + embedded copies, since
they're checked-in twins under `internal/substrate/data/skills/`).
A doctor-style drift check (`checkSubstrateDrift` in `doctor.go`)
already detects when project copies diverge from embedded copies; we
make sure that check fires green after the rewrite by editing both.

## 5. Non-goals (this spec)

- Mode A pivot. Stays out.
- Inter-harness messaging. See `2026-04-28-bones-inter-harness-comms-design.md`.
- Template collapse. See `2026-04-28-roles-bases-template-collapse-design.md`
  — note the §3.1 bundling work creates new embedded skills; if the
  collapse spec moves templates around, the bundling pipeline needs
  to follow but doesn't block.
- New skills authoring beyond `superpowers` + `spec-kit` minimum-
  viable shells. Full skill content is a follow-on.

## 6. Risks

| Risk | Mitigation |
|---|---|
| `make vendor-skills` source paths drift between operator machines | Manifest declares each skill's source explicitly (in-tree vs vendored); CI runs `make vendor-skills` + `git diff --exit-code`. |
| `os.Expand` on a manifest substitutes a `$VAR` we didn't intend | Restrict substitution to `${DARKEN_HUB_ENDPOINT}` and `${DARKEN_*}` prefix only via custom `os.Expand` mapper. |
| Operator on Linux without docker0 access still hits the URL bug | Doctor check + remediation message now both fire; the default no longer assumes the magic name resolves. |
| Skill rewrite surprises an operator who explicitly wanted Agent for everything | Operator override clause ("just do it", "no spawn") preserved verbatim. |
| Bundling breaks the substrate-drift doctor check | `checkSubstrateDrift` only checks `orchestrator-mode/SKILL.md` today; expanding it to all canonical skills is a future improvement, not a blocker. |
| `scion broker provide` semantics don't match the assumption | Plan task includes a smoke step that runs the command in a clean grove and asserts spawn works after. If the assumption is wrong, the implementer escalates rather than guessing. |

## 7. Out-of-scope deferrals captured here

- A proper IPC channel from a subharness up to the orchestrator (so a
  planner's clarifying questions can reach the operator). Bug #2's
  brief flagged this — see referenced design doc for the path.
- Multi-grove broker discovery. Once #10 ships, every grove sees
  every template, but cross-grove agent dispatch (e.g. orchestrator
  in grove A spawns in grove B) is its own design.
- The `bones join` semantics. We rule it out for #1; if the
  operator later wants explicit "join existing workspace" support,
  it's a separate subcommand.
