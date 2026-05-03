# Harness Roster

Reference table for every harness in the darken pipeline. Read alongside README §5.1 (sub-harness descriptions) and §5.7 (harness configuration as code), and `docs/superpowers/specs/2026-04-26-harness-and-image-configuration-design.md` §3.1 (backend matrix). All fields are sourced from `.scion/templates/*/scion-agent.yaml`.

The `designer` entry collapses the `spec-writer` and `architect` roles from README §5.1 — both produced specs with structural decisions; one harness does it more efficiently. The single `planner` from the original README has been split into four tiers (`planner-t1`..`planner-t4`); the routing classifier picks the tier (see `.design/pipeline-mechanics.md` §9 and spec §8). `darwin` is the post-pipeline evolution agent that emits operator-gated YAML recommendations under `.scion/darwin-recommendations/` for `darken apply`.

## Roster

| Role | Backend | Model | Max turns | Max duration | Detached | Escalation-axis affinity | One-line role |
|---|---|---|---|---|---|---|---|
| `orchestrator` | claude | claude-opus-4-7 | 200 | 4h | false | All axes (classifier runs here) | Runs the §7 loop; dispatches sub-harnesses; batches escalations; the operator's only handle — routes everything |
| `researcher` | claude | claude-sonnet-4-6 | 30 | 1h | false | — | Sandboxed web access; produces compressed briefs; cheap recon, no skills bundled |
| `designer` | claude | claude-opus-4-7 | 50 | 1h | false | Architecture, Taste | Spec author — converts intent + research into a spec with structural decisions and tradeoffs |
| `planner-t1` | claude | claude-sonnet-4-6 | 15 | 30m | false | — | Lightweight ad-hoc planner; think-then-do; small bug fixes; no plan doc |
| `planner-t2` | claude | claude-opus-4-7 | 30 | 1h | false | — | Claude-code-style mid planner; light plan doc; multi-file but bounded |
| `planner-t3` | claude | claude-opus-4-7 | 50 | 2h | false | Architecture | Superpowers full planner — design + detailed plan; escalates taste/ethics/reversibility (default for ambiguous routing) |
| `planner-t4` | codex | gpt-5.5 | 100 | 4h | false | All axes (constitution-gated) | Spec-kit constitution-driven planner; full ratification: constitution + spec.md + plan.md + tasks/ |
| `tdd-implementer` | claude | claude-sonnet-4-6 | 100 | 2h | false | — | Failing-test-first code; refuses production code without a failing test |
| `verifier` | codex | gpt-5.5 | 50 | 2h | false | — | Adversarial second-vendor execution; runs tests, edges, fuzzing; loops back to implementer up to N times |
| `reviewer` | codex | gpt-5.5 | 30 | 1h | false | All axes (constitution enforcer) | Cross-vendor block-or-ship review; senior-engineer pass against the claude implementer |
| `sme` | codex | gpt-5.5 | 10 | 15m | false | Architecture, Taste | Focused single-question SME; rejects poorly-formed questions |
| `admin` | claude | claude-haiku-4-5 | 100 | 8h | true | — | Append-only narrative chronicle; observer-only; forbidden from touching the audit log |
| `darwin` | codex | gpt-5.5 | 50 | 4h | false | — | Post-pipeline evolution agent; reads completed sessions and emits YAML recommendations for `darken apply` |

Counts: claude × 8, codex × 5. Pi and gemini are not pinned defaults; they are sub-in overrides invoked at spawn time via `scion start --harness <backend> --image local/darkish-<backend>:latest` (spec §3.2).

## Capacity and summoning notes

Scion has no built-in `single_use`, `summoned`, or `spawn_limit` fields. Every constraint on how often a harness is started — rate limits, per-feature caps, maximum concurrent instances — is state the orchestrator maintains and enforces itself. `detached: true` means the process runs in the background and does not block the orchestrator's monitor loop; `admin` is the only detached harness. The convention that `sme` is summoned for one question per dispatch and then stopped is enforced by the orchestrator's own rules, not by anything Scion guarantees. If the orchestrator fails mid-run, Scion does not prevent a new orchestrator instance from spawning duplicate sub-harnesses; preventing that is the orchestrator's responsibility via the audit log and worktree ownership check (README §5.5).

`darwin` never mutates harness state directly — it writes recommendations only. Application is gated by `darken apply`, which presents each recommendation to the operator (`y/n/skip/edit`) before mutating manifests, committing changes, or re-staging skills (spec §12.4).
