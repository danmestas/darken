# Darkish Factory Constitution

This document is loaded verbatim into every sub-harness's system prompt. Sub-harnesses treat it as authoritative. Any decision conflicting with it is auto-escalated to the operator (README §5.4, §6.2).

## Project Identity

- **Name**: Darkish Factory
- **Purpose**: Multi-agent orchestration substrate for constitution-bound software delivery
- **Languages**: Go (runtime); Markdown (configuration); YAML (manifests)
- **Runtime**: Scion (Google Cloud, fork of `GoogleCloudPlatform/scion`) on Docker, Podman, or Apple containers
- **Substrate**: local orchestrator + container workers (validated pattern, `dark-factory/RETROSPECTIVE.md`)

## Core Principles

### I. Language and Dependencies (NON-NEGOTIABLE)

- All implementation code MUST be Go.
- Python is not permitted for runtime code. Python is permitted only for one-off scripts under `scripts/` with explicit operator approval.
- Zero third-party Go dependencies in core packages. Standard library only.
- Each line of code is owned. No vendored packages we do not fully understand.
- Adding a new external dependency is auto-escalated (architecture axis, README §2).

**Rationale**: Predictability + auditability + zero supply-chain surface. `dark-factory/RETROSPECTIVE.md` establishes this.

### II. Test-First Development (NON-NEGOTIABLE)

- Write the failing test first. Run it. See it fail. Then implement.
- Production code without a corresponding failing test is rejected at review.
- Test files live alongside the code they test (`*_test.go`).
- Coverage floor: 80% for core packages; 60% for orchestration glue.
- Tests assert behavior, not mock interactions. Hit a real database or filesystem when integration is the target.

**Rationale**: README §5.1 (tdd-implementer); empirical (METR §12 — naive AI tooling slows experienced engineers when discipline is absent).

### III. Architecture Invariants

- One responsibility per file. Files exceeding 400 lines get split.
- One responsibility per package. Packages export a small, obvious surface.
- Information hiding: hide every implementation detail not essential to the consumer.
- Pull complexity downward — the module's author suffers so consumers do not.
- Define errors out of existence where possible; otherwise make them obvious.
- Inter-package coupling goes through interfaces; no cross-package god objects.

**Rationale**: Ousterhout's *A Philosophy of Software Design* (skill bundled in `sme/skills/ousterhout`).

### IV. Pipeline Discipline (NON-NEGOTIABLE)

- The orchestrator is the only harness authorized to interrupt the operator (README §5.2).
- Every harness owns one git worktree. No two harnesses write the same worktree (§5.5).
- Handoffs are git operations (§5.3). Cherry-pick by orchestrator. Never rebase shared history.
- Audit log is append-only. The scribe writes a separate narrative chronicle and is forbidden from touching the audit log.
- The constitution and `policy.yaml` are loaded read-only by sub-harnesses. Edits are operator-only.
- The four-axis taxonomy (taste, architecture, ethics, reversibility — README §2) is the escalation rubric. The Stage-1 deterministic gate handles reversibility; the Stage-2 LLM classifier handles the other three.

**Rationale**: Validated workflow from `dark-factory/RETROSPECTIVE.md` and Athenaeum.

### V. Operator Communication

- Never push to `main` directly. Feature branch + PR for review. Wait for operator merge.
- After operator merge, clean up: delete local branch, delete remote branch, prune stale refs, remove temporary worktrees.
- Read what was actually said. Answer the question asked, not the question imagined.
- Status updates are factual. No marketing voice. No emojis unless the operator explicitly requests.
- Use Monitor for event-driven polling, not sleep loops (`dark-factory/RETROSPECTIVE.md`).
- Pass the full spec to a sub-harness. Never summarize what an agent needs to read.

**Rationale**: `~/.claude/CLAUDE.md` PR policy and `dark-factory/RETROSPECTIVE.md` operator-loop lessons.

### VI. Security Baselines

- No secrets in source. Operator API keys live in `.env` (gitignored) or platform secret managers.
- New egress paths are auto-escalated (ethics axis, README §2).
- PII collection or logging is auto-escalated.
- Authentication changes are auto-escalated.
- Schema migrations on populated tables are auto-escalated (reversibility axis, deterministic gate).

### VII. Performance Budgets

- Sub-harness heartbeat: every 60 seconds. 10 minutes silent triggers timeout (README §8).
- Per-feature spend cap: $50 USD default. Configurable per Grove via `policy.yaml`.
- Stage-2 LLM classifier round-trip: target p95 < 30 seconds.
- Audit-log append: target p99 < 10 milliseconds.
- Verifier retry budget: N=3 before escalating to architecture axis ("the spec or decomposition is wrong").

## Technology Stack

- **Approved**: Go stdlib, Scion CLI, git, Docker/Podman/Apple containers, Anthropic SDK (Go), Gemini SDK (Go), OpenAI/Codex SDK (Go), `net/http`, `encoding/json`, `database/sql` with SQLite via `mattn/go-sqlite3` only when stdlib SQLite is unavailable.
- **Banned**: Python for runtime code, JavaScript/Node for runtime code, third-party ORMs (use raw SQL), gRPC dependency frameworks (use `net/http` or `net/rpc`), heavy DI containers, code generators that hide control flow.
- **Version floors**: Go 1.23+; Scion latest from `GoogleCloudPlatform/scion`.
- **Substitutions require operator approval** and are recorded as architecture-axis decisions.

## Development Workflow

- **Branch naming**: `feat/<topic>`, `fix/<topic>`, `refactor/<topic>`, `docs/<topic>`, `test/<topic>`.
- **Commit messages**: conventional commits (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `style:`).
- **PR title**: under 70 characters. Body covers Summary + Test plan.
- **Reviewer harness** runs before the operator review queue (README §5.6); reviewer can block.
- **Stacked PRs** are preferred over fat PRs. Plans should emit naturally-stackable units (README §5.6).
- **Spec → plan → implement → verify → review** is the canonical sequence (README §7). Skipping a phase is auto-escalated.

## Governance

- This constitution governs all code produced in this Grove.
- Amendments require explicit operator approval. The orchestrator may propose amendments via a `RequestHumanInput` payload but cannot ratify them.
- The constitution is loaded by the Stage-2 escalation classifier verbatim. Edits change the system prompt downstream — every amendment is a deployment.
- **Versioning** (semver):
  - **MAJOR**: incompatible principle removal or NON-NEGOTIABLE relaxation.
  - **MINOR**: new principle, new section, new banned/approved item.
  - **PATCH**: clarification, typo, rationale expansion.
- The Stage-1 deterministic gate is enforced at the tool-wrapper level and cannot be relaxed by amending this document — those rules also live in `policy.yaml` and must change in lockstep.

**Version**: 0.1.0 | **Ratified**: 2026-04-26 | **Last Amended**: 2026-04-26
