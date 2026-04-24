# The Darkish Factory

## TL;DR

Run multiple Claude Code instances in parallel, building at least 4x the work at the same time, get bothered half as much, with more deterministic results, observability, and evolution over time. It's *darkish* rather than dark because the human stays in the loop for taste, architecture, ethics, and reversibility. A fully dark variant is possible — see §13 — but not the goal here.

## Thesis

Put a classifier between a multi-agent pipeline and the operator. Let it ratify routine decisions and escalate only taste, architecture, ethics, and reversibility. The operator's attention stops being the bottleneck on decisions they have no unique signal on.

## 1. Problem

In spec-kit-style sessions, the AI surfaces multiple decisions per feature and the operator ratifies most of them without changing the recommendation. The AI isn't asking because it needs a human; it's asking because the harness was configured to ask. The rate at which this happens is a Week 3 measurement (Section 10), not a stipulation.

Each escalation has a real cost: reload context, read the summary, decide, resume. Flow disruption adds to it. At several features a day, consultation time compounds.

Under-consultation is the opposing risk. A wrong AI decision that propagates downstream can cost more than the escalations it would have prevented. The bet is that cost asymmetry favors under-consultation *provided* the classifier reliably catches the four categories below. Outside them, fixes are cheap; inside them, they aren't.

## 2. The Four Axes

Escalate on any of:

- **Taste** — API names, user-visible copy, picking among stylistically equivalent options, naming a new abstraction.
- **Architecture** — service boundaries, data model shape, new dependencies that import an ecosystem, consistency tradeoffs, module seams.
- **Ethics** — PII collection or logging, auth changes, new egress paths, dark-pattern UX, dual-use risk, regulated domains (health, finance, minors, identity).
- **Reversibility** — schema migrations on populated tables, data deletion, public releases, outbound communications, destructive filesystem ops outside a harness worktree, pushes to protected branches, spend above threshold.

Everything else — library choice, algorithm selection, error handling, test layout, routine refactors — the AI decides.

## 3. Why Containerized Harnesses, Not Subagents

Subagents share a failure domain. That breaks three things:

**Fresh context per phase.** Attention degrades as context fills. A subagent inherits its parent's context pollution; a containerized harness starts clean. This is the only way each phase stays out of the failure region as feature count grows.

**Blast radius.** A subagent that goes off the rails from prompt injection or a runaway loop can corrupt peers. Containerization with per-agent credentials bounds the damage. Pause/resume can be done in-process; credential and filesystem isolation can't.

**Different dispositions.** The researcher wants web access, no writes. The implementer wants shell and filesystem, no web. The verifier wants an adversarial posture actively wrong for an implementer. Different containers, different tools, different models.

All three resolve to the same architecture: a container per harness, orchestrated by another harness.

## 4. Scion (and its descendants)

Scion is Google Cloud's open-source orchestration testbed (April 2026). It runs agent runtimes (Claude Code, Gemini CLI, Codex, OpenCode) as isolated processes, each with its own container, git worktree, and credentials. It's experimental and not an officially supported Google product.

Scion is one harness-of-harnesses; more are coming. The category is already forming — Gas Town from Steve Yegge, Anthropic's Managed Agents, and others that will emerge as the orchestration layer consolidates. What matters is that viable substrates in this category are, and will remain, **harness-agnostic and model-agnostic**: they treat any containerized agent runtime as a drop-in and any model provider as a swap. That agnosticism is why the Darkish Factory's architecture survives substrate change — the isolation, handoff, and audit-log patterns don't depend on which orchestrator runs underneath, or which model runs inside each harness.

Vocabulary (Scion's; other substrates will use their own): **Grove** = workspace, **Hub** = control plane, **Harness** = agent adapter, **Runtime** = container backend.

Scion specifically provides isolation, harness-agnostic runtime choice, OpenTelemetry across the swarm, and pause/resume/attach for any agent. It deliberately doesn't prescribe orchestration logic — that's what the Darkish Factory adds.

Substrate choice is a reversible decision. If Scion regresses or stalls, or a better descendant appears, the architecture moves.

## 5. Architecture

**Sub-harnesses**, each a container with its own config and worktree:

- `researcher` — produces compressed briefs, not transcripts
- `spec-writer` — intent + research → spec, bound by the constitution
- `architect` — structural decisions with tradeoffs; output feeds the escalation queue
- `planner` — spec → units of work with file paths and test strategy
- `tdd-implementer` — failing test first, then code; refuses production code without a failing test
- `verifier` — adversarial; runs tests, edges, fuzzing
- `reviewer` — senior-eng disposition; can block
- `docs` — produces or updates documentation

The exact set is an open question; this is a starting point.

**Orchestrator.** Receives intent, picks a pipeline, dispatches work, runs the escalation classifier on every proposed decision, batches escalations, merges worktrees on completion, maintains the audit log. The only harness authorized to interrupt the human.

**Handoffs.** Each sub-harness owns a git worktree. Handoffs are git operations. Version control replaces protocol design; every intermediate state has a diff and a rollback. Anthropic's work on long-running agent harnesses arrives at the same conclusion from the other direction: they pair an initializer agent with a coding agent and use a `claude-progress.txt` file plus git history as durable cross-session state. The Darkish Factory generalizes that pattern — worktree + audit log + shared memory store is the same idea applied across many specialized harnesses instead of two.

**Constitution.** A `constitution.md` per Grove: coding conventions, testing rules, architectural invariants, security baselines, performance budgets. Sub-harnesses treat it as authoritative. Any decision that conflicts with it is an automatic escalation.

**Concurrency.** Sequential within a feature, parallel across features. No two harnesses write the same worktree. Semantic merge conflicts escalate.

## 6. The Classifiers

### 6.1 Routing: light vs. heavy

Structured LLM call against a rubric: LOC affected, modules touched, external deps, user-visible surface, data-model changes, security concerns. Output: `light | heavy | ambiguous`. Ambiguous routes heavy. Heavy-on-light wastes tokens; light-on-heavy is an unreviewed architectural commitment. Operator can override; overrides tune the classifier.

### 6.2 Escalation: hybrid, not pure LLM

Two stages:

**Stage 1: deterministic gate.** Reversibility triggers are enforced at the tool-wrapper level. Destructive operations — schema migrations, deletions, protected-branch pushes, spend over threshold — are intercepted before execution. No LLM is asked. This closes the "single point of probabilistic failure" loophole for the highest-stakes category.

**Stage 2: LLM classifier.** For taste, architecture, and ethics, a separate LLM call (not the deciding harness — that's a known self-consistency failure) evaluates against the policy file. Adversarial system prompt: its job is to find reasons to escalate.

**Policy file** (per-Grove YAML):

```yaml
escalate_on:
  taste:
    triggers: [public_api_naming, user_visible_copy, new_abstraction_naming]
  architecture:
    triggers: [new_top_level_module, new_service_boundary,
               data_model_change_affecting_other_code, new_external_dependency,
               consistency_model_choice, sync_async_boundary_choice]
  ethics:
    triggers: [pii_collection_or_logging, auth_change, new_egress_path,
               dark_pattern_risk, dual_use_risk,
               regulated_domain: [health, finance, minors, identity]]
  reversibility:  # deterministic, not LLM
    triggers: [schema_migration_on_populated_table, data_deletion,
               public_release, external_communication,
               destructive_fs_op_outside_worktree, git_push_protected_branch,
               spend_above: 50_usd]

thresholds:
  confidence_floor: 0.7
  batch_size: 5
  max_queue_latency_min: 30
```

**Calibration.** Optimize for recall over precision. Target: recall ≥ 0.95 on a hand-labeled set from the evaluation runs. If that turns out to be unachievable at the quality bar the architecture promises, re-architect — don't lower the target.

**Drift detection.** Auto-tuning on ratifications has a silent-decay failure mode. Mitigation: 5% of auto-ratified decisions get re-surfaced for spot-check. Systematic errors reset category thresholds to defaults.

### 6.3 Escalation format

Structured tool call, not prose:

```python
class RequestHumanInput:
    question: str
    context: str
    urgency: "low" | "medium" | "high"
    format: "yes_no" | "multiple_choice" | "free_text"
    choices: list[str]
    recommendation: str
    reasoning: str
    categories: list[str]
    worktree_ref: str
```

Batched CLI summary at the orchestrator prompt. Yes/no answers in one keystroke. High-urgency bypasses batching. Answers return as `ratify | choose <option> | rework <direction> | abort`. Free-text gets normalized by the orchestrator, which confirms interpretation before resuming. A contradiction with committed work triggers rollback.

## 7. The Loop

1. Operator hands intent to the orchestrator.
2. Routing classifier picks light or heavy pipeline. Operator can override.
3. **Research** (heavy only) → compressed brief.
4. **Plan.** Spec-writer, architect, planner produce a plan. Validated against the constitution.
5. **Implement.** TDD-implementer works each unit. Failing test first.
6. **Verify.** Adversarial suite. Failures loop back up to N times before escalating.
7. **Review.** Senior-eng pass. Block or ship.
8. Every fork: sub-harness emits a proposed decision with reasoning and confidence. Escalation classifier runs (deterministic first, then LLM). Ratified decisions proceed; escalations batch.
9. Operator answers the batch.
10. Orchestrator merges worktrees, runs final verification, shows a reviewable diff.
11. 5% of auto-ratified decisions get spot-checked the next day.

## 8. Failure Modes

1. **Classifier misses an escalation.** Caught by 5% sampling and post-merge defect analysis. Recovered by policy update and a sweep of recent auto-ratifications in that category.
2. **Sub-harness hangs.** 10-minute heartbeat timeout. Pause-and-inspect or kill-and-redispatch.
3. **Orchestrator crashes mid-merge.** Worktrees intact by construction. Resume from last committed state.
4. **Semantic merge conflict across features.** Escalate with both diffs and reconciliation options.
5. **Prompt injection via fetched content.** Researcher has no shell; fetched content passes through summarization before reaching privileged harnesses.
6. **Policy drift.** Drift detection plus periodic operator review of the policy diff.
7. **Token runaway.** Per-feature spend cap. Pause and escalate with the spend trace.

## 9. Why Not Just One Good System Prompt

The simplest alternative: one Claude Code session with a good constitution and "only ask me about taste/architecture/ethics/reversibility." Why isn't that enough?

1. One session accumulates context; quality degrades past ~40% fill. Multi-harness gives each phase a fresh window.
2. No failure-domain isolation. Prompt injection in one session is everyone's problem.
3. No concurrency. Three features in flight means three times wall-clock.
4. One system prompt can't simultaneously be a thorough researcher, disciplined implementer, and adversarial verifier.
5. No deterministic reversibility gates. Data deletion and protected pushes need tool-level enforcement, not good intentions.

The simplest alternative handles simple work. It doesn't scale to "operator out of the loop most of the time without sacrificing correctness on what matters."

## 10. Evaluation

**Baseline.** Five representative features through the current workflow. Instrument: wall-clock, engineer-active-minutes, consultations, consultations-where-answer-changed-the-recommendation, 2-week defect tail.

**Darkish Factory run.** Five matched features through the new pipeline. Same instrumentation plus token spend, escalation count by category, classifier precision and recall against post-hoc labels, 2-week defect tail.

**v0 targets.** Engineer-active-minutes per feature: ≥50% reduction. Classifier recall on the four axes: ≥0.95. Defect rate: no worse than baseline.

**Labeling.** Every decision in the evaluation runs — ratified and escalated — hand-labeled post-hoc. The labeled set seeds classifier tuning and harness evaluation (§11).

**Kill criteria.** After one month: if defect rate is materially worse, or classifier recall is below 0.85, halt and revisit.

**Exit criterion.** v1 is frozen when two consecutive months meet v0 targets, cold-start on a new Grove takes under a week, and no category has needed a drift reset in four weeks.

## 11. Harnesses as Evaluable Artifacts

Every component in a harness encodes an assumption about what the model can't do on its own, and those assumptions go stale as models improve. Anthropic's own harness engineering work documents this directly: a context-reset pattern needed for Sonnet 4.5 to prevent "context anxiety" became unnecessary on Opus 4.5. The architectural implication is that harnesses need continuous evaluation, not a one-time tune.

Because every harness is a versioned config — system prompt, tool allowlist, model choice, skills — and every decision flows through the audit log with post-hoc labels, harnesses become the unit of optimization. Two properties the architecture provides by construction:

**Per-harness metrics.** The audit log records, per harness per decision: the proposed decision, the escalation classifier's verdict, the operator's answer if escalated, and the 2-week defect tail on what that harness produced. From this you can compute, per harness: escalation rate, escalation-by-category distribution, rework rate, defect attribution, tokens per shipped unit, wall-clock per phase.

**Replayability.** Because every input to a decision is in the audit log, historical decisions are replayable. A candidate config can be evaluated against a replay set before it touches live work.

Together these make the standard "self-improving AI system" failure mode avoidable. The usual failure is no ground truth and no replay, so "improvement" is vibes. The Darkish Factory has both.

**What this enables.** Changing a harness config is a feature. It flows through the same loop: spec the change, plan it, implement it (as a config diff), verify by replay against historical decisions and A/B against the incumbent on live features, review, and — because a harness config *is* an architectural decision — escalate the final call to the operator.

**Cost-mode experiments.** Because harnesses are swappable configs and metrics are per-harness, the operator can run named cost profiles as first-class experiments. A `caveman` mode swaps smaller models and tighter context budgets across the swarm; a `context` mode does the opposite, giving each harness more room and a stronger model. Running the same labeled feature set through each mode produces a direct token-spend vs. quality curve — rework rate, defect tail, and escalation rate per dollar. Modes are just config bundles in the Grove, version-controlled alongside the harnesses themselves. The operator picks the point on the curve they want, per project or per feature type, rather than guessing.

**One guard.** Drift compounds across a self-improving population. If the planner gets tuned to produce plans the verifier likes, and the verifier gets tuned to accept plans the planner produces, they can reach a joint equilibrium that neither the operator nor reality endorses. The escalation classifier, the constitution, and the deterministic reversibility gates are *not* self-tuned. They stay anchored to operator-authored ground truth. Harnesses can optimize within the bounds those three define; they can't move the bounds.

## 12. Scope

Solo operator or very small team with well-defined taste, well-defined codebase. The operator is always the single decider; the classifier economizes that decider's attention. Contested taste needs a different coordination layer. Regulated environments requiring pre-merge human audit on every decision are out of scope.

## 13. Going Fully Dark

The Darkish Factory keeps a human in the loop for the four escalation axes. A fully dark variant — no human feedback at all — is a straightforward extension if it's ever wanted, because the architecture is already organized around structured inputs and outputs at every boundary.

**The bridge.** Replace the operator at the top of the pipeline with a spec-producing upstream system — another agent, a product-spec generator, a requirements pipeline, an incident response system, anything that emits intents in the same structured format the orchestrator already accepts. The orchestrator doesn't care whether intents come from a human CLI or an upstream API; intents are just typed messages.

**The escalation sink.** Replace the human at the other end of the escalation classifier with a second agent — call it a *principal* — configured with the operator's taste, architectural preferences, ethical bright lines, and reversibility rules encoded as structured policy. Escalations that would have gone to the human CLI instead go to the principal, which answers in the same `ratify | choose | rework | abort` schema. The constitution and the deterministic reversibility gates stay anchored to operator-authored ground truth — the principal doesn't rewrite them, only applies them.

**What this buys.** Spec-in, shipped-code-out, no human attention required. Useful when the upstream system is itself trustworthy and the work being done is within a domain the principal has been calibrated on.

**What stays load-bearing.** Everything in §6 (hybrid classifier), §8 (failure modes), and §11 (per-harness metrics, replay, drift guards) applies unchanged. The dark variant does not reduce the safety surface; it just moves the signature on each escalation from a human to an agent configured to act like that human.

**Why this isn't the v0.** The principal is a model calibrated on the operator's judgment, and calibrating it requires the operator's judgment in the first place. The Darkish Factory produces that calibration data as a side effect — every escalation and every answer is in the audit log, which is exactly the training corpus a principal needs. The fully dark variant is what the Darkish Factory naturally becomes after enough audit-log volume accumulates. Don't build it first; let it emerge.

## 14. Open Questions

- Exact set of sub-harnesses.
- Semantic merge conflicts across concurrent features — current answer is "escalate." Probably insufficient at scale.
- Cold-start cost per new Grove.
- Classifier pathologies not yet observed.
- Scion's maturity as a dependency.
- Minimum decision volume before harness self-tuning (§11) is statistically meaningful.
- Minimum audit-log volume before a principal (§13) is trustworthy.

---

## Prior Art

- Anthropic, *Building Effective Agents* — orchestrator-workers and evaluator-optimizer patterns
- Anthropic Engineering, *Effective harnesses for long-running agents* (Nov 2025) and *Harness design for long-running application development* (Mar 2026) — initializer/coding agent split with durable cross-session state; three-agent planner/generator/evaluator pattern; "assumptions go stale" framing
- Anthropic Engineering, *Scaling Managed Agents* — session and harness as stable abstractions
- Dex Horthy / HumanLayer, *12-Factor Agents* — context ownership, human-contact-as-tool-call, small focused agents
- Horthy, *Advanced Context Engineering for Coding Agents* — dumb-zone data, Research-Plan-Implement
- Steve Yegge (Gas Town, Beads) — practitioner validation at the operator level
- GitHub spec-kit — constitution pattern
- Google Cloud Scion — isolation substrate
- Stanford/Berkeley, "Lost in the Middle" — attention fidelity over context position
