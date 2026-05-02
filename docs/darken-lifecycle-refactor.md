# Darken lifecycle refactor

A plan to simplify `darken up` / `darken down` without changing scion (which lives upstream at Google and we don't control). All work stays inside this repo. The goal is to remove the bug-classes that produced PRs #43 and #45, eliminate the asymmetry that made #45 possible at all, and shrink the surprise area when an underlying tool (scion, bones, docker) misbehaves.

## Problem

`darken up` is 8 hand-rolled steps; `darken down` is 4 hand-rolled steps; almost every step shells out to scion or a bash script and forwards stdout/stderr unfiltered. Three bug-classes have surfaced in the last two weeks, all sharing the same root cause:

1. **Stderr forwarding noise.** `scion templates import --all <empty-dir>` (#43) and `scion grove delete -y` (#45, the latter doesn't even exist) both dumped cobra's full Usage block to stderr because darken just forwarded scion's output. We fixed each one in isolation; the next one will look the same.

2. **Up/down asymmetry.** `darken up` calls `BrokerProvide()` at step [4/8]. `darken down` had no symmetric `BrokerWithdraw()` until #45 added one. The forgetting was structural — the up and down code paths are independent slices of `func() error` with no enforced pairing.

3. **Layering opacity.** `bones down --yes` legitimately starts a hub mid-teardown to deregister state. The user reasonably read that as a contradiction, because darken's output makes no distinction between "darken's intent" and "what bones decided to do." Comments help; structure helps more.

The unifying observation: **darken is a thin orchestration layer that doesn't model what it's orchestrating**. It calls commands and forwards output. When a command misbehaves, the operator sees raw shell-y output. When a step is missing its symmetric pair, nothing notices.

## Target shape

### 1. `Resource` interface — model the things, not the steps

Replace `[]func() error` (and the planned `Step` struct) with a small interface representing each piece of state darken manages:

```go
type Resource interface {
    Name() string
    Ensure() error  // idempotent: bring this resource to ready state
    Release() error // idempotent: bring this resource to clean state
}
```

Each managed thing — broker registration, scion server, hub secrets, agent worktrees, substrate — becomes a struct implementing this interface. **Both lifecycle directions are interface methods**, so the bug class from #45 (forgot the symmetric pair) becomes a *compile error* rather than a runtime gap. You cannot add a resource without writing both directions.

```go
type GroveBroker struct{}
func (GroveBroker) Name() string    { return "broker provided to grove" }
func (GroveBroker) Ensure() error   { return defaultScionClient.BrokerProvide() }
func (GroveBroker) Release() error  { return defaultScionClient.BrokerWithdraw() }

type ScionServer struct{}
func (ScionServer) Name() string    { return "scion server running" }
func (ScionServer) Ensure() error {
    if _, err := defaultScionClient.ServerStatus(); err == nil {
        return nil // already running; no-op
    }
    return scionCmdWithEnv([]string{"server", "start"}).Run()
}
func (ScionServer) Release() error  { return nil /* leave running between projects */ }
```

For down-only concerns (stop agents, clean worktrees), `Ensure()` is a no-op and `Release()` does the work. The interface stays uniform; the code is honest about what each resource actually does:

```go
type ProjectAgents struct{}
func (ProjectAgents) Name() string   { return "project agents" }
func (ProjectAgents) Ensure() error  { return nil /* spawned via darken spawn, not up */ }
func (ProjectAgents) Release() error { return stopAndDeleteAllAgentsInGrove() }
```

The lifecycle is then a single declared slice of `Resource`:

```go
var lifecycle = []Resource{
    DockerDaemon{},   // up: check reachable;     down: no-op
    ScionCLI{},       // up: check on PATH;       down: no-op
    ScionServer{},    // up: start if not;         down: no-op (leave running)
    GroveBroker{},    // up: provide;              down: withdraw
    DarkenImages{},   // up: make per-backend;     down: no-op (keep cache)
    HubSecrets{},     // up: stage from $HOME;     down: no-op
    Substrate{},      // up: stage skills+import;  down: no-op (clean covers it)
    ProjectAgents{},  // up: no-op;                down: stop + delete each
    AgentWorktrees{}, // up: no-op;                down: git worktree remove + prune
    Grove{},          // up: ensure registered;    down: scion clean
}
```

`Up()` walks forward calling `Ensure()`. `Down()` walks reverse calling `Release()`. Each resource implementation is a *deep module* — caller sees three methods; implementation hides scion-command choice, idempotency check, error wrapping, and any per-resource state.

Why this is deeper than a `Step` struct of function pointers: a Step is a tuple — depth still lives in the closures. A Resource is a noun the system manages. Each implementation can carry its own state (e.g. `DarkenImages{Backends: []string{"claude","codex","pi","gemini"}}`) without leaking it through the interface.

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

### 4. Agent worktree cleanup as a Resource

`darken spawn` creates real git worktrees at `.scion/agents/<name>/workspace` (see the `agent-worktree-discipline` skill). `darken down` currently calls `scion stop` and `scion delete` per agent in `stopProjectAgents`, then `scion clean` removes the entire `.scion/` directory. That deletes the *files* under each worktree but leaves the parent repo's worktree registry pointing at directories that no longer exist — a future `git worktree list` will show orphans, and `git worktree prune` won't run unless something invokes it.

In the Resource model this is just one more entry in `lifecycle`:

```go
type AgentWorktrees struct{}
func (AgentWorktrees) Name() string  { return "agent worktrees" }
func (AgentWorktrees) Ensure() error { return nil /* spawn manages creation */ }
func (AgentWorktrees) Release() error {
    paths, err := listWorktreesUnder(".scion/agents/")
    if err != nil { return err }
    for _, p := range paths {
        _ = exec.Command("git", "worktree", "remove", "--force", p).Run()
    }
    return exec.Command("git", "worktree", "prune").Run()
}
```

Best-effort by design: an operator who manually `rm -rf`'d a worktree dir shouldn't be blocked from teardown by a stale registry entry. The walker logs and continues.

### 5. Bash scripts disappear *into* resources

`scripts/stage-creds.sh` and `scripts/stage-skills.sh` are subprocess-of-subprocess layers (darken → bash → scion). With the Resource pattern there's no separate "port the bash" phase — they just become the body of the relevant `Ensure()`:

- **`stage-creds.sh`** → `HubSecrets.Ensure()`. Walk known credential files in `$HOME`, call `defaultScionClient.PushSecret(name, path)` for each. ~30 lines of Go.
- **`stage-skills.sh`** → folded into `Substrate.Ensure()`. Walk role manifests, resolve modes, copy skill bundles into per-role staging dirs, then call `defaultScionClient.ImportAllTemplates(root)`. ~80 lines of Go (some already exists in `internal/staging/`; the bash wrapper becomes redundant).

This collapses two subprocess hops into in-process Go calls, gives us proper error handling, and removes ~200 lines of bash that's hard to test. The `runSubstrateScript` machinery and the embed.FS bash bodies can be deleted entirely.

### 6. `darken doctor` becomes a third traversal

The Resource list models the world. Three commands are three traversals of the same list:

| Command | Traversal | Per-resource action |
|---|---|---|
| `darken up`     | forward  | `Ensure()` |
| `darken down`   | reverse  | `Release()` |
| `darken doctor` | forward  | `Observe()` (read-only state report) |

For `Observe()` to work without bloating the interface, extend `Resource` with one optional method via an embedded type-assertion check:

```go
type Observer interface {
    Resource
    Observe() (status, detail string)  // pretty-printable state
}
```

Resources that can cheaply report state implement `Observer`; ones that can't are reported as "(no observer)" in doctor output. This is opt-in — no impact on the core 3-method `Resource` interface.

The win: `darken doctor`, `darken up`, and `darken down` become three views of the same world model. Adding a new resource means it shows up in all three commands automatically. The current code has separate per-command logic that drifts.

## Migration path

Each phase below is an independent, shippable PR. They do not need to land in this exact order, but each builds context for the next.

| # | PR | Scope |
|---|---|---|
| A | Define `Resource` interface + `Up()`/`Down()` walkers; port two simplest resources (`DockerDaemon`, `ScionCLI`) | Establishes the pattern, validates the migration ergonomics. New tests target each resource in isolation. |
| B | Port up-side resources: `ScionServer`, `GroveBroker`, `DarkenImages`, `HubSecrets`, `Substrate` | Bulk of the work. Each lives in `internal/resources/<name>.go`. `runBootstrap` shrinks to a 5-line walker. |
| C | Port down-only resources: `ProjectAgents`, `AgentWorktrees`, `Grove` | These have no-op `Ensure()`. `runDown` shrinks to a 5-line reverse walker. The forgotten-pair bug class is now structurally impossible. |
| D | Extract `runScionCmd` helper; route all `ScionClient` methods through it | Removes duplicated stderr-buffering. Each method declares its own `knownNoisy` list. |
| E | Add `StopAgent`, `DeleteAgent`, `ServerStop`, `DeleteTemplate`, `PushSecret` to `ScionClient`; convert remaining raw `exec.Command` paths | Closes the "everything goes through the interface" invariant. Resources route exclusively through the interface. |
| F | Native Go for `stage-creds.sh` host-side calls (folded into `HubSecrets.Ensure`, `runCreds`, `runSpawn`); bash remains for container-side `spawn.sh` and operator-facing `darken creds` invocations the embed still ships | Remove host-side subprocess hop. Full deletion of the bash file is blocked on the container-side spawn.sh refactor — out of scope for this PR. |
| G | Native Go for `stage-skills.sh` rebuild mode (folded into `Substrate.Ensure` and `stageSkillsForRole`); bash remains for `darken skills add/remove/diff` operator commands and container-side use | Same trade as Phase F — host-side subprocess hop gone, full deletion deferred. |
| H | Add `Observer` interface; ship `lifecycleObservations()` helper and Observe() impls on 4 resources (Docker, ScionCLI, ScionServer, Grove). `darken doctor` consolidation deferred to a follow-up PR — the existing DoctorCheck registry has severity/remediation/ID features that need to merge into the Observer model first | Closes the design loop and proves Observer is workable; the doctor consolidation is mechanical polish. |

After H: host-side orchestration code shrinks ~50%. `darken up`/`darken down` are two traversals of `lifecycle`; `darken doctor` is positioned to become the third in a follow-up.

This refactor ships in one PR (the operator's "specs ship with implementation" rule), as a sequence of focused commits A→H. Each commit is small enough to review in one sitting (~200–400 LOC including tests).

## What's actually deleted vs deferred

This PR removes:
- The hand-rolled `[]struct{name, fn}` step lists in `runBootstrap` and `runDown`
- 5 standalone `ensure*` functions (logic moved into Resource methods)
- 3 standalone down-side functions (logic moved into Resource methods)
- All raw `exec.Command` paths for scion in `cmd/darken/` (everything routes through `ScionClient`)
- ~125 lines of bash credential-staging logic from the host invocation path
- ~250 lines of bash skill-staging logic from the host invocation path

This PR does *not* delete:
- `scripts/stage-creds.sh` and `scripts/stage-skills.sh` files — the embedded copies are still consumed by container-side `spawn.sh` (out of scope).
- The DoctorCheck registry in `doctor.go` — full unification is a follow-up that needs to merge severity/remediation/ID semantics into the Observer model.

## What we're deliberately NOT doing

- **Declarative YAML manifest for the lifecycle.** Considered. With ~10 resources the structure cost outweighs the configurability benefit, and the `Resource` interface already gives us symmetry-by-construction in plain Go.
- **A `Step` struct of paired function pointers** (the first-draft of this doc). Considered and rejected: a tuple is shallower than an interface. `Step{Up: f, Down: nil}` compiles fine, leaving the asymmetry-bug class prevented only by convention. The Resource interface makes "forgot the down side" a type error.
- **Terraform-style reconciliation engine** (desired-state ↔ actual-state diff loop). Wrong abstraction for a CLI that runs once and exits. Right answer for a daemon, but darken isn't one. The Resource pattern gets most of reconciliation's clarity (idempotent Ensure/Release) without the planning machinery.
- **Plugin system / hooks** so operators can inject resources. Premature — no operator has asked for this. Add when there's a second project that needs different lifecycle behavior.
- **`Resource.DependsOn() []Resource`** for explicit DAG ordering. Premature — slice order encodes the topology fine for ~10 resources. Add if the list grows past ~20 or if resources start being conditionally enabled.
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

- Should `lifecycle` be a package-level `var` or a function that returns the list? Function lets us inject test doubles; var is simpler. Lean: var, with a test-only helper that overrides individual entries by type.
- Where do "criticality" and "soft-fail logging" live? In the `Step` draft these were struct fields. With `Resource` they belong inside the implementation: a resource decides for itself whether `Ensure()` returning an error should abort up, and whether `Release()` failures should log. The walker just propagates errors; the resource decides what to return. This pulls a bit more complexity into each resource but removes a config field from the interface.
- Should `Resource.Ensure()` always be safe to call (no-op when already ready), or should we add a separate `Status() State` method and let the walker decide? Lean: keep `Ensure()` self-idempotent. The walker stays trivial. Doctor uses `Observer` for read-only state.
- Naming: `Ensure`/`Release` vs `Up`/`Down` vs `Acquire`/`Release` vs `Start`/`Stop`. Picked `Ensure`/`Release` because (a) `Up`/`Down` collides with `Project.Up()`/`Project.Down()` walkers, (b) `Acquire` implies locking, (c) `Start`/`Stop` implies a process. `Ensure`/`Release` are direction-symmetric and idempotent-flavored.
