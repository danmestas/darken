# Slice 4: Review-and-Merge Surface

Status: design
Date: 2026-04-25
Slice of: Darkish Factory (README §5.6)

## 1. Purpose & scope

Once features run in parallel the bottleneck moves off the human's mid-feature attention and onto the human's review queue. "Ten PRs landing a day is not a throughput win if one human is still reviewing each one." This slice is the structural fix: a reviewer harness that runs before anything reaches the operator, stacked-PR submission, a `coordinate(a, b)` query for inter-feature coordination, and a pre-merge surface-area overlap check at the review queue.

The throughput claim depends on all four; any one of them missing collapses 4x feature throughput to 1x review throughput. This spec defines the mechanics for all four and the operator-side queue that consumes their output.

In scope:
- `reviewer` harness mechanics (constitution enforcement, suite execution, regression check against the audit log, classifier-trigger flagging, block/ship/escalate disposition).
- Stacked-PR submission (GitHub native or Graphite), planner-emitted stack shape, partial rejection semantics.
- `coordinate(a, b)` function for cross-feature coordination, computed at query time over the unified audit log (Slice 2), the stacked-PR submitter's stack-state queries, and the orchestrator's runtime state.
- Pre-merge surface-area diff overlap detector at the queue.
- Operator-side review queue UX, ordering, batching, conflict reconciliation.

## 2. Out of scope

Linked sibling specs:

- Escalation classifier internals: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`
- Orchestrator core, worktrees, runtime adapter, audit-log writer: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`
- Other harness configs (researcher, planner, etc.) and pipeline DAG: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Per-harness metrics, replay, drift guard, cost modes: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`
- Dark variants: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md`

## 3. Architecture

```
   features in flight (parallel)
   feat-A    feat-B    feat-C    feat-D
     |         |         |         |
     v         v         v         v
  +------------------------------------+
  |     verifier output per feature    |
  +------------------------------------+
                    |
                    v
  +------------------------------------+
  |     reviewer harness (per feat.)   |
  |  - constitution check              |
  |  - suite execution                 |
  |  - regression vs audit log         |
  |  - escalation-classifier triggers  |
  |  - surface-area overlap probe      |
  +------------------------------------+
       |              |             |
       v              v             v
   block/rework   auto-ratify   escalate
       |              |             |
       |              v             v
       |       stacked-PR       operator review queue
       |       submitter        (batched, ordered)
       |          |                  |
       |          v                  v
       |   GitHub / Graphite    one-keystroke
       |     stack of PRs       ratify/reject
       |
       v
    rework loop (back to implementer / planner)

   coordination: when scheduling or merging features, the orchestrator
   calls coordinate(a, b) -> ConflictReport | None, which queries the
   unified audit log (Slice 2), the stacked-PR submitter's stack-state,
   and the orchestrator's own runtime state at query time. No shared
   graph storage.
```

The reviewer harness sits in front of the operator's queue. Inter-feature coordination is a query (`coordinate`), not a stored graph: at scheduling and merge time the orchestrator joins the unified audit log, stacked-PR submitter state, and runtime state on demand. The surface-area detector reads diffs from in-flight feature worktrees and emits surface-claim events into the unified audit log defined in Slice 2.

## 4. Components

### 4.1 Reviewer harness

A containerized harness with senior-engineer disposition (§5.1). Tool allowlist: shell (test runner), filesystem read on the feature worktree, audit-log read/write (for reviewer-disposition events), classifier-trigger emitter. No web. No write to other worktrees.

Inputs (from planner/verifier handoff):
- feature ID, worktree ref, plan, completed unit list, verifier report, intent string.

Pipeline (run in order; first failure short-circuits):
1. **Constitution gate.** Load `constitution.md` for the Grove. For every rule, run a structured check (regex, AST, lint, or LLM-graded for semantic rules). Any violation is a hard block. The reviewer never overrides the constitution; it only enforces it.
2. **Suite execution.** Run the full test suite in the feature worktree. Failure is a hard block.
3. **Regression check.** For every file the diff touches, query the audit log for prior decisions on the same path. Surface decisions older than the diff that the diff appears to contradict (renamed symbol, deleted abstraction, reverted policy). Contradictions go to the classifier as architecture or taste triggers.
4. **Classifier sweep.** Run the escalation classifier (slice 1) against the diff and the planner's decision log. Anything the classifier flags goes to the operator queue with the classifier's reasoning attached.
5. **Surface-area probe.** Compute the diff's surface area (see 4.4) and compare against in-flight peers. Overlap above threshold triggers the conflict-reconciliation flow.

Disposition (one of):
- `ship` — auto-ratify, hand to stacked-PR submitter.
- `block-rework` — return to the implementer with a structured rework directive.
- `block-abort` — return to the planner; the unit was structurally wrong.
- `escalate` — surface to the operator queue.

The reviewer's own outputs are written to the audit log so its decisions can be replayed and spot-checked the same way the classifier's are.

### 4.2 Stacked-PR submitter

The orchestrator submits each feature as a stack rather than one PR. The planner emits plans whose units are naturally stackable: each unit is a self-consistent, separately-reviewable diff, in dependency order. The submitter has two backends.

- **GitHub native.** One branch per stack level, each PR targeting the level below. The top of the stack targets `main`.
- **Graphite.** `gt submit --stack` creates the chain and reorders on rebase.

Either backend is selected per Grove via configuration. The submitter is the only component that talks to the chosen backend.

Stack lifecycle:
- **Create.** On reviewer `ship`, submitter creates levels 1..N for the unit list.
- **Update.** When an upstream level merges, lower levels rebase. Submitter detects and triggers rebase; conflicts in rebase escalate.
- **Partial rejection.** Operator rejects level K. Levels 1..K-1 are preserved and remain reviewable. Levels K..N are returned to the reviewer with the rejection reason as a rework directive.
- **Abandon.** Hard reject of level 1: full stack closed; feature returns to the planner.

The submitter writes each level's PR URL and merge state as event types in the unified audit log (Slice 2) so other features can see what has landed; `coordinate()` queries these events at scheduling and merge time. The submitter does not maintain adjacent storage.

### 4.3 `coordinate(a, b)` function

Inter-feature coordination is a query, not a stored graph. The orchestrator calls `coordinate(feature_a, feature_b) -> ConflictReport | None` when scheduling a new feature against the in-flight set and when a stack reaches merge. The function joins three sources at query time: the unified audit log (Slice 2 — pipeline phase, surface-claim events, reviewer dispositions, operator decisions), the stacked-PR submitter's stack-state queries (PR levels, merge state, rebase status), and the orchestrator's runtime state (live worktrees, in-progress harnesses). It returns `None` when the features are independent, or a `ConflictReport` describing the overlap, the blocking direction, and any stack relationship — the same payload the planner uses to wait, rebase onto a peer's stack tip, or trigger conflict reconciliation (§4.6). No SQLite mirror, no separate node/edge schema; the README's "two features negotiate order through shared state" is satisfied by the audit log being that shared state.

### 4.4 Surface-area overlap detector

Runs at the review queue, not at merge time. Detects semantic overlap between in-flight features before the operator sees any of them.

Inputs: the candidate feature's diff plus every in-flight feature's diff.

Surface-area representation: a vector of `(path, span, kind)` triples, where:
- `path` is the file.
- `span` is a structural unit (function, class, top-level config block, schema migration, route handler) extracted by an AST/LSP pass — not raw line ranges, since line numbers shift under rebase.
- `kind` is `read | write | rename | delete | new`.

Overlap score between feature X and Y is the Jaccard of their write-or-stronger spans, weighted by how destructive the overlap kind is (delete and rename count more than write).

Threshold: per-Grove configurable; defaults to 0.15 for any pairwise overlap, with hard cuts at delete-on-delete or rename-on-write. Crossing the threshold:
1. The reviewer holds both features out of the queue.
2. The orchestrator emits a conflict-reconciliation escalation (§4.6) with both diffs and reconciliation options.

The detector's persisted state — surface-claim spans per feature, overlap scores, threshold crossings — is written as event types in the unified audit log (Slice 2), not in adjacent storage. `coordinate()` reads these events when joining sources.

Calibration: false-positive rate is tuned against a labeled fixture of historical merges; the threshold is the lowest value where precision stays above some Grove-defined minimum, recall preferred over precision (matches the classifier's calibration philosophy in §6.2).

### 4.5 Operator review queue

What the operator sees. CLI surface, presented at the orchestrator prompt; designed for one-keystroke disposition.

Each entry contains:
- Feature ID, intent, stack level, reviewer reasoning, classifier triggers (if any), surface-area peers (if any), one-line summary, recommended disposition.
- A diff link (the stacked PR URL) and an inline summarized diff for trivial entries.

Ordering (top to bottom):
1. **High-urgency** classifier escalations (auth, schema migration, ethics — anything `urgency: high` per §6.3).
2. **Conflict reconciliations** (two features need a single decision).
3. **Stack roots** (level 1 of each stack) before stack tips, so merging the root unblocks the tip.
4. **Older first** within tier (FIFO under the latency bound).

Batching follows §6.3: low and medium urgency entries batch up to `batch_size: 5` or `max_queue_latency_min: 30`, whichever comes first. High urgency bypasses batching.

Operator inputs map to §6.3's `ratify | choose <option> | rework <direction> | abort`. One keystroke per yes/no entry. The orchestrator confirms free-text interpretation before resuming.

### 4.6 Conflict reconciliation flow

Triggered by the surface-area detector or by a rebase conflict in the stacked-PR submitter. Per README §8: "escalate with both diffs and reconciliation options."

Reconciliation options the orchestrator generates and offers:

- `serialize: A then B` — feature A merges first; B rebases on A's tip, replans the overlapping units.
- `serialize: B then A` — symmetric.
- `merge-into-one` — collapse both features into a single feature, replan jointly. Used when overlap is large enough that splitting them is artificial.
- `extract-shared` — create a third feature C that contains the shared surface; both A and B depend on C.
- `abort: A` / `abort: B` — one feature is wrong or duplicative; drop it.

The escalation carries both diffs, the surface-area overlap report, and the orchestrator's recommendation. The operator picks one. Choice is written to the graph as `blocked-on` edges and as a planner directive.

## 5. Interfaces

### 5.1 Planner ↔ reviewer handoff

YAML, committed to the feature worktree:

```yaml
feature_id: feat-2026-04-25-add-rate-limit
worktree_ref: feat/add-rate-limit
plan_ref: plans/feat-2026-04-25-add-rate-limit.md
units:
  - id: 1
    paths: [services/api/rate.py, services/api/rate_test.py]
    stackable: true
verifier_report: artifacts/verify/feat-2026-04-25-add-rate-limit.json
classifier_log: artifacts/classifier/feat-2026-04-25-add-rate-limit.jsonl
```

### 5.2 Reviewer ↔ classifier integration

The reviewer surfaces only what the classifier (slice 1) cannot auto-ratify. The reviewer calls the classifier as a library; the classifier returns `ratify | escalate` per decision with reasoning. The reviewer aggregates the classifier's escalations into the review-queue entry and never second-guesses an auto-ratify.

### 5.3 Stack submission API

Backend trait:

```
submit_stack(feature_id, levels: [{branch, base, title, body, diff}]) -> [PR_URL]
update_stack(feature_id, level_idx, new_diff) -> PR_URL
rebase_stack(feature_id, onto: ref) -> [conflict] | ok
close_stack(feature_id, reason) -> ok
```

GitHub and Graphite each implement this trait.

### 5.4 `coordinate(a, b)` interface

```
coordinate(feature_a: FeatureId, feature_b: FeatureId) -> ConflictReport | None
```

Pure read function. Joins the unified audit log (Slice 2 — pipeline phase events, surface-claim events, reviewer dispositions, operator decisions), the stacked-PR submitter's stack-state queries (level branches, merge state, rebase status), and the orchestrator's in-memory runtime state (live worktrees, in-progress harnesses) at call time. Returns `None` for independent features. A `ConflictReport` carries: the overlap span set, the blocking direction (if any), the stack relationship (if any), and whichever feature is further along the pipeline. Callers: the orchestrator's scheduler (when admitting a new feature into the in-flight set) and the merge gate (before submitting or rebasing a stack). No persistent state of its own; cross-ref Slice 2 for the canonical audit-log schema.

### 5.5 Surface-area diff representation

JSON, emitted as a surface-claim event into the unified audit log (Slice 2) and consumed by the detector and `coordinate()`:

```json
{
  "feature_id": "feat-...",
  "spans": [
    {"path": "services/api/rate.py", "span": "fn:limit", "kind": "write"},
    {"path": "db/migrations/0042_add_quota.sql", "span": "migration", "kind": "new"}
  ]
}
```

## 6. Data model

Feature relationships are derived from the unified audit log (Slice 2) and the stacked-PR backend's stack-state queries — no separate graph storage. The shapes below are event types written to that audit log and queries computed over it; the canonical audit-log schema lives in Slice 2 §5.4.

### 6.1 Review-queue entry (audit-log event)

A `review_queue_entry` event in the unified audit log:

```yaml
entry_id: q-...
feature_id: feat-...
stack_level: 1
intent: "Add rate-limit middleware to /v1/api"
reviewer_reasoning: "Constitution OK, suite green, no audit-log contradiction"
classifier_triggers: [public_api_naming]
surface_peers: [feat-other]   # computed from coordinate() at emission time
recommendation: ratify
diff_url: "https://github.com/.../pull/123"
urgency: medium
created_at: 2026-04-25T14:02:11Z
```

### 6.2 Stack-state (query over the stacked-PR backend + audit log)

The stacked-PR submitter is the source of truth for live PR/branch state; durable transitions are also emitted as `stack_state` events to the audit log. There is no adjacent stack-state table. A query returns:

```yaml
feature_id: feat-...
backend: graphite
levels:
  - idx: 1
    branch: feat/add-rate-limit/01-skeleton
    base: main
    pr_url: ...
    state: open | merged | closed | rejected
created_at: ...
last_rebase_at: ...
```

### 6.3 Conflict-reconciliation option set (audit-log event)

A `conflict_reconciliation` event in the unified audit log, emitted by the orchestrator when `coordinate()` reports an overlap above threshold:

```yaml
conflict_id: c-...
features: [feat-A, feat-B]
overlap_score: 0.42
options:
  - id: serialize_A_then_B
  - id: serialize_B_then_A
  - id: merge_into_one
  - id: extract_shared
  - id: abort_A
  - id: abort_B
recommendation: extract_shared
diffs: [artifacts/diff/feat-A.patch, artifacts/diff/feat-B.patch]
```

## 7. Operator review-queue UX

See §4.5. Headline properties:

- One screen per batch. The operator sees up to `batch_size` entries with one-line summaries and disposition keys.
- Stack roots before tips, conflict reconciliations before single-feature entries, high-urgency above all.
- Single keystroke for ratify/reject on yes/no entries; multi-choice for conflict reconciliations and architecture decisions; free-text answers re-prompt for confirmation.
- Latency bound: `max_queue_latency_min: 30` from §6.3 caps how long any low-urgency entry sits.
- The operator's inputs go straight to the orchestrator and through to the affected harnesses; no other path interrupts the human.

## 8. Failure modes & recovery

| Failure | Detection | Recovery |
|---|---|---|
| Semantic merge conflict across features | Surface-area detector at queue | Conflict reconciliation flow with both diffs and options (§4.6). |
| Stale stack after upstream merge | Submitter on rebase | Auto-rebase; conflicts escalate as classifier `architecture` triggers. |
| Runaway PR count | Per-Grove cap on open PRs | Pause planner intake for new features until queue drains; surface queue depth in the operator prompt. |
| Reviewer harness false-positive blocking | 5% spot-check of blocks (mirrors §6.2 calibration) | Loosen specific check; reset reviewer config to last-good. |
| Stale surface claim (feature aborted but claim event still latest) | `coordinate()` includes a recency check; orchestrator reaper emits a `surface_claim_retracted` audit-log event when a worktree is abandoned | Subsequent `coordinate()` calls ignore retracted claims; if retraction is missed, hold queue and escalate. |
| Reviewer disagrees with verifier | Audit log compares verdicts | Reviewer block wins; escalate as classifier architecture trigger. |

## 9. Testing & verification strategy

- **Reviewer regression set.** A labeled fixture of historical PRs, each annotated `should-block | should-ship | should-escalate`. The reviewer harness is rerun against the set on every config change; drop in precision/recall is a regression. Same shape as the classifier's calibration set so they share infrastructure.
- **Stacked-PR end-to-end fixture.** A scripted multi-unit feature whose plan emits a four-level stack. The fixture exercises create, update, partial-rejection-at-level-3, rebase-after-level-1-merges, and full close. Run on both GitHub and Graphite backends.
- **Surface-area detector calibration.** Historical merges replayed. Score Jaccard threshold against true-positive overlap (an actual semantic conflict) vs false-positive (overlap that didn't conflict). Tune for recall over precision.
- **Conflict-reconciliation playback.** Recorded operator choices on past conflicts replayed against the current option-generator; mismatch flags either a regressed option set or a real reconciliation we'd now choose differently.
- **`coordinate()` tests.** Synthetic audit-log + stack-state fixtures cover: independent features (returns `None`), overlapping surface above threshold, stack-blocked features, and stale claims after retraction events. Each scenario asserts the join across audit log, submitter, and runtime state returns the correct `ConflictReport`.

## 10. Open questions

- **Semantic merge conflicts at scale.** README §13: "the current answer is 'escalate'... probably insufficient once feature concurrency exceeds the operator's review capacity even with §5.6 mitigations." The conflict reconciliation flow in §4.6 still routes through the operator. Beyond a certain feature concurrency the operator becomes the bottleneck again. Open: at what concurrency does it bind, and what (if anything) the orchestrator can auto-resolve without operator input — possibly a principal-agent (§11.2) for reconciliation decisions, but that is downstream of having enough audit-log volume.
- **Surface-area span granularity.** Function-level vs block-level vs file-level. Too coarse misses real overlap; too fine produces noise. Likely needs per-language tuning and an LSP pass that this slice does not specify.
- **Stack rebase semantics under rejection.** When level K is rejected and the operator says `rework <direction>`, do levels K+1..N get held or also reworked? Holding risks staleness; reworking risks invalidating already-approved levels. Probably config per Grove; default unset.
- **Backend selection.** GitHub native stacked PRs vs Graphite. Graphite is sharper but adds a tool dependency; GitHub native is rougher but stays in the substrate. We define the trait but punt the default.
- **Reviewer-harness self-tuning.** Like the classifier, the reviewer is replayable. Whether it tunes against operator overrides (and risks drift in lockstep with the classifier) or stays anchored is the same question §9 raises for harnesses generally.
- **Cost of the surface-area pass.** AST/LSP per-feature, every queue tick, on diffs that may be large. Wall-clock and token cost are unmeasured.
- **`coordinate()` cost at high feature concurrency.** The function joins three sources at every call. At low concurrency the join is trivial; at high concurrency (dozens of in-flight features, audit log scaled accordingly) the per-call cost is unmeasured, and it is open whether `coordinate()` needs a memoized cache keyed on `(audit_log_high_water, stack_state_version)` or whether the join stays cheap enough to recompute on every scheduling and merge event.

## 11. Cross-references

- README sections of record: `/Users/dmestas/projects/darkish-factory/README.md` §§5.1, 5.4, 5.5, 5.6, 6, 6.3, 7, 8, 12, 13.
- Slice 1 — Escalation classifier: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`
- Slice 2 — Orchestrator skeleton: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`
- Slice 3 — Specialized harnesses: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Slice 5 — Cost mode and drift guard: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`
- Slice 6 — Dark variants: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md`
- Prior art: Beads (Yegge); GitHub spec-kit; Graphite stacked PRs; Faros AI 47% PR throughput; DORA 2025 review-time and PR-size deltas.
