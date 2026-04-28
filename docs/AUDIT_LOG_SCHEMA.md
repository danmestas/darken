# `.scion/audit.jsonl` — schema

The orchestrator's audit log is a JSON-Lines file at `.scion/audit.jsonl` in any darken-init'd repo. One JSON object per line. Append-only.

The orchestrator-mode skill writes these entries; `darken history` reads + formats them.

## Required fields

| Field | Type | Description |
|---|---|---|
| `timestamp` | string (RFC3339) | When the decision was made |
| `decision_id` | string (UUID) | Globally unique per decision |
| `harness` | string | Which harness made the call (e.g. `orchestrator`, `darwin`) |
| `type` | string | Decision category — see Type values below |
| `outcome` | string | Terminal state — see Outcome values below |
| `payload` | object | Type-specific freeform data |

## Type values

| Value | When it fires | Payload fields |
|---|---|---|
| `route` | Routing classifier picks light/heavy/tier | `tier`, `confidence`, `reasons` (array) |
| `dispatch` | Orchestrator spawns a subharness | `target_role`, `agent_name`, `task` (truncated) |
| `escalate` | Stage-1 or Stage-2 classifier escalates | `axis` (taste/architecture/ethics/reversibility), `summary` |
| `ratify` | Decision auto-ratified (no operator involvement) | `axis`, `confidence` |
| `apply` | `darken apply` ratifies a darwin recommendation | `recommendation_id`, `target_harness` |

## Outcome values

| Value | Meaning |
|---|---|
| `ratified` | Auto-approved by classifier |
| `escalated` | Sent to operator for decision |
| `applied` | Operator approved + applied |
| `aborted` | Operator declined or system rolled back |

## Example

```jsonl
{"timestamp":"2026-04-28T07:14:32Z","decision_id":"uuid-1","harness":"orchestrator","type":"route","outcome":"ratified","payload":{"tier":"heavy","confidence":0.92,"reasons":["multi-module","schema-change"]}}
{"timestamp":"2026-04-28T07:14:35Z","decision_id":"uuid-2","harness":"orchestrator","type":"dispatch","outcome":"ratified","payload":{"target_role":"researcher","agent_name":"r1","task":"audit auth flow"}}
{"timestamp":"2026-04-28T07:18:01Z","decision_id":"uuid-3","harness":"orchestrator","type":"escalate","outcome":"escalated","payload":{"axis":"reversibility","summary":"propose drop populated table"}}
```

## Stability

The schema is **frozen at v1**. New types may be added; existing fields won't be removed or repurposed without a `schema_version` bump on individual entries. `darken history` skips entries with unknown types gracefully (one-line warning to stderr).
