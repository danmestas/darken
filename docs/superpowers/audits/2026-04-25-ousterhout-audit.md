# Ousterhout Audit: Darkish Factory Slices 1-6

Date: 2026-04-25
Framework: *A Philosophy of Software Design*, J. Ousterhout
Audit target: README.md plus six slice specs under `docs/superpowers/specs/`

## Slice 1 — Escalation Classifier (`2026-04-25-escalation-classifier-design.md`)

Central design move: a single library that wraps four mechanisms — deterministic tool-wrapper gate, separate adversarial LLM call, routing classifier, batcher — behind one façade so any caller can ask "should this escalate?" without owning a runloop.

**Red flags.**

- *Shallow façade hiding a god module* — §1 "The library owns: the constitution loader, the YAML policy file, the deterministic tool-wrapper gate, the separate-call LLM classifier, the routing classifier, the `RequestHumanInput` data model, batching with high-urgency bypass, the 5% spot-check sampler, override capture, and the audit-log entries it emits." Ten responsibilities through one library boundary violates **deep-modules**: the surface listed in §5 (six top-level functions plus a `Batcher` class) is not the depth of a single concept; it is the union of three concepts (gate, classify, batch) that the README treats as one. Splitting `wrap_tool` (Stage 1) from `classify_decision` (Stage 2) and from `Batcher` removes the conceit that they share an abstraction.
- *Conjoined methods* — §4.2 "Confidence below `thresholds.confidence_floor` and `escalate=false` route to escalation anyway." The Stage-2 LLM verdict and the confidence floor are two outputs the caller cannot interpret independently; every caller has to know "if confidence low, treat as escalate." Pull this downward into the verdict itself: return one resolved `escalate|ratify` and hide the floor.
- *Information leak* — §6.4 `AuditEntry` exposes both `stage_1_result` and `stage_2_result` as separate nullable fields. The reader of the log must know which stage produced the verdict to interpret it. **Information hiding** is violated; one `verdict` plus one `produced_by` field is enough.
- *Over-generalization* — §4.7 "training-signal stream" is built but §4.7 also says "the library does not retrain." This is "we might need this someday" surfaced as a permanent API. **Strategic programming** says delete until calibration spec lands and re-introduces the seam from a real consumer.
- *Cognitive load (massive config)* — §6.3 policy YAML stacks four axes, four trigger lists, three thresholds, two optional sections, and two extension hooks. The reader must hold the whole tree before they can edit one trigger.
- *Pass-through method* — §5 `record_override` is a one-line wrapper around audit-log writing the caller already does. **Pass-through methods** add no behavior; either move it into `classify_decision` or delete it.
- *Errors not defined out of existence* — §7's "free-text answer ambiguity" row and Slice 4's §7 normalization round-trip describe the same failure handled in two specs. Eliminate at the boundary: classifier never accepts free text from a yes/no question (§4.1's `format` enum already constrains it; enforce structurally).

**Suggested edits.**

1. Split the library into three submodules — `gate` (Stage 1), `classify` (Stage 2 plus routing), `batch` — each with one externally visible function. The "library" name becomes a re-export.
2. Hide `confidence_floor` behind the verdict: `classify_decision` returns `Escalate` or `Ratify`, never a confidence number callers must threshold.
3. Delete `record_override` from §5; have `classify_decision` write the override record itself when the caller passes the operator's answer back via a single `resume(verdict, answer)` call.
4. Collapse `stage_1_result | stage_2_result` in §6.4 to one `verdict` plus `produced_by: gate|classifier`.

**Designed twice — worst offender: the public API in §5.**
*Alternative.* Two functions: `decide(decision, ctx) -> Verdict` and `resume(verdict, answer) -> Resolution`. `wrap_tool` becomes an implementation detail of `decide` (the gate runs as a precondition); `Batcher` is internal state hidden behind `decide`'s asynchronous return; `record_override` and `maybe_spot_check` are postconditions of `resume`.
*Score.* Interface simplicity: 2 functions vs. 7. Depth: each function hides Stage 1 + Stage 2 + batching + sampling vs. exposing them. Cognitive load: one mental model (propose, then resume) vs. five. Future maintenance: changing the spot-check rate or adding a stage doesn't touch callers.

## Slice 2 — Orchestrator Skeleton (`2026-04-25-orchestrator-skeleton-design.md`)

Central design move: a runtime-adapter trait plus a worktree-per-harness invariant lets one orchestrator process drive any container backend with git as the only inter-harness protocol.

**Red flags.**

- *Shallow trait with broad surface* — §5.1 `RuntimeAdapter` exposes eight methods: `spawn, exec, pause, resume, kill, attach, heartbeat, logs`. Every backend must implement all eight; most callers use only `spawn` and `heartbeat`. **Deep modules** prefers a small interface (e.g., `spawn` returning a handle that itself owns its lifecycle methods) hiding rich behavior. The current shape mirrors Scion's CLI surface — see §9 open question — which is the README's vocabulary leaking into the trait.
- *Obscure dependency* — §4.7 "On exceedance, the orchestrator pauses every harness on that feature and emits an escalation with the spend trace." The spend cap behavior is documented in `Components` but invoked from the audit-log writer (§4.5) and the runtime adapter (§4.7) in different ways depending on whether the meter lives in-orchestrator or in-container (§9 open question). This is an **obscure dependency** — the meter location changes who must catch the cap.
- *Conjoined methods* — §5.3 "control protocol" defines five outbound messages and seven inbound replies, none of which can be read without §4.4's worktree state machine and §6.3's transitions. Three sections to understand one handoff.
- *Configuration sprawl already starting* — §5.2 harness-config schema has 13 top-level keys spanning model, prompt, tools, skills, budget, hooks, lifecycle. Slice 5's cost-profile registry layers another override map on top (see cross-cutting). **Minimize cognitive load** is violated before the second slice lands.
- *Pass-through stubs* — §5.5 `EscalationClassifier` and §5.6 `ReviewGate` traits are defined here, then re-defined in Slice 1's §5 and Slice 4's §4.1 with different signatures. The trait in Slice 2 is a pass-through pretending to be a seam.
- *Errors handled across specs* — §7 row "Semantic merge conflict" says "Detection: cherry-pick fails or post-merge tests fail. Reconciliation logic deferred to slice 4." Slice 4's §4.4 detects overlap *before* merge. Two detection points for one error class. **Define errors out of existence**: pick the pre-merge detection (Slice 4) and remove the cherry-pick-fail recovery from Slice 2.

**Suggested edits.**

1. Reduce `RuntimeAdapter` to `spawn(spec) -> HarnessHandle`; move `pause/resume/kill/attach/heartbeat/logs` onto `HarnessHandle` so callers never touch the backend after spawn. Closer to Scion's actual idiom and shrinks every backend's conformance surface.
2. Make the spend meter authoritatively orchestrator-side per §9, then delete the open question; the in-container variant is dead-on-arrival because it trusts the container.
3. Delete the `EscalationClassifier` and `ReviewGate` Protocol stubs from Slice 2; import the real types from Slices 1 and 4. Slice 2 should not own seams it does not consume.

**Designed twice — worst offender: §5.1 `RuntimeAdapter`.**
*Alternative.* `spawn(spec) -> HarnessHandle`, where `HarnessHandle` is a context manager with `pause/resume/kill/attach/heartbeat/logs` as methods. Backend authors implement one constructor and one handle class, with `pause/resume` defaultable to `kill_redispatch` if the backend doesn't support live pause.
*Score.* Interface simplicity: one entrypoint vs. eight. Depth: handle hides the "is the container running, paused, attached" state machine. Cognitive load: caller never holds a `HarnessHandle` and a separate adapter at once. Future maintenance: a new lifecycle action (e.g., `checkpoint`) is one method on a handle, not a trait-wide change.

## Slice 3 — Specialized Harnesses (`2026-04-25-specialized-harnesses-design.md`)

Central design move: eight harness configs, each a YAML artifact, wired by a routing-emitted DAG with one rule (sequential within feature, parallel across).

**Red flags.**

- *God object: harness config* — §4 "Each harness ships one config file at `groves/<grove>/harnesses/<name>.yaml`. Fields: `system_prompt`, `tools_allowed`, `tools_denied`, `model`, `temperature`, `skills`, `budget` (tokens, wallclock, spend), `hooks` (pre/post tool, on-escalation), `worktree_layout`." That config plus Slice 2's §5.2 plus Slice 5's cost-profile override (`§5.6`) plus Slice 4's reviewer-specific allowlist all describe the same artifact at three layers of override. Three specs each modify the same schema. **Change amplification**: adding `temperature_decay` means edits in three places.
- *Pass-through harnesses* — §4.8 `docs` and §4.3 `architect` are flagged in §10 ("whether `docs` and `architect` survive contact with reality") as candidates for collapse. The spec leaves them in the DAG anyway. **Strategic programming**: a harness whose role is in question is a complexity bet against current evidence.
- *Conjoined harnesses* — §4.2 spec-writer and §4.3 architect "share inputs; their outputs differ in framing more than content" (§10). Two configs, one concept. **Conjoined methods** at the harness level.
- *Cognitive load* — §3 ASCII DAG shows 12 nodes for the heavy path; §5 enumerates light vs. heavy with a `planner-lite` mode whose status (separate harness vs. flag) is undecided in §10. The reader must hold three taxonomies (harness list, light/heavy, mode flag) to read one feature's flow.
- *Information leak across boundary* — §4.6 verifier "writes new failing tests committed back to the worktree." That puts the verifier into the implementer's commit graph; the reviewer in §4.7 must then disambiguate which commits came from which harness. The README's "every handoff is a git operation" stops being a clean handoff once two harnesses commit to the same worktree.
- *Errors handled in multiple specs* — §6 summarization gate filters prompt injection; Slice 1's §7 row "Prompt injection in proposed-decision text" says the classifier prompt distrusts decision text; Slice 4's reviewer also reads diffs that may contain injection. Three places handle the same threat. **Define out of existence**: the gate is the only place fetched text crosses into a privileged worktree; once that contract is enforced, downstream harnesses do not need defensive prompts.

**Suggested edits.**

1. Merge `spec-writer` and `architect` into a single `designer` harness with two output sections (acceptance criteria, decisions log). Keep the architecture-axis trigger; lose the second LLM call.
2. Demote `docs` from harness to a `tdd-implementer` skill invoked when doc tests reference shipped surface area. Two harnesses out of eight; -25% on the surface.
3. Forbid the verifier from writing to the implementer worktree; verifier writes to its own worktree, and a "failing test artifact" is the cherry-pick payload.
4. Make `planner-lite` a flag on `planner`, not a separate harness — the §10 question resolves to "mode" because replay isolation is cheap (config hash already captures the mode).

**Designed twice — worst offender: the eight-harness DAG in §3.**
*Alternative.* Five harnesses: `researcher`, `designer` (merges spec-writer + architect), `planner`, `tdd-implementer`, `verifier`. `reviewer` moves into Slice 4 where it lives. `docs` is a skill. The DAG flattens to: research → design → plan → (implement+verify per unit) → review.
*Score.* Interface simplicity: 5 configs vs. 8. Depth: `designer` hides the spec/decisions distinction that adds no review value. Cognitive load: one fewer taxonomy (no light/heavy/mode triple). Future maintenance: a new harness role (e.g., `security-reviewer`) joins as a peer instead of a fork in an already-dense DAG.

## Slice 4 — Review-and-Merge (`2026-04-25-review-and-merge-design.md`)

Central design move: a reviewer harness, a stacked-PR submitter, a Beads-shaped graph, and a surface-area detector that together hold operator review at constant cost as feature throughput grows.

**Red flags.**

- *God object: shared dependency graph* — §4.3 stores "Every in-flight feature with current pipeline phase. Every file path each feature has touched. `blocked-on` edges. Stack edges. Reviewer dispositions and operator decisions, joined to feature IDs." That is the orchestrator's runtime state plus the audit log plus the PR submitter's state. **Information hiding** says each owner should keep its own. The graph is a junk drawer pretending to be a coordination layer.
- *Change amplification* — §6.3 lists four edge types and four node types; §6.1 review-queue entry duplicates feature_id, intent, urgency from the graph; §6.2 stack-state record duplicates feature_id and PR URLs from the graph. One feature ID change touches three schemas.
- *Shallow class* — §5.3 stack submission API has four methods (`submit_stack, update_stack, rebase_stack, close_stack`). Three of the four are obviously implementable as a CRUD over `submit`'s return value. **Shallow class** masking what is fundamentally one operation per stack.
- *Conjoined methods* — §4.1 reviewer pipeline has five steps in mandatory order; §4.4 surface-area probe is step 5 but is also documented as "runs at the review queue, not at merge time" (§4.4) — i.e., not actually inside the reviewer pipeline. The reviewer harness and the surface-area detector cannot be understood independently.
- *Over-generalization* — §4.2 "two backends" (GitHub native, Graphite). §10 punts the default. Building two when zero are validated is **strategic programming** failure: pick one and ship.
- *Errors handled in multiple specs* — §8 row "Reviewer disagrees with verifier" says reviewer wins. Slice 3's §4.6 verifier "adversarial posture" implies verifier is the truth gate. Slice 5's §4.8 equilibrium detector watches verifier-acceptance climb. Three specs disagree about who arbitrates verifier output. **Define out of existence**: the reviewer is the merge gate; the verifier produces evidence the reviewer reads. Slice 3 should say so explicitly.
- *Configuration sprawl* — §6.4 conflict-reconciliation option set has six options the operator chooses from. Half overlap with the planner's job (`merge-into-one`, `extract-shared` are replans). The reviewer is generating planner inputs.

**Suggested edits.**

1. Split the shared graph: PR state lives in the submitter, surface claims live in the orchestrator, reviewer dispositions live in the audit log. Drop the SQLite mirror; expose three read views instead.
2. Reduce the stack submission API to `submit(stack) -> Stack`, where `Stack` owns `update`, `rebase`, `close` as methods on the returned object. Same shape as the Slice 2 fix.
3. Pick one stacked-PR backend (GitHub native is "in the substrate") and delete the trait. Add Graphite when a Grove asks for it.
4. Move conflict reconciliation option generation into the planner via Slice 3's surface-area output; the reviewer flags the conflict, the planner proposes options.

**Designed twice — worst offender: §4.3 shared dependency graph.**
*Alternative.* No graph. The audit log already records every feature's phase (Slice 2 §4.5). The submitter records PR state. Each planner emits surface-area spans (Slice 3 §4.4 implicit). A small `coordinate(feature_a, feature_b)` function reads from these three sources to answer "do these conflict, and is one blocked on the other." The Beads pattern stays as inspiration; the SQLite mirror disappears.
*Score.* Interface simplicity: one function vs. four CRUD shapes. Depth: hides the join across audit log + submitter + planner output. Cognitive load: zero new schemas; readers already know the audit log. Future maintenance: a new edge type (e.g., security-blocked) is a new query, not a schema migration.

## Slice 5 — Cost Mode + Drift Guard (`2026-04-25-cost-mode-and-drift-guard-design.md`)

Central design move: three artifacts (constitution, policy, reversibility wrappers) are anchored read-only-to-harnesses; everything else self-tunes against replay sets and A/B on live features.

**Red flags.**

- *Massive config* — §4.6 cost-profile registry adds a per-role override map *on top of* Slice 3's harness config *on top of* Slice 2's `ContainerSpec`. Three layers of YAML resolved by string merge. **Cognitive load**: a reader debugging why `verifier` ran with `model: sonnet` must trace three files.
- *Pass-through* — §4.1 metrics extractor is "a pure function `audit_log -> per_harness_metrics`." Pure functions are correct, but §5.2 schema duplicates fields the audit log already has (`harness, config_version, window`). The extractor is a pass-through of audit-log projections renamed.
- *Obscure dependency* — §4.7 drift anchors require "a CODEOWNERS rule routing them only to the operator; a CI check rejecting auto-generated PRs that touch them; and a runtime invariant in the orchestrator that refuses to start if the running anchor hashes do not match a signed manifest." Three enforcement layers in three different systems (git, CI, orchestrator). **Information hiding**: the orchestrator's hash check (§5.7) is the only one that matters at runtime; the other two are review-time hygiene that should live in the configs repo's README, not in this spec's component list.
- *Conjoined methods* — §4.5 A/B and §4.4 candidate-config evaluator are described separately but neither is invokable without the other (replay must run before A/B; A/B reads the eval report). §7 "Self-tuning protocol" walks through both as one workflow. Two components, one operation.
- *Over-generalization* — §4.8 equilibrium detector watches three signals; §10 admits "new pathologies will require new signals." Building a generic signal framework before two pathologies are observed is speculative. **Strategic programming**: hard-code the three signals; expose a hook when the fourth shows up.
- *Errors handled in multiple specs* — §8 row "Silent calibration decay" recovers by "Reset that category's thresholds to defaults; freeze auto-tuning." Slice 1's §7 row for the same failure says the same thing in different words. **Define out of existence** at one boundary: Slice 1 owns the spot-check and the reset; Slice 5 reads the audit log to *report* drift but does not *act*.

**Suggested edits.**

1. Merge §4.4 candidate-config evaluator and §4.5 A/B harness into one `evaluate(candidate)` function whose return value is `recommendation: promote | reject | needs_more_data`. The replay vs. A/B distinction is internal.
2. Delete §4.1 metrics extractor's separate output schema (§5.2). Consumers query the audit log; the projections are SQL views.
3. Move the "calibration decay" recovery row from Slice 5 §8 into Slice 1's §7; Slice 5 should not own that recovery path.
4. Pick the orchestrator hash check (§5.7) as the single enforcement and drop the other two layers from the component list (they are operational discipline, not architecture).

**Designed twice — worst offender: §4.6 cost-profile registry.**
*Alternative.* Cost profiles are not a separate registry; they are tags on the harness config (e.g., `groves/<grove>/harnesses/planner.caveman.yaml`). Resolution is `<role>.<profile>.yaml || <role>.yaml`. No override merge logic. Sweeping profiles is `for profile in profiles: run(features, profile)`.
*Score.* Interface simplicity: file-based vs. nested override map. Depth: zero merge code to read or test. Cognitive load: one config file to inspect per (role, profile). Future maintenance: a new profile is a new file, not a schema change.

## Slice 6 — Dark Variants (`2026-04-25-dark-variants-design.md`)

Central design move: the same orchestrator runs two dark paths — a formal-spec/oracle path that promotes the verifier, and a principal-agent path that calibrates a model on the audit log — and a per-feature mode selector composes them.

**Red flags.**

- *Conjoined methods* — §3 architecture diagram shows the mode selector forking into two pipelines that share the verifier, reviewer, classifier, reversibility gates, and audit log. The two paths are not symmetric: formal-spec changes the verifier's role (§4.5 "verifier promoted"); principal-agent changes the classifier's downstream (§5.1 principal substitutes for the operator). One spec describes two distinct architectural changes. **Conjoined**: the formal-spec path can ship without the principal; the spec wires them as one slice.
- *Shallow harness* — §4.1 oracle harness "does not delete; supersession is by new entries" plus §4.2 conformance/diff acceptance gate. The oracle is a thin wrapper over a probing strategy and a corpus store; the gate is a thin wrapper over a diff. Two components, neither hiding much. **Deep modules**: collapse to a single `oracle` harness whose output is the diff, not the corpus.
- *Information leak* — §5.1 principal model artifact "Stored as a versioned artifact (same lifecycle as the constitution and policy file)." But §5.4 also says "the principal applies them but doesn't rewrite them." If the principal lifecycle mirrors the constitution, what enforces "applies-not-rewrites" structurally? The §5.4 invariants ("read-only to the principal", "reversibility gates not gated on the principal") are runtime checks. **Define errors out of existence**: store the principal in a directory the principal's container is mounted into read-only; rewrite becomes filesystem-impossible.
- *Over-generalization* — §6 hybrid mode: "formal-spec inside, principal-agent at the boundary." Per-file? Per-call-site? Per-commit? §11 admits the boundary is open. Specifying hybrid before either single-mode path has run is **strategic programming** debt.
- *Cognitive load* — §3 architecture text holds: two paths, one mode selector, four anchor invariants, three failure-mode rows specific to dark mode, plus the entire slice-1-5 surface as inheritance. The reader holds the whole stack to read this slice.
- *Errors handled in multiple specs* — §9 row "Spec ambiguity" detects via "normative-keyword preprocessor". Slice 1's §4.2 LLM classifier is supposed to detect the same ambiguity by reading the constitution. Two detectors, one signal.

**Suggested edits.**

1. Split this slice into 6a (oracle + formal-spec) and 6b (principal). The two paths share machinery from Slices 1-5, not from each other; bundling them into one slice creates coupling that does not exist in the architecture.
2. Make the principal a container with the constitution and policy mounted read-only. Drop the §5.4 list of runtime invariants.
3. Delete §6 hybrid mode from this slice; reintroduce when one path has shipped.
4. Move "spec ambiguity" detection into Slice 1's classifier prompt; Slice 6 references it rather than redefining a preprocessor.

**Designed twice — worst offender: §6 mode selector + hybrid.**
*Alternative.* No selector. Each feature is tagged at intake with `mode: formal-spec | principal | hybrid`. Default is `principal`. Hybrid is "this feature has a formal-spec sub-feature" expressed as a parent-child relationship in the planner's output, not as a runtime branch.
*Score.* Interface simplicity: a feature tag vs. a selector component. Depth: zero new code; planner already decomposes. Cognitive load: one mode per feature, never per call-site. Future maintenance: a new mode is a new tag value, not a selector rewrite.

## Cross-cutting

**Inter-spec interface bloat.**

- *Audit-log entry shape* is defined in Slice 1 §6.4, Slice 2 §5.4, and Slice 5 §5.2 with overlapping but non-identical fields. Slice 2 is the writer; Slices 1, 4, and 5 are readers. The writer's schema should be authoritative; the others should be projections (views), not redefined types. **Information hiding** at the audit-log boundary.
- *`RequestHumanInput`* is defined in README §6.3 and Slice 1 §6.1, then consumed verbatim by Slice 4 §4.5 (review queue), Slice 6 §7 (principal). One definition, four citations is fine; what is not fine is Slice 4 adding `stack_level, surface_peers, diff_url` to the queue entry (§6.1) without saying whether those compose with `RequestHumanInput` or replace it. Decide and document.
- *Harness config schema* sprawls across Slices 2, 3, and 5. **Change amplification** confirmed; see Slice 3's first red flag. Single source: Slice 2's §5.2 with Slice 3 contributing per-role required fields and Slice 5 contributing override resolution.

**Concepts in 3+ specs.**

- *Worktree* (Slices 2, 3, 4) — owned by Slice 2; Slices 3 and 4 should reference, not redescribe.
- *Surface area* (Slices 3 §4.4, Slice 4 §4.4, Slice 5 implicitly via cost-profile sweeps) — owned by Slice 4 (it has the most precise representation, AST/LSP spans); Slice 3 emits the data into Slice 4's schema.
- *Drift* (Slices 1, 4, 5) — Slice 1 detects in policy via spot-check, Slice 4 detects in reviewer config via false-positive blocks, Slice 5 detects in equilibrium across roles. Three detectors, three response paths. Consolidate into Slice 5's drift guard with sub-detectors per layer.
- *Constitution conflict* (Slices 1, 3, 4) — Slice 1's classifier checks, Slice 3's spec-writer self-checks, Slice 4's reviewer checks. Three checkers. Pick one (the reviewer) as authoritative; the others escalate to it.

**Errors handled at multiple boundaries (candidates for "define out of existence").**

| Error | Spec(s) handling | Resolution |
|---|---|---|
| Prompt injection from fetched content | Slice 3 §6 (gate), Slice 1 §7 (classifier prompt), Slice 4 §4.1 (reviewer reads diffs) | Slice 3 gate is the only place untrusted text crosses into a privileged context; downstream specs reference, do not defend. |
| Constitution conflict | Slice 1 §7, Slice 3 §4.2, Slice 4 §4.1 | Slice 4 reviewer authoritative; Slice 1 escalates on confidence; Slice 3 self-check removed. |
| Calibration decay | Slice 1 §7, Slice 5 §8 | Slice 1 owns the recovery; Slice 5 reports. |
| Semantic merge conflict | Slice 2 §7, Slice 4 §4.4 / §4.6 | Slice 4 detects pre-merge; Slice 2's cherry-pick-fail row deleted. |
| Verifier false-green from co-evolution | Slice 3 §4.6, Slice 5 §4.8 (equilibrium) | Slice 5 owns; Slice 3 references. |

**Configuration sprawl.**

Distinct config schemas across the six specs:
1. `constitution.md` (Slice 1, parsed)
2. `policy.yaml` (Slice 1)
3. `harness.yaml` per role (Slice 2 schema, Slice 3 per-harness extensions)
4. `ContainerSpec` (Slice 2, derived from harness.yaml)
5. Cost-profile YAML (Slice 5 §5.6)
6. Replay-set spec YAML (Slice 5 §5.3)
7. Reviewer regression-set fixture (Slice 4 §9)
8. Beads/SQLite shared graph schema (Slice 4 §6.3)
9. Oracle behavioral corpus schema (Slice 6 §8)
10. Principal calibration set schema (Slice 6 §8)
11. Mode-selection record (Slice 6 §8)
12. Conflict-reconciliation option set (Slice 4 §6.4)

Twelve schemas. Recommendation: collapse to four — constitution, policy, harness (with cost-profile as a tag, not a layer), and audit log (everything labeled, queryable, no separate replay/calibration/corpus tables). Slice 4's review-queue entry, Slice 6's mode-selection record, and the equilibrium signals are all audit-log event types, not separate schemas.

**Biggest single complexity reduction available.**

Treat the audit log as the only persistent data structure across slices. Today the audit log is one schema (Slice 2 §5.4) plus eleven adjacent schemas that are either projections of it (metrics, replay sets, calibration sets) or runtime state that should be in it (review-queue entries, surface claims, mode-selection records, conflict options). Eliminating the adjacent schemas pulls complexity downward into the audit-log writer (Slice 2's author suffers) and out of every reader (Slices 1, 4, 5, 6 simplify). It collapses the cross-cutting "concepts in 3+ specs" list (every concept becomes a query), eliminates the shared dependency graph (Slice 4 §4.3) entirely, and resolves the "errors handled in multiple specs" table by giving every detector one place to write its signal and every reactor one place to read it. The constraint stays the same — the constitution, policy, and reversibility wrappers remain anchors — but the rest of the system stops inventing schemas to coordinate around them.

If only one edit ships from this audit, ship that.
