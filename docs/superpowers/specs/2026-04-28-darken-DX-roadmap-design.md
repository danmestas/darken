# Darken DX Roadmap — Phases 5-9

**Status:** approved (in-conversation, 2026-04-28)
**Author:** dmestas
**Source PRs:** TBD (Phase 5 plan exists at `docs/superpowers/plans/2026-04-28-darken-phase-5-init-completeness.md`)

## Context

A live orchestrator session in `~/projects/edgesync-fslite` surfaced multiple gaps in `darken init` and `darken spawn`. A subsequent `dx-audit` (`docs/...` not committed; reproduced inline below) scored the "init a new project" workflow at 2.6/10 today — broken without manual intervention. Phase 5 (already planned) gets that workflow to ~8/10. This spec defines what the **rest** of the path to "great" (10/10) looks like, decomposed into four additional phases (6-9).

## Audit summary (input to this spec)

| Workflow | Frequency | Today | After Phase 5 | After Phase 9 |
|---|---|---|---|---|
| Init a new project | Per-project | 2/10 | 8/10 | 9/10 |
| Verify init worked | Per-project | 2/10 | 7/10 | 9/10 |
| First spawn after init | Per-project | ~5/10 | 6/10 | 9/10 |
| Routine orchestrator session | Daily | ~6/10 | 7/10 | 9/10 |
| Recovery / update | When broken | 3/10 | 4/10 | 8/10 |

**Today's overall:** 2.6/10. **Post-Phase-9 target:** 8.6/10 (cap below 10 because operator-grade is asymptotic; multi-machine + collab is non-goal).

## Goals

After all five phases ship:

- `darken init <fresh-repo>` produces a working orchestrator session with zero manual recovery
- Operator can verify init worked at any time via a single command
- First spawn after init has visible progress, async dispatch by default, ~10s ready-time confirmation
- Daily use surfaces "what's running / what happened" via scion's web dashboard for workers + Claude Code chat for the host orchestrator
- Worker hangs auto-recover up to a threshold; substrate-hash drift after `brew upgrade` is detected and explained

## Non-goals

- **Multi-machine sync** — laptop ↔ desktop hub-secret transfer
- **Multi-project portfolio management** — listing all darken'd repos, jumping between them
- **Collab / teammates** — multi-operator escalation routing, shared init state
- **Darken-side web UI** — scion ships a web dashboard; we wrap it
- **Mode A pivot** — host orchestrator (Mode B) stays the default; containerized orchestrator remains an option but isn't the focus
- **`tmux`-wrap host orchestrator into the dashboard** — noted as future work, not in scope here

## Phase decomposition

Each phase ships as one PR, executed sequentially with eval-and-merge between.

| Phase | Workflow it fixes | New top-level commands | Implementation skill |
|---|---|---|---|
| **5** (in flight) | A. Init scaffolding completeness | `darken status` | superpowers:writing-plans → executing-plans |
| **6** | A. Init verify + refresh + prereqs | `darken doctor --init`, `darken init --refresh` | writing-plans |
| **7** | B. First-spawn UX | (modifies `darken spawn`) | writing-plans |
| **8** | C. Routine observability | `darken dashboard`, `darken history` | writing-plans |
| **9** | D. Recovery + update | `darken redispatch`, `darken upgrade-init` | writing-plans |

## Cross-cutting decisions

These apply across all 5 phases.

1. **Observability split is natural, not a problem to solve** — scion's web dashboard for workers + Claude Code chat for the host orchestrator. Two windows by design.
2. **Spawn returns at "ready" state, not at completion** — `darken spawn` polls scion broker for "agent ready" within a timeout (~15s), returns once ready. Failed handshake (auth, image missing) maps to a remediation hint via the existing `darken doctor` table.
3. **`darken init` is fully self-contained** — embedded skills + scripts + statusLine + bones init + gitignore append. No host-side dependency on a darkish-factory clone.
4. **Substrate hash is the version coherence anchor** — every `darken status`, `darken doctor`, `darken history` row stamps it. Mismatches between binary and project skills trigger drift warnings.
5. **State stays append-only files** — no SQLite of our own, no daemon. `.scion/audit.jsonl` is canonical for darken history. Scion's hub.db is read-only to us (via API/CLI).
6. **Mode B is the default forever** — host Claude Code is the orchestrator; Mode A (containerized) is the override.

## Per-phase scope

### Phase 5 — Init completeness (planned)

`docs/superpowers/plans/2026-04-28-darken-phase-5-init-completeness.md` covers the full task list. Summary:

- Substrate resolver in `spawn.go` and `bootstrap.go` (extracts embedded scripts to temp at runtime)
- `darken init` scaffolds `.claude/skills/{orchestrator-mode,subagent-to-subharness}/SKILL.md`
- `darken init` runs `bones init` (soft-fail if absent)
- `darken init` writes `.claude/settings.local.json` with statusLine → `darken status`
- `darken init` appends `.gitignore` entries
- New `darken status` subcommand
- Makefile fix: `AGENT_INFRA_PATH` → `BONES_SOURCE_PATH` (agent-infra was renamed bones)

Closes audit items 1-5.

### Phase 6 — Init verify + refresh + prereqs

- **`darken doctor --init`** — scoped doctor pass that asserts an init'd repo has all expected files (CLAUDE.md, both skills, settings.local.json, gitignore entries, bones workspace). Reports per-item PASS/FAIL with remediation. Backs audit #6.
- **`darken init --refresh`** — re-runs scaffolding without overwriting CLAUDE.md unless `--force`. Pulls newer skill bodies from the binary's embedded substrate. The path operators take after `brew upgrade darken` to inherit substrate fixes. Backs audit #7.
- **Init-time prereq checks** — `darken init` errors fast if `bones`/`scion`/Docker missing, with one-liner install hints. Today these surface mid-spawn; Phase 6 surfaces at init. Backs audit #8.

### Phase 7 — First-spawn UX

- **Async spawn (Mode D from brainstorm)** — `darken spawn` polls scion broker for the agent's lifecycle state up to a configurable timeout (default 15s, override via `DARKEN_SPAWN_READY_TIMEOUT`). Returns once the agent transitions out of `Starting` (typically into `Thinking` or `Waiting` per scion's lifecycle states). Failed handshake (auth, image missing, etc.) surfaces as exit-1 with broker error mapped to a remediation hint via the existing `darken doctor` mapping table. **Open question:** verify what scion exposes as the "ready" predicate — `scion list --format json` lifecycle field is the most likely surface; will confirm during Phase 7 implementation.
- **Cold-start progress** — while spawn waits for "ready", print one-line progress to stderr: `[spawning researcher-1] container starting → broker handshake → ready (12.3s)`. Quiet on success, loud on failure.
- **`darken spawn --watch`** flag for the legacy "block + tail" mode if operator wants today's behavior. Default is async.

Open question (deferred): timeout calibration. Cold-start times vary by image. Will need a sane default + env override.

### Phase 8 — Routine observability

- **`darken dashboard`** — opens scion's web dashboard URL in the operator's default browser. Detection: parse `scion server status` to confirm web is enabled (default port 8080 in workstation mode). If web is OFF, friendly error: `scion server restart --workstation` (or `--enable-web`).
- **`darken history`** — reads `.scion/audit.jsonl` from CWD, prints a tabular summary: timestamp / decision_id / harness / type / outcome. Filters: `--last <N>`, `--since <duration>` (Go `time.ParseDuration` syntax: `5m`, `1h`, `24h`), `--format json` (for `jq` piping). Audit-log schema gets documented (was implicit before).
- **Status line enrichment** — `darken status` (already in Phase 5) gains optional active-worker count if `scion list` runs sub-100ms. Fallback: `--no-workers` flag for embedded-hash-only output.

### Phase 9 — Recovery + update

- **`darken redispatch <agent>`** — kills the existing agent (via `scion stop`) and re-spawns with the same task. Wraps the §7 loop's "10-minute heartbeat → kill + redispatch" pattern. Worker worktree preserved across redispatch (treat as fresh start; commits are the durable state, in-flight uncommitted work is acceptable to lose).
- **Substrate-hash drift detection** — `darken doctor` adds a "Project skills hash matches binary substrate" check. Mismatch on `<repo>/.claude/skills/orchestrator-mode/SKILL.md` vs the hash from `darken version` triggers a warning + "run `darken upgrade-init`" suggestion.
- **`darken upgrade-init`** — convenience wrapper: `darken init --refresh` against CWD then `darken doctor --init` to verify. Single command for the post-`brew upgrade darken` operator action.
- **Worker auto-redispatch policy** — orchestrator-mode skill already says "loop up to 3 times before escalating to the operator with the failure trace." Phase 9 makes that policy executable: orchestrator detects hang via heartbeat, calls `darken redispatch` automatically, escalates after N. Implementation lives in the orchestrator skill (no Go code change required) but Phase 9 documents the contract.

## CLI surface (consolidated)

**New commands across phases 5-9:**

| Command | Phase | Purpose |
|---|---|---|
| `darken status` | 5 | one-line status for statusLine (mode + substrate hash; future: + worker count) |
| `darken init --refresh` | 6 | re-scaffold an existing init'd repo from current binary |
| `darken init --force` | 6 | overwrite existing CLAUDE.md / settings during refresh |
| `darken doctor --init` | 6 | verify an init'd repo has all expected scaffolds |
| `darken dashboard` | 8 | open scion's web dashboard URL in browser |
| `darken history` | 8 | tabular view of `.scion/audit.jsonl` |
| `darken redispatch <agent>` | 9 | kill + re-spawn with same task |
| `darken upgrade-init` | 9 | refresh + doctor in one command |

**Modified existing commands:**

| Command | Phase | Change |
|---|---|---|
| `darken spawn` | 7 | default async (returns at "ready"); `--watch` for old blocking behavior |
| `darken spawn` | 7 | cold-start progress on stderr |
| `darken spawn` | 7 | broker errors mapped via existing remediation table |
| `darken init` | 5 | scaffolds skills + statusLine + gitignore + bones init |
| `darken init` | 6 | adds prereq checks at start; fail fast |
| `darken doctor` | 9 | adds substrate-hash drift check |
| `darken status` | 8 | optional active-worker count if `scion list` is fast |

**Subcommand structure stays flat** — no `darken obs <subcmd>` namespace nesting. Each command is one word at the top level. ~13 commands total after Phase 9.

## State / persistence model

```
~/.scion/hub.db                 SQLite — scion's state (agents, groves); we read via API/CLI, never write
~/.scion/dev-token              Auto-written by scion at server start; used for hub API auth
~/.config/darken/               User-scoped overrides (skills, templates) — manual ops only

Per project:
  CLAUDE.md                       Scaffolded by darken init
  .claude/skills/<name>/          Project copies of host-mode skills
  .claude/settings.local.json     Claude Code statusLine config
  .scion/audit.jsonl              Append-only decision log; canonical for darken history
  .scion/agents/<name>/           Scion's per-spawn worktrees (gitignored)
```

**Darken does NOT introduce its own state DB.** No SQLite, no daemon, no JSON config file beyond `settings.local.json` (which is Claude Code's). Everything queryable lives in scion's hub state (read via API/CLI) or `.scion/audit.jsonl` (read by `darken history`).

Substrate hash is the version anchor — lives in:
- The binary (`darken version` reports first-12-chars)
- Stat'd hashes of project skills (Phase 9 drift detection)
- Audit log entries (so `darken history` can show which substrate version made each decision)

## Discoverability

- `darken --help` lists all top-level commands with one-line descriptions (already does for current subset)
- `darken doctor` post-mortem messages reference the relevant new subcommand by name (e.g., "run `darken upgrade-init` to refresh stale skills")
- README's Quick Start gets one new line per phase as they ship

## Risks + open questions

1. **Async spawn timeout calibration (Phase 7)** — cold-start times vary by image (claude vs codex, first vs cached pull). Will need a sane default (15s) + env override. Risk: too short → spurious failures; too long → operator waits when something's already broken.

2. **Substrate-hash drift remediation UX (Phase 9)** — `darken upgrade-init` could clobber operator customizations under `.claude/skills/`. Mitigation: refresh writes side-by-side `SKILL.md.new`, operator diffs and merges. Slightly more work; safer.

3. **`darken history` performance (Phase 8)** — `.scion/audit.jsonl` could grow large over months. Linear scan acceptable up to ~1MB; beyond that we'd want rotation (`.scion/audit.jsonl.<date>`). Phase 8 ships linear; rotation lands in a Phase 10+ if anyone hits it.

4. **`darken redispatch` blast radius (Phase 9)** — `scion stop` + new spawn risks orphaning partial work in the worker's worktree. Decision: preserve worktree, treat redispatch as fresh start; commits are the durable state.

5. **Scion's web dashboard may not be enabled by default (Phase 8)** — `--enable-web` flag is required (or `--workstation` mode for combo). If operator's scion server was started without it, `darken dashboard` errors with a one-liner remediation suggesting `scion server restart --workstation`.

## Versioning

- Phase 5 ships under `v0.1.4` (or whatever is current at the time of merge)
- Each subsequent phase bumps the patch version: 5→v0.1.4, 6→v0.1.5, 7→v0.1.6, 8→v0.1.7, 9→v0.1.8
- After Phase 9 ships, consider `v0.2.0` to mark "operator-grade complete for the solo path"

## Cross-references

- Audit input: in-conversation; reproduced in this doc's Audit Summary section
- Phase 5 plan: `docs/superpowers/plans/2026-04-28-darken-phase-5-init-completeness.md`
- Original substrate spec: `docs/superpowers/specs/2026-04-27-darken-installable-substrate-design.md`
- Scion docs: https://googlecloudplatform.github.io/scion/hub-user/dashboard/, https://googlecloudplatform.github.io/scion/hub-admin/hub-server/
