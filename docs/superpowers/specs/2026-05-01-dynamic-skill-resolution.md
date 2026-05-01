# Spec: dynamic skill resolution via modes

**Date:** 2026-05-01
**Status:** Draft, awaiting operator approval of written spec
**Topic:** Replace per-role static skill bundling with operator-selectable *modes* — named, explicit skill lists picked at `darken spawn` time. Goal: quickly configure a harness for the task at hand without forking the role.

## Problem

Today every spawn of a given role gets the same fixed skill bundle. The skill set is declared in `.scion/templates/<role>/scion-agent.yaml` under `skills:` and copied to `.scion/skills-staging/<role>/` at spawn time.

Two consequences:

1. A `tdd-implementer` doing a Go systems task and one doing a TypeScript UI task see identical skills. The bundle is the union of "things any tdd-implementer might need," which means the worker reads more skill content than the task warrants.
2. The operator has no way to override the bundle for a one-off task without editing the role manifest.

The operator wants to apply skillsets per task, fast. Skills are already authored (49 in `agent-config/skills/` plus repo-local skills), but the per-role bundles are hand-curated and grow stale.

## Goals

- At spawn time, the operator names a *mode*. The mode declares which skills the harness should see. The stager copies exactly those skills.
- Default behavior preserved: no `--mode` flag means the role's default mode resolves, which produces the same staging output as today.
- One operator-facing command (`darken modes list`) to enumerate available modes.

## Non-goals

- Category-based composition (suit's `categories ∩` model). Modes are explicit lists. Tagging skills with categories is a future evolution if explicit lists prove too rigid; this spec does not preclude it.
- Marketplace registry. `agent-config/marketplace/` is a pattern worth borrowing later for a public skill index; out of scope here.
- Mode authoring tools. `darken modes new`, `darken modes validate`, scaffolding — author by hand for v1.
- System-prompt prepends per mode. suit's modes inject prose; darkish-factory's role manifests own that via `system-prompt.md`. Modes affect skills only.
- Targeting modes to specific harness backends (`targets: [claude-code, codex]`). Role implies backend; mode-level targeting is double-spec.

## Design

### Mode YAML schema

Located at `.scion/modes/<name>.yaml`:

```yaml
name: tdd-go
description: "Go systems work — TDD-driven, philosophy-aware."
skills:
  - tdd
  - ousterhout
  - idiomatic-go
  - hipp
```

Three required fields:

- `name` — string, must match the filename stem. Used as the value of `--mode`.
- `description` — one-liner shown by `darken modes list`.
- `skills` — ordered list of skill names. Each name is resolved against the existing path resolver (currently `agent-config/skills/<name>/` via the `danmestas/agent-skills` rewrite).

Schema is deliberately tight. Optional fields (`extends`, `targets`, `categories`, prose body) are deferred until concrete demand.

### Resolution at spawn time

`darken spawn <name> --type <role> [--mode <name>] [--skills foo,bar] "<task>"` resolves the skill set in this order:

1. **Pick mode.** If `--mode <m>` is passed, use `m`. Otherwise read `default_mode` from `.scion/templates/<role>/scion-agent.yaml`. Load `.scion/modes/<m>.yaml`.
2. **Base set.** Mode's `skills:` list.
3. **Ad-hoc additions.** If `--skills foo,bar` is passed, take the union with the base set. Duplicates collapse. Order preserved (mode's skills first, ad-hoc additions appended).
4. **Stage.** For each resolved skill name, the existing path resolver locates the source dir; the stager copies it to `.scion/skills-staging/<harness>/<skill>/` as today.
5. **Mount.** Container mounts the staging dir. No change below the stager.

### Validation

- Missing mode file: `mode <name>: not found at .scion/modes/<name>.yaml`. Spawn aborts.
- Mode references a skill that doesn't resolve: `mode <name>: skill <skill> not found at <resolved_path>`. Spawn aborts. Same failure shape as today's manifest path that doesn't resolve.
- `name` field doesn't match filename stem: `mode <name>: name field "<X>" does not match filename "<Y>.yaml"`. Spawn aborts. Catches copy-paste errors.

Validation runs at spawn time only — no separate `darken modes validate` command in v1.

### Default-mode naming convention

The 14 canonical roles each get a default mode named *after the role*: `tdd-implementer`, `planner-t3`, `researcher`, etc. Mode and role share names but live in different files (`.scion/templates/<role>/scion-agent.yaml` vs `.scion/modes/<role>.yaml`). One-to-one mapping, mechanical migration.

This is intentionally not a taxonomy. Operator-friendly mode names (`code`, `design`, `plan`, `recon`) are a follow-up consolidation once duplicates among the 14 default modes emerge in practice.

### Role manifest changes

Each `.scion/templates/<role>/scion-agent.yaml`:

- **Add** `default_mode: <role>`.
- **Remove** the existing `skills:` field. Modes are the single source of truth post-migration.

Example before/after for `planner-t3`:

```yaml
# Before
skills:
  - danmestas/agent-skills/skills/hipp
  - danmestas/agent-skills/skills/ousterhout
  - danmestas/agent-skills/skills/superpowers

# After
default_mode: planner-t3
```

The `.scion/modes/planner-t3.yaml` file then carries the skill list.

### CLI surface additions

- `darken modes list` — print all `.scion/modes/*.yaml` with `name` and `description` columns.
- `darken modes show <name>` — cat the mode file.
- `darken spawn` gains `--mode <name>` and `--skills <comma-list>` flags.

Out for v1: `darken modes new`, `darken modes validate`, mode authoring scaffolds.

### Stager changes

`internal/substrate/staging/` (where the path resolver lives) gains a `mode_resolver.go` that:

- Loads a mode by name from `.scion/modes/<name>.yaml`.
- Validates required fields and filename-stem match.
- Returns the resolved skill name list.

The existing path resolver — which knows how to find `<repo>/skills/<name>` locally and via the `agent-config` rewrite — gets called with the mode's skill names instead of the manifest's path strings. No new resolution logic; bare names go in, paths come out.

The spawn entry point in `cmd/darken/spawn.go`:

- Parses new flags `--mode` and `--skills`.
- Resolves mode (explicit or via role's `default_mode`).
- Computes the skill set per the resolution algorithm above.
- Hands off to the existing stager.

## Migration (big-bang)

Single PR. Steps:

1. **Extract.** Read each of 14 role manifests; capture the current `skills:` list (paths like `danmestas/agent-skills/skills/hipp`).
2. **Translate.** Convert paths to bare skill names (`hipp`, `ousterhout`, etc.). The path-rewrite logic that turns `danmestas/agent-skills` into `agent-config` already exists; extract the bare-name parser into a helper if not already done.
3. **Author modes.** Write `.scion/modes/<role>.yaml` for each canonical role. `name: <role>`, `description: "Default skills for the <role> harness."`, `skills:` populated from the translated list.
4. **Edit manifests.** Each `.scion/templates/<role>/scion-agent.yaml`: add `default_mode: <role>`, remove `skills:`.
5. **Update stager.** New mode-resolver pathway; remove the manifest-`skills:`-field reading path.
6. **Test.** Golden-file equivalence per role (see Test strategy).

Atomic. The PR is shippable when all 14 golden tests pass.

## Test strategy

### Unit — mode resolver

`mode_resolver_test.go`:

- Loads a fixture mode YAML, asserts parsed `name`/`description`/`skills`.
- Filename-stem mismatch returns the validation error.
- Missing mode file returns the not-found error.
- Skill name not found returns the per-skill resolution error.

### Integration — golden equivalence

For each of the 14 canonical roles, a golden test verifies the staging directory contents are byte-identical pre- and post-migration:

1. **Pre.** On `main`, spawn-dry-run for `<role>` produces a staging tree. Hash-tree captured as golden.
2. **Post.** On the migration branch, spawn-dry-run for the same `<role>` (no `--mode` flag — defaults resolve via `default_mode`) produces a staging tree. Hash-tree must match the golden.

Big-bang is safe iff all 14 goldens match. Mismatch indicates the migration translated paths or skill order incorrectly.

### End-to-end — explicit vs. implicit mode

`darken spawn x --type researcher` and `darken spawn x --type researcher --mode researcher` must produce byte-identical staging output. Both pathways go through the resolver after migration; the implicit form just reads `default_mode` from the manifest before calling the same code.

### Ad-hoc — `--skills` union

`darken spawn x --type researcher --skills extra-skill` produces the researcher mode's skills plus `extra-skill`, in that order, no duplicates.

### Failure-path

- Spawn with unknown `--mode unknown` exits non-zero with the mode-not-found error.
- Spawn with mode that references a non-existent skill exits non-zero with the skill-not-found error.

## Backward-compatibility

- **Operators using `darken spawn --type <role>` with no flags.** Behavior preserved: role's `default_mode` resolves to the migrated mode, which lists the same skills the manifest's `skills:` field listed before. Staging output identical (golden tests enforce).
- **External consumers of `.scion/templates/<role>/scion-agent.yaml`.** The `skills:` field is removed. If anything outside the cmd/darken codebase reads it, that breaks. Spec assumes nothing else reads it; verify during migration with a `grep -r "skills:" .scion/templates/` and a code-search for manifest-loading paths.
- **Role-default mode rename.** A future operator-friendly taxonomy (`code`, `design`, etc.) would change `default_mode` values. That's an additive follow-up, not a breaking change for this spec.

## Open questions for operator

These are the proposed defaults; flag if any need flipping.

1. **Default-mode naming convention.** `<role>` (proposed; e.g. `tdd-implementer`) vs `<role>-default` (more explicit; e.g. `tdd-implementer-default`).
2. **`--skills` ad-hoc.** Yes (proposed; one flag, union step). vs No (single-source-of-truth purism — operators must author a new mode for any deviation).
3. **PR shape.** Single PR with all 14 mode files + 14 manifest edits + stager change (proposed). vs Stager-change PR followed by 14 small migration PRs (one per role, parallelizable, easy to review individually).

## File-paths cited

- `.scion/templates/<role>/scion-agent.yaml` (14 files)
- `.scion/modes/<name>.yaml` (new directory)
- `cmd/darken/spawn.go` (CLI parsing)
- `internal/substrate/staging/` (mode resolver lives here)
- `agent-config/skills/<name>/` (skill source pool, unchanged)

## References

- Researcher-2 brief: `.scion/agents/researcher-2/workspace/docs/research-brief-suit.md`
- suit (algorithm reference, not adopted): `https://github.com/danmestas/suit`, `src/lib/resolution.ts`, `src/lib/types.ts`
- agent-config (skills source + existing modes): `https://github.com/danmestas/agent-config`, `modes/code/mode.md` for format-comparison
