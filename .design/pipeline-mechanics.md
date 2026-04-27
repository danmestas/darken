# Pipeline Mechanics

## 1. Overview

This document is the operational expansion of README §7. The README describes the architecture: why containerized harnesses, what the four axes are, how the classifiers work, what the audit log records. This document describes the execution: how each phase of the §7 loop runs, what commands invoke it, what files move between phases, how the orchestrator detects and recovers from failures, and what the operator needs to understand to run the pipeline in local mode. Read the README first. This document assumes it.

---

## 2. Pipeline Phases

The §7 loop has two variants: light and heavy. Heavy includes the research phase. Light skips directly to plan.

```
heavy:  intent → routing → research → design → plan → implement → verify → review → merge
light:  intent → routing → design → plan → implement → verify → review → merge
```

Each phase is a separate sub-harness, started, monitored, and then stopped before the next begins. One sub-harness per phase per feature; parallel features are allowed (README §5.5).

### Phase 0: Intent

**Harness:** orchestrator (inline — no sub-harness started)
**Inputs:** operator-provided task description
**Outputs:** routing decision logged to audit log; raw intent appended to audit log
**Command:** none — orchestrator receives intent at the top of its prompt
**Handoff:** orchestrator proceeds internally to routing

The orchestrator reads the intent, identifies what success looks like, what the minimal deliverable is, and what is explicitly out of scope. Raw intent is the first audit log entry for this pipeline run.

### Phase 1: Routing

**Harness:** orchestrator (inline — structured classifier call, no sub-harness)
**Inputs:** intent text; routing rubric (LOC affected, modules touched, external deps, user-visible surface, data-model changes, security concerns)
**Outputs:** `light | heavy | ambiguous`; routing decision + confidence logged
**Command:** none — structured LLM call by the orchestrator
**Handoff:** heavy → research; light → design

Ambiguous routes heavy (README §6.1). Operator override is accepted and logged. The override itself is an audit log event.

**Failure:** Routing error (heavy task classified light) is detected post-facto when the designer flags missing research. Recovery: operator reclassifies, orchestrator dispatches researcher and replays from research phase forward.

### Phase 2: Research (heavy only)

**Harness:** `researcher`
**Inputs:** intent summary; relevant context from the orchestrator prompt
**Outputs:** `docs/research-brief.md` committed to the researcher's worktree
**Command:**
```bash
scion start researcher --type researcher --notify "Produce a compressed research brief for: <intent summary>. Context: <relevant details>. Output a brief to your worktree at docs/research-brief.md. Do not produce transcripts."
```
**Monitor:** `scion look researcher`
**Handoff:** orchestrator cherry-picks the commit containing `docs/research-brief.md` to its own staging area, then stops and deletes the researcher before starting the designer.

```bash
git cherry-pick <commit-sha>
scion stop researcher --yes
scion delete researcher --yes
```

The researcher is sandboxed: web access allowed, no write access to any worktree other than its own. Fetched content passes through a summarization step before any privileged harness sees it (README §8, prompt injection row).

**Failure (§8):** If the researcher hangs for 10 minutes without output, the orchestrator pauses-and-inspects or kills and redispatches. If fetched content shows signs of injection, the summarization gate absorbs it; the researcher's isolation prevents it from reaching the designer.

### Phase 3: Design (spec + architecture)

**Harness:** `designer`
**Inputs:** intent summary; `docs/research-brief.md` (if heavy); `constitution.md`
**Outputs:** `docs/spec.md` committed to the designer's worktree
**Command:**
```bash
scion start designer --type designer --notify "Convert the following intent (and research brief if provided) into a spec. Emit spec to your worktree at docs/spec.md. Validate against constitution.md. Flag any decision that conflicts with the constitution as an escalation. Intent: <intent summary>. Research brief: <path or summary>."
```
**Monitor:** `scion look designer`
**Handoff:** orchestrator reviews the spec, runs the escalation classifier on any flagged decisions, cherry-picks `docs/spec.md`, then stops and deletes the designer.

The designer collapses the `spec-writer` and `architect` roles from README §5.1. Any architectural decision it proposes goes through the escalation classifier (README §6.2) before the orchestrator accepts it. Architecture-axis triggers at this stage are cheaper to resolve than after the planner has decomposed them.

**Failure (§8):** If the spec conflicts with the constitution, the orchestrator pushes back before cherry-picking. If the designer hangs, the 10-minute heartbeat rule applies.

### Phase 4: Plan

**Harness:** `planner`
**Inputs:** `docs/spec.md`
**Outputs:** `docs/plan.md` committed to the planner's worktree; plan is a stacked sequence of implementation units, each with file paths, test strategy, and failing-test-first requirement
**Command:**
```bash
scion start planner --type planner --notify "Decompose the attached spec into implementation units with file paths and test strategy. Each unit must have a failing-test-first requirement. Output plan to your worktree at docs/plan.md. Spec: <path>."
```
**Monitor:** `scion look planner`
**Handoff:** orchestrator audits the plan as a principal engineer — checks for overengineering, missing test coverage, unclear boundaries — then cherry-picks `docs/plan.md`, stops and deletes the planner.

**Failure (§8):** A plan that overengineers is pushed back before the implementer starts. Once the implementer has run, rework is expensive. Push back at planning stage, not after implementation.

### Phase 5: Implement

**Harness:** `tdd-implementer`
**Inputs:** `docs/plan.md`; `constitution.md`
**Outputs:** one atomic commit per implementation unit, each with a passing test; all commits to the tdd-implementer's worktree
**Command:**
```bash
scion start tdd-implementer --type tdd-implementer --notify "Execute the plan at docs/plan.md. Write a failing test before each unit of production code. Commit each unit atomically. Do not proceed to the next unit without a passing test. Plan: <path>."
```
**Monitor:** `scion look tdd-implementer` — this is the longest phase; answer questions directly and tersely; when a question hits the escalation classifier, run both stages before answering
**Handoff:** orchestrator cherry-picks all commits from the tdd-implementer's worktree in order, logging each commit ref, then stops and deletes the tdd-implementer.

**Failure (§8):** Token runaway is detected via per-feature spend cap; orchestrator pauses and escalates with the spend trace. Sub-harness hangs trigger the 10-minute heartbeat timeout.

### Phase 6: Verify

**Harness:** `verifier`
**Inputs:** commit range from the tdd-implementer's worktree
**Outputs:** pass/fail report with evidence; failures include specific failing tests and edge cases
**Command:**
```bash
scion start verifier --type verifier --notify "Run full adversarial verification of the implementation. Run all tests. Test edges and failure modes. Your posture is adversarial: assume the implementation is wrong until proven otherwise. Report pass/fail with evidence. Implementation ref: <commit range>."
```
**Monitor:** `scion look verifier`
**Handoff (pass):** orchestrator logs result, stops and deletes the verifier, proceeds to review.
**Handoff (fail):** orchestrator sends failure details to a new `tdd-implementer` instance for targeted fixes, then re-runs the verifier. Loops up to 3 times; if still failing, escalates to the operator with the failure trace.

**Failure (§8):** Verification failures that persist after 3 implementer-verifier loops escalate to the operator with the full failure trace. The operator decides whether to rework the spec, the plan, or the implementation.

### Phase 7: Review

**Harness:** `reviewer`
**Inputs:** commit range; `constitution.md`; audit log entries for this pipeline run
**Outputs:** `ship | block <blocking issues>`
**Command:**
```bash
scion start reviewer --type reviewer --notify "Senior-engineer code review. Check correctness, test coverage, code quality, style consistency, constitution compliance, security. You may block. Report: ship | block with blocking issues. Implementation ref: <commit range>."
```
**Monitor:** `scion look reviewer`
**Handoff (ship):** orchestrator proceeds to merge.
**Handoff (block):** orchestrator evaluates the blocking issues. Valid block → dispatch targeted fixes. Disagreement with the block → escalate to operator with the reviewer's finding and the orchestrator's assessment.

**Failure (§8):** If the reviewer blocks on something the orchestrator disagrees with, the escalation goes to the operator — the orchestrator does not unilaterally override a block.

### Phase 8: Merge and Completion

**Harness:** orchestrator (inline) + brief `verifier` pass
**Inputs:** all committed worktrees from this pipeline run
**Outputs:** merged main branch; reviewable diff; completion record in audit log
**Procedure:**
1. Merge all worktrees to the main branch.
2. Run a final brief `scion start verifier` pass.
3. Present the operator: reviewable diff; summary of auto-ratified decisions that affected the outcome; any deferred escalations.
4. Append completion record to audit log.
5. Stop and delete all remaining sub-harnesses.

---

## 3. Worktree and Handoff Convention

Every sub-harness owns exactly one git worktree for the duration of its phase. No two harnesses write the same worktree (README §5.5). This is not enforced by Scion; it is enforced by the orchestrator's discipline: a harness is started, does its work, and is deleted before the next is started.

**Naming convention:** worktrees are named after their role. Scion manages worktree lifecycle; the orchestrator references them by role name in `scion start` and `scion look` commands.

**Handoff mechanics:** when a phase completes, the orchestrator cherry-picks the relevant commit(s) from the completed harness's worktree to its own staging area, then to the next harness's worktree before starting it. Version control replaces protocol design (README §5.3). Every intermediate state has a diff and a rollback. No uncommitted work changes hands.

**Logging:** every handoff is logged: source harness, destination harness, commit ref. This is the primary input to audit log replay (README §9).

---

## 4. Escalation Routing

When a sub-harness emits a question or proposes a decision, the orchestrator runs the escalation classifier before responding.

**Stage 1 (deterministic):** reversibility triggers — schema migrations, deletions, protected-branch pushes, spend above threshold — are intercepted before execution. No LLM call. High urgency. Bypasses the batch queue. The operator is interrupted immediately (README §6.2).

**Stage 2 (LLM classifier):** taste, architecture, and ethics triggers are evaluated against the per-Grove policy file (`escalate_on` and `thresholds` in README §6.2 YAML). The classifier's posture is adversarial; its job is to find reasons to escalate. Confidence floor defaults to 0.7.

**Batching:** non-urgent escalations accumulate in a queue. The batch is presented to the operator when it reaches `batch_size` (default 5) or `max_queue_latency_min` (default 30 minutes) from the policy file, whichever comes first. The batch is a numbered CLI summary; yes/no answers are one keystroke.

**Escalation schema** (README §6.3):
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

**Operator response:** `ratify | choose <option> | rework <direction> | abort`. Free-text responses are normalized by the orchestrator, which confirms interpretation before resuming. A `rework` or `abort` that contradicts committed work triggers rollback.

---

## 5. Audit Log Discipline

The orchestrator owns the audit log. The admin harness does not touch it.

Every audit log event records: `decision_id`, `timestamp`, `constitution_hash`, `policy_hash`, plus the decision content, confidence, category, and auto-ratified/escalated status (README §6.4). The `constitution_hash` and `policy_hash` fields make the log reproducible: any historical decision can be replayed against the same policy state that produced it (README §9).

The audit log is append-only. The orchestrator writes to it; no sub-harness writes to it directly. The admin harness writes a separate narrative chronicle of pipeline activity to its own worktree, distinct from the audit log. Conflating the two would corrupt the log's integrity as a replay source.

Post-pipeline, 5% of auto-ratified decisions are spot-checked against operator judgment. Systematic errors in a category reset that category's thresholds to defaults (README §6.2). This is the primary feedback mechanism for classifier calibration.

---

## 6. Local Mode vs. Fully-Autonomous Mode

The validated execution pattern is: **local Claude Code session as orchestrator, Scion-managed containers as sub-harness workers.** The operator runs a local Claude Code instance, pastes the orchestrator's `agents.md` and `LOCAL-MODE.md` instructions, and that session runs the §7 loop — dispatching, monitoring, cherry-picking, and escalating — while Scion handles container isolation for each sub-harness.

This is the local-orchestrator + container-workers pattern. The operator is always present and is the only entity authorized to interrupt the human.

For full details on the local-mode setup — hub enable commands, notification subscription, monitoring cadence, and interrupt procedures — see `.scion/templates/orchestrator/LOCAL-MODE.md`.

**Fully-autonomous mode** (README §11) substitutes a principal agent for the operator at the top of the escalation chain. The orchestrator and sub-harnesses run unchanged. Escalations route to the principal, which answers in the same `ratify | choose | rework | abort` schema. The constitution and deterministic reversibility gates remain operator-authored and are not modified by the principal. Fully-autonomous mode is not the starting configuration; it emerges after sufficient audit log volume to calibrate a trustworthy principal (README §11.2, §13).

---

## 7. Failure Recovery

For each row in README §8:

**Classifier misses an escalation.** Detection: the 5% post-pipeline spot-check surfaces decisions the operator would have escalated; post-merge defect analysis correlates defects back to auto-ratified decisions. Recovery: update the policy file for the affected category; sweep recent auto-ratifications in that category against the new policy; notify the operator of any decisions that would now escalate.

**Sub-harness hangs.** Detection: the orchestrator's monitor loop checks `scion look <name>` on each active harness; no new output for 10 minutes triggers the heartbeat timeout. Recovery: pause-and-inspect first — read the terminal state and send targeted guidance via `scion message <name> --interrupt`; if the harness does not proceed after the interrupt, stop and redispatch from the last committed state. Every intermediate state is committed, so redispatch does not lose work.

**Orchestrator crashes mid-merge.** Detection: operator notices the session is gone; worktrees are intact by construction (each sub-harness committed its output before being stopped). Recovery: resume from the last committed state in the audit log; identify which cherry-picks completed and which did not; restart the merge from the first uncommitted handoff. No sub-harness work is lost because the handoff convention requires committed state.

**Semantic merge conflict across features.** Detection: the orchestrator runs a pre-merge surface-area check before any feature's worktree is merged; overlapping file-path sets between in-flight features are flagged before merge time, not at merge time (README §5.6). Recovery: escalate to the operator with both diffs and reconciliation options; the operator decides ordering or requests a targeted rework.

**Prompt injection via fetched content.** Detection: the researcher is sandboxed and its output passes through a summarization step before any privileged harness sees the raw fetched text. Recovery: the summarization gate absorbs injection attempts at the researcher boundary; the isolation prevents injected content from reaching the designer, planner, or implementer. If the researcher's summary itself shows signs of manipulation, the orchestrator discards it and redispatches with a narrower fetch scope.

**Policy drift.** Detection: drift detection runs on the policy file; periodic review is scheduled by the operator (cadence is project-specific). Recovery: the operator reviews the policy diff; any threshold that has drifted from its default is either ratified explicitly or reset; categories with systematic spot-check failures reset to defaults automatically.

**Token runaway.** Detection: the orchestrator tracks per-feature spend against the configured cap; the cap is checked before each new sub-harness is started and during the implementer phase (the longest). Recovery: pause the active sub-harness and escalate to the operator with the full spend trace — cumulative spend by phase, projected final spend, remaining budget. The operator decides whether to continue with a higher cap, rework the plan to reduce scope, or abort.

**Verifier loops without convergence** (implied by §7 step 6). Detection: the orchestrator counts implementer-verifier loop iterations per feature; at N=3 the loop is terminated regardless of verifier state. Recovery: escalate to the operator with the full failure trace — failing tests, edge cases, and the implementer's last explanation of why it did not fix them. The operator diagnoses at the spec or plan level, not the implementation level.

---

## 8. What This Document Is Not

This document is the operational runbook for the §7 loop. It is not:

- **The architecture.** For why containerized harnesses, what the four axes are, how classifiers work, and what the audit log records at a conceptual level, read the README.
- **The harness configurations.** For model, max\_turns, max\_duration, detached status, and description of each harness, read `.design/harness-roster.md` and the individual `.scion/templates/*/scion-agent.yaml` files.
- **Feature specs.** When the factory is used to ship features, specs live in `.design/specs/<date>-<feature>.md`. That directory does not exist yet and will be created when the first feature is specced.
- **The orchestrator's system prompt or agent instructions.** Those live in `.scion/templates/orchestrator/system-prompt.md` and `agents.md`. The operational specifics in this document were lifted from `agents.md`; as this document matures, `agents.md` can shrink to a reference to this file for the detail and retain only the executable command patterns the orchestrator needs inline. TODO: once this document stabilizes, audit `agents.md` for content that duplicates what is here and remove the duplication.

---

## 9. Planner Tier Routing

The original §7 loop dispatched a single `planner` harness. The roster now has four planner tiers (`planner-t1`..`planner-t4`); the routing classifier in Phase 1 picks one. Source of truth is the spec at `docs/superpowers/specs/2026-04-26-harness-and-image-configuration-design.md` §8.

The classifier's output extends from `light | heavy | ambiguous` to a tier label `t1..t4`. The light/heavy distinction collapses into the tier spectrum.

| Classification signals | Tier | Harness | When to pick |
|---|---|---|---|
| Tiny ad-hoc; single-file change; obvious fix; no spec needed; no architectural decisions | `t1` | `planner-t1` (claude-haiku) | Thin planner — think-then-do; no plan doc; small bug fixes |
| Mid-complexity; multi-file but bounded; follows established patterns; no constitution gates | `t2` | `planner-t2` (claude-sonnet) | Claude-code conventions; light plan doc; few clarifying questions |
| Architectural decisions; new feature surface; needs design discipline; brainstorming → spec → plan → tasks | `t3` | `planner-t3` (claude-opus + superpowers) | Full superpowers TDD plan; default for ambiguous routing |
| Constitution gates matter; cross-vendor planner pass desired; formal spec-kit workflow; highest rigor | `t4` | `planner-t4` (codex spec-kit) | Constitution-driven; full ratification: constitution + spec.md + plan.md + tasks/ |
| `ambiguous` | `t3` | `planner-t3` | Default fallback — most cases benefit from design discipline; t4 is reserved for "we know we need a spec" |

The operator can override at dispatch with `--planner=t<N>`. Overrides are logged to the audit log as classifier overrides (same shape as the existing routing override path).

Phase 4 ("Plan") in §2 above runs whichever planner tier the classifier picked. The handoff convention (cherry-pick the output, stop and delete before starting the implementer) is unchanged across tiers. Planner-tier output may be `docs/plan.md` (t2/t3), the full spec-kit tree under `specs/` and `plans/` (t4), or no plan doc at all (t1, where the implementer receives the intent directly).

---

## 10. Darwin Loop

`darwin` runs after pipeline completion. It is the post-pipeline evolution agent: codex-backed (gpt-5.5), 50 turns, 4h, not detached. It reads completed harness sessions (transcripts, audit log entries, metrics) and emits **structured recommendations** for the operator — never direct mutations.

```
pipeline run completes
  → audit log + transcripts persisted
  → darwin invoked over the run window
  → darwin emits .scion/darwin-recommendations/<date>-<run-id>.yaml
  → operator reviews via `darken apply <file>` (y/n/skip/edit per recommendation)
  → approved recommendations mutate the relevant manifest, commit the change in git, re-stage skills if needed
  → ratifications recorded in the audit log
```

Recommendation types (spec §12.4): `skill_add`, `skill_remove`, `skill_upgrade`, `model_swap`, `prompt_edit`, `rule_add`. Each has a target harness, a one-paragraph rationale, evidence pointers (transcript lines / audit log entries / metrics), a confidence score, and a reversibility tag (`trivial | moderate | high`).

`darwin` never mutates state directly. The mechanism is intentionally operator-gated:

- The escalation classifier (taste / ethics / reversibility axes) runs against each recommendation. Trivial-reversibility recommendations may auto-apply if operator policy permits; everything else routes to operator review.
- `darken apply --dry-run <file>` prints what would change without modifying anything.
- The audit log records every ratification (approved / skipped / edited), making the evolution loop replay-safe.

This grounds darwin's "evolves rules and skills" loop in a concrete, auditable, operator-gated mechanism. The recommendations may add or remove skills, adjust tier defaults, swap models, edit system prompts, or tighten constitution clauses — but only the operator decides what lands.
