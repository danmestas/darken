---
name: bones-tasks-swarm-coordination
description: Use when dispatching 2+ parallel subharnesses for related work; replaces orchestrator-side file-disjoint task partitioning with a claim-and-close work queue
---

# Bones Tasks Swarm Coordination

## Overview

When dispatching parallel implementers, the orchestrator usually has to partition tasks across them by file boundary so they do not trample each other. That gets fragile fast: tasks shift, file boundaries blur, and a partition mistake creates merge conflicts.

The bones tasks queue replaces orchestrator-side partition with worker-side **claim arbitration**. Each implementer pulls one open task, locks it, works it, closes it. The next worker picks the next open one. No partition decision required.

## When to Use

- Dispatching 3+ parallel subharnesses for related but independent work (Wave B, Wave C, bug-batch)
- Tasks could overlap files but the work itself is independent
- New backlog items arriving while a swarm is in flight (orchestrator can drop a task; live swarm picks it up next claim)
- Replanning a stalled or partially-failed swarm (untouched tasks remain claimable)

Skip when:
- Single task, single agent (no coordination needed)
- Tasks have hard ordering dependencies (use sequential dispatch instead)
- One-off ad-hoc work where queue setup overhead exceeds the dispatch overhead

## Setup (one-time per pipeline)

```
mkdir -p .bones && bones up -R .bones/<pipeline-name>.bones
```

`bones up` brings up:
- Fossil server at http://127.0.0.1:8765 (artifact storage + claim arbitration)
- NATS at nats://127.0.0.1:4222 (event stream — workers can subscribe)

## Authoring Tasks

```
REPO=.bones/<pipeline-name>.bones
bones tasks create "<title>" -R $REPO \
  --context priority=high \
  --context category=substrate \
  --context source=<where-this-came-from>
```

Title is searchable. Use `--context k=v` (repeatable) for filterable metadata. `--files` declares files-touched if the implementer should know up front.

## Worker Brief (paste into every swarm dispatch)

```
After spawn:
  bones tasks list -R <REPO-path>      # see open tasks
Loop:
  ID=$(bones tasks list -R <REPO> --format json | jq -r 'first(.[]|select(.claimed==null)) | .id')
  [ -z "$ID" ] && exit 0       # queue empty, stop
  bones tasks claim "$ID" -R <REPO>
  ... work, commit, push ...
  bones tasks close "$ID" -R <REPO>
```

The orchestrator does NOT decide which worker gets which task. The queue does.

## Recorded Failures (RED — what we did instead before this skill)

- **v0.1.17 Phase 2 (parallel impl-2b/c/d/e dispatch)**: orchestrator hand-partitioned 16 tasks across 4 implementers by file. Two impls got tasks on `cmd/darken/doctor.go` (B3+B5+B7) — they had to be the SAME impl to avoid trampling. Required careful manual planning and the partition was still imperfect (impl-2c/d/e merge commits show pulls from each other).
- Cost: ~30 min planning + risk of partition mistakes. With bones tasks queue: just fire 4 impls with the brief above; queue handles ordering.

## Quick Reference

| Op | Command |
|---|---|
| Bootstrap | `bones up -R <path>` |
| Add task | `bones tasks create "<title>" -R <path>` |
| List | `bones tasks list -R <path>` |
| Claim | `bones tasks claim <id> -R <path>` |
| Close | `bones tasks close <id> -R <path>` |
| Watch | `bones tasks watch -R <path>` (events from NATS) |

## Anti-Patterns

- Authoring tasks too granular (every commit a task) — keep tasks at logical-unit granularity
- Letting workers run after queue empty — they will idle, costing budget
- Forgetting to close — claims orphan, future workers will not pick them up
- Mixing claim/close with file-disjoint partitioning — pick one model, do not double-coordinate
