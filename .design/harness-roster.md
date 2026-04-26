# Harness Roster

Reference table for every harness in the Darkish Factory pipeline. Read alongside README §5.1 (sub-harness descriptions) and §5.7 (harness configuration as code). All fields are sourced from `.scion/templates/*/scion-agent.yaml`.

The `designer` entry collapses the `spec-writer` and `architect` roles from README §5.1 — both produced specs with structural decisions; one harness does it more efficiently.

## Roster

| Role | Model | Max turns | Max duration | Detached | Escalation-axis affinity | One-line role |
|---|---|---|---|---|---|---|
| `orchestrator` | claude-opus-4-7 | 200 | 4h | false | All axes (classifier runs here) | Runs the §7 loop; dispatches sub-harnesses; batches escalations to the operator |
| `researcher` | claude-sonnet-4-6 | 30 | 1h | false | — | Sandboxed web access; produces compressed briefs; no write access to privileged worktrees |
| `designer` | claude-opus-4-7 | 50 | 1h | false | Architecture, Taste | Converts intent + research into a spec with structural decisions and tradeoffs |
| `planner` | claude-opus-4-7 | 30 | 30m | false | Architecture | Decomposes the spec into a stacked sequence of units with file paths and test strategy |
| `tdd-implementer` | claude-sonnet-4-6 | 100 | 2h | false | — | Writes a failing test first, then code; refuses production code without a failing test |
| `verifier` | claude-sonnet-4-6 | 50 | 1h | false | — | Adversarial posture; runs tests, edges, fuzzing; loops back to implementer up to N times |
| `reviewer` | claude-opus-4-7 | 30 | 30m | false | All axes (constitution enforcer) | Senior-engineer pass; enforces the constitution; can block before anything reaches the operator |
| `sme` | claude-opus-4-7 | 10 | 15m | false | Architecture, Taste | Summoned for one focused software-engineering question; rejects poorly-formed questions |
| `scribe` | claude-haiku-4-5-20251001 | 100 | 8h | true | — | Observer-only; writes append-only narrative chronicle; forbidden from touching the audit log |

Base template defaults (inherited by all roles unless overridden): model `claude-sonnet-4-6`, max\_turns 50, max\_duration 3600s. The `orchestrator`, `designer`, `planner`, `reviewer`, and `sme` all override model upward to `claude-opus-4-7`.

## Capacity and summoning notes

Scion has no built-in `single_use`, `summoned`, or `spawn_limit` fields. Every constraint on how often a harness is started — rate limits, per-feature caps, maximum concurrent instances — is state the orchestrator maintains and enforces itself. `detached: true` means the process runs in the background and does not block the orchestrator's monitor loop; `scribe` is the only detached harness. The convention that `sme` is summoned for one question per dispatch and then stopped is enforced by the orchestrator's own rules, not by anything Scion guarantees. If the orchestrator fails mid-run, Scion does not prevent a new orchestrator instance from spawning duplicate sub-harnesses; preventing that is the orchestrator's responsibility via the audit log and worktree ownership check (README §5.5).
