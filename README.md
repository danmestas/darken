# darken

By Daniel Mestas

## TL;DR

Run multiple Claude Code instances in parallel, building at least 4x the work at the same time, bother the operator half as much, with deterministic guarantees, observability, and evolution over time. The human stays in the loop only for taste, architecture, ethics, and reversibility. For work with machine-checkable correctness — formal specs, reference binaries, tunable objectives — the same architecture runs fully dark. See §11.

## Install

```bash
# Recommended (Homebrew):
brew install danmestas/tap/darken

# Alternative (Go modules):
go install github.com/danmestas/darken/cmd/darken@latest

# Direct download:
# https://github.com/danmestas/darken/releases/latest
```

The `darken` binary is self-contained — templates, scripts, Dockerfiles, and the host-mode skills are embedded. Workers spawn as containerized subharnesses (claude / codex / pi / gemini); see §5 for the roster. You'll need Docker, [scion](https://github.com/ptone/scion), and credentials for whichever backends you use (`darken creds` populates them from your local Keychain / `~/.codex/auth.json` / env vars).

## Quick Start

**Fresh project:**

```bash
darken up
```

That's it — scaffolds CLAUDE.md, stages skills, ensures Docker/scion/images/secrets, and chains `bones up`. Pass `--no-bones` to skip the bones chain.

**Tear it back down:**

```bash
darken down
```

Stops project agents, deletes the project grove, removes scaffolds, runs `bones down`. Prompts before acting; pass `--yes` to skip the prompt in scripts. Add `--purge` to also stop the scion server and clean user-scope hub templates.

**Existing project, post-`brew upgrade darken`:**

```bash
darken upgrade-init
```

Refreshes scaffolds against the new binary's substrate; verifies via `darken doctor --init`.

Open Claude Code in the project; CLAUDE.md auto-loads the orchestrator-mode skill. Give it a task — the orchestrator routes (light/heavy), dispatches subharnesses, echoes decisions, and pauses only when the escalation classifier fires.

## CLI Reference

See [docs/CLI.md](docs/CLI.md) for the grouped command reference (Lifecycle / Operations / Inspection / Targeted setup / Authoring). `darken --help` lists everything in registration order.

### Customizing the substrate

The embedded substrate is the always-present fallback. Override per-machine in `~/.config/darken/overrides/`, per-project in `<repo>/.scion/templates/<role>/`, or per-invocation via `darken --substrate-overrides <path>`. Run `darken create-harness <name> --backend codex --model gpt-5.5 ...` to scaffold a new role into your overrides.

### Updating

```bash
brew upgrade darken
# or:
go install github.com/danmestas/darken/cmd/darken@latest
```

`darken version` reports the binary version + a 12-char prefix of the embedded substrate hash. Operators on the same release tag should see the same substrate hash; divergence indicates a local build.

## 1. Problem

In spec-kit-style sessions, the AI surfaces multiple decisions per feature and the operator ratifies most without changing the recommendation. The AI isn't asking because it needs a human; it's asking because the harness was configured to ask.

Each escalation costs reload, read, decide, resume. Flow disruption adds to it. At several features a day, consultation time compounds.

Under-consultation is the opposing risk. A wrong decision that propagates downstream can cost more than the escalations it would have prevented. The bet is that under-consultation is the cheaper error, so long as escalation catches four specific categories. Outside them, fixes are cheap; inside them, they aren't.

## 2. The Four Axes

A wrong AI decision outside these four axes is cheap to fix. An interruption is not. Escalate on any of:

- **Taste** — API names, user-visible copy, picking among stylistically equivalent options, naming a new abstraction.
- **Architecture** — service boundaries, data model shape, new dependencies that import an ecosystem, consistency tradeoffs, module seams.
- **Ethics** — PII collection or logging, auth changes, new egress paths, dark-pattern UX, dual-use risk, regulated domains (health, finance, minors, identity).
- **Reversibility** — schema migrations on populated tables, data deletion, public releases, outbound communications, destructive filesystem ops outside a harness worktree, pushes to protected branches, spend above threshold.

Everything else the AI decides: library choice, algorithm selection, error handling, test layout, refactors.

## 3. Why Containerized Harnesses

The simplest alternative is one session with a good system prompt and a clear rule about what to escalate. It doesn't work. Five reasons, each pointing at the same architectural answer:

**Fresh context per phase.** Attention degrades as context fills. A single session accumulates research, test output, and chat history until it is operating inside the failure region. Containerized harnesses start each phase clean. A bigger context window delays the symptoms without changing the curve.

**Failure-domain isolation.** A process that goes off the rails — prompt injection from a fetched page, a runaway loop, a malformed spec — must not corrupt its peers. Containerization bounds the damage. Pause-resume works in-process; credential and filesystem isolation does not.

**Concurrency.** Different features, and independent phases within a feature, run in parallel. Three features in flight means one-third the wall-clock, not three times.

**Specialization.** The researcher wants web access and no writes. The implementer wants shell and filesystem, no web. The verifier wants an adversarial posture actively wrong for an implementer. One system prompt cannot be all three simultaneously.

**Deterministic gates.** Data deletion, schema migration, and protected-branch pushes need tool-level enforcement, not good intentions encoded in a prompt.

All five resolve to the same shape: a container per harness, orchestrated by another harness.

## 4. Scion and Its Descendants

Scion is Google Cloud's open-source orchestration testbed (April 2026). It runs agent runtimes — Claude Code, Gemini CLI, Codex, OpenCode — as isolated processes, each with its own container, git worktree, and credentials. Scion is experimental and not an officially supported Google product.

Scion is one harness-of-harnesses; more are coming. Gas Town from Steve Yegge and Anthropic's Managed Agents are already in the category, and others will follow as the orchestration layer consolidates. Viable substrates in this category are, and will remain, **harness-agnostic and model-agnostic**: any containerized agent runtime is a drop-in, any model provider is a swap. That agnosticism is why the architecture here survives substrate change.

Vocabulary is Scion's; other substrates will use their own. **Grove** is the workspace, **Hub** is the control plane, **Harness** is an agent adapter, **Runtime** is the container backend (Docker, Podman, Apple containers, Kubernetes).

Scion provides isolation, harness-agnostic runtime choice, OpenTelemetry across the swarm, and pause-resume-attach for any agent. It does not prescribe orchestration logic. That is what darken adds.

Substrate choice is reversible. If Scion regresses or a better descendant appears, the architecture moves.

## 5. Architecture

### 5.1 Sub-harnesses

Each sub-harness is a container with its own configuration, tool allowlist, model, system prompt, and git worktree:

| Harness | Role |
|---|---|
| `researcher` | Produces compressed briefs, not transcripts |
| `spec-writer` | Converts intent + research into a spec, bound by the constitution |
| `architect` | Emits structural decisions with tradeoffs; outputs feed the escalation queue |
| `planner` | Decomposes the spec into units of work with file paths and test strategy |
| `tdd-implementer` | Writes a failing test first, then code; refuses production code without a failing test |
| `verifier` | Runs tests, edges, fuzzing with an adversarial posture |
| `reviewer` | Senior-engineer disposition; can block |
| `docs` | Produces or updates documentation |

### 5.2 Orchestrator

The orchestrator receives intent, picks a pipeline via the routing classifier, dispatches work, runs the escalation classifier on every proposed decision, batches escalations, merges worktrees on completion, and maintains the audit log. It is the only harness authorized to interrupt the human.

### 5.3 Handoffs

Each sub-harness owns a git worktree. Handoffs are git operations: the spec-writer commits a spec, the orchestrator cherry-picks it to the planner's worktree, and so on. Version control replaces protocol design. Every intermediate state has a diff and a rollback. Anthropic's own long-running-agent harness pairs an initializer with a coding agent and uses a `claude-progress.txt` file plus git history as cross-session state — the same idea applied to two agents instead of eight.

### 5.4 The Constitution

Each Grove has a `constitution.md`: coding conventions, testing rules, architectural invariants, security baselines, performance budgets. Sub-harnesses treat it as authoritative. Any decision that conflicts with it is an automatic escalation.

### 5.5 Concurrency

Sequential within a feature, parallel across features. No two harnesses write the same worktree. Semantic merge conflicts escalate.

### 5.6 The Review-and-Merge Burden

Running many features in parallel shifts the bottleneck. The operator stops being interrupted mid-feature; the pressure moves to the merge surface. Ten PRs landing a day is not a throughput win if one human is still reviewing each one. The fix is structural:

- **Automated review gate.** The `reviewer` harness runs before anything reaches the operator. It enforces the constitution, runs the suite, checks for regressions against the audit log, and flags anything that trips the escalation classifier. The human sees only what the reviewer can't confidently ratify.
- **Stacked PRs.** GitHub's native stacked-PR support and tooling like Graphite let a feature become a chain of small, dependency-ordered diffs instead of one fat PR. Stacks review in minutes; a rejection at level N doesn't block levels 1 through N–1. The planner emits plans that stack naturally.
- **Agent collaboration tools.** Beads and similar external-memory systems let harnesses coordinate on dependency graphs and blocked-on relationships without the operator mediating. Two features touching overlapping code negotiate order through shared state rather than colliding at merge.
- **Semantic conflict detection up front.** The orchestrator checks for surface-area overlap between in-flight features at the review queue, not at merge time.

The throughput claim depends on all four. 4x features with 1x review capacity is a broken system.

### 5.7 Harness Configuration is Code

Every harness is a versioned configuration artifact — a declarative file specifying system prompt, tool allowlist, model, temperature, skills, resource budget, and hooks. The constitution, policy file, and routing rubric are all code. Cost profiles (§9) are code. Nothing about a harness lives in someone's head.

Six consequences:

- **Reviewable.** Harness changes go through PR review like application code.
- **Diffable.** Drift between last month's planner and this month's is visible.
- **Reproducible.** A Grove rebuilds from its configuration repo. Two operators running the same configs get the same pipeline behavior, within model sampling variance.
- **Shareable.** A well-tuned harness is a file you can commit, fork, and hand off — the way Docker images and Terraform modules are.
- **Testable.** Configs are deterministic inputs. A candidate can be exercised against a replay set before touching live work.
- **Evolvable.** Changing a harness is a feature, and it flows through the same pipeline that ships application code.

A factory whose machines are ad-hoc prompts cannot be evaluated, improved systematically, or reproduced. A factory whose machines are version-controlled configuration can.

## 6. The Classifiers

### 6.1 Routing

A structured LLM call against a short rubric — LOC affected, modules touched, external deps, user-visible surface, data-model changes, security concerns — outputs `light | heavy | ambiguous`. Ambiguous routes heavy. Heavy-on-light wastes tokens; light-on-heavy is an unreviewed architectural commitment. The operator overrides when needed; overrides tune the classifier.

### 6.2 Escalation: Hybrid, Not Pure LLM

Two stages.

**Stage 1 is deterministic.** Reversibility triggers are enforced at the tool-wrapper level. Destructive operations — schema migrations, deletions, protected-branch pushes, spend over threshold — are intercepted before execution. No LLM is asked. This is how the highest-stakes category avoids being a single point of probabilistic failure.

**Stage 2 is an LLM classifier.** For taste, architecture, and ethics, a separate LLM call evaluates against a policy file. Separate, because asking the deciding model whether it should escalate is a known self-consistency failure. The classifier's system prompt is adversarial: its job is to find reasons to escalate.

The policy file is per-Grove YAML:

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

Calibration optimizes for recall over precision — missing a real escalation is worse than an unnecessary one. Auto-tuning thresholds against operator ratifications has a silent-decay failure mode, so 5% of auto-ratified decisions are re-surfaced for spot-check; systematic errors in a category reset its thresholds to defaults.

### 6.3 Escalation Format

Escalations are structured tool calls, not prose.

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

They batch to a CLI summary at the orchestrator prompt. Yes/no answers are one keystroke. High-urgency bypasses batching. The operator's answer returns as `ratify | choose <option> | rework <direction> | abort`. Free-text gets normalized by the orchestrator, which confirms interpretation before resuming. A contradiction with committed work triggers rollback.

## 7. The Loop

The pipeline, top to bottom:

1. Operator hands intent to the orchestrator.
2. Routing classifier picks light or heavy. Operator can override.
3. **Research** (heavy only) → compressed brief.
4. **Plan.** Spec-writer, architect, planner produce a plan. Validated against the constitution.
5. **Implement.** TDD-implementer works each unit. Failing test first.
6. **Verify.** Adversarial suite. Failures loop back up to N times before escalating.
7. **Review.** Senior-engineer pass. Block or ship.

At every fork in steps 3–7, the sub-harness emits a proposed decision with reasoning and confidence. The escalation classifier runs deterministic first, then LLM. Ratified decisions proceed; escalations batch.

At the end, the orchestrator merges worktrees, runs final verification, and shows the operator a reviewable diff. The next day, 5% of auto-ratified decisions are spot-checked.

## 8. Failure Modes

| Failure | Detection | Recovery |
|---|---|---|
| Classifier misses an escalation | 5% spot-check, post-merge defect analysis | Policy update; sweep recent auto-ratifications in that category |
| Sub-harness hangs | 10-minute heartbeat timeout | Pause-and-inspect or kill-and-redispatch |
| Orchestrator crashes mid-merge | Worktrees intact by construction | Resume from last committed state |
| Semantic merge conflict across features | Pre-merge surface-area check | Escalate with both diffs and reconciliation options |
| Prompt injection via fetched content | Researcher is sandboxed | Summarization step before any privileged harness sees fetched text |
| Policy drift | Drift detection plus periodic review | Operator reviews the policy diff |
| Token runaway | Per-feature spend cap | Pause and escalate with the spend trace |

## 9. Harnesses as Evaluable Artifacts

Harness configurations encode assumptions about what the model can't do on its own. Those assumptions go stale as models improve — Anthropic's engineering team documents a context-reset pattern that was necessary for Sonnet 4.5 and unnecessary on Opus 4.5. Harnesses need continuous evaluation, not a one-time tune.

Because configs are code (§5.7) and the audit log records every decision, two properties fall out for free:

**Per-harness metrics.** For each harness, the audit log yields escalation rate, escalation-by-category distribution, rework rate, defect attribution, tokens per shipped unit, and wall-clock per phase.

**Replayability.** Every input to a decision is in the audit log, so historical decisions can be replayed. A candidate config is evaluated against a replay set before it touches live work.

Together these eliminate the standard failure mode of self-improving AI systems: no ground truth, no replay, so "improvement" is vibes.

Changing a harness is itself a feature. It flows through the same loop — spec the change, plan it, implement the diff, verify by replay and A/B against the incumbent on live features, review, and escalate the final call because a harness config is an architectural decision.

**Cost-mode experiments.** Named cost profiles become first-class experiments. A `caveman` mode uses smaller models and tighter context budgets across the swarm; a `context` mode gives each harness more room and a stronger model. Running the same labeled feature set through each produces a direct spend-vs-quality curve — rework rate, defect tail, and escalation rate per dollar. The operator picks a point on the curve per project or feature type rather than guessing.

**Drift guard.** A self-improving population can reach a joint equilibrium that neither the operator nor reality endorses — the planner tuned to produce plans the verifier likes, the verifier tuned to accept what the planner produces. The escalation classifier, the constitution, and the deterministic reversibility gates are not self-tuned. They stay anchored to operator-authored ground truth. Harnesses optimize within the bounds those three define; they can't move the bounds.

## 10. Scope

Solo operator or very small team, well-defined taste, well-defined codebase. The operator is always the single decider; the classifier economizes their attention. Contested taste needs a coordination layer this document doesn't provide. Regulated environments requiring pre-merge human audit on every decision are out of scope.

## 11. Going Fully Dark

Any piece of working software can serve as a specification. Agent tools probe it feature by feature, record the observed behavior, and validate a new implementation against what they saw. The binary becomes a living RFC — no taste decisions remain, because the running system already made them.

This is the load-bearing insight of the dark variant: if correctness is machine-checkable, the human isn't needed. The four-axis escalation taxonomy collapses because three of the four axes have been answered upstream.

Two paths get there.

### 11.1 The Formal-Spec Path

Feed the orchestrator a specification that defines correctness precisely enough that an implementation is either correct or it isn't. Domains:

- **Explicit specs.** Protocol implementations (TCP, QUIC, TLS, HTTP/3), codecs against published formats, compilers against language specs, filesystems against POSIX. Anything with a conformance suite or a published reference implementation.
- **Behavioral specs from binaries.** Legacy reimplementation where no document exists but the running system does. Cross-language ports. Open-source equivalents of closed-source tools. Replacing a vendored dependency with an in-house equivalent.
- **Internal specs.** Systems where the operator has written a rigorous interface specification — the constitution extended into full behavioral definition.

The mechanism for the binary-as-spec subcase: a dedicated `oracle` harness exercises the reference binary with structured inputs, records outputs and side effects, and emits a growing behavioral corpus. The verifier runs the same inputs against the new implementation and diffs. Discrepancies are bugs to fix or intentional deviations to escalate. Over iterations the corpus converges on a complete-enough spec for the subset of behavior that matters — and what matters is observable, because the operator's actual usage patterns are the sampling distribution.

**How the pipeline adapts.** The constitution becomes the spec (or a pointer to the oracle). The `architect` is demoted because most of its decisions were reducing ambiguity the spec already resolved. The `verifier` is promoted; the conformance suite or oracle diff becomes the primary acceptance gate. Taste and architecture triggers go quiet by construction. Ethics and reversibility triggers remain — even an RFC implementation can touch PII or run destructive operations during testing.

**What still breaks.** Spec ambiguity. Every RFC has places where the text allows multiple readings; real-world protocol implementations accumulate footnotes about "what X does vs. what Y does when the spec was silent." These are taste and architecture decisions in disguise. Mitigation: the classifier treats "the spec is silent" or "the spec says MAY" as automatic escalation even in formal-spec mode. For the binary-as-spec variant, the analogous failure is *sampling gap* — the oracle never exercised a code path the new implementation gets wrong. Mitigation: coverage-driven probing, fuzzing the oracle, and treating any un-probed-but-reachable behavior as implicit escalation until sampled.

### 11.2 The Principal-Agent Path

For work without a machine-checkable spec, substitute an agent that acts like the human. Replace the operator at the top of the pipeline with a spec-producing upstream system — another agent, a product-spec generator, a requirements pipeline — and the human at the end of the escalation classifier with a *principal* configured with the operator's taste, architectural preferences, ethical bright lines, and reversibility rules. Escalations go to the principal, which answers in the same `ratify | choose | rework | abort` schema. The constitution and the deterministic reversibility gates stay anchored to operator-authored ground truth; the principal applies them but doesn't rewrite them.

**Why start darkish.** The principal is a model calibrated on the operator's judgment, and the calibration data is the audit log darken produces as a byproduct. The fully dark variant is what darken becomes after enough volume accumulates. Don't build it first; let it emerge.

### 11.3 Composition

A real fully-dark system uses both paths. A QUIC implementation flows through the formal-spec route; a product-feature spec flows through the principal. Both share the same orchestrator, audit log, and reversibility gates. §6, §8, and §9 apply unchanged — going dark does not reduce the safety surface, it just changes who signs each escalation.

## 12. Evidence

The architecture composes patterns that have been validated independently.

**Parallelization produces the throughput multiplier.** [Cosine's commercial platform](https://cosine.sh/blog/parallelising-software-development-multi-agent-productivity) reports 3x or greater throughput from multi-agent parallel execution in production. Their phrasing: "Even a 50% faster coder could only do ~1.5× more work; five agents in parallel can deliver 5×." [Anthropic's 2026 Agentic Coding Trends Report](https://resources.anthropic.com/2026-agentic-coding-trends-report) names orchestrator-coordinated specialized agents with dedicated context windows as the emerging production pattern.

**Naive AI-assisted development slows experienced engineers.** [METR's July 2025 randomized controlled trial](https://metr.org/blog/2025-07-10-early-2025-ai-experienced-os-dev-study/) — 16 experienced open-source developers, 246 tasks, codebases they knew well — found allowing AI tools increased completion time by 19% against a forecast of 24% speedup. Developers felt faster while being measurably slower. The single-agent babysitter model is negative-ROI for senior engineers. The fix is structural, not incremental.

**The review burden is real and quantified.** [Faros AI's telemetry](https://www.faros.ai/blog/ai-software-engineering) from 10,000+ developers across 1,255 teams: high-adoption teams interact with 9% more tasks and 47% more PRs per day. Context switching overhead correlates with cognitive load and reduced focus. This is exactly what §5.6 addresses.

**Constitutions measurably improve quality.** [Constitutional Spec-Driven Development (arxiv 2602.02584)](https://arxiv.org/abs/2602.02584) reports a banking case study: 73% reduction in security vulnerabilities, 56% faster time to first secure build, 4.3x improvement in compliance documentation coverage. The paper also documents the constitution-as-attack-surface failure mode, which the audit log and deterministic reversibility gates contain.

**Orchestrator + specialized workers + adversarial verifier is the convergent pattern.** [Augment Code's Coordinator/Implementor/Verifier](https://www.augmentcode.com/guides/coordinator-implementor-verifier), Cosine's task-decomposition-with-specialized-agents, and [Anthropic's evaluator-optimizer pattern](https://www.anthropic.com/engineering/building-effective-agents) describe the same structure from different angles. darken's contribution is not the pattern itself but the escalation classifier on top.

**Legacy modernization via behavioral characterization is in production.** [Red Hat's "agent mesh"](https://www.redhat.com/en/blog/refactoring-speed-mission-agent-mesh-approach-legacy-system-modernization-red-hat-ai) uses coding agents that "analyze legacy Python 2 or Java source code, identify deprecated APIs, generate refactored equivalents, and produce characterization tests that capture the original behavior before anything changes." That is the §11.1 oracle-harness pattern, already shipping.

**Differential-oracle testing has academic backing.** [SmartOracle (arxiv 2601.15074)](https://arxiv.org/abs/2601.15074) demonstrates agent-coordinated differential testing for JavaScript runtimes. [AIProbe (arxiv 2507.03870)](https://arxiv.org/abs/2507.03870) uses oracle planners to detect model flaws and environment-induced infeasibility. [UnitTenX (arxiv 2510.05441)](https://arxiv.org/abs/2510.05441) combines AI agents with formal verification for legacy test generation.

**The four-axis taxonomy matches field behavior.** The Anthropic 2026 Agentic Coding Trends Report documents that engineers delegate tasks they can "sniff-check for correctness" or that are low-stakes, while keeping conceptually difficult or design-dependent work for themselves. That is §2, observed independently.

## 13. Open Questions

- **Exact set of sub-harnesses.** The list in §5.1 is the convergent starting point across published practice; the right set for any given operator and codebase is empirical.
- **Semantic merge conflicts at scale.** The current answer is "escalate." That is probably insufficient once feature concurrency exceeds the operator's review capacity even with §5.6 mitigations.
- **Cold-start cost per new Grove.** How long does it take a fresh Grove to reach steady-state escalation volumes? Unknown; best guess is a week of supervised operation.
- **Classifier pathologies not yet observed.** Any system with learned thresholds eventually finds failure modes its designers didn't anticipate. The 5% spot-check is a hedge, not a proof.
- **Scion's maturity as a dependency.** It is explicitly experimental. The architecture tolerates substrate swap, but the operator pays the cost of the swap.
- **Minimum decision volume for harness self-tuning.** Replay-based evaluation needs enough decisions per harness to be statistically meaningful. How many is enough is an open calibration question.
- **Minimum audit-log volume for a trustworthy principal.** Going dark via §11.2 requires the principal to have seen enough operator judgments to act like the operator. The volume required is unknown.

darken is not the endpoint. It is the scaffolding that produces the data — the audit log, the per-harness metrics, the labeled decisions — from which a fully dark system calibrates itself.

This document is itself a formal specification of the kind §11.1 describes. If the thesis holds, it is buildable from this text alone — a spec, a constitution, and a convergent set of published patterns. Whether a fully-dark pipeline can actually produce darken from its own description is the first natural test.

---

## Prior Art

### Architecture Patterns

- Anthropic, [*Building Effective Agents*](https://www.anthropic.com/engineering/building-effective-agents) — orchestrator-workers and evaluator-optimizer patterns
- Anthropic, [*2026 Agentic Coding Trends Report*](https://resources.anthropic.com/2026-agentic-coding-trends-report) — orchestrator + specialized parallel agents with dedicated context
- Anthropic Engineering, [*Effective harnesses for long-running agents*](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents) and [*Harness design for long-running application development*](https://www.anthropic.com/engineering/harness-design-long-running-apps) — initializer/coding-agent split; planner/generator/evaluator pattern; "assumptions go stale"
- Anthropic Engineering, [*Scaling Managed Agents*](https://www.anthropic.com/engineering/managed-agents) — session and harness as stable abstractions
- Augment Code, [*What Is Spec-Driven Development*](https://www.augmentcode.com/guides/spec-driven-development-ai-agents-explained) — Coordinator/Implementor/Verifier in production
- Dex Horthy / HumanLayer, [*12-Factor Agents*](https://github.com/humanlayer/12-factor-agents) — context ownership, human-contact-as-tool-call, small focused agents
- Horthy, [*Advanced Context Engineering for Coding Agents*](https://www.humanlayer.dev/blog/advanced-context-engineering) — dumb-zone data, Research-Plan-Implement
- Steve Yegge — [*Gas Town*](https://steve-yegge.medium.com/welcome-to-gas-town-4f25ee16dd04) and [*Beads*](https://steve-yegge.medium.com/introducing-beads-a-coding-agent-memory-system-637d7d92514a) — operator-level practitioner validation
- GitHub, [*spec-kit*](https://github.com/github/spec-kit) — constitution pattern

### Production Case Studies

- Cosine, [*Parallelising Software Development*](https://cosine.sh/blog/parallelising-software-development-multi-agent-productivity) — commercial multi-agent platform, 3x+ throughput
- Red Hat, [*Refactoring at the speed of mission*](https://www.redhat.com/en/blog/refactoring-speed-mission-agent-mesh-approach-legacy-system-modernization-red-hat-ai) — oracle-harness pattern in production

### Empirical Evidence

- METR, [*Measuring the Impact of Early-2025 AI on Experienced Open-Source Developer Productivity*](https://metr.org/blog/2025-07-10-early-2025-ai-experienced-os-dev-study/) — RCT showing 19% slowdown for experienced engineers using naive AI tooling
- Faros AI, [*The AI Productivity Paradox Research Report*](https://www.faros.ai/blog/ai-software-engineering) — 9% more tasks, 47% more PRs per day at high AI adoption
- Google, [*DORA Report 2025*](https://dora.dev/dora-report-2025/) — 90% AI adoption correlating with 9% bug rate increase, 91% review time increase, 154% PR size increase
- [*Constitutional Spec-Driven Development*](https://arxiv.org/abs/2602.02584) — 73% vulnerability reduction, 56% faster time to first secure build, 4.3x compliance coverage
- Stanford/Berkeley, [*Lost in the Middle*](https://arxiv.org/abs/2307.03172) — attention fidelity degradation over context position

### Supporting Infrastructure and Tooling

- Google Cloud, [*Scion*](https://github.com/GoogleCloudPlatform/scion) — isolation substrate
- [SmartOracle](https://arxiv.org/abs/2601.15074) — agent-coordinated differential testing
- [AIProbe](https://arxiv.org/abs/2507.03870) — differential testing with oracle planners
- [UnitTenX](https://arxiv.org/abs/2510.05441) — AI agents with formal verification for legacy test generation
