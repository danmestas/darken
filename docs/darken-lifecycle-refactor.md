# Darken lifecycle refactor

A plan to simplify `darken up` / `darken down` without changing scion (which lives upstream at Google and we don't control). All work stays inside this repo. The goal is to remove the bug-classes that produced PRs #43 and #45, eliminate the asymmetry that made #45 possible at all, and shrink the surprise area when an underlying tool (scion, bones, docker) misbehaves.

## Problem

`darken up` is 8 hand-rolled steps; `darken down` is 4 hand-rolled steps; almost every step shells out to scion or a bash script and forwards stdout/stderr unfiltered. Three bug-classes have surfaced in the last two weeks, all sharing the same root cause:

1. **Stderr forwarding noise.** `scion templates import --all <empty-dir>` (#43) and `scion grove delete -y` (#45, the latter doesn't even exist) both dumped cobra's full Usage block to stderr because darken just forwarded scion's output. We fixed each one in isolation; the next one will look the same.

2. **Up/down asymmetry.** `darken up` calls `BrokerProvide()` at step [4/8]. `darken down` had no symmetric `BrokerWithdraw()` until #45 added one. The forgetting was structural — the up and down code paths are independent slices of `func() error` with no enforced pairing.

3. **Layering opacity.** `bones down --yes` legitimately starts a hub mid-teardown to deregister state. The user reasonably read that as a contradiction, because darken's output makes no distinction between "darken's intent" and "what bones decided to do." Comments help; structure helps more.

The unifying observation: **darken is a thin orchestration layer that doesn't model what it's orchestrating**. It calls commands and forwards output. When a command misbehaves, the operator sees raw shell-y output. When a step is missing its symmetric pair, nothing notices.

## Target shape

### 1. `Step` abstraction with paired up/down

Replace `[]func() error` in both `runBootstrap` and `runDown` with a single declared list of steps that own both directions:

```go
type Step struct {
    Name     string
    Up       func() error // forward (darken up)
    Down     func() error // reverse (darken down); nil = no-op
    Critical bool         // hard-fail vs best-effort
}

var lifecycle = []Step{
    {Name: "docker daemon reachable",
     Up: checkDocker, Down: nil, Critical: true},
    {Name: "scion CLI present",
     Up: checkScion, Down: nil, Critical: true},
    {Name: "scion server running",
     Up: ensureScionServer, Down: nil /* leave running */, Critical: true},
    {Name: "broker provided to grove",
     Up: ensureBrokerProvide, Down: withdrawBrokerInline, Critical: false},
    {Name: "darken images built",
     Up: ensureImages, Down: nil /* keep images */, Critical: true},
    {Name: "hub secrets pushed",
     Up: ensureHubSecrets, Down: nil, Critical: true},
    {Name: "templates staged + imported",
     Up: ensureAllSkillsStaged, Down: deleteProjectGrove, Critical: true},
    {Name: "final doctor",
     Up: finalDoctor, Down: nil, Critical: false},
}
```

`runBootstrap` walks forward calling `step.Up`. `runDown` walks reverse calling `step.Down` where non-nil. The asymmetry that produced #45 literally cannot recur — every step ships with its own teardown, and the reviewer can read up/down side-by-side in one place.

### 2. Route every scion call through `ScionClient`

`down.go` still uses raw `exec.Command` in `stopProjectAgents` (lines 90–91) and `purgeHostState` (lines 139, 143). Add `StopAgent`, `DeleteAgent`, `ServerStop`, `DeleteTemplate` methods to the interface so the entire surface is uniform. Benefits:

- One env-propagation policy (`scionCmdWithEnv` already centralises this for the methods that use it).
- One stderr-triage policy (see §3).
- Mockable end-to-end for tests, no more PATH-stub-scion gymnastics.

### 3. One stderr-triage helper

We've now hand-written stderr buffering twice (`ImportAllTemplates` in #43, and the planned-but-not-shipped extension to other methods). Extract:

```go
// runScionCmd executes cmd, buffering stderr so callers can suppress
// known-noisy failure modes without losing operator visibility on
// unknown errors. The matchers are substrings; the first one that hits
// causes stderr to be dropped (the wrapped error still describes the
// failure). Unknown failures pass stderr through verbatim.
func runScionCmd(cmd *exec.Cmd, knownNoisy ...string) error {
    var stderrBuf bytes.Buffer
    cmd.Stdout = os.Stdout
    cmd.Stderr = &stderrBuf
    err := cmd.Run()
    stderr := stderrBuf.String()
    if err != nil {
        for _, pat := range knownNoisy {
            if strings.Contains(stderr, pat) {
                return err // suppress; caller wraps with friendly message
            }
        }
        os.Stderr.WriteString(stderr) // unknown: surface verbatim
        return err
    }
    if stderr != "" {
        os.Stderr.WriteString(stderr)
    }
    return nil
}
```

Three caller patterns:
- `runScionCmd(cmd)` — no triage; replaces the raw "set stdout/stderr; run" boilerplate
- `runScionCmd(cmd, "no importable agent definitions")` — suppress one known mode
- Methods that need a friendly error wrap their own message after the helper returns

### 4. Agent worktree cleanup

`darken spawn` creates real git worktrees at `.scion/agents/<name>/workspace` (see the `agent-worktree-discipline` skill). `darken down` currently calls `scion stop` and `scion delete` per agent in `stopProjectAgents`, then `scion clean` removes the entire `.scion/` directory. That deletes the *files* under each worktree but leaves the parent repo's worktree registry pointing at directories that no longer exist — a future `git worktree list` will show orphans, and `git worktree prune` won't run unless something invokes it.

The lifecycle should explicitly own this. Two changes:

- A new teardown-only step that walks `git worktree list --porcelain`, filters to anything under `.scion/agents/`, and calls `git worktree remove --force <path>` for each before `deleteProjectGrove` runs. After the loop, `git worktree prune` for safety.
- The `Step` struct supports teardown-only steps (`Up` is nil-able), so this slots into the same list as everything else:

```go
{Name: "stop project agents",
 Up: nil, Down: stopProjectAgents,    Critical: false},
{Name: "clean agent worktrees",
 Up: nil, Down: cleanAgentWorktrees,  Critical: false},
```

These run in reverse order on `darken down` and are skipped on `darken up`. Keeps the model uniform.

Open question: should worktree cleanup be best-effort (log + continue) or hard-fail? Probably best-effort — an operator who manually `rm -rf`'d a worktree dir shouldn't be blocked from teardown by a stale registry entry.

### 5. Replace bash scripts with native Go

`scripts/stage-creds.sh` and `scripts/stage-skills.sh` are subprocess-of-subprocess layers (darken → bash → scion). Both do work that's clearly Go-shaped:

- **`stage-creds.sh`** — reads `~/.claude/.credentials.json`, `~/.codex/auth.json`, `~/.gemini/oauth_creds.json`, calls `scion hub secret push <name> --from-file <path>`. ~50 lines of Go in `internal/staging/creds.go`.
- **`stage-skills.sh`** — copies skill bundles from `.scion/skills-staging/` into per-role directories. ~80 lines of Go in `internal/staging/skills.go` (some of this already exists; the bash wrapper is now redundant).

This collapses two subprocess hops into in-process Go calls, gives us proper error handling, and removes ~200 lines of bash that's hard to test.

## Migration path

Each phase below is an independent, shippable PR. They do not need to land in this exact order, but each builds context for the next.

| # | PR | Scope |
|---|---|---|
| A | Introduce `Step` abstraction; migrate `runBootstrap` | Pure refactor of `bootstrap.go`. No behavior change. New tests verify forward order matches existing tests. |
| B | Migrate `runDown` to use the same step list | Folds the down-side logic into `lifecycle`. The asymmetry guard becomes "if you add a step, you write its `Down` field at the same time." |
| C | Extract `runScionCmd` helper; route all ScionClient methods through it | Removes duplicated stderr-buffering. Each method gains its own `knownNoisy` list. |
| D | Add `StopAgent`, `DeleteAgent`, `ServerStop`, `DeleteTemplate` to ScionClient; route `down.go`'s remaining raw `exec.Command` paths | Closes the "everything goes through the interface" invariant. |
| E | Add `cleanAgentWorktrees` step to the lifecycle | New teardown-only step. `git worktree list --porcelain` → filter to `.scion/agents/` → `worktree remove --force` per entry → `worktree prune`. Best-effort; log + continue on stale entries. |
| F | Port `stage-creds.sh` to `internal/staging/creds.go` | Behavior-preserving rewrite. Delete the bash file at the end. |
| G | Port `stage-skills.sh` to `internal/staging/skills.go` | Same. |

After G: `scripts/` directory is empty (or close to it), and the embed.FS no longer needs to ship bash bodies.

Estimated total: ~5–6 PRs spread over a week of focused work. Each PR is small enough to review in one sitting (~200–400 LOC including tests).

## What we're deliberately NOT doing

- **Declarative YAML manifest for the lifecycle.** Considered. With 8 steps the structure cost outweighs the configurability benefit, and the `Step` struct already gives us 80% of the readability win.
- **Terraform-style reconciliation engine** (desired-state ↔ actual-state diff loop). Wrong abstraction for a CLI that runs once and exits. Right answer for a daemon, but darken isn't one.
- **Plugin system / hooks** so operators can inject steps. Premature — no operator has asked for this. Add when there's a second project that needs different lifecycle behavior.
- **Replacing bones.** Bones is a parallel layer (cross-machine coordination), not scion-replaceable, not darken-replaceable. Stays as-is.
- **Replacing scion.** Out of scope by definition — scion is upstream at Google.
- **Building a `darken compose` (docker-compose-style) interface.** Considered. Bones is its own daemon, not a docker service; scion sometimes runs containerized but historically doesn't. Compose would fight the existing layering.

## Deferred-to-scion appendix

If we ever talk to the scion maintainers, these are the things that *should* live there but currently can't because we don't control scion. Listed for completeness — no expectation of action.

1. **`scion up` / `scion down`** as composite commands.  
   Current: darken sequences `server start`, `broker provide`, `templates import`, `clean`, etc.  
   Better: `scion up` does all of those for the current grove; darken just calls one command. `scion down` is the inverse.

2. **`scion doctor --json`** for machine-readable preflight.  
   Current: darken has its own `darken doctor` that reimplements docker/PATH/server checks.  
   Better: scion exposes the checks as structured output; darken consumes them and decorates with project-specific concerns.

3. **`scion hub secret push --from-file`** with directory-of-files semantics.  
   Current: `stage-creds.sh` loops over harness types and calls `scion hub secret push` per credential.  
   Better: scion accepts a manifest or directory and stages all of them.

4. **Cobra `SilenceUsage = true`** on runtime failures.  
   Current: scion dumps the full Usage block on errors like "no importable agent definitions" — a user error, not a syntax error. Cobra's default is to print Usage on any non-nil return from `RunE`, which is wrong for runtime failures.  
   Better: scion sets `SilenceUsage = true` and only prints Usage on cobra-detected arg errors.

5. **`scion grove delete <id>`** as an explicit subcommand.  
   Current: `scion clean` is the canonical path, but the discoverability is poor — operators (and orchestration layers like darken) reach for `scion grove delete` first and get a Usage dump. The aliasing or subcommand naming is the real issue.

If you do file these upstream: file 4 first. It's a one-line change in scion that would have prevented #43 entirely on the scion side.

## Out-of-scope, but flagged

- **`bones down` starts a hub mid-teardown.** Not a darken bug, but a confusing log line. The chainBonesDown comment added in #45 documents it. Long-term: ask bones if the hub-spinup can be silent or labeled as "deregistering."
- **`.bones/` and `.scion/init-manifest.json` are runtime artifacts left in the working tree** after `darken up`. Belong in `.gitignore`. Trivial, separate PR.

## Open questions

- Should `lifecycle` be a package-level `var` or a function that returns the list? Function lets us inject test doubles; var is simpler. Lean: var, with a test-only helper that overrides individual steps.
- Should `Step.Critical = false` steps log to stderr on failure (current behavior) or be silent? Today `darken down`'s loop logs "step failed: <err> (continuing best-effort)" for every soft-fail. That's noisy when nothing is actually wrong (e.g. `withdrawBroker` when broker was never provided). Probably worth a `Step.SuppressErrorLog` flag, decided per step.
