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
- Modes can compose via `extends:` so that shared skill-prefixes (e.g. philosophy baselines used by every planner) are declared once.
- One operator-facing command (`darken modes list`) to enumerate available modes.

## Non-goals

- Category-based composition (suit's `categories ∩` model). Modes are explicit lists with optional `extends:` parent reference. Tagging skills with categories is a future evolution; this spec does not preclude it.
- Marketplace registry. `agent-config/marketplace/` is a pattern worth borrowing later for a public skill index; out of scope here.
- Mode authoring tools. `darken modes new`, `darken modes validate`, scaffolding — author by hand for v1.
- System-prompt prepends per mode. suit's modes inject prose; darkish-factory's role manifests own that via `system-prompt.md`. Modes affect skills only.
- Targeting modes to specific harness backends (`targets: [claude-code, codex]`). Role implies backend; mode-level targeting is double-spec.
- Ad-hoc `--skills foo,bar` at spawn time. Single way to specify skills: name a mode. If composition becomes painful for one-offs, author a small mode (4-line YAML) and delete it after use, or use `extends:` to compose. A side-channel skills flag would create two ways to do one thing.

## Design

### Mode YAML schema

Located at `.scion/modes/<name>.yaml`. The file's name on disk IS the mode's name — there is no `name` field in the YAML.

```yaml
description: "Go systems work — TDD-driven, philosophy-aware."
extends: philosophy-base
skills:
  - tdd
  - idiomatic-go
```

Fields:

- `description` (required) — one-liner shown by `darken modes list`.
- `skills` (required) — ordered list of skill names. Names are resolved against the existing path resolver (currently `agent-config/skills/<name>/` via the `danmestas/agent-skills` rewrite). Empty list is allowed when `extends:` provides everything.
- `extends` (optional) — name of another mode to compose with. The resolved skill set is `extends'd-mode-skills ++ this-mode-skills`, then deduped (first occurrence wins). Single inheritance only — no multiple parents in v1.

Schema deliberately tight. Optional fields (`targets`, `categories`, prose body) are deferred until concrete demand.

### Resolution at spawn time

`darken spawn <name> --type <role> [--mode <name>] "<task>"` resolves the skill set in this order:

1. **Pick mode.** If `--mode <m>` is passed, use `m`. Otherwise read `default_mode` from `.scion/templates/<role>/scion-agent.yaml`.
2. **Resolve skills.** Load the mode file. If it has `extends: <parent>`, recursively resolve the parent's skill set first; concatenate this mode's `skills:` after; dedupe (first occurrence wins). Cycle detection: if the recursion revisits a mode, abort with an error.
3. **Stage.** For each resolved skill name, the existing path resolver locates the source dir; the stager copies it to `.scion/skills-staging/<harness>/<skill>/` as today.
4. **Mount.** Container mounts the staging dir. No change below the stager.

### Validation

- Missing mode file: `mode <name>: not found at .scion/modes/<name>.yaml`. Spawn aborts.
- Mode references a skill that doesn't resolve: `mode <name>: skill <skill> not found at <resolved_path>`. Spawn aborts.
- `extends:` references a mode that doesn't exist: `mode <name>: extends "<parent>" not found at .scion/modes/<parent>.yaml`. Spawn aborts.
- Cycle in `extends:` chain (`a → b → a`): `mode <name>: cycle detected in extends chain: a → b → a`. Spawn aborts.

Validation runs at spawn time only — no separate `darken modes validate` command in v1.

### Default-mode naming convention

The 14 canonical roles each get a default mode named *after the role*: `tdd-implementer`, `planner-t3`, `researcher`, etc. Mode and role share names but live in different files (`.scion/templates/<role>/scion-agent.yaml` vs `.scion/modes/<role>.yaml`). One-to-one mapping; the role-default mode is the single artifact each role needs to function unchanged.

Where the role-default modes share skill prefixes (every planner uses `[hipp, ousterhout]`; every implementer uses `[tdd, idiomatic-go]`), the migration extracts those into shared base modes (e.g. `philosophy-base`, `implementer-base`) that the role-default modes `extends:`. See migration steps below.

This is intentionally not a final taxonomy. Operator-friendly mode names (`code`, `design`, `plan`, `recon`) are a follow-up consolidation once usage clarifies which compositions are worth elevating to first-class names.

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

Then `.scion/modes/planner-t3.yaml`:

```yaml
description: "Default skills for the planner-t3 harness."
extends: philosophy-base    # provides hipp, ousterhout
skills:
  - superpowers
```

(The exact base-mode partition is decided during migration — see migration steps.)

### CLI surface additions

- `darken modes list` — print all `.scion/modes/*.yaml` with mode-name (filename stem) and `description` columns.
- `darken modes show <name>` — cat the mode file. Resolved-skills output (the recursive `extends` expansion) shown as a separate block below the raw YAML.
- `darken spawn` gains `--mode <name>` flag.

Out for v1: `darken modes new`, `darken modes validate`, mode authoring scaffolds.

### Stager changes

`internal/substrate/staging/` (where the path resolver lives) gains a `mode_resolver.go` that:

- Loads a mode by filename (`<name>.yaml`).
- Recursively resolves `extends`, deduping with first-occurrence-wins.
- Detects cycles.
- Returns the final ordered skill name list.

The existing path resolver — which knows how to find `<repo>/skills/<name>` locally and via the `agent-config` rewrite — gets called with the mode's resolved skill names instead of the manifest's path strings. No new resolution logic; bare names go in, paths come out.

The spawn entry point in `cmd/darken/spawn.go`:

- Parses the new `--mode` flag.
- Resolves mode (explicit or via role's `default_mode`).
- Computes the resolved skill list via the mode resolver.
- Hands off to the existing stager.

## Migration (big-bang)

Single PR. Steps:

1. **Extract.** Read each of 14 role manifests; capture the current `skills:` list (paths like `danmestas/agent-skills/skills/hipp`).
2. **Translate.** Convert paths to bare skill names (`hipp`, `ousterhout`, etc.). The path-rewrite logic that turns `danmestas/agent-skills` into `agent-config` already exists; extract the bare-name parser into a helper if not already done.
3. **Identify shared prefixes.** Group the 14 translated lists by common skill subsets. Likely partitions to expect:
   - `philosophy-base`: `[hipp, ousterhout]` — used by every planner and reviewer.
   - `implementer-base`: `[tdd, idiomatic-go]` plus whatever else implementers share — used by `tdd-implementer` and possibly `darwin`.
   - others as observed in the data.

   The exact partition is empirical: pick the smallest set of base modes that lets every role-default mode be expressed as `extends: <base>` plus 0–3 additions. If a role has nothing meaningful to share, it gets a flat role-default mode with no `extends:`.

4. **Author base modes.** Write `.scion/modes/<base>.yaml` files for each shared partition. `description:` explaining what the base provides; `skills:` listing the shared prefix.
5. **Author role-default modes.** Write `.scion/modes/<role>.yaml` for each of the 14 canonical roles. `description:` per role. `extends: <base>` if applicable. `skills:` listing only the additions on top of the parent.
6. **Edit manifests.** Each `.scion/templates/<role>/scion-agent.yaml`: add `default_mode: <role>`, remove `skills:`.
7. **Update stager.** New mode-resolver pathway with recursive `extends` resolution; remove the manifest-`skills:`-field reading path.
8. **Test.** Golden-file equivalence per role (see Test strategy).

Atomic. The PR is shippable when all 14 golden tests pass.

The spec does not prescribe the base-mode partition because it depends on what the 14 current bundles actually look like. The migration step's first sub-task is to read the 14 manifests and observe the structure; the partition falls out of the data, not from speculation.

## Test strategy

### Unit — mode resolver

`mode_resolver_test.go`:

- Flat mode (no `extends`) loads and returns its `skills:` list.
- Mode with `extends: parent` recursively resolves; result is parent's skills first, then child's, with duplicates dropped (first wins).
- Two-level chain (`a extends b extends c`) resolves through both levels.
- Cycle (`a extends b extends a`) returns the cycle-detected error.
- Missing parent file returns the not-found error.
- Skill name that doesn't resolve returns the per-skill resolution error.

### Integration — golden equivalence

For each of the 14 canonical roles, a golden test verifies the staging directory contents are byte-identical pre- and post-migration:

1. **Pre.** On `main`, spawn-dry-run for `<role>` produces a staging tree. Hash-tree captured as golden.
2. **Post.** On the migration branch, spawn-dry-run for the same `<role>` (no `--mode` flag — defaults resolve via `default_mode`) produces a staging tree. Hash-tree must match the golden.

Big-bang is safe iff all 14 goldens match. Mismatch indicates the migration translated paths, partitioned base modes, or chained `extends` incorrectly.

### End-to-end — explicit vs. implicit mode

`darken spawn x --type researcher` and `darken spawn x --type researcher --mode researcher` must produce byte-identical staging output. Both pathways go through the resolver after migration; the implicit form just reads `default_mode` from the manifest before calling the same code.

### Failure-path

- Spawn with unknown `--mode unknown` exits non-zero with the mode-not-found error.
- Spawn with mode that references a non-existent skill exits non-zero with the skill-not-found error.
- Spawn with mode whose `extends` chain has a cycle exits non-zero with the cycle-detected error.

## Backward-compatibility

- **Operators using `darken spawn --type <role>` with no flags.** Behavior preserved: role's `default_mode` resolves to the migrated mode, which (after `extends` expansion) lists the same skills the manifest's `skills:` field listed before. Staging output identical (golden tests enforce).
- **External consumers of `.scion/templates/<role>/scion-agent.yaml`.** The `skills:` field is removed. If anything outside the cmd/darken codebase reads it, that breaks. Spec assumes nothing else reads it; verify during migration with a `grep -r "skills:" .scion/templates/` and a code-search for manifest-loading paths.
- **Role-default mode rename.** A future operator-friendly taxonomy (`code`, `design`, etc.) would change `default_mode` values. That's an additive follow-up, not a breaking change for this spec.

## Open questions for operator

These are the proposed defaults; flag if any need flipping.

1. **Default-mode naming convention.** `<role>` (proposed; e.g. `tdd-implementer`) vs `<role>-default` (more explicit; e.g. `tdd-implementer-default`).
2. **PR shape.** Single PR with base modes + 14 role-default modes + 14 manifest edits + stager change (proposed). vs Stager-change PR followed by 14 small migration PRs (one per role). Big-bang matches the operator-approved (i) migration scope.

## File-paths cited

- `.scion/templates/<role>/scion-agent.yaml` (14 files)
- `.scion/modes/<name>.yaml` (new directory; ~5 base modes + 14 role-default modes after migration)
- `cmd/darken/spawn.go` (CLI parsing)
- `internal/substrate/staging/` (mode resolver lives here)
- `agent-config/skills/<name>/` (skill source pool, unchanged)

## References

- Researcher-2 brief: `.scion/agents/researcher-2/workspace/docs/research-brief-suit.md`
- suit (algorithm reference, not adopted): `https://github.com/danmestas/suit`, `src/lib/resolution.ts`, `src/lib/types.ts`
- agent-config (skills source + existing modes): `https://github.com/danmestas/agent-config`, `modes/code/mode.md` for format-comparison
