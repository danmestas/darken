# Local Orchestrator Mode

To run the darken orchestrator from a local Claude Code session (observable mode), add the following to your project's CLAUDE.md or paste it as instructions.

## Prerequisites

- Scion server running: `scion server start`
- `scion` CLI on PATH
- `ANTHROPIC_API_KEY` exported

## Instructions for Claude Code

When the user asks you to run the darken pipeline on a task:

### 1. Enable the hub

```bash
scion hub enable --yes 2>/dev/null; true
```

Verify with `scion list` before proceeding.

### 2. Classify the task

Before starting any sub-harness, run the routing classifier mentally or via a structured call. Decide: `light` (skip research, go to plan) or `heavy` (research first). Log the decision.

### 3. Start sub-harnesses in §7 loop order

Light: designer → planner → tdd-implementer → verifier → reviewer.
Heavy: researcher → designer → planner → tdd-implementer → verifier → reviewer.

Start each with `--notify`:

```bash
scion start <role> --type <role> --notify "<full task description with context>"
```

### 4. Subscribe to notifications (optional but recommended)

```bash
scion notifications subscribe --agent <role> --triggers WAITING_FOR_INPUT,COMPLETED,LIMITS_EXCEEDED
```

### 5. Monitor each sub-harness

Poll `scion list` every 20–30 seconds, or wait for a notification.

When a sub-harness signals waiting, read its terminal:

```bash
scion look <role>
```

Then respond:

```bash
scion message <role> --notify "<your response>"
```

### 6. Run the escalation classifier before answering

Before responding to any sub-harness question, check whether it touches any of the four axes (README §2): taste, architecture, ethics, reversibility.

- Reversibility triggers: intercept immediately, escalate to the operator with high urgency.
- Taste, architecture, ethics: evaluate against the policy file. If confidence is below the floor, escalate to the operator.
- If auto-ratified: log the decision and respond.

Batch non-urgent escalations. Present them to the operator as a numbered summary. One-keystroke answers where possible.

### 7. Handle handoffs

When a phase completes, cherry-pick its committed output to the next sub-harness's worktree before starting the next phase:

```bash
git cherry-pick <commit-sha>
```

Log each handoff: source harness, destination harness, commit ref.

### 8. Stop each sub-harness before starting the next phase

```bash
scion stop <role> --yes
scion delete <role> --yes
```

### 9. On completion

Merge worktrees, run a final brief verification pass, and present the operator:
- Reviewable diff.
- Summary of auto-ratified decisions.
- Any remaining escalations.

Then stop. Do not ask follow-up questions.

## Key rules

- You are the only harness authorized to interrupt the human.
- Do not do the implementation work yourself — dispatch sub-harnesses.
- If you cannot answer a sub-harness question without triggering the escalation classifier, route it to the operator.
- If a sub-harness is stuck for over 10 minutes, interrupt with `scion message <role> --interrupt` and provide specific guidance.
- If the sub-harness receives an interrupt and still does not proceed, stop and redispatch.

---

Implements README §5.2, §7.
