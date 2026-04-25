# Orchestrator Skeleton + Single Sub-Harness (Slice 2)

## 1. Purpose & scope

Define the minimum viable orchestrator for the Darkish Factory: one orchestrator process, one runtime adapter, one sub-harness (`tdd-implementer`). The slice validates the end-to-end path **intent → orchestrator → containerized harness → git worktree → merged result**. Specialization, classifier internals, and review gates are stubbed at well-defined seams.

Concretely in scope:

- Orchestrator process lifecycle, persistent state, recovery semantics.
- Runtime adapter abstraction with one interface and pluggable backends (Scion, Docker, Podman, Apple containers, k8s).
- Sub-harness lifecycle primitives: spawn, run, pause, resume, kill, attach, heartbeat.
- Git-worktree-per-harness as the canonical handoff (orchestrator cherry-picks between worktrees).
- Harness configuration as a single declarative artifact (§5.7).
- Append-only audit log indexed by `worktree-ref` and `harness-id`.
- Heartbeat / 10-min timeout (§8 "sub-harness hangs").
- Per-feature spend cap (§8 "token runaway").
- Crash-mid-merge recovery (worktrees intact by construction).
- Integration seams for the escalation classifier (slice 1) and review/merge (slice 4).
- `tdd-implementer` MVP: its config artifact, its worktree, expected outputs.

## 2. Out of scope

- Routing/escalation classifier internals → `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`
- Other specialized harnesses (researcher, planner, verifier, reviewer, etc.) → `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Stacked PRs, surface-area conflict check, Beads coordination → `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md`
- Replay, metrics, cost profiles, drift guard → `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`
- Dark variants → `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md`

## 3. Architecture

```
                       host process
  +--------------------------------------------------+
  |                  Orchestrator                    |
  |  state store · audit log writer · spend meter    |
  |  worktree manager · classifier seam · merge seam |
  +-----------------+--------------------------------+
                    | control plane (Hub)
                    v
  +--------------------------------------------------+
  |                Runtime Adapter                   |
  |  one trait, backends: Scion | Docker | Podman    |
  |              | Apple containers | k8s            |
  +-----------------+--------------------------------+
                    | spawn/exec/heartbeat
                    v
  +--------------------------------------------------+
  |   Harness container (tdd-implementer)            |
  |   bind-mount: <grove>/worktrees/<harness-id>     |
  |   read-only:  <grove>/constitution.md            |
  +--------------------------------------------------+
                    | git ops only
                    v
  +--------------------------------------------------+
  |  Host git repo (Grove)                           |
  |  main + worktrees/<harness-id>/<feature>         |
  +--------------------------------------------------+
                    ^
                    |
  +--------------------------------------------------+
  |  Audit log (append-only, JSONL)                  |
  |  indexed by worktree_ref, harness_id, feature_id |
  +--------------------------------------------------+
```

## 4. Components

### 4.1 Orchestrator core

A single long-running process. Owns: feature registry, harness registry, control loop, audit-log writer handle, spend meter, classifier seam, merge seam. Reconciles desired state (a feature's pipeline graph) against observed state (running harnesses, committed worktree refs). Stateless in memory beyond the audit log and a small state-store snapshot; on cold start, replays uncommitted features from the log.

### 4.2 Runtime adapter

Responsible only for spawning containers. One thin protocol, multiple backends. The orchestrator never imports backend SDKs directly; it calls `spawn(harness_config, worktree)` and receives a `HarnessHandle` (§4.9) that owns every subsequent lifecycle interaction. Backend selection is a Grove-level config (`runtime: scion|docker|podman|apple|k8s`). The conformance test suite (§8) gates new backends.

### 4.9 HarnessHandle

The lifecycle surface returned by `RuntimeAdapter.spawn`. Exposes `pause`, `resume`, `kill`, `attach`, `heartbeat`, `logs`, `exec` as methods on the handle itself, so callers never hold a backend reference and a separate handle at once. The handle is a context manager: `__exit__` is a hard guarantee that the underlying container is killed and backend resources (network namespace, mount, log buffers) are released, even if the orchestrator's control loop raised mid-operation. Any new lifecycle action lands here as a new method, not as a change to the runtime adapter trait.

### 4.3 Harness container

One container per harness instance. Mounts its own worktree read-write; mounts `constitution.md` read-only. Egress restricted by the harness config's tool allowlist (e.g. `tdd-implementer` denies `web_fetch`). Emits structured events to stdout/stderr; the orchestrator captures via the runtime adapter.

### 4.4 Worktree manager

Wraps `git worktree add/remove/list`. Allocates `<grove>/worktrees/<harness-id>/<feature-id>` per spawn. Enforces invariant from §5.5: **no two harnesses write the same worktree**. Cherry-picks commits between worktrees on orchestrator command. Worktrees survive orchestrator crashes by construction (§8 row "orchestrator crashes mid-merge").

### 4.5 Audit log writer

Append-only JSONL, fsync per record. One file per Grove. Every harness lifecycle transition, every emitted decision, every classifier result, every cherry-pick, every spend tick is one record. Records are content-addressed by `(feature_id, harness_id, seq)` and carry the current `worktree_ref` (HEAD of the harness's worktree).

### 4.6 Heartbeat & timeout

The container emits a heartbeat at most every 30 s. The orchestrator marks a harness `stalled` after 10 min without one (§8). On stall, default action is `pause + escalate`; `kill + redispatch` is opt-in per harness config.

### 4.7 Spend cap enforcer

Each feature carries a `spend_cap_usd`. The orchestrator increments a per-feature counter from token usage events emitted by the harness. On exceedance, the orchestrator pauses every harness on that feature and emits an escalation with the spend trace (§8 "token runaway").

### 4.8 Config loader

Reads the harness-config YAML on spawn, validates against schema, hashes the resolved config, and records the hash on the spawn audit-log entry. A harness binds to one config hash for its lifetime.

## 5. Interfaces

### 5.1 Runtime adapter trait

```python
class RuntimeAdapter(Protocol):
    def spawn(self, harness_config: HarnessConfig,
              worktree: WorktreePath) -> HarnessHandle: ...

class HarnessHandle(Protocol):
    # Context-manager surface: __exit__ kills + cleans up the container.
    def __enter__(self) -> "HarnessHandle": ...
    def __exit__(self, exc_type, exc, tb) -> None: ...

    # Lifecycle, owned by the handle (not the adapter).
    def pause(self) -> None: ...
    def resume(self) -> None: ...
    def kill(self, signal: str = "TERM") -> None: ...
    def attach(self) -> AttachStream: ...
    def heartbeat(self) -> Heartbeat | None: ...
    def logs(self, since: str | None) -> Iterator[LogRecord]: ...
    def exec(self, cmd: list[str]) -> ExecResult: ...
```

`RuntimeAdapter` is a one-method protocol: `spawn(harness_config, worktree) -> HarnessHandle`. After spawn the orchestrator interacts only with the returned `HarnessHandle`; backends never see the orchestrator again. The handle is a context manager — its `__exit__` kills the container and releases backend resources, so a `with adapter.spawn(...) as h:` block guarantees cleanup on any exit path. A new lifecycle action (e.g. `checkpoint`) becomes one method on the handle, not a trait-wide change.

The `HarnessConfig` passed to `spawn` carries: image ref, mounts (worktree rw, constitution ro), env, network policy, cpu/mem caps, tool allowlist hash, plus the resolved fields from §5.2. Backends that cannot live-pause may implement `pause`/`resume` as `kill` + redispatch.

### 5.2 Harness config schema

```yaml
harness: tdd-implementer
version: 1
model: claude-opus-4-7
temperature: 0.2
system_prompt_path: ./prompts/tdd-implementer.md
tool_allowlist: [shell, fs_read, fs_write, git, run_tests]
tool_denylist: [web_fetch, web_search]
skills: [test-driven-development, verification-before-completion]
resource_budget:
  wall_clock_min: 60
  spend_cap_usd: 25
  cpu: 2
  mem_gb: 4
hooks:
  pre_spawn: ./hooks/seed-worktree.sh
  post_exit: ./hooks/collect-coverage.sh
heartbeat_sec: 30
stall_action: pause   # or kill_redispatch
```

### 5.3 Orchestrator → harness control protocol

JSON over the runtime adapter's exec channel. Messages: `start_unit`, `pause`, `resume`, `cancel`, `request_state`. Replies: `progress`, `decision_proposed`, `tool_call_intent`, `committed`, `stalled`, `done`, `error`. Every `decision_proposed` is fed to the classifier seam before the harness is allowed to act.

### 5.4 Audit-log entry shape

```yaml
ts: 2026-04-25T12:34:56.789Z
seq: 4711
feature_id: F-2026-0042
harness_id: tdd-impl-3a9c
harness_role: tdd-implementer
config_hash: sha256:...
worktree_ref: refs/worktrees/tdd-impl-3a9c/F-2026-0042@b1e2f3
event: decision_proposed   # spawn|heartbeat|decision_proposed|classifier_result|tool_call|commit|cherry_pick|stall|spend_tick|exit|merge|...
payload: { ... }           # event-specific
spend_usd_cumulative: 3.41
tokens_in_cumulative: 184302
tokens_out_cumulative: 22118
```

### 5.5 Classifier integration seam (slice 1)

```python
class EscalationClassifier(Protocol):
    def classify(self, decision: ProposedDecision,
                 context: DecisionContext) -> ClassifierVerdict: ...
# verdict: ratify | escalate(category, urgency, reasoning) | needs_more_context
```

The orchestrator calls `classify` on every `decision_proposed`. Slice 2 ships a stub returning `ratify` always; slice 1 swaps it in.

### 5.6 Review-gate integration seam (slice 4)

```python
class ReviewGate(Protocol):
    def evaluate(self, feature_id: str,
                 worktree_refs: list[str]) -> ReviewVerdict: ...
# verdict: ship | block(reasons) | needs_human(escalation)
```

Called after final verification, before merge to main. Slice 2 ships a stub that ships unconditionally if tests pass.

## 6. Data model

The audit log is the **single persistent cross-slice schema** for the Darkish Factory. Every other slice (1 escalation classifier, 3 specialized harnesses, 4 review-and-merge, 5 cost mode + drift guard, 6 dark variants) reads from and writes to it; none of them maintains an adjacent persistent schema. Review-queue entries, surface claims, mode-selection records, drift signals, calibration sets, and the worktree state machine are all event types in this one log, not separate tables. Slice 2 owns the writer and the canonical entry shape (§5.4); other slices consume projections (SQL views over the JSONL mirror), they do not redefine the type. The only non-log artifacts are *input configs* — the constitution, the policy file, and the harness-config YAML (§6.2) — none of which are stored in the log because they are author-time inputs, not runtime events.

### 6.1 Audit log

JSONL, append-only, fsync per record. Indexed offline by `feature_id`, `harness_id`, `worktree_ref`. The schema in §5.4 is the source of truth.

### 6.2 Harness config

YAML (§5.2). Stored in the Grove's config repo. Resolved config (after include/inheritance) is hashed; the hash binds to the harness instance.

### 6.3 Worktree state machine (as audit-log event types)

The worktree lifecycle is **not a separate schema**. Each transition below is an `event` value in the unified audit log (§5.4); the "state" of a worktree at any moment is a fold over its event stream filtered by `(feature_id, harness_id)`.

```
allocated -> assigned -> running -> committed -> picked -> retired
                  \-> stalled -> resumed | killed
                  \-> failed
```

Event semantics: `committed` means the harness pushed at least one commit on its worktree branch. `picked` means the orchestrator cherry-picked into a downstream harness's worktree or into the integration branch. Readers (slices 3, 4, 5, 6) reconstruct worktree state by querying the log; nothing else stores it.

## 7. Failure modes & recovery

| Failure | Handled here | Notes |
|---|---|---|
| Sub-harness hangs | Yes | 10-min heartbeat timeout → `pause` (default) or `kill_redispatch`. |
| Orchestrator crashes mid-merge | Yes | Worktrees intact by construction (§5.3). On restart, reconcile from the audit log: any `cherry_pick` without a matching `merge_complete` is replayed from the worktree HEAD. |
| Token runaway | Yes | Per-feature spend cap; pause-and-escalate with spend trace. |
| Semantic merge conflict | Detected here, resolved elsewhere | Detection: cherry-pick fails or post-merge tests fail. Reconciliation logic deferred to slice 4 (review/merge spec). |
| Prompt injection in harness output | Bounded here, mitigated elsewhere | Container isolation bounds blast radius. The summarization step before privileged harnesses sees fetched text is a researcher-harness concern (slice 3). |
| Classifier missed escalation | Out of scope | Handled by slice 1 (5% spot-check, policy update). |

## 8. Testing & verification strategy

- **Runtime-adapter conformance suite.** A fixed set of black-box tests every backend must pass: spawn-hello-exit, exec-roundtrip, pause/resume/kill semantics, heartbeat timing, log capture under load, mount permission enforcement.
- **Single-harness end-to-end test on Docker.** Hand a trivial intent ("add `add(a,b)` with tests") to the orchestrator; assert that `tdd-implementer` spawns, writes a failing test first (verified by inspecting commit order), implements, the orchestrator cherry-picks to the integration branch, and the audit log replays cleanly.
- **Crash-recovery tests.** SIGKILL the orchestrator at: (a) just-spawned, (b) mid-cherry-pick, (c) post-merge-pre-audit-fsync. Assert each restart converges without data loss and without duplicate commits.
- **Audit-log replay sanity.** Load the JSONL into a sqlite mirror; reconstruct the worktree-state machine for every feature; assert no illegal transitions.
- **Spend-cap fault injection.** Force-feed token usage events past the cap; assert pause + escalation entry within one heartbeat.

## 9. Open questions

- **Substrate choice for first build: Scion vs. Docker.** Scion gives Hub/Grove vocabulary, OpenTelemetry, and pause-resume-attach for free, but is explicitly experimental (§4). Docker is boring, ubiquitous, and requires us to implement pause-resume-attach ourselves. Recommendation pending a spike.
- **Orchestrator language.** Python (fastest path; matches the classifier and harness ecosystem) vs. Go (stronger lifecycle/IPC story, single binary, better for long-running daemons). The runtime adapter trait is language-agnostic; commit later.
- **Scion-vocab → non-Scion runtime mapping.** Grove maps cleanly to "the host repo + config dir." Hub maps to the orchestrator process. Harness/Runtime carry over. Open question: is there value in literally re-using Scion's CLI surface for non-Scion backends, or do we keep our own `RuntimeAdapter` trait as the only contract?
- **`claude-progress.txt`-style state vs. the audit log.** The README cites Anthropic's pattern of pairing a progress file with git history. Decision needed: is `claude-progress.txt` (a) per-harness in-worktree state distinct from the audit log, (b) a projection of the audit log written for the harness's own context, or (c) redundant once the audit log exists. Default: (b).
- **Cherry-pick vs. merge for handoffs.** §5.3 says cherry-pick. For a single-harness slice this is trivial, but multi-harness slices may want merge commits to preserve history graphs. Defer.
- **`HarnessHandle` concurrency: sync, async, or both.** A synchronous handle is the simplest contract; an async handle composes naturally with concurrent feature pipelines and with backends whose native SDKs are async (k8s, modern Docker SDKs). Offering both doubles the conformance surface. The choice interacts with the orchestrator language question above — Python lets us ship `async def` methods cheaply, Go would model this with goroutines + channels rather than `async`. Decide alongside the substrate spike.
- **Heartbeat transport.** Stdout JSON line vs. a sidecar socket vs. the runtime adapter's native channel. Driven by the substrate decision above.
- **Where the spend meter lives.** In the orchestrator (authoritative, but lags real spend by one heartbeat) vs. in a tool-wrapper inside the container (real-time, but trusts the container). Default: orchestrator-side, with the container forced to use a metered API client.

## 10. Cross-references

- Escalation classifier (slice 1): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`
- Specialized harnesses (slice 3): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Review and merge (slice 4): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md`
- Cost mode and drift guard (slice 5): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`
- Dark variants (slice 6): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md`
- Source: `/Users/dmestas/projects/darkish-factory/README.md` (§3, §4, §5.1–5.3, §5.5, §5.7, §7, §8)
