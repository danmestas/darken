# Cost-mode experiments, harness evaluation, and drift guard

Slice 5 of the Darkish Factory. Covers §9 in full, plus the supporting machinery from §5.7, §6.2, §8 (policy drift row), and §12 (assumptions go stale).

## 1. Purpose & scope

The audit log records every decision; harness configs are versioned code. This slice consumes both to do four things:

1. Compute per-harness metrics from the audit log.
2. Replay historical decisions against candidate configs before promoting them.
3. Run cost-profile experiments to produce a spend-vs-quality curve per project or feature type.
4. Hold three artifacts — the escalation classifier policy, the constitution, the deterministic reversibility gates — anchored to operator-authored ground truth so the rest of the population cannot drift into a self-endorsed equilibrium.

Without this slice, "improvement" is vibes (§9). With it, every harness change is itself a feature that flows through the pipeline that ships application code.

## 2. Out of scope

Each handled by a sibling spec:

- Escalation classifier internals and policy file authoring — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`
- Orchestrator core, audit-log schema and writer — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`
- Specialized harness configurations under evaluation — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Reviewer harness, stacked PRs, surface-area conflict detection — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md`
- Dark variants and principal calibration — `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md`

## 3. Architecture

```
            operator-authored ground truth
            +-------------------------------------+
            | constitution.md                     |
            | escalation-policy.yaml              |
            | reversibility-gates (tool wrappers) |
            +-------------------------------------+
                       |  read-only to all harnesses; mutation = operator PR
                       v
+---------------+   +-------------------+   +----------------------+
|  audit log    |-->| metrics extractor |-->| per-harness metrics  |
|  (slice 2)    |   +-------------------+   +----------------------+
|               |        |
|               |        v
|               |   +-------------------+   +----------------------+
|               |-->| replay-set builder|-->| replay set (pinned)  |
+---------------+   +-------------------+   +----------------------+
                                                   |
                                                   v
   +----------------+    +---------------------+   +-----------------+
   | candidate cfg  |--->| replay engine       |-->| eval report     |
   | (PR'd file)    |    +---------------------+   +-----------------+
   +----------------+                                       |
                                                            v
                                                  +-----------------+
                                                  | A/B on live     |
                                                  | features        |
                                                  +-----------------+
                                                            |
   +----------------+    +---------------------+   +-----------------+
   | cost-profile   |--->| cost-profile runner |-->| spend/quality   |
   | registry       |    +---------------------+   | curve           |
   +----------------+                               +-----------------+

   +----------------+
   | equilibrium    |  reads metrics + audit log; emits signals
   | detector       |
   +----------------+

   +----------------+
   | policy-diff    |  cadence-driven; surfaces ground-truth diffs
   | reviewer       |  to operator
   +----------------+
```

All optimization happens below the dashed line of the three anchors. The anchors mutate only via operator PR; that is the architectural guarantee.

## 4. Components

### 4.1 Metrics extractor

**Inputs:** queries over the unified audit log (Slice 2) — `decisions`, `merges`, and operator-action events filtered by `harness`, `config_version`, and time window.
**Outputs:** structured query results, materialized on demand. No new persistent schema.

A pure function `audit_log_query -> per_harness_metrics`. Stateless; re-runs deterministically over any time window. Computes, per harness role and per harness config-version:

- Escalation rate (escalations / proposed decisions).
- Escalation-by-category distribution across `taste | architecture | ethics | reversibility`.
- Rework rate (decisions that were ratified, then rolled back or contradicted by later operator action).
- Defect attribution (post-merge defects traced back via audit log to the harness whose decision introduced them).
- Tokens per shipped unit (sum of harness token spend / units of work delivered).
- Wall-clock per phase (start-to-handoff timestamps).

Downstream consumers run their own queries; this component is a library of canonical projections, not a store.

### 4.2 Replay-set builder

**Inputs:** audit-log queries selecting decisions and pinned-input references by `harness`, time window, category, or feature.
**Outputs:** a versioned YAML artifact in the configs repo (committed to git). No new database.

A replay set pins:

- The decisions to be replayed (a list of audit-log event references resolved at run time).
- The exact inputs to each decision (prior-phase artifacts, fetched briefs, file diffs, constitution version, policy version) — referenced from blob storage already addressed by audit-log events.
- The model identifier and sampling parameters used originally.
- An RNG seed where the harness exposes one; a documented non-determinism budget where it does not.

### 4.3 Replay engine

**Inputs:** a replay-set artifact (§4.2) and a candidate-config PR.
**Outputs:** `candidate_evaluation` events written to the unified audit log, one per pinned decision plus an aggregate roll-up.

For each pinned decision, instantiates the candidate config in a clean container, supplies the pinned inputs, captures the new proposed decision, and diffs against the historical proposal and outcome. The engine does not write to live worktrees; it operates in throwaway groves.

### 4.4 Candidate-config evaluator

**Inputs:** audit-log queries over `candidate_evaluation` events emitted by §4.3, joined with §4.1 metrics queries scoped to the replay window.
**Outputs:** a `candidate_recommendation` event in the audit log carrying `promote | reject | needs A/B`.

Aggregates the per-decision diff stream into a recommendation. Comparison is incumbent vs candidate over the same replay set.

### 4.5 A/B harness on live features

**Inputs:** a candidate config that passed §4.4; live features tagged for assignment.
**Outputs:** `ab_assignment` events at split time and `ab_readout` events at power.

Once replay is favorable, the candidate runs against a fraction of live features alongside the incumbent. Feature assignment is a deterministic hash of the feature ID modulo the split. Run continues until pre-declared statistical power is reached on the primary metric (typically rework rate or escalation rate). The reviewer harness still gates merges; A/B does not loosen safety, only chooses among configs that already pass the gates.

### 4.6 Cost-profile registry

**Inputs:** none — the registry *is* the input. Cost-profile YAML files are versioned config in the configs repo.
**Outputs:** consumed by the orchestrator at dispatch; selection per feature is emitted as `cost_profile_selected` events in the audit log.

A profile is a per-role override map: model identifier, context budget, max tokens per call, temperature, optional skill allowlist trim. The orchestrator resolves a feature's effective harness configs as `base_config + cost_profile_override`. Profiles are first-class citizens of the "configs are code" stance.

A cost-profile runner sweeps a labeled feature set across a list of profiles by issuing audit-log queries over the resulting `cost_profile_selected` and outcome events; it emits a spend-vs-quality curve: rework rate, defect tail, escalation rate per dollar. The operator selects a point on the curve per project or feature type.

### 4.7 Drift-guard anchors

Drift-anchor enforcement now lives in the principal-agent path of /Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md, where the principal container mounts the constitution and policy file read-only — making 'applies-not-rewrites' a filesystem fact rather than runtime checks. This slice consumes the anchors as inputs, not enforcement logic.

### 4.8 Equilibrium detector

**Inputs:** audit-log queries over verifier-acceptance events, post-merge defect events, escalation-by-category events, and 5%-spot-check override events.
**Outputs:** `equilibrium_signal` events in the audit log; each signal triggers §4.9.

A self-improving population can reach a joint equilibrium that neither the operator nor reality endorses (§9). The detector watches three signals:

- **Mutual-acceptance climb.** Verifier acceptance rate of planner output rising while post-merge defect rate is flat or rising. Plans the verifier likes increasingly fail in production.
- **Escalation-rate collapse on architecture/taste.** Sharp decline in those two categories without a corresponding policy change suggests the classifier has learned to stop firing rather than that decisions got better.
- **5%-spot-check delta.** §6.2 already requires re-surfacing 5% of auto-ratified decisions; if operator overrides on the spot-check rise above a threshold, the detector flags systematic error and resets that category's thresholds to defaults (§6.2).

Any signal opens a policy-diff review.

### 4.9 Policy-diff reviewer

**Inputs:** audit-log queries — the anchors-as-inputs view (§4.7), `harness_config_version` events since last review, and metric deltas over the interval (per §4.1).
**Outputs:** `policy_diff_review` events in the audit log; each review surfaces to the operator as a single batched escalation.

Cadence-driven (per §8 policy-drift row). Runs on a fixed schedule and on any equilibrium-detector signal.

## 5. Interfaces

### 5.1 Audit-log read API

Read-only consumer of the schema slice 2 defines. This slice does not write the log. Required projections: `decisions(harness, timestamp, inputs_ref, proposal, classifier_outcome, operator_action, outcome_ref)` and `merges(feature, harness_config_versions, defect_refs)`.

### 5.2 Metrics: queries over the audit log

Per-harness metrics are not stored; they are queries over the unified audit log defined in Slice 2. The metrics extractor projects fields the audit log already records (`harness`, `config_version`, `timestamp`, `category`, `operator_action`, `outcome_ref`) into the aggregates listed in §4.1. A query against the log over a window returns the equivalent of:

```yaml
# Result of a query, not a stored schema.
harness: planner
config_version: a1b2c3d
window: 2026-03-01..2026-04-01
escalation_rate: 0.18
escalation_by_category: {taste: 0.04, architecture: 0.11, ethics: 0.01, reversibility: 0.02}
rework_rate: 0.07
defect_attribution: {count: 3, refs: [...]}
tokens_per_unit: 41200
wall_clock_p50_min: 6.2
sample_size: 412
```

Consumers compose these queries on demand; nothing in this slice owns a separate metrics table.

### 5.3 Replay-set: versioned artifact in git

A replay set is a versioned YAML artifact committed alongside the configs it evaluates — not a row in a new database. Its `decisions[]` field is a list of audit-log event references; the replay engine resolves them by querying the audit log at run time. Example:

```yaml
# Versioned file under /replay-sets/<id>.yaml in the configs repo.
id: plan-arch-2026q1
selector: {harness: planner, category: architecture, window: 2026-01-01..2026-04-01}
decisions: [<audit-log refs>]
pinned_inputs_ref: blob://...
model_id: claude-opus-4-7
constitution_version: 7f3e
policy_version: 2a91
non_determinism_budget: "temperature=0.2; n_samples=3 per decision; majority-vote diff"
```

### 5.4 Candidate-config submission

A standard PR against the configs repo. CI runs the replay engine against the most recent matching replay set; the eval report attaches as a PR comment and is also written as a `candidate_evaluation` event to the audit log. Anchor files are owned by the principal-agent path (see §4.7 cross-reference).

### 5.5 A/B result: event type in the audit log

A/B feature assignments and readouts are emitted as `ab_assignment` and `ab_readout` events in the unified audit log. Consumers query the log to see incumbent vs candidate trajectories. A readout query returns the equivalent of:

```yaml
# Shape of an ab_readout event read from the audit log.
incumbent: a1b2c3d
candidate: e4f5g6h
features_assigned: 64
primary_metric: rework_rate
delta: -0.04
ci_95: [-0.07, -0.01]
power_reached: true
recommendation: promote
```

### 5.6 Cost-profile registry: input config

Cost profiles are versioned YAML files under the configs repo, one per named profile. They are *input config*, not stored state — the orchestrator reads them at dispatch time and resolves a feature's effective harness configs as `base_config + cost_profile_override`. Profile selection per feature is recorded as a `cost_profile_selected` event in the audit log.

```yaml
profile: caveman
overrides:
  researcher: {model: haiku, max_tokens: 4000, context_budget: 32000}
  planner:    {model: sonnet, max_tokens: 8000, context_budget: 64000}
  verifier:   {model: sonnet, max_tokens: 8000, context_budget: 64000}
```

### 5.7 Equilibrium and policy-diff outputs as audit-log events

The equilibrium detector (§4.8) emits `equilibrium_signal` events; the policy-diff reviewer (§4.9) emits `policy_diff_review` events bundling the relevant audit-log queries and metric deltas as references. Neither component owns a separate table; both are producers of audit-log event types defined in Slice 2.

## 6. Data model

This slice introduces **no new persistent schemas**. Everything below is either a query over the unified audit log (Slice 2) or a versioned artifact in the configs repo:

- **Replay set** — versioned YAML artifact in git: `{id, selector, decisions[], pinned_inputs_ref, model_id, anchor_versions, non_determinism_budget}`.
- **Cost-profile entry** — versioned YAML artifact in git (input config): `{profile, overrides{role: {...}}, created_by, created_at}`.
- **Per-harness metrics** — query result over audit log; no storage.
- **Candidate-config evaluation report** — query result joining a replay-set artifact with `candidate_evaluation` events; persisted only as the event itself.
- **Equilibrium-detection signal** — `equilibrium_signal` event type in the audit log.
- **Policy-diff record** — `policy_diff_review` event type in the audit log.

## 7. Self-tuning protocol

A harness change is a feature (§9). End-to-end:

1. **Spec the change.** A spec describes the harness config diff, the hypothesis, and the success metric (e.g., "reduce planner rework rate by ≥3pp without raising escalation rate").
2. **Plan.** Decompose into the config edit, the replay-set selection, and the A/B plan.
3. **Implement the diff.** PR against the configs repo.
4. **Verify by replay.** CI runs §4.3 against the chosen replay set; eval report attaches.
5. **A/B against incumbent.** §4.5 on live features until power is reached.
6. **Review.** Reviewer harness inspects the diff, the eval report, and the A/B result.
7. **Escalate the final call.** Even with green replay and A/B, the merge is an architectural decision and escalates per §6 — anchors require operator ratification by definition; non-anchor harness merges may be auto-ratified at the operator's discretion under the same policy that governs application code.

A harness with fewer than `min_decision_volume` historical decisions in the replay window cannot self-tune (§10 open question on what that number is).

## 8. Failure modes & recovery

| Failure | Detection | Recovery |
|---|---|---|
| Silent calibration decay (§6.2) | 5% spot-check operator-override rate climbs | Reset that category's thresholds to defaults; freeze auto-tuning for that category until next policy-diff review |
| Planner-verifier equilibrium | Equilibrium detector signals (§4.8) | Open policy-diff review; rotate one side to a snapshot config; re-baseline |
| Replay-set staleness from model-version turnover (§12) | Replay engine refuses to run if `model_id` is retired or if anchor hashes have moved | Rebuild replay set against current model; mark old runs unreproducible |
| Cost-profile gaming | Profile authors selecting feature labels that flatter their profile | Labeled feature sets are operator-curated; profile runs publish full per-feature results, not just aggregates |
| Drift-anchor tampering | §5.7 invariant assertion mismatch | Halt dispatch; high-urgency escalation; require operator-signed manifest update before resume |
| Auto-ratified regression in a category | Post-merge defect analysis (§8 README row "classifier misses an escalation") | Sweep recent auto-ratifications in that category; re-run them through the (now updated) classifier |

## 9. Testing & verification strategy

- **Replay-set golden runs.** Each replay set has an expected eval report under the incumbent config, recomputed on every CI run; drift in the golden run signals environmental non-determinism that must be budgeted or eliminated.
- **A/B statistical power.** Pre-declare effect size, primary metric, and minimum sample size per A/B. Refuse to read out before power is reached; refuse to assign more than the declared sample without re-declaring.
- **Equilibrium-detector synthetic-failure injection.** Periodically inject synthetic decision streams that simulate a known mutual-acceptance climb. The detector must fire within a bounded window. A miss is a P0 bug.
- **Drift-anchor mutation test.** CI pipeline mutates an anchor file in a throwaway branch and confirms the orchestrator refuses to boot. A pass means the architectural guarantee holds.
- **Spot-check audit replay.** §6.2's 5% spot-check is replayable: pulled samples re-run through the current classifier on each weekly cadence to confirm classifier behavior on decided cases is stable.

## 10. Open questions

- **Minimum decision volume per harness for statistically meaningful self-tuning** (§13). The replay engine and A/B harness need a floor; it is empirical and unset.
- **Classifier pathologies not yet observed** (§13). The equilibrium detector and the 5% spot-check are hedges, not proofs. New pathologies will require new signals.
- **Model-version turnover invalidating replay sets.** When the underlying model is retired or replaced, every pinned replay run becomes unreproducible. Open: how to bridge the discontinuity — re-baseline against the new model on a frozen feature set, run a hand-labeled gold set, or accept a reset of harness self-tuning history.
- **Cost-profile labeling.** Who labels feature sets for cost-profile sweeps, and how those labels are kept honest, is not yet specified.
- **A/B contamination across stacked PRs.** Stacked PRs mean a candidate config in level N can affect levels 1..N–1; isolation strategy is unresolved.
- **Cost of audit-log queries at high decision volume — is a materialized-view layer needed?** With every metric, evaluation report, and equilibrium signal expressed as a query rather than a stored projection, query cost may dominate at scale. Open whether (and where) to introduce materialized views without re-creating the schema sprawl this slice deliberately removed.

## 11. Cross-references

- `/Users/dmestas/projects/darkish-factory/README.md` — §5.7, §6.2, §8, §9, §12, §13.
- `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md` — classifier policy file consumed here as an anchor.
- `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md` — audit-log writer; this slice is a read-only consumer.
- `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md` — the harness configs evaluated here.
- `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md` — the reviewer harness that gates candidate-config merges.
- `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md` — the principal-agent calibration consumes per-harness metrics produced here.
