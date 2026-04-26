# Darkish Factory Images

Per-harness-class container images that extend Scion's runtime images with
the Darkish Factory's tool baseline, folder-trust prelude, and credential
mounting hooks.

## Layout

```
images/
├── README.md            # this file
├── Makefile             # build + push entry points
└── claude/
    ├── Dockerfile       # FROM <registry>/scion-claude:<tag>
    └── darkish-prelude.sh   # entrypoint script run before sciontool init
```

## What each layer does

`darkish-claude` extends `scion-claude` with:

1. **Apt utilities** — `jq`, `ripgrep`, `fzf`, `less`, `gh`. Universal CLI
   tools harnesses depend on but scion-base does not ship.
2. **Go-installed tools** — placeholders. Add `go install <module>@<tag>`
   lines for custom CLIs (semantic search, your own utilities) when you
   know the list.
3. **NPM-installed MCPs** — placeholders. Add `npm install -g <pkg>` lines
   for MCP servers the harnesses should be able to invoke.
4. **Pip-installed MCPs** — placeholders for Python-based MCPs.
5. **`darkish-prelude.sh` entrypoint** — runs before `sciontool init`:
   - Pre-populates `~/.claude.json` with `hasTrustDialogAccepted: true`
     for `/repo-root/.scion/agents/${SCION_AGENT_NAME}/workspace`. Bypasses
     Claude Code's first-encounter trust dialog (which `--dangerously-skip-permissions`
     does NOT cover — that flag only governs per-tool permissions).
   - If `~/.claude/.credentials.json` is mounted into the container by the
     harness manifest, parses it and exports `CLAUDE_CODE_OAUTH_TOKEN`.
     Unsets `ANTHROPIC_API_KEY` to prefer OAuth over key-based auth.
   - Execs `/opt/scion/bin/sciontool init --` to hand off to the original
     entrypoint.

## Build

First-time setup needs `local/scion-claude:latest` already built (see
`~/projects/scion/image-build/scripts/build-images.sh`).

```bash
make -C images claude
```

That produces `local/darkish-claude:latest`.

## Push to GHCR

```bash
echo "${GH_TOKEN}" | docker login ghcr.io -u danmestas --password-stdin
make -C images claude REGISTRY=ghcr.io/danmestas
make -C images push-claude REGISTRY=ghcr.io/danmestas
```

After pushing, `scion config set image_registry ghcr.io/danmestas` and the
harnesses pull from there on first run.

## Wiring into harness manifests

Each `.scion/templates/<harness>/scion-agent.yaml` sets the image and (for
OAuth-using harnesses) mounts the staged credentials file:

```yaml
image: local/darkish-claude:latest
volumes:
  - host: ~/.scion-credentials/claude/.credentials.json
    container: /home/scion/.claude/.credentials.json
    readonly: true
env:
  # Optional overrides; the prelude prefers OAuth file over env vars.
```

The host-side credential file is staged from the macOS Keychain by
`scripts/stage-creds.sh` — see that script for details.

## Codex

`images/codex/` produces a `darkish-codex` image extending `scion-codex`.

**Status:** validated end-to-end on 2026-04-26. Smoke-tested against the
Codex Max plan: container starts, `~/.codex/auth.json` mounts in,
codex CLI v0.125.0 launches with `gpt-5.4 medium · YOLO mode`, accepts
the task prompt, returns the answer.

Build:

```bash
make -C images codex
```

**Mount the OAuth file directly.** Codex stores OAuth at
`~/.codex/auth.json`. Mount it into the container — no staging step
needed (unlike Claude, whose creds live in the macOS Keychain). Each
codex-using harness's `scion-agent.yaml` sets:

```yaml
default_harness_config: codex
image: local/darkish-codex:latest
volumes:
  - source: ~/.codex/auth.json
    target: /home/scion/.codex/auth.json
    read_only: true
```

**Use `scion --no-hub start` for codex agents (or push auth as a hub
secret).** Scion's hub-mediated start does not auto-detect
`~/.codex/auth.json` from the broker host — it expects the file to be
pushed as a hub secret. Two paths:

1. Local-only mode (simplest for solo dev):
   ```bash
   scion --no-hub start <agent-name>
   ```
   Scion auto-detects the file from `${HOME}/.codex/auth.json` on the
   broker host and treats it as the codex auth source. Verified working.

2. Hub-mode with explicit secret push:
   ```bash
   scion hub secret set --type file --target ~/.codex/auth.json --source ~/.codex/auth.json
   ```
   Then `scion start <agent-name> --harness-auth auth-file` works in
   hub mode.

The local-only path is what `scripts/spawn.sh` uses by default for codex.

**Trust state.** Codex tracks per-project trust in `~/.codex/config.toml`
as `[projects."<absolute-path>"] trust_level = "trusted"`. The codex
prelude appends this for `/repo-root/.scion/agents/${SCION_AGENT_NAME}/workspace`
at start. In practice the scion-codex image runs codex with `permissions: YOLO mode`
which already bypasses the prompt, so the prelude's trust block is
defensive — it survives if scion-codex's CMD ever changes.

## Spawning agents

Use `scripts/spawn.sh` instead of bare `scion start`. It refreshes the
staged OAuth files from the macOS Keychain first, then starts the
harness:

```bash
scripts/spawn.sh smoke-test --type researcher --workspace /tmp/wt "task..."
```
