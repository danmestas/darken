# Remote Human Comms — Deferral & Envelope Interoperability

**Status:** approved (in-conversation, 2026-04-28)
**Author:** dmestas
**Source PRs:** TBD
**Related specs:**
- `2026-04-28-bones-inter-harness-comms-design.md` (consumes the envelope defined here)
- `2026-04-28-roles-bases-template-collapse-design.md`

## Context

Eventually Darkish Factory operators will want remote human notifications — "agent X is blocked, ping me on my phone." A naïve build of this from scratch would duplicate primitives that the Scion substrate already ships:

| Need | Scion primitive (already exists) | Reference |
|---|---|---|
| Persistent message tray for humans | Hub Inbox Tray + SSE | `hub-user/messaging/` |
| External notification | Discord webhook integration | `hub-admin/hub-server/` (Discord Integration) |
| "Agent needs human input" semantics | `ask_user` tool → state flips to `WAITING_FOR_INPUT` + persistent message | `hub-user/messaging/` |
| Programmatic delivery backend | Message Broker Plugin (`hashicorp/go-plugin` + gRPC) | `concepts/`, `glossary/` |

Building our own remote-comms stack would compete with these. But adopting Scion's path requires going Hub-mode (Mode A) — OAuth, `GITHUB_TOKEN`, hub server, broker registration — none of which is justified by current operator volume (one human, host mode).

## Decision

**Don't build remote-to-human comms now.** Defer until either of these is true:

1. We have ≥2 human operators or one human running ≥2 machines that need a unified inbox.
2. We pivot to Mode A for an unrelated reason (e.g., remote-broker compute) — at which point Inbox/Discord come "free."

## What we DO commit to today

Make the message envelope used internally a **strict subset** of the shape Scion's Inbox/Discord adapter accepts. This means flipping the switch later is a transport swap, not a schema migration or a content-rewrite.

This work is small and lives entirely inside spec #2 (bones-inter-harness-comms). It is called out here to explicitly assign the responsibility.

## Envelope schema (target — also in spec #2)

```json
{
  "schema_version": "1",
  "id": "uuid-v4",
  "ts": "2026-04-28T18:32:11Z",
  "from": {
    "kind": "agent|orchestrator|operator",
    "name": "researcher-1",
    "role": "researcher",
    "grove": "darkish-factory"
  },
  "to": {
    "kind": "agent|orchestrator|operator|broadcast",
    "name": "planner-t1",
    "role": null
  },
  "severity": "info|warn|error|urgent",
  "kind": "report|question|status|ask_user|notification",
  "subject": "short headline (≤80 chars)",
  "body": "markdown-ok longer text",
  "actionable": true,
  "corr_id": "optional-thread-id",
  "links": [
    {"label": "PR #42", "url": "https://github.com/..."}
  ]
}
```

**Why this shape:**

- `severity` maps 1:1 to Discord color coding (info=blue, warn=yellow, error=red, urgent=red+mention) per Scion's documented behavior.
- `kind: ask_user` is the trigger for Scion's `WAITING_FOR_INPUT` state flip — preserve the term verbatim.
- `actionable: true` aligns with Inbox Tray "needs attention" filtering.
- `links` is forward-compatible with Inbox contextual links ("link directly to the agent that sent them").
- `from.grove` will already be populated when Mode A activates; harmless empty/static today.

## Non-goals

- Building a notification daemon, mobile app, push service, or webhook adapter ourselves.
- Writing a Discord webhook adapter (Scion ships one — we'd just configure `discord_webhook_url` if Mode A activates).
- Defining a richer envelope than Scion expects (we'd just have to trim it later).

## Migration trigger / re-open conditions

Re-open this spec and write an addendum when **any** of:

- Operator runs Darkish Factory across ≥2 machines and wants unified notifications.
- Mode A pivot lands (Hub server stood up for another reason).
- An agent flow needs `ask_user` semantics that block until a human responds — the file-based audit log is wrong substrate for that; promote to Inbox at that point.

Until then: bones (#2) writes envelopes that conform to this schema. If we ever export them to Scion, the export is a transport adapter, not a transform.

## Open questions

- **Threading**: Scion's Inbox Tray semantics for `corr_id` aren't documented in the public docs we scraped. Confirm via source or punt to Phase C of #2.
- **Attachments**: Do we want `attachments: [{path, mime}]` in the envelope today? Lean no; revisit if a worker ever needs to ship a binary diff or screenshot to a human.
