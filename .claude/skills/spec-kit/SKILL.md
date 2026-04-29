---
name: spec-kit
version: 0.1.0
description: >-
  Constitution-driven specification flow mounted by planner-t4. Activates
  when an agent must drive the full ratification pipeline: constitution
  review, spec.md authoring, plan.md production, and tasks/ decomposition.
  Use when operating under the specify CLI, when a feature requires formal
  ratification before implementation, or when producing structured spec
  artifacts for downstream worker harnesses.
type: skill
targets:
  - claude-code
  - apm
  - codex
  - gemini
  - copilot
  - pi
category:
  primary: planning
---

# Spec-Kit — Constitution-Driven Ratification Flow

This skill is the constitution-driven flow bundle that planner-t4 mounts.
planner-t4 is the "full ratification" harness: it produces constitution
review, spec.md, plan.md, and tasks/ decomposition before any worker
harness touches implementation. The spec-kit skill governs how that
pipeline runs.

Canonical content lives in the operator agent-config repo. This shell
activates the spec-driven ratification mode and establishes the artifact
production sequence.

## Purpose

planner-t4 operates under the `specify` CLI. Its job is to take an
operator-supplied feature request and produce a complete, ratified artifact
set that downstream implementer harnesses can execute without ambiguity.

The spec-kit skill encodes the protocol for that production: the order of
artifacts, the ratification gates, the escalation rules, and the shape of
each output document.

## Artifact Pipeline

### Stage 1: Constitution Review

Before authoring any spec, read the project constitution (typically
`CONSTITUTION.md` or the equivalent ratified document). Identify:

- The four escalation axes (taste, ethics, reversibility, architecture)
- Any structural decisions already ratified that constrain the feature
- Dependency boundaries that the spec must respect

Do not proceed to spec authoring if the constitution is absent or
unratified. Emit a `RequestHumanInput` and halt.

### Stage 2: spec.md

The spec describes the feature contract. Every spec must contain:

| Section | Content |
|---------|---------|
| Problem | One-sentence statement of what must change and why |
| Acceptance Criteria | Numbered, verifiable, testable conditions |
| Out of Scope | Explicit list of what this spec does NOT address |
| Ratification Notes | Any constitution axes touched and how they are resolved |
| Open Questions | Items that require orchestrator escalation before plan authoring |

A spec is ratified when the orchestrator has reviewed open questions and
marked them resolved. Do not proceed to plan authoring with unresolved
open questions.

### Stage 3: plan.md

The plan describes the implementation sequence. Every plan must contain:

| Section | Content |
|---------|---------|
| Units | Ordered list of implementation units |
| Dependencies | Dependency graph between units |
| Interface Contracts | Public interfaces each unit must satisfy |
| Test Strategy | What tests each unit requires before merge |
| Risk Notes | Reversibility and rollback plan for each risky unit |

The plan is the artifact that worker harnesses (implementer, fixer) consume
directly. It must be unambiguous: a worker reading the plan should be able
to implement its assigned unit without asking clarifying questions.

### Stage 4: tasks/

Decompose the plan into per-unit task files under `tasks/`. Each task file:

- Is named `tasks/<unit-name>.md`
- Contains exactly one unit's spec slice: name, files, failing test
  description, acceptance criterion, dependencies
- Is self-contained: a worker can execute it without reading the full plan

## Specify CLI Integration

The `specify` CLI drives the artifact pipeline. When spec-kit is active:

1. Use `specify init` to create the artifact scaffold
2. Use `specify check` to validate spec.md against the constitution
3. Use `specify split` to decompose plan.md into tasks/
4. Use `specify status` to report ratification progress to the orchestrator

Do not bypass the `specify` CLI to write artifacts directly. The CLI
enforces constitution compliance and schema validation.

## Ratification Gates

| Gate | Condition to Pass |
|------|-------------------|
| spec-ready | All open questions resolved, orchestrator sign-off |
| plan-ready | Interface contracts complete, test strategy specified |
| tasks-ready | Every unit has a task file, no missing dependencies |

A gate failure halts artifact production and requires orchestrator
resolution before continuing.

## Escalation Rules

Escalate to the orchestrator when:

- The spec touches a constitution escalation axis (taste, ethics,
  reversibility, architecture)
- An open question cannot be resolved from existing ratified decisions
- A dependency on an external team or system is discovered
- The feature scope expands beyond what the original request described

Escalation uses the standard `RequestHumanInput` format with the
`categories` field set to the relevant axis.

## Output Shape

When the ratification pipeline is complete, the artifact set must include:

1. `spec.md` — ratified, all open questions resolved
2. `plan.md` — complete, interface contracts and test strategy present
3. `tasks/<unit>.md` — one file per implementation unit
4. Ratification log — orchestrator sign-off record

The artifact set is the handoff boundary between planner-t4 and the
worker harnesses. Nothing in the artifact set should require a worker to
make a design decision — all design decisions are resolved at ratification.
