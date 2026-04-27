# Darken — installable orchestration substrate

**Status:** approved (in-conversation, 2026-04-27)
**Owner:** dmestas
**Source PR:** TBD (Phase 1 branch)

## Context

Today the `darkish` CLI is fused with the working repo: it reads `.scion/templates/`, `scripts/`, and `images/` from the CWD. That works only when the CWD is `~/projects/darkish-factory/`, which means orchestrator mode can't be used against any other project.

The substrate (templates, scripts, dockerfiles, the orchestrator-side skill files) and the working repo (where workers commit, where the audit log lives, where the operator's project source lives) need to decouple so the substrate becomes a versioned, installable tool consumed by many working repos.

## Goals

- One-line install on a fresh Mac: `brew install danmestas/tap/darken`. Fallbacks: `go install github.com/danmestas/darken/cmd/darken@latest` or downloading a release binary directly.
- Run orchestrator mode against any repo: `cd ~/projects/foo && darken init && claude code` — no clone of the substrate required.
- Templates, scripts, the two host-orchestrator skills, and the four backend Dockerfiles ship inside the binary.
- Operators can override at two scopes: project-local `.scion/templates/<role>/` (versioned with the working repo), user-scoped `~/.config/darken/overrides/<role>/` (per-machine).
- Atomic versioning: binary version pins template version pins skill version. Roll forward with one command.

## Non-goals

- In-binary self-update (homebrew or `go install` handles that).
- npm / docker-image / curl-pipe-bash distribution channels.
- Public docs site or marketplace listing.
- A daemon or long-running service.

## Naming

Binary: `darken`. The repo stays `darkish-factory`. The Go package path becomes `github.com/danmestas/darken/cmd/darken`. Avoids conflict with the unix `df` builtin (the user's first preference); `darken` is unambiguous on PATH.

## Architecture

```
                                  ┌─────────────────────────┐
   working repo CWD               │  $ darken spawn r1 ...  │
   ───────────────                └────────────┬────────────┘
   ./.scion/agents/...                          │
   ./.scion/audit.jsonl                         │ resolves substrate via:
   ./.scion/skills-staging/...                  │
   ./CLAUDE.md  (scaffolded by `darken init`)   ▼
   ./.specify/memory/constitution.md   ┌─────────────────────────────────────────┐
                                       │ 1. --substrate-overrides <path>         │
                                       │ 2. $DARKEN_SUBSTRATE_OVERRIDES          │
                                       │ 3. ~/.config/darken/overrides/          │
                                       │ 4. <CWD>/.scion/templates/<role>/       │  (templates only)
                                       │ 5. embedded fs in the binary            │  ← always present
                                       └─────────────────────────────────────────┘
```

First match wins per resolution. The embedded layer is always present so a fresh-installed binary works with no further filesystem setup.

## Substrate contents (embedded)

- All 13 harness manifests at `.scion/templates/<role>/{scion-agent.yaml, agents.md, system-prompt.md}`
- Stage scripts: `scripts/stage-creds.sh`, `scripts/stage-skills.sh`, `scripts/spawn.sh`, `scripts/bootstrap.sh`
- Dockerfiles + preludes: `images/{claude,codex,pi,gemini}/{Dockerfile, darkish-prelude.sh}` + `images/Makefile`
- The two orchestrator-side skills: `.claude/skills/{orchestrator-mode,subagent-to-subharness}/SKILL.md`

Worker-side skills (ousterhout, hipp, dx-audit, tigerstyle, caveman, etc.) are NOT embedded; they continue to resolve from `~/projects/agent-skills/` via the existing APM-style refs in harness manifests. Documented dependency.

Total embedded payload: ~50 KB. Final binary: ~5 MB stripped.

## Resolution chain

```
For a substrate file at relative path P:

  if --substrate-overrides X is set and X/P exists  → return X/P
  if $DARKEN_SUBSTRATE_OVERRIDES Y is set and Y/P exists → return Y/P
  if ~/.config/darken/overrides/P exists            → return that
  if P starts with ".scion/templates/" and CWD/P exists → return CWD/P  (project-scoped templates)
  return embeddedFS.Open(P)                          (always succeeds)
```

The CWD-templates-only special case lets a working repo version-control its own role overrides without polluting other override scopes.

## CLI surface changes

| Subcommand | Status | Behavior |
|---|---|---|
| `darken spawn` | refactor | resolves templates via chain; CWD becomes worker worktree root |
| `darken bootstrap` | refactor | runs from any CWD; bootstraps the working repo only (audit dir, skill staging) |
| `darken creds` | refactor | invokes embedded `scripts/stage-creds.sh` |
| `darken skills` | refactor | invokes embedded `scripts/stage-skills.sh` |
| `darken images` | refactor | extracts embedded Dockerfiles to a tmp dir, runs `make` against it |
| `darken doctor` | refactor | reports substrate version + which layer served each role |
| `darken apply` | refactor | reads recommendations from CWD's `.scion/darwin-recommendations/` |
| `darken create-harness` | new flag | `--scope=user|project`, default `user` (writes to `~/.config/darken/overrides/<role>/`) |
| `darken list` | unchanged | wraps `scion list` |
| `darken orchestrate` | refactor | reads embedded `orchestrator-mode/SKILL.md` (or override layer if present) |
| `darken init [<path>]` | NEW | scaffolds `CLAUDE.md` + `.darken/config.yaml` + `.gitignore` entries in target dir |
| `darken version` | NEW | prints binary version + embedded substrate hash |
| `darken overrides list\|edit\|reset` | NEW | manages `~/.config/darken/overrides/` |

## Distribution

Phase 4 deliverables:

- **GitHub Releases**: `goreleaser` builds darwin/amd64+arm64 + linux/amd64+arm64 on tag push (`v*.*.*`).
- **Homebrew tap**: `danmestas/homebrew-tap` repo, auto-published from goreleaser. Users: `brew install danmestas/tap/darken`.
- **`go install`**: works directly from `github.com/danmestas/darken/cmd/darken@vX.Y.Z` once tagged.
- **Direct download**: GitHub Release artifacts for users who can't use the above.

Initial tag: `v0.1.0`. Pre-1.0 reflects "single user, breaking changes likely."

## Update story

- `brew upgrade darken`
- `go install ...@latest`
- (No in-binary self-update.) `darken doctor` warns if `version` is older than the latest GitHub release tag.

User-scope overrides at `~/.config/darken/overrides/` survive upgrades. Embedded substrate is replaced atomically with the binary.

## Phasing

1. **Phase 1** — Substrate decoupling resolver + rename `darkish` → `darken`. No embed yet; resolver layers 1-4. Backward-compatible: project-local `.scion/templates/` continues to work in the darkish-factory repo itself.
2. **Phase 2** — Embed substrate (resolver layer 5) + `darken init`.
3. **Phase 3** — Release pipeline: goreleaser + GH Actions + homebrew tap. Tag `v0.1.0`.
4. **Phase 4** — Dogfood: darkish-factory consumes the released binary; README updates; migration note.

Each phase ships as its own PR. Total: ~5-6 hours of focused work.

## Risks

- **Embedded templates drift from override semantics.** Mitigation: `darken doctor` per-role layer report + CI test that embedded templates parse cleanly.
- **Backward-compat break for the existing darkish-factory CLI workflow.** Mitigation: Phase 1 keeps project-local `.scion/templates/` resolution active; explicit migration test in Phase 4.
- **Homebrew tap repo maintenance overhead.** Accepted — the tap is operator-owned; goreleaser does the publishing.
- **Worker-side skills still require an `agent-skills` clone.** Documented in `darken doctor` output and `darken init` README scaffolding.
- **`embed` makes binary huge.** Audited ~50 KB total embedded payload. Final binary remains ~5 MB.

## Open questions (resolved)

| Question | Decision |
|---|---|
| Distribution channels | Homebrew tap + go install + binary releases |
| What to embed | Templates, scripts, Dockerfiles, the two orchestrator-side skills |
| Initial tag | `v0.1.0` |
| Binary name | `darken` |

All four answered in conversation 2026-04-27.

## Cross-references

- PR #2 (merged): the original substrate (templates + scripts + CLI without decoupling)
- PR #3 (merged): host-mode orchestrator + skills
- Will reference: forthcoming Phase 1 PR
