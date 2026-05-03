# `darken` CLI Reference

`darken --help` lists everything in registration order. The grouping below is by purpose.

## Lifecycle

Use these for the standard project lifecycle.

| Command | Purpose |
|---|---|
| `darken up` | Bring the project up: scaffold + machine prereqs + `bones up` (one-shot fresh-repo onboarding). Pass `--no-bones` to skip the bones chain. |
| `darken down` | Tear the project down: stop agents + delete project grove + uninstall scaffolds + `bones down`. Pass `--yes` for non-interactive, `--no-bones` to skip, `--purge` for host-wide cleanup. |
| `darken setup` | Deprecated alias for `darken up`. |
| `darken upgrade-init` | Refresh project scaffolds after `brew upgrade darken` |
| `darken uninstall-init` | Remove project scaffolds (preserves customizations + .scion/ runtime state) |
| `darken init` | Project-only scaffolds (CLAUDE.md, .claude/skills/, .gitignore). Prefer `up` for first-time use. |

## Operations (the §7 loop)

Run, watch, and recover sub-harness workers.

| Command | Purpose |
|---|---|
| `darken spawn <name> --type <role> [task]` | Start an agent (async; default: returns at "ready") |
| `darken redispatch <name>` | Kill + re-spawn an agent with the same role |
| `darken list` | Pass-through to `scion list` |
| `darken apply` | Review + apply darwin recommendations |

## Inspection

Check state, recent decisions, and version coherence.

| Command | Purpose |
|---|---|
| `darken doctor [--init \| <harness>]` | Preflight + post-mortem health checks |
| `darken status` | One-line statusLine output (mode + substrate hash) |
| `darken dashboard` | Open scion's web UI in the default browser |
| `darken history` | Tabular view of `.scion/audit.jsonl` |
| `darken version` | Binary version + embedded substrate hash |

## Targeted setup

Use these for surgical operations when full `up` is overkill.

| Command | Purpose |
|---|---|
| `darken bootstrap` | Machine prereqs + per-harness skill staging |
| `darken creds [<backend>]` | Refresh hub secrets |
| `darken images` | Wrap `make -C images` |
| `darken skills <harness> [--diff \| --add SKILL \| --remove SKILL]` | Manage staged skills per harness |

## Authoring

| Command | Purpose |
|---|---|
| `darken create-harness <name>` | Scaffold a new harness directory |

`darken doctor` runs preflight + per-harness checks. `darken doctor <role>` shows which substrate layer (override / project / embedded) served that role's manifest.
