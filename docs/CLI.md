# `darken` CLI Reference

`darken --help` lists everything in registration order. The grouping below is by purpose.

## Lifecycle

Use these for the standard project lifecycle.

| Command | Purpose |
|---|---|
| `darken setup` | One-shot fresh-repo onboarding (init + bootstrap) |
| `darken upgrade-init` | Refresh project scaffolds after `brew upgrade darken` |
| `darken uninstall-init` | Remove project scaffolds (preserves customizations + .scion/ runtime state) |
| `darken init` | Project-only scaffolds (CLAUDE.md, .claude/skills/, .gitignore). Prefer `setup` for first-time use. |

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

Use these for surgical operations when full `setup` is overkill.

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
| `darken orchestrate` | Print host-mode orchestrator skill body (for piping into a fresh Claude Code session) |

`darken doctor` runs preflight + per-harness checks. `darken doctor <role>` shows which substrate layer (override / project / embedded) served that role's manifest.
