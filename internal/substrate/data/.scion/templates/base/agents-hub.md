# Scion Hub Messaging Protocol

This file defines how Darkish Factory sub-harnesses interact with the Scion hub. Role-specific files do not override these rules.

---

## Core Rules

- Always use `--non-interactive` with the Scion CLI. This flag implies `--yes`. Any command that requires input will error instead of blocking. Omitting it can leave you stuck at an interactive prompt indefinitely.
- Use `--format json` for machine-readable output from any command where you need to parse the result.
- Do not use `--no-hub`. All communication goes through the hub API.
- Do not use `--global`. You operate in a grove workspace; scope is set implicitly.
- Do not use the `sync` or `cdw` commands.

---

## Messaging

Send a message to another agent:

```bash
scion message <agent-id> --notify "<message text>"
```

Always include `--notify`. This ensures the recipient is notified when the message arrives.

Receive messages from the system, orchestrator, or other agents. Messages arrive with markers:

```
---BEGIN SCION MESSAGE---
<sender, content>
---END SCION MESSAGE---
```

Read the full message before acting. Do not act on a partial message.

---

## Inspecting Agents

To read an agent's current terminal state:

```bash
scion look <agent-id>
```

This is the primary way to determine whether an agent is working, waiting, or errored. `scion list` status is not sufficient on its own.

---

## Listing and Status

```bash
scion list
```

Returns all agents in the current grove. Use `--format json` for structured output. An `idle` agent may still be working; do not assume idle means done.

```bash
scion status
```

Returns the current grove status including spend and active agents.

---

## Heartbeats and Timeout

Sub-harnesses are expected to produce observable progress. The orchestrator applies a 10-minute heartbeat timeout. If a sub-harness produces no terminal output for 10 minutes, the orchestrator will pause-and-inspect or kill-and-redispatch.

From a sub-harness perspective: if you are in a long operation (running a test suite, processing a large file), you do not need to emit artificial heartbeats. The orchestrator reads `scion look` output; visible tool activity counts as a heartbeat.

---

## Spend Tracking

Each grove has a per-feature spend cap. The orchestrator monitors spend via `scion status`. If spend approaches the cap, the orchestrator will pause the current phase and escalate to the operator before proceeding.

Sub-harnesses do not manage spend directly. If you receive a `LIMITS_EXCEEDED` notification, stop immediately and signal the orchestrator:

```bash
sciontool status ask_user "Limits exceeded in <phase>. Stopping. Current state: <brief description>."
```

---

## Starting and Stopping

The orchestrator starts and stops sub-harnesses. Sub-harnesses do not start or stop other sub-harnesses unless their role explicitly requires it (e.g., a researcher that spawns a brief sub-task).

If your role does require starting a child agent:

```bash
scion start <child-id> --type <type> --notify "<task>"
```

Clean up when done:

```bash
scion stop <child-id> --yes
scion delete <child-id> --yes
```

Do not leave idle child agents running.

---

Implements README §5.1, §8 (heartbeat timeout, token runaway).
