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

`images/codex/` scaffolds a `darkish-codex` image extending `scion-codex`.
Same shape as `darkish-claude` minus the OAuth shim — scion auto-detects
`~/.codex/auth.json` and supports `auth-file` mode natively.

**Status:** scaffold only. Not yet validated end-to-end. The codex
prelude's trust mechanism is a placeholder modeled on Claude's
pattern. Validate when first running a codex harness and update.

To build:

```bash
# Requires local/scion-codex:latest already built (currently only
# scion-claude has been built; build the rest via scion's build-images.sh
# or direct docker build against image-build/codex/Dockerfile).
make -C images codex
```

When a Darkish Factory harness wants to call codex (e.g., `sme` for
GPT-class reasoning), set `default_harness_config: codex` and
`image: local/darkish-codex:latest` in its `scion-agent.yaml`. The
volumes block is unnecessary; scion handles `~/.codex/auth.json`
natively.

## Spawning agents

Use `scripts/spawn.sh` instead of bare `scion start`. It refreshes the
staged OAuth files from the macOS Keychain first, then starts the
harness:

```bash
scripts/spawn.sh smoke-test --type researcher --workspace /tmp/wt "task..."
```
