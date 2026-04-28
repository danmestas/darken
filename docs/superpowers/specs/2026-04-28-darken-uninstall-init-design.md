# `darken uninstall-init` — design

**Date:** 2026-04-28
**Status:** Approved (brainstorming session)
**Versioning target:** `v0.1.11` (or `v0.2.0` if cut alongside)

## Goal

Provide a project-level teardown command that removes the artifacts `darken init` writes — the symmetric counterpart to `init` and `upgrade-init`. After `darken uninstall-init`, the project looks like it never had `darken init` run, but operator-customized files and operator-owned `.scion/` runtime state are preserved.

## Non-goals

- Machine-level uninstall (`brew uninstall darken` + `~/.config/darken/` is operator-managed; can be a Phase 11+ if anyone hits it).
- `.scion/` removal — operator-owned data (audit log, worker worktrees, staging dirs) stays. Document `rm -rf .scion/` separately if a full nuke is wanted.
- Reversing `bones init` — bones manages its own state; we only undo what `darken init` itself wrote.
- Per-file interactive prompt mode — single manifest-then-prompt (Q1: A).

## Architecture

`darken uninstall-init` is a single new subcommand in `cmd/darken/uninstall_init.go`. It walks the known list of artifacts that `init` writes, classifies each as `PRISTINE` / `CUSTOMIZED` / `MISSING`, prints a manifest table, prompts the operator, and removes the PRISTINE ones (plus CUSTOMIZED if `--force`).

No new state, no new files outside the subcommand and its test file. The artifact list is inlined into `uninstall_init.go` rather than refactoring `runInit` to expose a shared helper — pins scope. A drift between the two is caught by a manifest test (see Testing section).

### Flags

| Flag | Effect |
|---|---|
| `--dry-run` | Print manifest, exit without prompting |
| `--yes` | Skip interactive prompt (for scripts / CI) |
| `--force` | Also remove CUSTOMIZED artifacts |

## Disposition matrix

| Path | Comparison source | Match action | Differ action |
|---|---|---|---|
| `<root>/CLAUDE.md` | `renderCLAUDE(root)` (existing helper — RepoName + SubstrateHash12 from current binary) | REMOVE | KEEP customized |
| `<root>/.claude/skills/orchestrator-mode/SKILL.md` | embedded `data/skills/orchestrator-mode/SKILL.md` | REMOVE | KEEP customized |
| `<root>/.claude/skills/subagent-to-subharness/SKILL.md` | embedded `data/skills/subagent-to-subharness/SKILL.md` | REMOVE | KEEP customized |
| `<root>/.claude/settings.local.json` | hardcoded init body (the `{"statusLine":...}` JSON) | REMOVE | KEEP customized |
| `<root>/.gitignore` | the 7 lines `init` appends (see `scaffoldGitignore` in `cmd/darken/init.go` — 1 comment + 6 path entries) | strip those 7 lines if present | (file isn't owned, never deleted) |

After REMOVE pass: empty-rmdir these four directories, each only if empty:
1. `<root>/.claude/skills/orchestrator-mode/`
2. `<root>/.claude/skills/subagent-to-subharness/`
3. `<root>/.claude/skills/`
4. `<root>/.claude/`

Never `rm -rf` a tree. Operator-added skills under `.claude/skills/` survive untouched.

### Sample manifest

```
darken uninstall-init — manifest for /Users/dmestas/projects/foo

REMOVE   CLAUDE.md                                       (matches embedded template)
REMOVE   .claude/skills/orchestrator-mode/SKILL.md       (matches embedded)
KEEP     .claude/skills/subagent-to-subharness/SKILL.md  (customized — pass --force to remove)
REMOVE   .claude/settings.local.json                     (matches init body)
PATCH    .gitignore                                      (will strip 7 darken-managed lines)

3 files to remove, 1 file to patch, 1 customized file kept.
Proceed? [y/N]:
```

## Data flow

1. Resolve `root` via existing `repoRoot()` helper. If it fails, error with hint: `"not in an init'd repo (run from a directory where 'darken init' was run; no CLAUDE.md / .claude/ found)"`.
2. Build artifact list. CLAUDE.md's comparison source comes from `renderCLAUDE(root)` (reused).
3. For each artifact: `os.ReadFile` project bytes, read source bytes, `bytes.Equal` → set state.
4. Print manifest (sample format above) to stdout.
5. If `--dry-run`: exit 0.
6. If not `--yes`: print prompt, read line from stdin. Anything other than `y` / `yes` / `Y` → exit 0 with `"aborted"`.
7. For each REMOVE-disposition artifact (or CUSTOMIZED if `--force`): `os.Remove`. Continue on error; track failures.
8. After file pass: empty-rmdir the four candidate dirs (each only if empty).
9. After dir pass: `removeGitignoreLines(<root>/.gitignore, the7lines)` — read file, drop matching lines, atomic write via temp + rename.
10. Print summary: `"removed N files, patched .gitignore, kept M customized"`.

## Error handling

- `repoRoot()` failure → friendly error before any work; exit non-zero.
- Embedded read failure (shouldn't happen — `//go:embed`) → wrap with the embedded path; non-zero exit.
- Per-file `os.Remove` failure → log to stderr `"uninstall: failed to remove <path>: <err>"`, continue. End with non-zero exit if any failed.
- `.gitignore` patch failure → log + non-zero exit. Don't half-write the file (read-modify-write into memory, then atomic write via temp + rename).
- Stdin read in step 6 fails (e.g. piped from `/dev/null` without `--yes`) → treat as "no" and exit 0 with `"aborted: no confirmation"`.

## Testing

Six tests in `cmd/darken/uninstall_init_test.go`:

1. **`TestUninstallInit_PristineRemovesAll`** — plant pristine artifacts via `runInit`, run `runUninstallInit([]string{"--yes"})`. Verify all 4 files gone + .gitignore patched (no darken lines remain) + `.claude/skills/` rmdir'd.
2. **`TestUninstallInit_CustomizedKept`** — plant pristine, then overwrite `orchestrator-mode/SKILL.md` with `"customized\n"`. Run `--yes`. Verify the customized file remains; manifest output contains `"KEEP"` and `"customized"`.
3. **`TestUninstallInit_ForceRemovesCustomized`** — same setup as #2 but `--yes --force`. Verify customized file removed.
4. **`TestUninstallInit_GitignoreSurgical`** — plant pristine, append operator lines (`*.log`, `node_modules/`) to `.gitignore`. Run `--yes`. Verify operator lines preserved, init's 7 lines gone.
5. **`TestUninstallInit_DryRunMakesNoChanges`** — plant pristine, run `--dry-run`. Verify no files removed + manifest printed.
6. **`TestUninstallInit_NotInitdRepoErrors`** — empty tmpdir as `DARKEN_REPO_ROOT`, run `runUninstallInit(nil)`. Verify error mentions `"not in an init'd repo"`.

**Manifest-drift guard:** add a 7th test that confirms the artifact list in `uninstall_init.go` matches the artifacts `init` actually writes — e.g., run `runInit`, `os.Walk` the resulting tree, and assert every path under `.claude/` (and `CLAUDE.md`) is covered by the uninstall manifest. This catches the case where init grows a new artifact and uninstall forgets it.

## CLI registration

In `cmd/darken/main.go` `subcommands` slice, place after `upgrade-init`:

```go
{"uninstall-init", "remove the project scaffolds darken init wrote (preserves customizations)", runUninstallInit},
```

## Risks / open questions

1. **Init's artifact list grows out-of-band.** If a future Phase adds a new file to `runInit` (e.g. a new skill, a new settings file), uninstall-init won't know about it. Mitigation: the 7th manifest-drift test will fail loudly — when it does, the implementer adds the new artifact to the disposition matrix.

2. **`.gitignore` line-stripping is brittle to whitespace drift.** If an operator hand-edits a darken-managed line (e.g. trailing space), the strip pass won't match it. Mitigation: strip uses `strings.TrimSpace` line comparison; if that's still too brittle, future enhancement could mark lines with a sentinel comment (e.g. `# darken-managed:`).

3. **Templated CLAUDE.md drifts with binary version.** `renderCLAUDE(root)` includes `SubstrateHash12` from the current binary. If the operator ran `darken init` with v0.1.4 and now runs `darken uninstall-init` from v0.1.11, the comparison against today's render will mismatch and the file will be classified CUSTOMIZED — falsely. Mitigation: accept the false-positive (operator passes `--force`); document in --help. Real fix is Phase 11+: store the rendered body's hash in `.scion/init-manifest.json` at init time.

4. **Empty-rmdir race with concurrent operator activity.** If the operator has a shell open in `.claude/skills/` while uninstall-init runs, the rmdir may fail because the OS holds the dir as the cwd. Mitigation: best-effort; log + continue.

## What's next after this ships

- `darken upgrade-init` could grow a `--clean-first` flag that runs `uninstall-init --yes --force` first — full reset workflow. Not in initial scope.
- A persisted init manifest (`.scion/init-manifest.json`) recording exact paths + hashes at init time, eliminating the templated-CLAUDE.md false-positive in Risk #3.

## Cross-references

- DX roadmap spec: `docs/superpowers/specs/2026-04-28-darken-DX-roadmap-design.md` — uninstall-init is post-roadmap; not part of Phases 5-9 but rounds out the symmetric init / upgrade-init / uninstall-init triad.
- Phase 9 plan: `docs/superpowers/plans/2026-04-28-darken-phase-9-recovery-and-update.md` — drift detection (Task 1) supplies the `bytes.Equal` comparison pattern reused here.
- `cmd/darken/init.go` — source of truth for the artifact list; `scaffoldSkill`, `scaffoldStatusLine`, `scaffoldGitignore`.
