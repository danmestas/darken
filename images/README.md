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

## Codex (future)

When we add codex-using harnesses (`sme` could plausibly call codex for
GPT-5.4-class reasoning), add `images/codex/Dockerfile` extending
`scion-codex`. Codex auth is simpler — scion already auto-detects
`~/.codex/auth.json` and supports `auth-file` mode natively, so the
codex prelude will only need the trust state injection (and any custom
tools), not the OAuth shim.
