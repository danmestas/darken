# Dark Variants Design — Slice 6

Status: design
Scope: §11 of the README — going fully dark, formal-spec path, principal-agent path, and their composition.
Depends on: slices 1–5. Calibrated against the audit log those slices accumulate.

## 1. Purpose & scope

This slice specifies how the Darkish Factory runs without a human at the top of the pipeline or at the escalation surface. The README is explicit about when this is reachable: "if correctness is machine-checkable, the human isn't needed." Two paths get there. The formal-spec path uses an explicit specification, a behavioral spec scraped from a reference binary, or an internal spec extended from the constitution. The principal-agent path replaces the operator with a model calibrated on the operator's prior judgments.

The four-axis classifier (§6) collapses asymmetrically. In formal-spec mode, taste and architecture have been answered upstream by the spec or the reference binary; ethics and reversibility persist. In principal-agent mode, all four axes persist but are answered by the principal instead of the operator. In both modes the orchestrator, audit log, and reversibility gates from slices 1–5 stand unchanged. The README states it directly: "going dark does not reduce the safety surface, it just changes who signs each escalation."

This spec defines: oracle harness mechanics, conformance/diff acceptance, spec-ambiguity and sampling-gap detectors, principal model artifact, calibration pipeline drawing on the audit log, the spec-producing upstream contract, drift-anchor invariants enforcing applies-not-rewrites, and the per-feature mode selector that composes the two paths.

The README's posture on sequencing is load-bearing: "Don't build it first; let it emerge." This slice is meant to be built last, after slices 1–5 have produced enough audit-log volume to calibrate against.

## 2. Out of scope

Sibling specs own the following surfaces. This slice references them; it does not redefine them.

- Classifier internals — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`
- Orchestrator core, runtime adapter, audit log — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`
- Specialized harnesses (researcher, planner, verifier, reviewer, etc.) — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Review-and-merge surface — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md`
- Replay, metrics, drift guard, cost profiles — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`

## 3. Architecture

The two paths are independent. They share only the orchestrator, the unified audit log (Slice 2 §5.4), and the deterministic reversibility gates (§6.2 stage 1). A reader can read §3.1 without §3.2 (or vice versa); §3.3 is the only section that touches both.

**Resolution of the "axes collapse" wording.** README §11 says three axes collapse when going dark. §11.1 retains ethics and reversibility, so only two collapse, not three. In **formal-spec mode**, taste and architecture collapse — they are answered upstream by the spec or the reference binary; ethics and reversibility persist and continue to escalate (the conformance gate does not adjudicate them). In **principal-agent mode**, none of the four axes collapse; all four persist but are answered by the principal instead of the operator. README phrasing is overcounting; this slice treats the canonical count as two.

### 3.1 Formal-spec architecture

The formal-spec path promotes the verifier to primary acceptance gate. The oracle harness probes a reference binary or consumes an explicit conformance suite; the verifier diffs new implementations against that corpus.

```
            feature_intent (mode = formal-spec)
                          |
                          v
        researcher? — planner — tdd-implementer — verifier (promoted) — reviewer
                          |
                          v
                +-----------------+
                | oracle harness  |
                | (reference bin) |
                +--------+--------+
                         |
                         v
                +-----------------------+
                | conformance/diff gate |
                +-----------+-----------+
                            |
                            v
                +-----------------------+
                | reversibility gates   |
                | (deterministic)       |
                +-----------+-----------+
                            |
                            v
                +-----------------------+
                | unified audit log     |
                +-----------------------+
```

Ethics and reversibility escalations route to the standard classifier path (Slice 1) regardless of mode.

### 3.2 Principal-agent architecture

The principal-agent path replaces the operator at the escalation surface with a configured model calibrated on the audit log. The principal's container is the structural enforcement of "applies-not-rewrites": the constitution and policy file are mounted read-only; rewrite attempts fail at the filesystem layer, not at runtime checks.

```
            feature_intent (mode = principal-agent)
                          |
                          v
        researcher — designer — planner — tdd-implementer — verifier — reviewer
                          |
                          v
                +-----------------+
                | escalation      |
                | classifier      |
                +--------+--------+
                         |
                         v
                +-----------------+
                | principal       |
                | container       |
                | (RO mounts)     |
                +--------+--------+
                         |
                         v
                +-----------------------+
                | reversibility gates   |
                | (deterministic)       |
                +-----------+-----------+
                            |
                            v
                +-----------------------+
                | unified audit log     |
                +-----------------------+
```

The principal answers the same `ratify | choose | rework | abort` schema the operator would; the deterministic reversibility gates trip on the principal's signature exactly as they would on the operator's.

### 3.3 Composition

Both paths terminate at the same audit log and pass through the same deterministic reversibility gates. Per-feature mode is a tag on the `feature_intent` artifact (`formal-spec | principal-agent | hybrid`); §6 below details composition rules and the hybrid case.

## 4. Components — formal-spec path

### 4.1 Oracle harness

A sub-harness configured as a probe of a reference binary, not a writer of code. Inputs: a reference binary or running system, a probing strategy (handcrafted seeds, fuzzed inputs, replayed traces of operator usage), and a recording surface. Outputs: behavioral-corpus entries written as audit-log events of type `oracle.observation`, content-addressed by `(source_binary_hash, input)`. The corpus grows monotonically; entries are immutable. The harness does not delete; supersession is by new entries. There is no separate corpus schema — corpus reads are queries over `oracle.observation` events in the unified audit log (Slice 2 §5.4).

The README phrases the goal: "the binary becomes a living RFC." The corpus is that RFC, materialized as audit-log events. It converges on a complete-enough spec for the subset that matters because the operator's actual usage patterns are the sampling distribution — replayed traces from the audit log feed the probing strategy.

### 4.2 Conformance/diff acceptance gate

The verifier runs corpus inputs (queried from the audit log) against the new implementation and diffs outputs and side effects. Discrepancies fall into one of three buckets:
- bug in new implementation — fix and re-run
- intentional deviation — escalate (ethics or reversibility may apply)
- nondeterminism in reference binary — record and exclude from diff with explicit annotation

For explicit-spec subdomains (RFC, POSIX, codec, compiler), the published conformance suite replaces or augments the corpus. The gate is the same: pass-or-fail diff, with deviations escalated.

### 4.3 Spec-ambiguity detector

The README is explicit that "MAY" clauses and "the spec is silent" cases are taste and architecture decisions in disguise. The detector is a deterministic preprocessor over explicit specs (regex over normative keywords: MAY, SHOULD, undefined behavior, implementation-defined) and a verifier-emitted signal for the binary subcase ("oracle has no entry for this input class"). Hits route to the escalation classifier with category `taste|architecture` even though the pipeline is in formal-spec mode.

### 4.4 Sampling-gap detector

Coverage instrumentation on the new implementation, intersected with the oracle corpus (queried from `oracle.observation` events), yields un-probed-but-reachable code paths. The README's mitigation: "treat any un-probed-but-reachable behavior as implicit escalation until sampled." The detector emits two signals: a coverage delta to the oracle (probe more) and an escalation signal when a reachable path remains unprobed past a configured threshold.

Coverage-driven probing and corpus fuzzing are oracle-side; the verifier consumes whatever the oracle produces.

### 4.5 Architect demotion / verifier promotion

Architect runs but its outputs are downgraded from acceptance gate to advisory. Most of its decisions were reducing ambiguity the spec already resolves. Verifier becomes the primary acceptance gate; the conformance suite or oracle diff is the green/red signal. Configuration changes are versioned harness diffs, reviewable as code.

## 5. Components — principal-agent path

### 5.1 Principal model artifact

A configured agent with four data slots: taste profile, architectural preferences, ethical bright lines, reversibility rules. Stored as a versioned artifact (same lifecycle as the constitution and policy file). The principal is invoked at the same surface the human is invoked at in slices 1–5: the output of the escalation classifier. It returns the same schema — `ratify | choose <option> | rework <direction> | abort`.

### 5.2 Principal container — read-only mounts of constitution and policy

The principal runs inside a container whose filesystem mounts make "applies-not-rewrites" a structural fact rather than a runtime invariant. The container spec:
- mounts `constitution.md` at a read-only path
- mounts `policy.yaml` at a read-only path
- exposes no write capability to either path; the kernel rejects writes at the syscall layer
- the principal model artifact itself is mounted from a separate read-only volume signed by the operator's release key

This replaces a runtime-check approach with a filesystem fact. There are no "is the principal trying to write to the constitution" guards because the operating system makes the question moot. Slice 5's drift guard is the **consumer** of these anchors (it reads the mount manifest to verify the running container matches the signed manifest); it is no longer the **enforcer**. The pre-edit version of this slice listed three enforcement layers (CODEOWNERS, CI, runtime hash check); the read-only mount collapses those to one structural property of the container plus one read of that property by the drift guard.

The principal's outputs are signed by an identity scoped to escalation answers only; that identity has no write capability to anchor paths anywhere in the system.

### 5.3 Calibration pipeline from audit log

The calibration "set" is not a separate schema — it is a query over the unified audit log (Slice 2 §5.4) filtered to `escalation.resolved` events with operator answers. Each result row materializes as `(escalation, context, operator answer, downstream outcome)`. The pipeline:
1. extracts decisions in each axis (taste, architecture, ethics, reversibility) from audit-log queries
2. holds out a temporal tail for evaluation
3. trains or fine-tunes the principal against the historical answers
4. reports per-axis agreement against the held-out tail

The minimum volume is open (§11 below). Below the threshold the principal stays in shadow mode: it answers, the human signs, the shadow answer is recorded as an audit-log event of type `principal.shadow_answer` alongside the operator's actual `escalation.resolved` event. There is no separate calibration-set table; agreement rates are SQL-style projections over those event types.

### 5.4 Spec-producing upstream system contract

The upstream system replaces the operator at the top of the pipeline. Contract:
- emits a `feature_intent` artifact with: free-text intent, target Grove, mode preference (`auto|formal-spec|principal-agent|hybrid`), priority, and provenance
- signs the artifact with an identity the orchestrator can authenticate
- accepts the same `ratify | choose | rework | abort` callbacks the operator would
- subject to the same reversibility gates and spend caps

The orchestrator treats it as a peer harness with a typed input interface, not a privileged caller.

## 6. Composition (§11.3)

Both paths share the orchestrator, audit log, and reversibility gates. §6 (classifier), §8 (failure modes), and §9 (evaluable artifacts) apply unchanged. Per-feature mode is a tag on `feature_intent`:
- explicit conformance suite or reference binary present → formal-spec
- spec absent, taste/architecture central → principal-agent
- both apply (a protocol implementation that also exposes a new API surface) → hybrid: formal-spec inside, principal-agent at the boundary

Hybrid features run formal-spec on the spec-bound subset and principal-agent on the rest. Mode selection is recorded as an audit-log event of type `feature.mode_selected` (not a separate schema), keyed by `feature_id` and queryable for retrospective analysis.

## 7. Interfaces

- **Oracle harness ↔ verifier.** Oracle writes `oracle.observation` events to the unified audit log; verifier queries those events by input shape and `source_binary_hash`. No separate corpus store.
- **Principal ↔ classifier.** Classifier emits the same `RequestHumanInput` payload (§6.3); the principal accepts it and returns the same answer schema. No new types.
- **Spec-producing upstream ↔ orchestrator.** `feature_intent` artifact with provenance and signature; one-shot or streaming, orchestrator-authenticated.
- **Audit log read for principal calibration.** A read-only audit-log view filtered to `escalation.resolved` and `principal.shadow_answer` events, partitioned by axis. The calibration pipeline is the only consumer with this view; it is a query, not a separate dataset.

## 8. Data model

This slice introduces no new persistent schemas. All dark-variant state lives as event types in the unified audit log defined by Slice 2 §5.4:

- **`oracle.observation`** — oracle harness output. Fields: `id, input, output, side_effects, observed_at, source_binary_hash, probe_strategy`. Content-addressed by `(source_binary_hash, input)` for deduplication; replaces the standalone corpus table from earlier drafts.
- **`principal.shadow_answer`** — recorded alongside operator answers during shadow mode. Fields: `decision_id, axis, context_ref, principal_answer, recorded_at`. Joined to the existing `escalation.resolved` event (which carries the operator answer and downstream outcome) by `decision_id` for held-out evaluation.
- **`feature.mode_selected`** — per-feature mode tag and rationale. Fields: `feature_id, mode, rationale, selector_confidence, override_by, timestamp`. Replaces the standalone mode-selection record.

Calibration sets, replay corpora, and mode-selection histories are all queries (or SQL views) over these event types — never separate tables. Slice 5's drift guard reads the same audit log to verify anchor-mount manifests; it does not own its own store.

## 9. Failure modes & recovery

| Failure | Detection | Recovery |
|---|---|---|
| Spec ambiguity (formal-spec) | normative-keyword preprocessor; oracle reports no entry | Escalate as taste/architecture even in formal-spec mode |
| Sampling gap | coverage minus oracle-corpus intersection | Probe more; escalate any un-probed-reachable path past threshold |
| Principal miscalibration | per-axis agreement on held-out tail below floor | Revert to operator-in-loop on that axis; resume calibration |
| Principal drift over time | rolling agreement window vs. recent operator overrides | Re-anchor against constitution/policy diffs; recalibrate |
| Mode-selection error | retrospective review of mode-selection record vs. outcome | Reclassify and recompute; selector retrained on the corrected label |
| Ethics/reversibility trip in dark mode | same triggers as §6.2 stage 1 | Unchanged — by design, these always trip; the principal cannot suppress |

The last row is load-bearing. The README states it: dark mode "does not reduce the safety surface."

## 10. Testing & verification strategy

- **Oracle-corpus coverage measurement.** Per feature, report `corpus_coverage = |reachable paths probed| / |reachable paths|`. Floor configurable; below floor the verifier escalates by sampling-gap rule.
- **Principal evaluated against held-out operator decisions.** Per-axis agreement, calibration error, and downstream defect attribution. Shadow mode runs the principal on every escalation and records its answer alongside the operator's; promotion to active is an explicit, auditable threshold crossing.
- **Mode-selection tested per feature class.** A labeled set of historical features replays the selector; misclassification rate is reported per class.
- **Reversibility-gate non-bypass test.** Adversarial features attempt to bypass reversibility gates via principal answers. The gates trip regardless of principal output. Test runs on every harness change.
- **Replay-set evaluation.** Per §9 of the README, principal candidates and oracle-harness configs are evaluated against historical replay sets before they touch live work.
- **Differential-oracle precedent (§12).** Red Hat's agent mesh, SmartOracle, AIProbe, and UnitTenX provide the validated pattern; this slice's oracle harness is the same pattern, configured against the operator's reference binaries.

## 11. Open questions

- **Minimum audit-log volume for a trustworthy principal (§13).** The README states it directly: "Going dark via §11.2 requires the principal to have seen enough operator judgments to act like the operator. The volume required is unknown." This slice cannot decide it; it accumulates the data and reports the held-out agreement curve.
- **When does mounting policy read-only become insufficient?** The principal-agent path (§5.2) makes the constitution and policy file read-only at the filesystem layer, which forecloses unauthorized rewrites. Open question: when does the principal need to *propose* edits to the constitution or policy — e.g., when sustained operator overrides reveal a missing rule — and how does that proposal route back to the operator? Candidate answer: the principal emits a `policy.amendment_proposed` audit-log event that lands in the operator's review queue exactly like any other escalation, with the proposed diff as the payload; the principal still cannot write the file. Open: should there be a confidence floor below which the principal cannot propose, and does sustained proposal volume itself become a drift signal?
- **When is a domain "machine-checkable enough" to use formal-spec mode?** The mode selector needs a falsifiable rubric. The README gives examples (RFC, POSIX, codec, compiler, internal binary) but no boundary. Empirical: route ambiguous features to hybrid and watch the formal-spec subset's escalation rate.
- **Hybrid-mode boundary definition.** Where does the formal-spec subset end and the principal-agent surface begin within a single change? Per-file? Per-call-site? Per-commit? Open.
- **Spec-producing upstream identity and trust.** What level of authentication does the upstream system need? What does revocation look like? Out of scope for this spec but on the boundary.
- **Sampling-gap floor.** What fraction of reachable behavior must be probed before the gate flips from escalate to ratify? Empirical, per domain.
- **Drift detection sensitivity.** How fast does principal drift show up in held-out agreement? The README's drift-guard mechanism is the anchor, but the detection latency is open.

## 12. Cross-references

- README — `/Users/dmestas/projects/darkish-factory/README.md`
- Classifier internals — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`
- Orchestrator skeleton — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`
- Specialized harnesses — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Review-and-merge surface — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md`
- Cost mode and drift guard — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`
