---
name: superpowers
version: 0.1.0
description: >-
  Multi-skill bundle mounted by planner-t3. Activates when a design session
  must scaffold an architecture that satisfies both Hipp zero-config
  reliability principles and Ousterhout deep-module complexity reduction.
  Triggers on requests to design libraries, data layers, or system modules
  where simplicity, embeddability, and minimal cognitive load must all be
  balanced together.
type: skill
targets:
  - claude-code
  - apm
  - codex
  - gemini
  - copilot
  - pi
category:
  primary: backpressure
---

# Superpowers — Combined Hipp + Ousterhout Design Session

This skill is the multi-skill bundle that planner-t3 mounts alongside the
`hipp` and `ousterhout` skills. Its job is to scaffold a design session that
unifies both philosophies into a single coherent reasoning pass.

Canonical content lives in the operator agent-config repo. This shell
activates the combined reasoning mode and establishes the session structure.

## Purpose

planner-t3 is the "design + detailed plan" harness. It escalates taste,
ethics, and reversibility decisions to the orchestrator while producing
deep, well-reasoned architecture plans. The superpowers bundle gives it the
philosophical vocabulary to scaffold that reasoning correctly.

The two philosophies reinforce each other:

- **Hipp** supplies the reliability axis: zero-config, embedded-first,
  economy of dependencies, long-term viability.
- **Ousterhout** supplies the complexity axis: deep modules, information
  hiding, pulling complexity downward, minimizing cognitive load.

Together they form a complete lens for evaluating any design choice.

## Session Protocol

When superpowers is active, scaffold every design session in this order:

### 1. Restate the Problem (one sentence)

What must "just work"? What is the cognitive contract the caller should have
with the finished module?

### 2. Apply the Dual Lens

Score the candidate design on both axes simultaneously:

| Axis | Question |
|------|----------|
| Hipp: Simplicity | Does it just work? Zero config? |
| Hipp: Reliability | Is every path tested? Predictable under failure? |
| Hipp: Independence | Embedded? No unnecessary servers or deps? |
| Ousterhout: Depth | Small interface, rich implementation? |
| Ousterhout: Information Hiding | Are internals opaque to callers? |
| Ousterhout: Cognitive Load | How much must a caller know to use it? |

### 3. Design It Twice

Sketch two approaches. The first is the obvious/conventional one. The
second must satisfy both axes more aggressively. Score both on the table
above. Pick the one with the lower combined complexity and higher
reliability.

### 4. Scaffold the Plan

Produce a detailed implementation plan that:

- Names every module and its interface contract
- Calls out where complexity is pulled downward (Ousterhout §5)
- Identifies what zero-config defaults remove from the caller's burden (Hipp §1)
- Lists the test strategy for every non-trivial path (Hipp §2)
- Flags any external dependency as a portability risk requiring justification (Hipp §3)

### 5. Escalate When Required

Escalate to the orchestrator when the decision touches:

- Taste: subjective tradeoffs where reasonable engineers disagree
- Ethics: data privacy, user trust, irreversible side effects
- Reversibility: choices that would be expensive to undo after commit

## Red Flags (Combined)

| Red Flag | Violates |
|----------|----------|
| Config file or setup ritual required | Hipp §1 — zero-config |
| Server-based when embedded suffices | Hipp §3 — independence |
| Shallow class with tiny behavior | Ousterhout §1 — deep modules |
| Internal representation leaks through interface | Ousterhout §3 — information hiding |
| Feature added "while we're here" | Hipp §5 — resist feature creep |
| Change requires edits in many places | Ousterhout §4 — change amplification |
| External dependency breaks portability | Hipp §3 — economy |
| Pass-through method that hides nothing | Ousterhout §7 — design it twice |

## Output Shape

When the design session is complete, the plan document must contain:

1. Problem statement (one sentence)
2. Two candidate designs with dual-lens scores
3. Selected design with justification referencing named principles
4. Module interface contracts
5. Dependency manifest with portability risk notes
6. Test strategy
7. Escalation log (empty if nothing was escalated)
