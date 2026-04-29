# Roles + Bases — Collapse 14 Templates into Per-Spawn Agent-Config Overrides

**Status:** approved (in-conversation, 2026-04-28) — implementation deferred
**Author:** dmestas
**Source PRs:** TBD
**Related specs:**
- `2026-04-28-bones-inter-harness-comms-design.md`
- `2026-04-28-remote-human-comms-deferral-design.md`

## Context

`.scion/templates/` currently holds 14 directories — one per role (plus a `base` placeholder):

```
admin/  base/  darwin/  designer/  orchestrator/
planner-t1/ planner-t2/ planner-t3/ planner-t4/
researcher/ reviewer/ sme/ tdd-implementer/ verifier/
```

Each contains the same three files:

- `agents.md` — role-specific instructions
- `system-prompt.md` — role-specific system prompt
- `scion-agent.yaml` — manifest

A representative manifest (`planner-t1/scion-agent.yaml`):

```yaml
schema_version: "1"
description: "Planner T1 - think-then-do; small bug fixes; no plan doc"
agent_instructions: agents.md
system_prompt: system-prompt.md
default_harness_config: claude
image: local/darkish-claude:latest
model: claude-sonnet-4-6
max_turns: 15
max_duration: "30m"
detached: false
skills:
  - danmestas/agent-skills/skills/hipp
volumes:
  - source: ./.scion/skills-staging/planner-t1/
    target: /home/scion/skills/role/
    read_only: true
```

**Problem**: most fields are duplicated across roles. The 14 manifests differ only in:

- `description`, `agent_instructions`, `system_prompt` (always different — these ARE the role)
- `model`, `max_turns`, `max_duration` (occasionally different)
- `image` / `default_harness_config` (varies by harness flavor — claude/gemini/codex/pi)
- `skills` and `volumes.source` path (varies by role's skill bundle)

Updating shared config (e.g., bumping the image tag) requires touching 14 files. Adding a new role means copy-pasting a directory. Manifest drift is a real risk.

## Background — Scion primitives we can lean on

From `reference/agent-config/` and `contributing/architecture/`:

| Primitive | What it gives us |
|---|---|
| **Template chain** (`base` field) | Manifests can declare a base template; configs merge bottom-up (base first, leaf wins). |
| **Per-instance `scion-agent.yaml`** | The active-agent manifest at `.scion/agents/<name>/scion-agent.yaml` can override any field. CLI flags override that. |
| **`home/` directory tree** | Templates ship a `home/` dir that gets copied into the agent's container home — useful for shared dotfiles, tooling. |
| **Resolution order** | CLI flag > per-agent override > template chain (leaf→base) > settings (profile/harness/runtime) > env vars > embedded defaults. |
| **`services` block** | Sidecar containers (e.g., headless browser) declared per-template. |

These mean: a template doesn't have to be the unit of role identity. **Role identity can live in role manifests merged into a base template at spawn time.**

## Goal

Reduce 14 role-specific templates to **3–4 base templates** (one per harness flavor) plus a flat `roles/` directory of per-role manifests. `bin/darken spawn` materializes a per-spawn agent-config overlay that references the base template and injects role identity.

## Goals

After implementation:

- Editing a role is **one file** (`roles/<role>.md` or `roles/<role>.yaml`).
- Bumping a shared image, env var, or skill is **one base** edit (`templates/darken-claude-base/scion-agent.yaml`).
- Adding a new role costs **one markdown body + one INDEX.yaml row**.
- Role discovery still works: `darken roles list`, `darken roles show <role>`.
- Per-role skill bundles still mount at the conventional `/home/scion/skills/role/` path.
- No regressions in existing spawn flows (`bin/darken spawn researcher-1 --type researcher`).

## Non-goals

- Replacing Scion's template format. We use it as designed (chain merge); we just stop writing one template per role.
- Eliminating the `.scion/skills-staging/` per-role staging directories. Those continue to exist; the manifests just point at them dynamically.
- Reducing role expressiveness. Anything a current template can do (sidecars, custom env, GPU requests, k8s overrides), the new shape can also do.
- Migrating skills out of role bundles into base templates. Out of scope.

## Target structure

```
.scion/
├── templates/
│   ├── darken-claude-base/
│   │   ├── home/                       # shared dotfiles (optional)
│   │   ├── system-prompt.md            # generic prompt; roles append
│   │   └── scion-agent.yaml            # claude harness wiring, image, default env
│   ├── darken-gemini-base/
│   │   └── ... (same shape, gemini harness)
│   ├── darken-codex-base/
│   │   └── ... (codex harness)
│   └── darken-pi-base/                 # pi = generic harness for utility containers
│       └── ...
├── roles/
│   ├── INDEX.yaml                      # roster + role→base + overrides
│   ├── researcher/
│   │   ├── agents.md                   # agent_instructions
│   │   └── system-prompt.md            # appended to base prompt (optional)
│   ├── planner-t1/
│   │   ├── agents.md
│   │   └── system-prompt.md
│   └── ... (one dir per role)
└── skills-staging/                     # unchanged — per-role skill bundles
    ├── researcher/
    ├── planner-t1/
    └── ...
```

### Base template manifest (`darken-claude-base/scion-agent.yaml`)

```yaml
schema_version: "1"
description: "Darken Claude base — claude harness wiring + shared defaults"
default_harness_config: claude
image: local/darkish-claude:latest
model: claude-sonnet-4-6
max_turns: 30
max_duration: "60m"
detached: true
# system_prompt and agent_instructions intentionally NOT set —
# they are supplied per-spawn from roles/<role>/.
env:
  DARKEN_HARNESS_FLAVOR: claude
```

### `roles/INDEX.yaml`

```yaml
schema_version: "1"
roles:
  researcher:
    base: darken-claude-base
    instructions: roles/researcher/agents.md
    system_prompt_append: roles/researcher/system-prompt.md
    skills_staging: .scion/skills-staging/researcher/
    skills:
      - danmestas/agent-skills/skills/hipp
    overrides:
      max_turns: 15
      max_duration: "30m"
      detached: false
  planner-t1:
    base: darken-claude-base
    instructions: roles/planner-t1/agents.md
    system_prompt_append: roles/planner-t1/system-prompt.md
    skills_staging: .scion/skills-staging/planner-t1/
    skills:
      - danmestas/agent-skills/skills/hipp
    overrides:
      max_turns: 15
      max_duration: "30m"
      detached: false
  darwin:
    base: darken-pi-base
    instructions: roles/darwin/agents.md
    skills_staging: .scion/skills-staging/darwin/
    overrides: {}
  # ... (one entry per role)
```

The INDEX is the **single source of truth** for the roster. `darken roles list` reads it. `darken spawn --type <role>` looks up the entry.

## Spawn flow change

### Today

```
bin/darken spawn researcher-1 --type researcher
  → calls scion start with --template researcher
  → scion reads .scion/templates/researcher/scion-agent.yaml
  → scion creates the agent
```

### Target

```
bin/darken spawn researcher-1 --type researcher
  → reads .scion/roles/INDEX.yaml → resolves "researcher" → base + overrides
  → reads roles/researcher/agents.md (instructions body)
  → reads roles/researcher/system-prompt.md (append to base prompt)
  → materializes .scion/agents/researcher-1/scion-agent.yaml as the OVERLAY:
        agent_instructions: <inlined body or relative ref>
        system_prompt:      <merged prompt>
        skills:             [<from INDEX>]
        volumes:
          - source: .scion/skills-staging/researcher/
            target: /home/scion/skills/role/
            read_only: true
        max_turns:    15        # from INDEX overrides
        max_duration: 30m       # from INDEX overrides
        detached:     false
  → calls scion start --template darken-claude-base
        (and Scion merges the per-agent overlay on top — its documented behavior)
```

The per-agent manifest at `.scion/agents/<name>/scion-agent.yaml` is exactly the merge layer Scion's resolution logic step 3 (CLI > **per-agent override** > template chain > settings) is designed for. We're using the substrate as documented.

## CLI surface (`bin/darken`)

New / changed subcommands:

```
darken roles list                    # print INDEX.yaml roster as table
darken roles show <role>             # print resolved manifest for a role (preview merge)
darken roles validate                # check INDEX entries against base templates / skills paths
darken spawn <name> --type <role>    # unchanged signature; new resolution internals
darken spawn <name> --base darken-claude-base \
                    --instructions <path>      # bypass INDEX for ad-hoc spawns
```

## Migration plan (one-shot, single PR)

### Step 1 — Inventory the diffs

Diff all 14 current `scion-agent.yaml` manifests pairwise. Categorize each field:

- **Always identical across roles using the same harness** → goes into the base template.
- **Always-different per role** (`agent_instructions`, `system_prompt`, `description`, `volumes.source`) → goes into `roles/<role>/` files or INDEX entry.
- **Sometimes-different** (`model`, `max_turns`, `max_duration`, `detached`) → goes into `INDEX.yaml` per-role `overrides:` map; left absent in the base default.

Output: a small markdown table in the migration PR description, e.g.:

| Field | Same across all? | Lift to base? | Note |
|---|---|---|---|
| `image` | Within harness flavor, yes | Yes (per-base) | claude→`darkish-claude:latest` etc. |
| `model` | No (planners use sonnet, others may differ) | No — INDEX override | |
| `max_turns` | No | INDEX override | |
| `agent_instructions` | No (always different) | No — `roles/<role>/agents.md` | |
| ... | | | |

### Step 2 — Build base templates

Create `.scion/templates/darken-{claude,gemini,codex,pi}-base/`. Populate each with:

- `scion-agent.yaml` (the lifted-common fields).
- A minimal `home/` if existing templates had shared home content (audit current `home/` dirs first; if none, omit).
- A minimal generic `system-prompt.md` if the role-level prompts share common preamble — otherwise leave empty and let roles supply the full prompt.

### Step 3 — Move role bodies

For each existing template `<role>/`:

1. Copy `agents.md` → `.scion/roles/<role>/agents.md`.
2. Copy `system-prompt.md` → `.scion/roles/<role>/system-prompt.md`.
3. Add the role's row to `INDEX.yaml` with `base`, `instructions`, `system_prompt_append`, `skills_staging`, `skills`, and any `overrides`.
4. Delete the original `.scion/templates/<role>/` directory.

### Step 4 — Rewrite `bin/darken spawn`

Replace the current "pass `--template <role>` to scion" path with the overlay assembler:

- Read `INDEX.yaml`.
- Read role `agents.md` and (if present) `system-prompt.md`.
- Merge with base template's `system-prompt.md` (concatenation: base then role).
- Materialize the per-agent overlay file at `.scion/agents/<name>/scion-agent.yaml`.
- Invoke `scion start --template <base>` with the agent-name flag pointing at the same dir.

### Step 5 — Add `darken roles` subcommands

`list`, `show`, `validate` — pure read operations against `INDEX.yaml` plus light filesystem probes.

### Step 6 — Tests

- Unit test the INDEX resolver (given an INDEX entry + base manifest, produces a correct overlay).
- Integration test: spawn each role end-to-end and assert `agent_instructions`, `system_prompt`, `skills`, and `volumes` all resolve to the same content the old templates would have produced.
- Snapshot test the INDEX schema with a JSON Schema or CUE definition (catch typos before they hit runtime).

### Step 7 — Documentation

- Update `CLAUDE.md`'s "13-role roster" reference.
- Document in `docs/CLI.md` the new `darken roles` subcommands.
- Add a one-paragraph "How roles work" section pointing at `roles/INDEX.yaml`.

### Step 8 — Cleanup

- Remove `.scion/templates/<role>/` for the 13 role-specific templates.
- Keep `.scion/templates/base/` only if it had a non-trivial purpose; otherwise delete.

## Risks

- **Discoverability regression**: `ls .scion/templates/` no longer shows the roster. Mitigated by `darken roles list` and a brief note in `CLAUDE.md`.
- **Overlay merge semantics drift**: if Scion ever changes how per-agent manifests merge with templates, our spawn flow breaks. Pin behavior with the integration test in Step 6.
- **Skill staging path coupling**: `INDEX.yaml`'s `skills_staging` paths must match `.scion/skills-staging/` directory layout. Validate at `darken roles validate` time.
- **Mid-migration breakage**: this is a single-PR change. Don't ship half-migrated. Use a feature branch and verify all 14 roles still spawn before merging.

## Open questions

### 1. Per-role `home/` overlays

Some roles (e.g., `darwin` which interacts with macOS configs, or future browser-using roles) may need extra dotfiles or scripts in `/home/<user>/`. Two options:

**A. Each role optionally has `roles/<role>/home/`** that the spawner copies on top of the base's `home/`. Cleanest, preserves base immutability.

**B. Keep `home/` exclusively in base templates.** If a role needs different home content, it gets its own base template (e.g., `darken-claude-darwin-base`).

Lean **A**. Add to INDEX schema as `home_overlay: roles/<role>/home/` (optional). Defer implementation until first role demands it.

### 2. Multiple bases per role (cross-harness flexibility)

Should a role be allowed to declare it works with `claude` OR `gemini`, picked at spawn time? E.g., `--type researcher --harness gemini`?

INDEX could become:

```yaml
researcher:
  bases:
    claude: darken-claude-base
    gemini: darken-gemini-base
  default_base: claude
  instructions: ...
```

Useful for cost/perf trades; complicates resolution. **Defer** — add only when first concrete need surfaces. For now, one base per role.

### 3. `system_prompt_append` semantics

When the base has a system prompt and the role has one too, do we:

- **Concatenate** (base + "\n\n---\n\n" + role)
- **Replace** (role wins entirely)
- **Template** (base has `{{role_prompt}}` interpolation point)

Lean **concatenate** (simplest, transparent in `darken roles show`). Templating is over-engineering. Replace is fine if base prompts are empty (likely the common case).

### 4. INDEX.yaml schema versioning

Add `schema_version: "1"` to INDEX from day one. When fields evolve, write a migrator. (Lesson learned: spawn-time format evolution is costly without versioning.)

### 5. Where INDEX lives

`.scion/roles/INDEX.yaml` is project-scoped. Should it be checked in? **Yes** — roles are part of the project's substrate, like templates were. Globbed `darken-*-base` templates remain the common scaffolding (potentially shipped via `darken init` or `make darken` like today).

## Dependencies

- Spec #2 (bones inter-harness comms) — independent design, but image rebuilds happen together.
- Spec #1 (remote human comms deferral) — independent.
- No external (Scion) dependency: this spec uses Scion's documented template-chain + per-agent-override merge, no new substrate features.
