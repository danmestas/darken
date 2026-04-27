# Harness and Image Configuration

Status: design
Date: 2026-04-26
Scope: how every darkish-* container image is composed, how every harness's
`scion-agent.yaml` is shaped, how skills are bundled and dynamically managed,
how auth flows uniformly through hub secrets, and how inter-agent
communication is tiered.

## 1. Purpose & scope

This spec answers two operator questions concretely:

1. **What goes into each darkish-\* image?** Tools, skills, MCPs, prelude,
   trust-state injection. Same for every backend; backend-specific deltas
   are intentionally minimal.
2. **What goes into each harness's `scion-agent.yaml`?** Backend choice,
   model, resource caps, role-specific skills, auth references.

The design takes as inputs:

- The 13-harness pipeline (orchestrator, researcher, designer, four planner
  tiers, tdd-implementer, verifier, reviewer, sme, admin, darwin).
- Operator's task-shaped backend rubric: claude for frontend, codex for
  long-running, gemini for vision (sub-in only), pi for fast/diverse models
  (sub-in only).
- The validated runtime substrate (scion + four built darkish-\* images).
- The audit-revised pipeline goals from
  `docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`.

## 2. Out of scope

- Implementation order. Tracked separately in the writing-plans output.
- Slice 1 escalation classifier internals:
  `docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`.
- Slice 2 orchestrator + runtime adapter:
  `docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`.
- Pipeline DAG and per-role behavior:
  `docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`.
- Constitution authoring (lives at `.specify/memory/constitution.md`).

## 3. Architecture

### 3.1 Backend matrix

| Harness | Backend | Model | Detached | Role |
|---|---|---|---|---|
| `orchestrator` | claude | claude-opus-4-7 | no | runs the §7 loop, dispatches sub-harnesses, manages skill state, sole authority to interrupt the operator |
| `researcher` | claude | claude-sonnet-4-6 | no | sandboxed web fetch + summarization |
| `designer` | claude | claude-opus-4-7 | no | converts intent + research into a spec with structural decisions |
| `planner-t1` | claude | claude-sonnet-4-6 | no | think-then-do; small bug fixes; no plan doc |
| `planner-t2` | claude | claude-opus-4-7 | no | claude-code-style; light plan doc; few clarifying questions |
| `planner-t3` | claude | claude-opus-4-7 | no | superpowers flow; design docs + detailed plan; escalates taste/ethics/reversibility |
| `planner-t4` | codex | gpt-5.5 | no | spec-kit flow; full ratification; constitution + spec.md + plan.md + tasks/ |
| `tdd-implementer` | claude | claude-sonnet-4-6 | no | failing-test-first code; codex sub-in for backend-heavy long features |
| `verifier` | codex | gpt-5.5 | no | adversarial test execution + fuzzing; long-running |
| `reviewer` | codex | gpt-5.5 | no | senior-engineer block-or-ship; cross-vendor second opinion vs claude implementer |
| `sme` | codex | gpt-5.5 | no | summoned single-shot deep-domain expert |
| `admin` | claude | claude-haiku-4-5 | yes | append-only narrative chronicle, runs detached for the full pipeline |
| `darwin` | codex | gpt-5.5 | no | reads completed harness sessions; evolves rules and skills; long-context |

Counts: claude × 8, codex × 5. Pi and gemini are not pinned defaults; they
are sub-in overrides invoked at spawn time when the operator wants to try
an OpenRouter model (pi) or vision capability (gemini).

### 3.2 Override mechanism

Per-spawn CLI flags on `scion start`. Operator runs:

```bash
scion start tdd-implementer-feat-X \
  --type tdd-implementer \
  --harness codex \
  --image local/darkish-codex:latest
```

to substitute codex for any harness whose default is claude (or vice
versa). Pi and gemini sub-ins follow the same pattern with
`--harness pi --image local/darkish-pi:latest` and
`--harness gemini --image local/darkish-gemini:latest`.

The orchestrator's dispatch logic chooses the override per task. For
operator-initiated runs, `scripts/spawn.sh` accepts a `--backend` shortcut
that resolves to the right `--harness` + `--image` combination.

### 3.3 Communication tiers

Every system prompt carries a Communication section that fixes the
verbosity and register based on the message's audience:

| From → To | Mode |
|---|---|
| sub-agent → sub-agent (intra-harness peers, via bones) | **caveman ultra** |
| harness → its sub-agent / sub-agent → its harness | **caveman ultra** |
| harness ↔ harness (top-level peers, rare) | **caveman standard** |
| orchestrator → harness (dispatch + messaging) | **caveman standard** |
| harness → orchestrator (response) | **caveman standard** |
| orchestrator → human (operator) | **full natural speech** |
| human → orchestrator | natural (operator's choice) |

The caveman skill (cloned from `juliusbrussee/caveman` at image build) is
universal and provides all tiers; the skill itself documents the rules per
tier. Each harness's system prompt names its tier explicitly; the
orchestrator branches on audience.

## 4. Image design

### 4.1 Universal baseline (every darkish-\* image)

| Layer | Contents | Why |
|---|---|---|
| apt utilities | `jq`, `ripgrep`, `fzf`, `less`, `gh` | Standard tools every harness uses |
| Go-installed | `mgrep` (semantic search), `agent-infra` binaries (the "bones" set: `agent-init`, `agent-tasks`, `assert`, `autoclaim`, `chat`, `compactanthropic`, `dispatch`, `fossil`, `holds`, `jskv`, `presence`, `tasks`, `testutil`, `workspace`) | mgrep for code search; bones for inter-agent task management and parallel sub-agent dispatch |
| Universal MCPs | `context-mode` (sandboxed raw-output handling) | Every harness faces context-window pressure |
| Universal skills | `caveman` (cloned from `juliusbrussee/caveman` at build) | Communication-tier discipline |
| Trust prelude | per-backend (validated for claude + codex; placeholders for pi + gemini) | Suppress first-encounter trust dialogs |
| `/etc/skel/.gitconfig` | `[safe] directory = /repo-root` | Already present from base/home |

These are baked into every darkish-* image so every container has them
without per-harness mount machinery.

### 4.2 Backend-specific deltas

Intentionally minimal. Each `darkish-<backend>` image extends its
`scion-<backend>` parent with the universal baseline above and nothing
else. The CLI for each backend (`claude`, `codex`, `pi`, `gemini`) is
already in the parent image.

### 4.3 Per-harness one-off installs (not in image)

Two cases require per-harness install at spawn time, not bake-in:

- **planner-t4 + spec-kit** — the spec-kit CLI is needed only by
  `planner-t4`. Installing it universally bloats every claude image. The
  manifest declares a prelude hook (script run during `sciontool init`)
  that fetches and installs spec-kit on first spawn.
- **Operator's own CLIs** — TBD pending the concrete list. Same
  per-harness prelude pattern when known.

This keeps images small and changes the maintenance model: image rebuilds
are rare, manifest edits handle role-specific tooling.

## 5. Skill repository, taxonomy, and mount mechanism

### 5.1 Taxonomy (five categories)

| Category | Definition | Examples |
|---|---|---|
| **Economy** | Minimize tokens / context | `caveman`, `context-mode` |
| **Workflow** | Structure how work flows | `bones`, `superpowers`, `spec-kit` |
| **Backpressure** | Quality gates and "slow down" disciplines | `ousterhout`, `hipp`, `tigerstyle`, `code-review`, sme experts |
| **Tooling** | Domain-specific tools | `idiomatic-go`, LSPs, language patterns |
| **Context** | Information loaders, reference docs | `context7`, `mgrep` (as skill), codebase loaders |

Five categories is the starting set. The taxonomy is open; new categories
can be added when a skill doesn't fit existing ones.

### 5.2 Layering (three scopes)

| Tier | Scope | Source | Mount mechanism |
|---|---|---|---|
| **Base** | Universal — every harness | Baked into every darkish-* image | Already at `/home/scion/skills/<name>/` in image |
| **Role-specific** | Per-harness, declared in manifest's `skills:` field | Canonical: `~/projects/agent-skills/skills/<name>/` (APM-style references) | Host script `stage-skills.sh` materializes copies into `<repo>/.scion/skills-staging/<harness-name>/`; manifest mounts that staging directory |
| **Ad-hoc** | Operator/orchestrator decides at runtime, between dispatches | Same canonical source | Orchestrator calls `stage-skills.sh <harness> --add <skill>` between dispatches; next spawn picks up the new set |

Live updates within a single running harness session are out of scope.
Updates take effect on the next dispatch.

### 5.3 Per-harness skill defaults

| Harness | Role-specific skills (beyond base) |
|---|---|
| `orchestrator` | `dx-audit` |
| `researcher` | (none) |
| `designer` | `norman`, `ousterhout` |
| `planner-t1` | `hipp` |
| `planner-t2` | `hipp`, `ousterhout` |
| `planner-t3` | `hipp`, `ousterhout`, `superpowers` |
| `planner-t4` | `hipp`, `ousterhout`, `spec-kit` |
| `tdd-implementer` | `idiomatic-go`, `tigerstyle` |
| `verifier` | `tigerstyle` |
| `reviewer` | `ousterhout`, `hipp`, `dx-audit` |
| `sme` | `ousterhout`, `hipp` |
| `admin` | (none) |
| `darwin` | `dx-audit` |

`darwin` mutates this table over time as evidence from completed sessions
shows what's actually used vs. dead weight.

### 5.4 Manifest declaration format

APM-style references in `scion-agent.yaml`:

```yaml
skills:
  - danmestas/agent-skills/skills/ousterhout
  - danmestas/agent-skills/skills/hipp
```

Same syntax as `apm.yml` at the project root. Single dependency-declaration
vocabulary across the project.

### 5.5 Stage script

`scripts/stage-skills.sh <harness-name>` is idempotent:

1. Reads `<repo>/.scion/templates/<harness>/scion-agent.yaml`.
2. Resolves each `skills:` reference against
   `~/projects/agent-skills/skills/<name>/` (or APM-resolved location for
   external refs).
3. Copies into `<repo>/.scion/skills-staging/<harness-name>/<name>/`,
   overwriting on each run.
4. The manifest's `volumes:` block mounts that staging directory at
   `/home/scion/skills/role/` in the container.

`scripts/stage-skills.sh <harness> --add <skill>` mutates the manifest
in-memory or appends to a sidecar override file before re-running step 1;
the orchestrator uses this for ad-hoc tier updates.

## 6. Auth strategy

### 6.1 Hub-secrets-everywhere

Uniform across all four backends. Every credential lives in scion's hub
secret store, not in operator shell env or harness manifests. This:

- Preserves OAuth (Codex Max, Claude Pro/Team) — file-type secrets project
  the OAuth credentials file into the container at the canonical path.
- Eliminates the "API key in shell env" leak surface — secrets never appear
  in `docker inspect`, scion error messages, or operator transcripts once
  hub-managed.
- Survives operator machine moves: any host running the scion broker can
  pull secrets from the hub.
- Refreshes via `scripts/stage-creds.sh` — operator re-runs after token
  rotation; the hub store is updated; next spawn picks it up.

### 6.2 Per-backend pattern

| Backend | Source on host | Hub secret type | Container target |
|---|---|---|---|
| claude | macOS Keychain `Claude Code-credentials` | file | `/home/scion/.claude/.credentials.json` |
| codex | `~/.codex/auth.json` | file | `/home/scion/.codex/auth.json` |
| pi | `OPENROUTER_API_KEY` env var | env | (inherits as env in container) |
| gemini | `GEMINI_API_KEY` env var (or `~/.gemini/oauth_creds.json` if OAuth) | env or file | as appropriate |

### 6.3 stage-creds.sh updates

Single script handles all four backends:

```
scripts/stage-creds.sh           # all
scripts/stage-creds.sh claude    # only claude
scripts/stage-creds.sh codex     # only codex
scripts/stage-creds.sh pi        # only pi
scripts/stage-creds.sh gemini    # only gemini
```

Each section: read source → `scion hub secret set --type <env|file>
--target <path> <NAME> @<source>`.

### 6.4 OAuth preservation

Both Anthropic OAuth (Claude Pro/Team via macOS Keychain) and OpenAI OAuth
(Codex Max via `~/.codex/auth.json`) flow through the file-type hub
secrets unchanged. The container receives the OAuth credentials file at
the path the CLI expects, the CLI uses OAuth for API calls, and no API
key appears anywhere in env or process listings. This is validated for
codex (gpt-5.5 ran on Codex Max OAuth in the smoke test).

### 6.5 Manifest auth references

With hub-secrets-everywhere, harness manifests no longer carry
`volumes:` blocks for auth. Each manifest just declares
`default_harness_config` and the model; scion's auth resolution finds the
hub secret automatically based on the harness type.

The previous `volumes:` block in `researcher` (and the eight other
flipped harnesses) is removed as part of this migration.

## 7. Per-harness manifest shapes

Every harness's `scion-agent.yaml` follows this shape:

```yaml
schema_version: "1"
description: "<one-sentence role description>"
agent_instructions: agents.md
system_prompt: system-prompt.md
default_harness_config: <claude | codex>
image: local/darkish-<backend>:latest
model: <model-id>
max_turns: <int>
max_duration: "<duration>"
detached: <bool>
skills:
  - <APM-style ref>
  - ...
```

Concrete defaults per harness:

| Harness | image | model | turns | duration | detached |
|---|---|---|---|---|---|
| orchestrator | darkish-claude | claude-opus-4-7 | 200 | 4h | false |
| researcher | darkish-claude | claude-sonnet-4-6 | 30 | 1h | false |
| designer | darkish-claude | claude-opus-4-7 | 50 | 1h | false |
| planner-t1 | darkish-claude | claude-sonnet-4-6 | 15 | 30m | false |
| planner-t2 | darkish-claude | claude-opus-4-7 | 30 | 1h | false |
| planner-t3 | darkish-claude | claude-opus-4-7 | 50 | 2h | false |
| planner-t4 | darkish-codex | gpt-5.5 | 100 | 4h | false |
| tdd-implementer | darkish-claude | claude-sonnet-4-6 | 100 | 2h | false |
| verifier | darkish-codex | gpt-5.5 | 50 | 2h | false |
| reviewer | darkish-codex | gpt-5.5 | 30 | 1h | false |
| sme | darkish-codex | gpt-5.5 | 10 | 15m | false |
| admin | darkish-claude | claude-haiku-4-5 | 100 | 8h | true |
| darwin | darkish-codex | gpt-5.5 | 50 | 4h | false |

## 8. Routing classifier extension

The README §6.1 routing classifier currently emits
`light | heavy | ambiguous`. With four planner tiers, the classifier
extends to choose the planner directly:

| Routing output | Planner |
|---|---|
| `t1` | planner-t1 (small bug, think-then-do) |
| `t2` | planner-t2 (claude-code style, light plan doc) |
| `t3` | planner-t3 (superpowers flow, design + plan) |
| `t4` | planner-t4 (spec-kit, full ratification) |
| `ambiguous` | route to t3 (most cases benefit from design discipline; t4 is reserved for "we know we need a spec") |

The light/heavy distinction collapses into the t1..t4 spectrum. The
operator can override at dispatch time.

## 9. Failure modes

| Failure | Detection | Recovery |
|---|---|---|
| `stage-skills.sh` fails (skill not found at canonical source) | Exit non-zero with skill name | Operator clones the missing skill into `~/projects/agent-skills/`; re-runs |
| `stage-creds.sh` fails (Keychain locked, OAuth expired, env var missing) | Exit non-zero per-backend section | Operator unlocks Keychain or re-authenticates the relevant CLI; re-runs |
| Hub secret missing at spawn time | Scion broker rejects start with "auth resolution failed" | Operator runs `stage-creds.sh <backend>`; re-tries |
| Image missing locally (`local/darkish-<backend>:latest`) | Scion broker rejects start with "no such image" | Operator runs `make -C images <backend>`; re-tries |
| Skill staging directory missing at spawn | Volume mount fails | Operator runs `stage-skills.sh <harness>`; re-tries |
| Symlink-to-directory in `skills:` (regression) | Scion's template-copy errors with "is a directory" | Use `stage-skills.sh` (copy, not symlink); never put a directory symlink in a template |
| Caveman tier mismatch (harness uses wrong register for audience) | Manifests in transcripts; `darwin` flags during analysis | Update the harness's system prompt; future dispatches honor it |
| Bones binary missing in image | Container start succeeds but harness's sub-agent commands fail | Rebuild darkish-* image with bones layer corrected |

## 10. Testing & verification

- **Image smoke**: each darkish-* image builds; `local/darkish-<backend>:latest` exists; `docker run --rm local/darkish-<backend>:latest jq --version` returns OK.
- **Universal-baseline check**: every darkish-* image has `jq`, `ripgrep`, `mgrep`, all 14 bones binaries on PATH, the caveman skill at `/home/scion/skills/caveman/`, and the context-mode MCP available.
- **Per-harness skill mount**: `stage-skills.sh <harness>` produces a populated `<repo>/.scion/skills-staging/<harness>/`; container mount makes them available at `/home/scion/skills/role/`.
- **Hub-secret integration**: `stage-creds.sh all` populates all four backends' secrets; subsequent `scion start <harness>` succeeds without `--no-auth`.
- **Communication tier**: each system prompt names its tier; smoke-test asserts the harness uses caveman wording when responding to orchestrator and natural wording is reserved for operator output.
- **Override mechanism**: `scion start <harness> --harness pi --image local/darkish-pi:latest` swaps backend without manifest edits.

## 11. Open questions

- **Operator's own CLIs.** Concrete list still pending. Image-bake vs. per-harness install pending that list.
- **Spec-kit prelude hook details.** The exact mechanism for installing spec-kit at planner-t4 spawn time (curl from release URL? clone the repo? brew?) needs verification on first run.
- **Pi trust mechanism.** Existing pi-templates work, suggesting `--non-interactive` bypasses any prompt; not yet validated for darkish-pi.
- **Gemini trust mechanism.** Unverified pending operator account setup.
- **Caveman tier enforcement.** Tier discipline is per-system-prompt directive. How strict should we be — soft directive in prose, or a `caveman:` field in the manifest that the prelude validates? Open.
- **Skill cycle detection.** If a skill `A` includes skill `B` and `B` includes `A`, the stage-skills script may loop. Needs cycle protection if APM-resolution starts pulling chains.
- **Darwin's mutation authority.** Can `darwin` directly edit harness manifests, or does it only emit recommendations the operator ratifies? Default: recommendations only — operator is the final gate. Confirm.
- **Stage-skills.sh in `--add` mode persistence.** Does an ad-hoc add survive across orchestrator restarts, or is it session-scoped? Default: session-scoped (orchestrator's audit log records it; restart re-stages from manifest only).
- **Bones authorization.** Sub-agents spawned by a harness run with what credentials? Inherit from parent harness, or get their own scoped secret? Default: inherit. Confirm.

## 12. Cross-references

- Slice 1 (escalation classifier): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-escalation-classifier-design.md`
- Slice 2 (orchestrator + runtime adapter): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-orchestrator-skeleton-design.md`
- Slice 3 (specialized harnesses + DAG): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-specialized-harnesses-design.md`
- Slice 4 (review and merge): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-review-and-merge-design.md`
- Slice 5 (cost mode and drift guard): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-cost-mode-and-drift-guard-design.md`
- Slice 6 (dark variants): `/Users/dmestas/projects/darkish-factory/docs/superpowers/specs/2026-04-25-dark-variants-design.md`
- Constitution: `/Users/dmestas/projects/darkish-factory/.specify/memory/constitution.md`
- Source: `/Users/dmestas/projects/darkish-factory/README.md` §2, §5.1, §5.7, §6.1, §6.3, §9
- External skill repos:
  - `https://github.com/danmestas/agent-skills` (canonical skill source)
  - `https://github.com/juliusbrussee/caveman` (universal communication-tier skill)
  - `https://github.com/mksglu/context-mode` (universal MCP)
  - `https://github.com/danmestas/agent-infra` (the "bones" binaries)
  - `https://github.com/obra/superpowers` (planner-t3 framework)
  - `https://github.com/github/spec-kit` (planner-t4 framework)
