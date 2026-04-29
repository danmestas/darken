---
name: subagent-to-subharness
version: 0.2.0
description: >-
  Use when you would normally dispatch a subagent via the Agent tool but
  you are operating as the Darkish Factory orchestrator. Translates the
  muscle memory into subharness dispatch. Maps task shapes to the right
  harness role, frames the task in caveman-standard, reads worker output
  back, decides next step.
type: skill
targets:
  - claude-code
category:
  primary: workflow
---

# Subagent to subharness translation

## Dispatch rule (non-negotiable)

**Subharness is DEFAULT.** Any task that fits one of the 14 canonical roles MUST be dispatched via `bin/darken spawn`. Do not reach for the Agent tool when a subharness role matches.

**Agent tool is FALLBACK.** Use only when one of these four conditions is true:
1. **substrate unavailable** -- scion is unreachable or no image exists for the required backend
2. **no role matches** -- the task shape genuinely does not fit any of the 14 canonical roles
3. **operator override** -- the operator has explicitly authorized inline Agent dispatch for this task
4. **already-spawned** -- a suitable harness for this exact task is already running (check `darken list` first)

If none of these four conditions apply, you are in the wrong dispatch path. Stop. Use `bin/darken spawn`.

## What is the difference?

You came to this repo with Claude Code muscle memory: when a task is delegate-shaped, dispatch a subagent via the Agent tool. In the Darkish Factory orchestrator role that is almost always the wrong reflex -- you should spawn a **subharness** instead.

Subagents and subharnesses look similar (both delegate work, both return output) but their costs and capabilities differ.

| Property | Agent tool (subagent) | `bin/darken spawn` (subharness) |
|---|---|---|
| Process | In-process, same Claude Code | Containerized, isolated |
| Backend | Same model as you | Per-role: claude-opus, claude-sonnet, claude-haiku, codex/gpt-5.5 |
| Skills | Inherits yours | Per-role staged bundle in `/home/scion/skills/role/` |
| Worktree | Shared with you | Own worktree, scion-managed |
| Auth | Yours | Hub secret per backend |
| Tool surface | Your tools | The role manifest tools |
| Lifecycle | Synchronous, you wait | Async, poll via `scion list` / `scion look` |
| Cost | Cheap (same context) | Expensive (cold start, separate auth) |
| When to use | Pure-text, host-bound, < 2 min | Anything else |

## Decision tree

```
Operator gives you a task.
|
+-- Does it fit one of the 14 canonical roles? (DEFAULT path)
|   admin, base, darwin, designer, orchestrator,
|   planner-t1..t4, researcher, reviewer, sme,
|   tdd-implementer, verifier
|   YES -> bin/darken spawn <name> --type <role> "<task>"
|          (substrate unavailable, no role matches, operator override,
|           or already-spawned are the only exits from this path)
|
+-- FALLBACK: none of the 4 FALLBACK conditions applies?
|   -> You should be on the DEFAULT path above. Re-check.
|
+-- FALLBACK conditions present:
|   substrate unavailable -> Agent (Explore or general-purpose)
|   no role matches       -> Agent (general-purpose)
|   operator override     -> Agent (per operator instruction)
|   already-spawned       -> wait for existing harness or Agent if urgent
```

Within the FALLBACK Agent path, use the `Explore` subagent type for open-ended codebase exploration where the answer is text ("how does X flow through this repo?"), reading and summarizing many files, or scratch-pad reasoning that does not touch the worktree.

## Mapping table

| Subagent reflex | Subharness equivalent | Notes |
|---|---|---|
| "I will dispatch a researcher to gather context" | `darken spawn r1 --type researcher "..."` | Researcher has no skills bundled (cheap recon); produces a brief in its worktree. |
| "I will Agent out for spec writing" | `darken spawn d1 --type designer "..."` | Designer is opus, gets ousterhout/hipp skills. |
| "I will plan this in a sub-Agent" | `darken spawn p1 --type planner-tN "..."` | Pick tier per orchestrator-mode skill. Default ambiguous -> planner-t3. |
| "I will have a sub-Agent write tests + impl" | `darken spawn i1 --type tdd-implementer "..."` | Implementer is claude-sonnet; commits to its worktree. |
| "I will ask a fresh Agent to verify" | `darken spawn v1 --type verifier "..."` | Codex/gpt-5.5 -- cross-vendor adversarial. |
| "I will get a code-review Agent opinion" | `darken spawn rev1 --type reviewer "..."` | Codex/gpt-5.5 -- block-or-ship. |
| "I will spin up an SME for one question" | `darken spawn s1 --type sme "..."` | Codex/gpt-5.5; 10 turns; rejects bad framing. |
| "I will have an Agent log activity" | `darken spawn admin1 --type admin "..."` | Detached; long-running chronicle. |
| "I will evolve the pipeline post-run" | `darken spawn dw1 --type darwin "..."` | Emits YAML to `.scion/darwin-recommendations/`; you gate via `darken apply`. |

## Framing the task -- caveman standard

Subharnesses talk to you in caveman tiers (per the `caveman` skill). When you compose a task for a subharness, write it caveman-standard: lead with the verb + objective, then bound the output, then context.

**Bad (subagent-style verbose):**
```
Hi! I need you to take a look at the authentication flow in our codebase. Specifically, I am wondering if the session token handling is going to cause any issues with our new compliance requirements. Could you maybe look into it and let me know what you think? It would be great if you could also suggest some improvements...
```

**Good (caveman standard):**
```
Audit session-token handling in pkg/auth for compliance gap.

Output: docs/research-brief.md -- 1 page max, evidence-first, list specific files+lines that fail compliance, propose minimum fix per finding.

Context: legal flagged token storage on 2026-04-22; constitution section 6 requires rotation every 24h.
```

The subharness manifest already constrains `max_turns` and `max_duration` -- your task framing constrains scope and output shape. The caveman tier prevents prose drift.

## Reading output back

After `darken spawn <name> --type <role> "<task>"`:

```bash
darken list                  # see live state of all agents
scion look <name>             # read the agent output (use --tail N for last N lines)
scion stop <name> --yes       # kill if hung (10-min heartbeat)
darken doctor <name>         # per-harness preflight + post-mortem on the agent.log
```

If the agent committed deliverables to its worktree, cherry-pick them into your branch. Always log the dispatch + outcome in `.scion/audit.jsonl`.

## When to escalate to the operator instead of dispatching

If the subharness output triggers the **escalation classifier** (Stage 1 deterministic gate or Stage 2 adversarial LLM), batch the question to the operator. Do not keep dispatching to "fix" something that hit a taste / ethics / reversibility / architecture / spec-silent issue. See the `orchestrator-mode` skill for the classifier specifics.

## Cross-vendor pattern

The pipeline pairs a claude-backed worker with a codex-backed cross-vendor pass for verification + review. This is deliberate: a single-vendor pipeline can mass-fail on the same blind spot. When you spawn an implementer (claude/sonnet), follow up with a verifier (codex/gpt-5.5) and reviewer (codex/gpt-5.5) -- those flips bake the cross-vendor opinion into the loop.
