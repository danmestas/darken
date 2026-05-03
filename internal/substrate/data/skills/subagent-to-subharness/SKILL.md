---
name: subagent-to-subharness
version: 0.3.0
description: >-
  Use in any darken-managed workspace (any repo containing
  `.scion/grove-id`) when you would normally dispatch a subagent via
  the Agent tool but you are operating as the host-mode orchestrator.
  Translates the muscle memory into subharness dispatch, maps task
  shapes to roles, frames the task in caveman-standard with a
  status-publishing recipe and a fixed milestone vocabulary, and reads
  worker output back via the bounded-transcript protocol.
type: skill
targets:
  - claude-code
roles:
  - orchestrator
category:
  primary: workflow
---

# Subagent → subharness translation

## Dispatch rule (non-negotiable)

**Subharness is DEFAULT.** Any task fitting one of the 14 canonical
roles MUST be dispatched via `darken spawn`. Do not reach for the
`Agent` tool when a subharness role matches.

**`Agent` is FALLBACK only.** Use only when one of these four
conditions is true; **name the condition out loud** before the call:

1. **substrate-unavailable** — `darken spawn` itself is broken
   (scion unreachable, missing image, stage-creds enum mismatch). Log
   the underlying error verbatim.
2. **no-role-matches** — the task shape genuinely fits none of the 14
   canonical roles.
3. **operator-override** — the operator explicitly authorized inline
   `Agent` dispatch (quote the authorization).
4. **already-spawned** — a suitable harness for this task is already
   running (verify with `darken list`; cite the agent name).

If none of these apply, you are in the wrong dispatch path. Stop and
`darken spawn`.

> Recon — open-ended codebase exploration where the answer is text —
> is **researcher work**, not generic `Agent` work. Dispatch with
> `darken spawn researcher-1 --type researcher "<recon brief>"`. The
> wrong reflex is `Agent({subagent_type: "Explore"})`. See
> `orchestrator-mode` Rule 2.

## Why a subharness, not a subagent

You came to this workspace with Claude Code muscle memory: when a task
is delegate-shaped, dispatch a subagent via the `Agent` tool. In
host-mode orchestration that is almost always the wrong reflex —
spawn a **subharness** instead. They look similar; their costs and
capabilities differ.

| Property | `Agent` (subagent) | `darken spawn` (subharness) |
|---|---|---|
| Process | In-process, same Claude Code | Containerized, isolated |
| Backend | Same model as you | Per-role: claude-opus, claude-sonnet, claude-haiku, codex/gpt-5.5 |
| Skills | Inherits yours | Per-role staged bundle in `/home/scion/skills/<role>/` |
| Worktree | Shared with you | Own worktree, scion-managed under `.scion/agents/<name>/workspace/` |
| Auth | Yours | Hub secret per backend |
| Tool surface | Your tools | The role manifest tools |
| Lifecycle | Synchronous, you wait | Async, poll via `darken list` / `scion look` |
| Cost | Cheap (same context) | Expensive (cold start, separate auth) |
| When to use | Pure-text host-bound recon when a fallback condition fires | Anything that mutates files, runs tests, or fits a canonical role |

## Decision tree

```
Operator gives you a task.
|
+-- Does it fit one of the 14 canonical roles? (DEFAULT path)
|   admin, base, darwin, designer, orchestrator,
|   planner-t1..t4, researcher, reviewer, sme,
|   tdd-implementer, verifier
|   YES -> darken spawn <name> --type <role> "<task>"
|
+-- FALLBACK conditions (name the one that applies):
|   substrate-unavailable -> Agent (Explore or general-purpose)
|   no-role-matches       -> Agent (general-purpose)
|   operator-override     -> Agent (per operator instruction)
|   already-spawned       -> wait for existing harness, or Agent if urgent
|
+-- None of the above? Re-check. Default path almost always applies.
```

Within the FALLBACK `Agent` path, use `Explore` for open-ended
codebase exploration where the answer is text and there is no role
match. Even then, prefer `researcher` first.

## Mapping table

| Subagent reflex | Subharness equivalent | Notes |
|---|---|---|
| "I will dispatch a researcher to gather context" | `darken spawn r1 --type researcher "..."` | Cheap recon; produces a brief in its worktree. |
| "I will Agent out for spec writing" | `darken spawn d1 --type designer "..."` | Designer is opus; gets ousterhout/hipp skills. |
| "I will plan this in a sub-Agent" | `darken spawn p1 --type planner-tN "..."` | Pick tier per orchestrator-mode skill. Default ambiguous → planner-t3. |
| "I will have a sub-Agent write tests + impl" | `darken spawn i1 --type tdd-implementer "..."` | Implementer is claude-sonnet; commits to its worktree. |
| "I will ask a fresh Agent to verify" | `darken spawn v1 --type verifier "..."` | Codex/gpt-5.5 — cross-vendor adversarial. |
| "I will get a code-review Agent opinion" | `darken spawn rev1 --type reviewer "..."` | Codex/gpt-5.5 — block-or-ship. |
| "I will spin up an SME for one question" | `darken spawn s1 --type sme "..."` | Codex/gpt-5.5; 10 turns; rejects bad framing. |
| "I will have an Agent log activity" | `darken spawn admin1 --type admin "..."` | Detached; long-running chronicle. |
| "I will evolve the pipeline post-run" | `darken spawn dw1 --type darwin "..."` | Emits YAML to `.scion/darwin-recommendations/`; gate via `darken apply`. |

## Framing the task — caveman standard

Subharnesses talk to you in caveman tiers. When you compose a task for
a subharness, write it caveman-standard: lead with the verb +
objective, bound the output, then the context.

**Bad (subagent-style verbose):**

```
Hi! I need you to take a look at the authentication flow in our
codebase. Specifically, I am wondering if the session token handling
is going to cause any issues with our new compliance requirements.
Could you maybe look into it and let me know what you think? It would
be great if you could also suggest some improvements...
```

**Good (caveman standard):**

```
Audit session-token handling in pkg/auth for compliance gap.

Output: docs/research-brief.md — 1 page max, evidence-first, list
specific files+lines that fail compliance, propose minimum fix per
finding.

Context: legal flagged token storage on 2026-04-22; constitution
section 6 requires rotation every 24h.
```

The subharness manifest already constrains `max_turns` and
`max_duration` — your task framing constrains scope and output shape.
The caveman tier prevents prose drift.

## Status-publishing recipe (REQUIRED for every dispatch)

Every dispatch brief MUST include a status-publishing block instructing
the subharness to emit milestones via `bones notify`. Without this,
the orchestrator goes blind for the run (see the
`orchestrator-mode` skill's "Background-dispatch monitoring
protocol").

### Standard milestone vocabulary (use these tokens, not free-form)

| Token | When to publish |
|---|---|
| `joined` | First action after the harness starts; confirms it is alive |
| `read-plan` | After reading the dispatch brief and any referenced docs |
| `started-file-X` | When opening file X for write (X = relative path) |
| `commit-N-of-M` | After commit N of M planned commits |
| `tests-running` | When the test suite starts |
| `tests-passed` | When the test suite finishes green |
| `tests-failed` | When the test suite finishes red (include short reason) |
| `closing` | Final action before the harness exits cleanly |
| `error-<reason>` | Unrecoverable error; `<reason>` is a kebab-case slug |

### Boilerplate to paste into every dispatch brief

```
Status protocol (REQUIRED):
- After every milestone listed below, run:
    bones notify send --subject "task:<your-agent-name>:<token>" \
      --body "<≤80 chars: short status>"
- Tokens to publish, in order: joined, read-plan, started-file-<path>
  (per file you open for write), commit-N-of-M (per commit), tests-
  running, tests-passed (or tests-failed: <reason>), closing.
- On any unrecoverable error, publish error-<kebab-case-reason> before
  exiting.
- If `bones notify` is unavailable, append the same line to
  $WORKSPACE_ROOT/.scion/agents/<your-agent-name>/status.log instead.
```

Replace `<your-agent-name>` with the actual agent name you passed to
`darken spawn`. The orchestrator wires `Monitor` on `bones notify
watch --subject-prefix "task:<name>"` BEFORE dispatching, so each
milestone wakes the orchestrator session.

## Reading output back

After `darken spawn <name> --type <role> "<task>"`:

```bash
darken list                  # see live state of all agents
scion look <name>            # read the agent output (use --tail N for last N lines)
scion stop <name> --yes      # kill if hung (10-min heartbeat)
darken doctor <name>         # per-harness preflight + post-mortem
```

If the agent committed deliverables to its worktree, cherry-pick them
into your branch.

### Bounded transcript-tail (the canonical evidence pull before any "agent failed" claim)

Reading the full transcript is forbidden — unbounded and will blow
your context. But before any "agent failed/lied/fabricated" escalation
to the operator, you MUST extract a bounded final-message tail:

```bash
TRANSCRIPT=".scion/agents/<name>/transcripts/$(ls -t .scion/agents/<name>/transcripts/ | head -1)"
jq -r 'select(.type == "assistant") | .message.content[]?.text? // empty' \
  "$TRANSCRIPT" 2>/dev/null \
  | tail -c 5120
```

This yields ~5KB — the agent's last assistant message. Use it as the
authoritative "what did the agent claim it did" record. See the
`orchestrator-mode` skill's "Transcript-read protocol" for the full
rule and the re-verify-filesystem-state requirement that pairs with
it.

### Re-verify before escalating "agent failed"

Before concluding the agent failed:

1. `ls -la <claimed path>` — does the file exist?
2. Compare `mtime` to dispatch start — written during this run?
3. If a commit was claimed, `git -C <agent worktree> log --oneline -5`.
4. Wait 30 seconds and re-`ls` — filesystem snapshots can lag.

Only after this passes AND the bounded transcript tail confirms the
mismatch may you escalate.

## Audit + log every dispatch

Append a one-line entry to the workspace audit log
(`<workspace-root>/.scion/audit.jsonl`) for every dispatch and every
outcome. Resolve the workspace root via the `.scion/grove-id` marker
(see `orchestrator-mode` skill's "Audit log" section for the recipe).

## When to escalate to the operator instead of dispatching

If the proposed dispatch (or its expected output) triggers the
**escalation classifier** — Stage 1 deterministic gate
(irreversibility + blast radius) or Stage 2 adversarial gate —
batch the question to the operator. Do not keep dispatching to "fix"
something that hit the gate. See the `orchestrator-mode` skill for the
classifier specifics and the auto-ratify list.

Every escalation MUST be labeled with exactly one of the four canonical
axes:

- `taste` — aesthetic / style preference with no functional impact
- `architecture` — cross-cutting structural decision the operator owns
- `ethics` — operator-relevant trust, safety, or norms boundary
- `reversibility` — change is hard to undo

If you cannot pick one axis, the question is not an escalation — frame
it as an open variable in the dispatch brief instead, with a
recommended default.

## Cross-vendor pattern

The pipeline pairs a claude-backed worker with a codex-backed
cross-vendor pass for verification + review. This is deliberate:
single-vendor pipelines can mass-fail on the same blind spot. When you
spawn an implementer (claude/sonnet), follow up with a verifier
(codex/gpt-5.5) and reviewer (codex/gpt-5.5) — those flips bake the
cross-vendor opinion into the loop.
