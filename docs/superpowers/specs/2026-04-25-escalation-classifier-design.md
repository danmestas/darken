# Slice 1: Constitution + Hybrid Escalation Classifier

Status: design
Date: 2026-04-25
Scope: a standalone, harness-agnostic library. No orchestrator dependency. Callable from any sub-harness, the orchestrator, or a test rig.

## 1. Purpose & scope

A library any caller invokes to ask two questions about a proposed agent decision:

1. Should this escalate to the operator? (hybrid: Stage 1 deterministic, Stage 2 adversarial LLM)
2. Is this work light, heavy, or ambiguous? (routing classifier; ambiguous routes heavy)

The library owns: the constitution loader, the YAML policy file, the deterministic tool-wrapper gate, the separate-call LLM classifier, the routing classifier, the `RequestHumanInput` data model, batching with high-urgency bypass, the 5% spot-check sampler, override capture, and the events it writes to the unified audit log defined in Slice 2.

Categories the classifier reasons over are the four axes from §2 of the README: taste, architecture, ethics, reversibility. Reversibility is enforced deterministically; the other three go through the LLM stage.

Calibration optimizes recall over precision: missing a real escalation is worse than an unnecessary one.

## 2. Out of scope

- Orchestrator skeleton, sub-harness lifecycles, runtime adapters: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`
- Specialized harness configurations: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Reviewer harness, stacked PRs, merge surface: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md`
- Per-harness metrics, replay, cost profiles, drift guard: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`
- Dark variants, principal-agent: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md`

## 3. Architecture

```
                     ┌──────────── caller (harness | orchestrator | test rig) ──────────┐
                     │                                                                  │
proposed_decision ──▶│  classify_decision(decision, ctx)                                │
                     │       │                                                          │
                     │       ├─▶ Stage 1: Deterministic gate (tool-wrapper interceptor) │
                     │       │     reversibility triggers ─▶ ESCALATE (no LLM)          │
                     │       │                                                          │
                     │       └─▶ Stage 2: LLM classifier (separate model call,          │
                     │             adversarial system prompt) ─▶ taste|arch|ethics      │
                     │                                                                  │
routing_inputs ─────▶│  classify_routing(rubric_inputs) ─▶ light | heavy | ambiguous    │
                     │                                                                  │
constitution.md ────▶│  load_constitution()  →  authoritative; conflict ⇒ ESCALATE      │
policy.yaml ────────▶│  load_policy()        →  triggers, thresholds, batching         │
                     │                                                                  │
                     │  Batcher (internal)  ─▶  high-urgency bypass | size/latency      │
                     │  Spot-check sampler (5%) (internal)                              │
                     │  Override capture (training signal) (internal)                   │
                     │  Audit-log writer → unified log defined in Slice 2               │
                     └──────────────────────────────────────────────────────────────────┘
```

The library is a pure dependency: input is a structured decision plus pointers to the constitution and policy file; output is a verdict plus, when escalating, a `RequestHumanInput` payload. It does not own a runloop. Batching is in-process queue state the caller drains.

## 4. Components

### 4.1 Deterministic gate (Stage 1) — internal

Enforced at the tool-wrapper level, not in prompts. The library installs an internal `wrap_tool(tool, policy)` wrapper, invoked as a precondition of `decide`; it is not part of the public surface. Before the wrapped tool executes, the gate inspects the call against `escalate_on.reversibility.triggers`. If any trigger matches (schema migration on populated table, data deletion, public release, external communication, destructive FS op outside the harness worktree, push to a protected branch, spend above threshold), the wrapper raises `EscalationRequired` and surfaces a `RequestHumanInput` through `decide`'s return value. No LLM is consulted. This is deliberate: the highest-stakes axis cannot be a single point of probabilistic failure.

Triggers are matched by structural facts (path globs, table-name regex, branch-name match, spend counter delta) declared in the policy. The library ships default matchers; the policy file can extend them.

### 4.2 LLM classifier (Stage 2)

A separate model call. The system prompt is adversarial: its only job is to find reasons to escalate against the four axes (excluding reversibility, which Stage 1 owns). Asking the deciding model whether it should escalate is a known self-consistency failure (§6.2); a separate process is the architectural answer, not a tunable.

Inputs: the proposed decision text, structured context (files touched, modules, diff stats), the constitution (authoritative), and the policy triggers. Output: `{escalate: bool, categories: [...], confidence: float, reasoning: str}`. The `confidence_floor` is internal: when confidence falls below it and `escalate=false`, the verdict is resolved to escalate inside `decide` — callers never see the raw confidence number — recall over precision.

The classifier is stateless. Replayability (caller concern) requires every input be persisted in the unified audit log defined in Slice 2.

### 4.3 Policy file

Per-Grove YAML, shape from §6.2 of the README. Keys:

- `escalate_on.<axis>.triggers` — list of named triggers. Reversibility triggers are deterministic; the others are hints to the LLM classifier and material for its prompt.
- `thresholds.confidence_floor` — minimum LLM confidence to auto-ratify.
- `thresholds.batch_size` — flush count.
- `thresholds.max_queue_latency_min` — time-bound flush.
- Optional: `spot_check_rate` (default 0.05), `routing_rubric` overrides, `tool_wrapper_matchers` extending the deterministic set.

Composition with the constitution: the constitution is read-only authoritative narrative; the policy file is operational triggers. A decision violating the constitution auto-escalates regardless of policy. The policy cannot relax constitutional invariants; it can only add or tighten triggers.

### 4.4 Constitution loader

Reads `constitution.md`. Parses headed sections (coding conventions, testing, architectural invariants, security, performance budgets) into named blocks the LLM classifier sees verbatim in its system prompt. Stable hash of the file is recorded in every audit entry so a decision is replayable against the exact constitution it saw.

### 4.5 Routing classifier

Same library, different entry point. Structured LLM call against a short rubric (LOC affected, modules touched, external deps, user-visible surface, data-model changes, security concerns). Output: `light | heavy | ambiguous`. Ambiguous routes heavy. Operator overrides are captured (§4.7).

### 4.6 Spot-check sampler — internal

Invoked from inside `resume` (and from `decide` on auto-ratification paths), not callable directly. After auto-ratification, with probability `spot_check_rate` (default 0.05), the decision is enqueued for asynchronous operator review. If a category accumulates a threshold count of operator-disagreement events, that category's thresholds reset to library defaults and the policy file is flagged as drifted.

### 4.7 Override capture — internal

Invoked from inside `resume` when the operator's answer disagrees with the verdict. When the operator answers `rework` or `abort` against an LLM-ratified decision, or overrides a routing verdict, the library writes a labeled override event to the unified audit log defined in Slice 2 (decision + classifier verdict + operator answer + reasoning). The library does not retrain; the labeled audit-log events are what the calibration job (sibling spec) consumes. No separate training-signal stream.

### 4.8 Batcher — internal

In-process queue state hidden behind `decide`'s asynchronous return; not exposed as a class to callers. Holds escalations until `batch_size` reached or `max_queue_latency_min` elapsed, then flushes to the caller as a single CLI summary. `urgency == "high"` bypasses the batch and flushes immediately. Yes/no items are rendered for one-keystroke answers.

Audit log schema lives in the orchestrator skeleton spec; this slice writes events of types: `stage_1_pass`, `stage_1_escalate`, `stage_2_verdict`, `routing_verdict`, `spot_check_sample`, `override_recorded`, `batch_flush`, `constitution_conflict`.

## 5. Public API

The library exposes two methods. Everything else — `wrap_tool`, the `Batcher`, `record_override`, `maybe_spot_check`, the `confidence_floor` resolution — is implementation detail invoked by these two and is documented in §4 Components, not here.

```python
def decide(proposed_decision: ProposedDecision) -> Answer | RequestHumanInput:
    """Run Stage 1 (deterministic gate, internal) then Stage 2 (separate adversarial
    LLM call, internal). Returns an Answer when the decision is auto-ratified or
    auto-resolved; returns a RequestHumanInput (with an opaque resume token) when
    the operator must answer. Confidence-floor resolution, batching, and the
    spot-check sampler all run inside this call."""

def resume(token: str, operator_answer: HumanAnswer) -> Answer:
    """Resume a previously-escalated decision keyed by the token from
    RequestHumanInput. Override capture and the post-ratification spot-check
    sampler run inside this call as postconditions."""
```

Constitution and policy loading happen at library construction; they are not call-time arguments. Routing classification is a mode of `decide` selected by the shape of the proposed decision, not a separate entry point.

## 6. Data model

### 6.1 RequestHumanInput

Mirrors §6.3:

```python
class RequestHumanInput:
    question: str
    context: str
    urgency: Literal["low", "medium", "high"]
    format: Literal["yes_no", "multiple_choice", "free_text"]
    choices: list[str]
    recommendation: str
    reasoning: str
    categories: list[str]    # subset of {taste, architecture, ethics, reversibility}
    worktree_ref: str
```

### 6.2 Answer schema

```python
class HumanAnswer:
    kind: Literal["ratify", "choose", "rework", "abort"]
    choice: str | None       # for "choose"
    direction: str | None    # for "rework"
    raw_text: str            # original operator text
    interpretation: str      # orchestrator-normalized; confirmed before resume
```

Free-text answers are normalized by the caller (orchestrator) and round-tripped to the operator for confirmation; the library only validates shape. A `rework` or `abort` against work already committed flags the verdict for rollback (caller acts).

### 6.3 Policy YAML schema (annotated)

```yaml
escalate_on:
  taste:                  # LLM stage; triggers seed the prompt
    triggers: [public_api_naming, user_visible_copy, new_abstraction_naming]
  architecture:           # LLM stage
    triggers: [new_top_level_module, new_service_boundary,
               data_model_change_affecting_other_code, new_external_dependency,
               consistency_model_choice, sync_async_boundary_choice]
  ethics:                 # LLM stage
    triggers: [pii_collection_or_logging, auth_change, new_egress_path,
               dark_pattern_risk, dual_use_risk,
               regulated_domain: [health, finance, minors, identity]]
  reversibility:          # deterministic; matched at tool-wrapper level
    triggers: [schema_migration_on_populated_table, data_deletion,
               public_release, external_communication,
               destructive_fs_op_outside_worktree, git_push_protected_branch,
               spend_above: 50_usd]
thresholds:
  confidence_floor: 0.7   # below this with escalate=false → escalate anyway
  batch_size: 5
  max_queue_latency_min: 30
spot_check_rate: 0.05     # 5% of auto-ratifications get re-surfaced
```

### 6.4 Event types written to the unified audit log

Audit-log schema lives in the orchestrator skeleton spec (Slice 2). This library is a writer, not a schema owner. The event types this slice emits are:

- `stage_1_pass` — deterministic gate cleared the call.
- `stage_1_escalate` — deterministic gate fired; carries the matched trigger.
- `stage_2_verdict` — separate-call LLM verdict (`escalate | ratify`, categories, reasoning); `confidence_floor` resolution is already applied, so the recorded verdict is the resolved one and `produced_by: gate | classifier` distinguishes origin.
- `routing_verdict` — `light | heavy` (ambiguous is materialized as heavy at write time).
- `spot_check_sample` — auto-ratification flagged for async operator review.
- `override_recorded` — operator answer disagreed with verdict; labeled record for calibration.
- `batch_flush` — escalation batch released to the caller.
- `constitution_conflict` — structural post-check matched a constitution invariant.

Every event records `decision_id`, `timestamp`, `constitution_hash`, and `policy_hash`. Field-level shape (column names, types, indexing) is owned by Slice 2.

## 7. Failure modes & recovery

| Failure | Detection | Recovery |
|---|---|---|
| Stage-2 misses a real escalation | 5% spot-check; post-merge defect attribution per category | Reset that axis's thresholds to library defaults; sweep recent auto-ratifications in the category; flag policy drift |
| Prompt injection in proposed-decision text | Stage-2 input is parsed as data, never executed; fetched content is summarized upstream before reaching the classifier; classifier prompt explicitly distrusts decision text | Quarantine offending decision; escalate; record adversarial sample |
| Policy drift (operator silently relaxing triggers) | Hash of policy file recorded per decision; periodic diff review; spot-check disagreement rate spike | Operator reviews the policy diff; library refuses to load if reversibility triggers are weakened below safe defaults |
| Calibration optimizing the wrong direction (precision over recall) | Recall metric tracked from spot-check labels; alarm when recall drops below floor | Revert to defaults; freeze auto-tuning; require operator-signed policy bump to resume |
| Free-text answer ambiguity | Caller normalizes; library round-trips `interpretation` for confirmation | If unconfirmed within timeout, re-ask; never resume on ambiguous answer |
| Contradiction with already-committed work (rework/abort late) | Unified audit log (Slice 2) links `decision_id` to commit refs in `worktree_ref` via this slice's events | Caller triggers rollback to the commit predating the contradicted decision |
| Constitution conflict missed by LLM | Constitution loaded as authoritative system prompt block; static post-check matches decision against constitution invariants | Auto-escalate on any match; entry tagged `constitution_conflict` |
| LLM provider outage during Stage 2 | Library fails closed: any decision the LLM cannot evaluate is escalated | Operator decides; do not auto-ratify on classifier failure |

## 8. Testing & verification strategy

- **Unit tests.** Loaders (constitution, policy), trigger matchers (each reversibility trigger), batcher (size flush, latency flush, urgency bypass), routing tie-break (ambiguous → heavy), event-type emission against the unified audit log (Slice 2).
- **Golden set.** A curated corpus of past proposed decisions hand-labeled by the operator (`should_escalate`, `categories`, expected `routing`). Library is run end-to-end; recall, precision, false-negative rate per axis are reported.
- **Adversarial probes.** Decisions with prompt-injection payloads in the `decision` text (instructions to ignore policy, to claim safety, to suppress reasoning); classifier must escalate.
- **Calibration over a labeled replay set.** A non-live job consumes overrides and spot-check labels and proposes new thresholds; proposed thresholds run shadow before promotion. Promotion requires operator sign-off (constitution-level change).
- **Stage-1 must never call the LLM.** Test asserts the LLM stub is uninvoked when a reversibility trigger fires.
- **Stage-2 separation.** Test asserts Stage-2 uses a distinct model client from any caller-supplied "deciding" model in `ctx`.
- **Determinism check.** Same inputs + same constitution-hash + same policy-hash → same Stage-1 verdict; Stage-2 sampling variance is bounded and reported.

## 9. Open questions

- **Default `confidence_floor` value.** README pins 0.7 in the example YAML. Whether 0.7 is the right floor for a real Grove is empirical and per-operator; the policy ships with 0.7 but calibration will move it. Note: `confidence_floor` is now an internal mechanism applied inside `decide`, not a public knob; calibration tunes it via the policy file.
- **Spend-counter source.** `spend_above: 50_usd` requires a live spend stream. Whether the library reads it, the orchestrator pushes it, or the runtime emits OpenTelemetry counters is a substrate decision.
- **What counts as "protected branch."** README says protected; the matcher needs a configured list per Grove. Operator input.
- **Trigger taxonomy completeness.** The four axes are stable; the named triggers are not exhaustive. Adding triggers requires evidence; removing them requires operator sign-off.
- **Override-as-training-signal scope.** This library captures labels by writing `override_recorded` events to the unified audit log; whether the calibration job updates `confidence_floor`, per-category thresholds, or the LLM prompt itself is the calibration spec's concern, not this slice's.
- **Spot-check rate.** 5% is the README default. The right rate is a function of decision volume and operator capacity; needs empirical tuning.
- **Free-text normalization owner.** README puts normalization in the orchestrator. The library defines the contract (`raw_text` → `interpretation` round-trip) but does not implement the normalizer. Confirm boundary.
- **Failure semantics on classifier outage.** This spec says fail-closed (escalate). Operator may want fail-open for low-stakes categories during incidents. Open.
- **Internal contradiction in README.** §6.2 lists `regulated_domain: [health, finance, minors, identity]` under `ethics.triggers` as if it were a flat list, but the value is a nested mapping; the schema parser must accept either form. Flagged for the policy schema.
- **Constitution conflict detection method.** README says conflicts auto-escalate but does not specify whether detection is LLM-driven, rule-driven, or both. This spec uses both (LLM block plus structural post-check); confirm that matches operator intent.

## 10. Cross-references

- Orchestrator skeleton: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`
- Specialized harnesses: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Review and merge: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md`
- Cost mode and drift guard: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`
- Dark variants: `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md`
- Source: `/Users/dmestas/projects/darkish-factory/README.md` §2, §5.4, §5.7, §6.1, §6.2, §6.3, §8
