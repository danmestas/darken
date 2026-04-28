# `darken uninstall-init` — design

**Date:** 2026-04-28
**Status:** Approved (brainstorming + Ousterhout review)
**Versioning target:** `v0.1.11` (or `v0.2.0` if cut alongside)

## Goal

Provide a project-level teardown command that removes the artifacts `darken init` writes — the symmetric counterpart to `init` and `upgrade-init`. After `darken uninstall-init`, the project looks like it never had `darken init` run, but operator-customized files and operator-owned `.scion/` runtime state are preserved.

## Non-goals

- Machine-level uninstall (`brew uninstall darken` + `~/.config/darken/` is operator-managed; can be a Phase 11+ if anyone hits it).
- `.scion/` removal beyond the persisted init manifest (operator-owned data: audit log, worker worktrees, staging dirs stay). Document `rm -rf .scion/` separately if a full nuke is wanted.
- Reversing `bones init` — bones manages its own state.
- Per-file interactive prompt mode — single manifest-then-prompt (Q1: A).

## Architecture

The design factors `init`'s artifact knowledge into a shared abstraction that both `runInit` and `runUninstallInit` consume. This eliminates change amplification: a future Phase that adds a scaffold updates one place, and uninstall picks it up automatically.

### Shared artifact abstraction

New file: `cmd/darken/artifacts.go`. Defines:

```go
// artifact describes one file or file-region that `darken init` writes
// into a target directory. Both runInit and runUninstallInit consume
// this list.
type artifact struct {
    // RelPath is the artifact's path relative to the init target dir.
    RelPath string
    // Kind is "file" (whole-file owned) or "gitignore-lines" (line-set
    // appended into a possibly-shared file).
    Kind string
    // Body returns the bytes init would write at the current binary's
    // substrate version. For "gitignore-lines" the bytes are the
    // newline-joined set of lines.
    Body func() ([]byte, error)
}

// initArtifacts is the single source of truth for what `darken init`
// scaffolds into a target directory. runInit writes them; runUninstallInit
// classifies them; the manifest read on uninstall verifies hashes.
func initArtifacts(targetDir string) []artifact { ... }
```

The artifact list:

| RelPath | Kind | Body source |
|---|---|---|
| `CLAUDE.md` | file | `renderCLAUDE(targetDir)` (templated) |
| `.claude/skills/orchestrator-mode/SKILL.md` | file | embedded `data/skills/orchestrator-mode/SKILL.md` |
| `.claude/skills/subagent-to-subharness/SKILL.md` | file | embedded `data/skills/subagent-to-subharness/SKILL.md` |
| `.claude/settings.local.json` | file | the hardcoded `{"statusLine":...}` body |
| `.gitignore` | gitignore-lines | the 7 lines (1 comment + 6 paths) appended by init |

### Persisted init manifest (`.scion/init-manifest.json`)

`runInit` writes a manifest at init time recording each artifact's path + SHA-256 of the bytes it wrote. This eliminates the "templated CLAUDE.md drifts with binary version" false-positive.

```json
{
  "schema_version": 1,
  "darken_version": "0.1.11",
  "substrate_hash": "cd680437ea02...",
  "artifacts": [
    {"path": "CLAUDE.md", "kind": "file", "sha256": "ab12..."},
    {"path": ".claude/skills/orchestrator-mode/SKILL.md", "kind": "file", "sha256": "cd34..."},
    {"path": ".gitignore", "kind": "gitignore-lines", "sha256": "ef56..."}
  ]
}
```

Comparison strategy in `runUninstallInit`:
1. **Manifest present** → compare project file against recorded `sha256`. If equal, PRISTINE; else CUSTOMIZED.
2. **Manifest absent** (older init or operator deleted it) → fall back to comparing against `Body()` of the current binary. The CLAUDE.md template false-positive remains in this fallback only.

The manifest itself is removed by `uninstall-init` as the last step after all other artifacts are gone.

### Subcommand surface

`darken uninstall-init` registered in `cmd/darken/main.go` after `upgrade-init`:

```go
{"uninstall-init", "remove the project scaffolds darken init wrote (preserves customizations)", runUninstallInit},
```

### Flags

| Flag | Effect |
|---|---|
| `--dry-run` | Print manifest, exit without prompting |
| `--yes` | Skip interactive prompt (for scripts / CI) |
| `--force` | Also remove CUSTOMIZED artifacts |

## Disposition states

For each artifact in `initArtifacts(root)`:

| State | When |
|---|---|
| `PRISTINE` | Project file exists and matches recorded sha (or `Body()` if no manifest) |
| `CUSTOMIZED` | Project file exists and differs |
| `MISSING` | Project file doesn't exist |

For the `.gitignore` artifact specifically: `PRISTINE` means all 7 lines are present; `CUSTOMIZED` means some are missing or operator-edited. The action is always "strip just our 7 lines"; the file itself is never deleted.

After REMOVE pass: empty-rmdir these four directories, each only if empty:
1. `<root>/.claude/skills/orchestrator-mode/`
2. `<root>/.claude/skills/subagent-to-subharness/`
3. `<root>/.claude/skills/`
4. `<root>/.claude/`

Never `rm -rf` a tree. Operator-added skills under `.claude/skills/` survive untouched.

### Sample manifest output

```
darken uninstall-init — manifest for /Users/dmestas/projects/foo
init-manifest: .scion/init-manifest.json (darken 0.1.11)

REMOVE   CLAUDE.md                                       (matches recorded hash)
REMOVE   .claude/skills/orchestrator-mode/SKILL.md       (matches recorded hash)
KEEP     .claude/skills/subagent-to-subharness/SKILL.md  (customized — pass --force to remove)
REMOVE   .claude/settings.local.json                     (matches recorded hash)
PATCH    .gitignore                                      (will strip 7 darken-managed lines)

3 files to remove, 1 file to patch, 1 customized file kept.
Proceed? [y/N]:
```

## Data flow

1. Resolve `root` via existing `repoRoot()` helper. If it fails, error: `"not in an init'd repo (run from a directory where 'darken init' was run; no CLAUDE.md / .claude/ found)"`.
2. Read `<root>/.scion/init-manifest.json` if present; build a `map[path]sha256` lookup.
3. For each `artifact` in `initArtifacts(root)`: read project bytes, hash, compare against manifest entry (or `Body()` if no manifest entry); set state.
4. Print manifest to stdout.
5. If `--dry-run`: exit 0.
6. If not `--yes`:
   - If stdin is not a terminal: error with `"non-interactive context: pass --yes to confirm"`. Non-zero exit.
   - Otherwise: print prompt, read line. Anything other than `y` / `yes` / `Y` → exit 0 with `"aborted"`.
7. For each REMOVE-disposition artifact (or CUSTOMIZED if `--force`):
   - For `kind: file`: `os.Remove`
   - For `kind: gitignore-lines`: `removeGitignoreLines` (read-modify-write via temp + rename)
8. After file pass: empty-rmdir the four candidate dirs (each only if empty).
9. Remove `<root>/.scion/init-manifest.json` (last; it's our own state).
10. Print summary: `"removed N files, patched .gitignore, kept M customized"`.

## Error handling

- `repoRoot()` failure → friendly error before any work; non-zero exit.
- Manifest read failure (file present but malformed JSON) → log warning, fall back to `Body()` comparison, continue.
- Embedded read failure (shouldn't happen — `//go:embed`) → wrap with the embedded path; non-zero exit.
- Per-file `os.Remove` failure → log to stderr `"uninstall: failed to remove <path>: <err>"`, continue. End with non-zero exit if any failed.
- `.gitignore` patch failure → log + non-zero exit. Don't half-write — atomic write via temp + rename.
- Non-tty stdin without `--yes` → error explicitly (don't silently abort).

## Refactor: `runInit` consumes `initArtifacts`

To make Approach 3 strategic instead of tactical, `runInit` itself is refactored to consume `initArtifacts` rather than duplicating the write list inline. After the refactor:

```go
func runInit(args []string) error {
    // ... flag parsing, prereq checks, target resolution (unchanged) ...

    arts := initArtifacts(target)
    var manifest initManifest

    for _, art := range arts {
        body, err := art.Body()
        if err != nil { return err }

        switch art.Kind {
        case "file":
            // existing scaffold logic (write/preserve)
        case "gitignore-lines":
            // existing scaffoldGitignore logic (append)
        }

        // Record the bytes we actually wrote.
        manifest.Add(art.RelPath, art.Kind, sha256(body))
    }

    if err := writeInitManifest(target, manifest); err != nil {
        return err
    }

    // bones init (unchanged)
    return runBonesInit(target)
}
```

Existing init tests update to verify (a) the same scaffolds are written, (b) `.scion/init-manifest.json` is created with the expected entries.

## Testing

Six tests in `cmd/darken/uninstall_init_test.go`:

1. **`TestUninstallInit_PristineRemovesAll`** — `runInit` then `runUninstallInit([]string{"--yes"})`. Verify all 4 files gone, `.gitignore` patched, `.claude/skills/` rmdir'd, manifest gone.
2. **`TestUninstallInit_CustomizedKept`** — `runInit`, then overwrite `orchestrator-mode/SKILL.md` with `"customized\n"`. Run `--yes`. Verify the customized file remains; manifest output contains `"KEEP"` and `"customized"`.
3. **`TestUninstallInit_ForceRemovesCustomized`** — same setup as #2 but `--yes --force`. Verify customized file removed.
4. **`TestUninstallInit_GitignoreSurgical`** — `runInit`, append operator lines (`*.log`, `node_modules/`) to `.gitignore`. Run `--yes`. Verify operator lines preserved, init's 7 lines gone.
5. **`TestUninstallInit_DryRunMakesNoChanges`** — `runInit`, run `--dry-run`. Verify no files removed + manifest printed.
6. **`TestUninstallInit_NotInitdRepoErrors`** — empty tmpdir as `DARKEN_REPO_ROOT`, run `runUninstallInit(nil)`. Verify error mentions `"not in an init'd repo"`.

Plus tests around the new `init-manifest.json` write/read in `cmd/darken/init_test.go`:

7. **`TestInit_WritesInitManifest`** — `runInit`, verify `.scion/init-manifest.json` exists with expected schema + matching hashes.
8. **`TestUninstallInit_FallbackWhenManifestMissing`** — `runInit`, delete `.scion/init-manifest.json`, run `runUninstallInit([]string{"--yes"})`. Verify it falls back to `Body()` comparison and removes pristine artifacts.

No "manifest-drift guard" test needed — the design eliminates the drift risk.

## Risks / open questions

1. **Refactoring `runInit` may break existing tests.** Mitigation: the implementation plan includes a step to update `init_test.go` cases that depend on inline scaffold logic. Behaviorally identical; only the internal structure changes.

2. **Manifest schema versioning.** The manifest carries `schema_version: 1`. Future changes (e.g. adding per-artifact `mtime`) bump the version; uninstall reads any v1 manifest and falls back to `Body()` for unrecognized versions.

3. **Empty-rmdir race with concurrent operator activity.** Best-effort; log + continue.

4. **`.gitignore` line matching brittleness.** Whitespace-trimmed line equality is the default. If an operator hand-edits a darken-managed line (e.g. trailing space or a path tweak), the strip pass treats it as CUSTOMIZED and leaves it. Acceptable: drift here is rare, and `--force` is the escape hatch.

## What's next after this ships

- `darken upgrade-init --clean-first` flag that runs `uninstall-init --yes --force` first — full reset workflow.
- `darken verify-init` that compares project artifacts against the manifest (subset of doctor's drift check, more granular).
- Manifest-version migrations as init's surface evolves.

## Cross-references

- DX roadmap spec: `docs/superpowers/specs/2026-04-28-darken-DX-roadmap-design.md` — uninstall-init is post-roadmap.
- Phase 9 plan (drift detection): `docs/superpowers/plans/2026-04-28-darken-phase-9-recovery-and-update.md` — Task 1's `bytes.Equal` pattern reused here.
- `cmd/darken/init.go` — contains the inline scaffold list to be refactored into `initArtifacts`.
