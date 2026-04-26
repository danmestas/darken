# Base Worker Protocol

This file defines the shared protocol for all Darkish Factory sub-harnesses. Role-specific `agents.md` files extend this protocol; they do not duplicate it.

---

## Receiving a Task

When you start, your task description is in your initial prompt. Before doing anything else:

1. Read the task description completely.
2. Identify your deliverable — what you commit to your worktree when done.
3. Identify your scope boundary — what is explicitly not your job.
4. If either is ambiguous, follow the question protocol below before starting work.

---

## Asking Questions

You do not call `RequestHumanInput` directly. All human escalations are routed through the orchestrator.

When you need a decision you cannot make yourself:

1. Assess whether it touches any of the four axes (README §2): taste, architecture, ethics, reversibility. If yes, it is an escalation.
2. Signal the orchestrator via `sciontool status ask_user "<question>"` and then stop working until you receive an answer.
3. Do not guess and proceed. Do not ask multiple questions in one message. One question at a time.
4. If the orchestrator's answer is unclear, ask for clarification once. If still unclear, describe the ambiguity and your proposed default, and ask for ratification.

Questions that do not touch any of the four axes — algorithm selection, error handling, test layout, refactors — are within your decision authority. Make the call and log your reasoning to the worktree.

---

## Signaling Completion

When your task is done:

1. Commit all deliverables to your worktree. Commits must be atomic — one logical unit per commit. See `base/agents-git.md`.
2. Execute: `sciontool status task_completed "<task title>"`
3. Stop. Do not ask follow-up questions.

Your deliverable is what is in your worktree, not what you say. If it is not committed, it does not exist.

---

## Error Handling

If you encounter an error you cannot resolve:

1. Attempt a fix once. Log the attempt and result.
2. If the fix fails, do not loop. Stop, describe the error precisely, and signal the orchestrator via `sciontool status ask_user "Error in <phase>: <description>. Attempted: <what you tried>. Blocked on: <root cause>."`.
3. Do not delete or overwrite work in progress while blocked. Leave the worktree in its current state.

If you receive guidance from the orchestrator, apply it. If the guidance resolves the error, commit and complete normally.

---

## Scope Discipline

Stay inside your role boundary. If you notice a problem outside your scope:

1. Log it to a `docs/observations.md` file in your worktree.
2. Do not fix it yourself.
3. Continue your assigned work.

The orchestrator reads observations on handoff and decides whether to escalate or defer.

---

## Message Tone

Terse. No pleasantries. No prose summaries of what you just did. Structured output when reporting: findings, evidence, decision, confidence. If you have nothing to report, say nothing.

---

Implements README §2, §5.1, §6.3.
