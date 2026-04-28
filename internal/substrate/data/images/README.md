# Darkish Factory Images

Per-harness-class container images that extend Scion's runtime images with
the Darkish Factory's tool baseline, folder-trust prelude, and credential
mounting hooks.

## Universal baseline

Every `darkish-*` image carries the same baseline regardless of backend:

| Layer | Contents | Notes |
|---|---|---|
| apt utilities | `jq`, `ripgrep`, `fzf`, `less`, `gh`, `ca-certificates`, `curl`, `git` | Standard tools every harness uses |
| bones unified CLI | `bones` (subcommands: `init`, `tasks`, `repo`, ...) | Pre-compiled on the host from `~/projects/agent-infra/cmd/bones` (`make prebuild-bones`) and copied from `<backend>/bin/`. agent-infra is currently private; in-Docker `git clone` blocked until it goes public. Replaces the prior `agent-init` + `agent-tasks` binaries — both are now subcommands. |
| Universal skill | `caveman` (cloned from `juliusbrussee/caveman`) | Communication-tier discipline; mounted at `/home/scion/skills/caveman/` |
| Universal MCP | `context-mode` (npm `@mksglu/context-mode` with git-clone fallback to `mksglu/context-mode`) | Sandboxed raw-output handling; every harness faces context-window pressure |
| Trust prelude | per-backend (validated for claude + codex; placeholders for pi + gemini) | Suppresses first-encounter trust dialogs |

`mgrep` was specified in the original plan but is intentionally omitted —
it is a paid product. Code search is handled by `context-mode` instead.

### Refresh procedure

```bash
make -C images prebuild-bones    # rebuild the bones CLI from ~/projects/agent-infra
make -C images all               # rebuild all four darkish-* images
```

For fast iteration on tool layers only:

```bash
make -C images tools-only-all
```

For prelude-script changes only:

```bash
make -C images prelude-only-claude   # or codex / pi / gemini
```

## Auth model: hub-secrets-everywhere

Every backend's auth flows through scion's hub secret store. No
`volumes:` blocks for credentials in any harness manifest. Refresh
secrets after token rotation by re-running `scripts/stage-creds.sh`.

| Backend | Source | Hub secret name | Container target |
|---|---|---|---|
| claude | macOS Keychain `Claude Code-credentials` | `claude_auth` | `/home/scion/.claude/.credentials.json` |
| codex | `~/.codex/auth.json` | `codex_auth` | `/home/scion/.codex/auth.json` |
| pi | `OPENROUTER_API_KEY` env var | `OPENROUTER_API_KEY` | (env, inherited by container) |
| gemini | `GEMINI_API_KEY` env var or `~/.gemini/oauth_creds.json` | `GEMINI_API_KEY` or `gemini_auth` | env or `/home/scion/.gemini/oauth_creds.json` |

Refresh:

```bash
scripts/stage-creds.sh         # all four backends
scripts/stage-creds.sh claude  # one backend
```

`stage-creds.sh` soft-fails per backend so partial environments still
stage what they have.

## Harness coverage

Which backend image each role runs on (see `.design/harness-roster.md`
for the full §3.1 matrix).

| Image | Roles |
|---|---|
| `darkish-claude` | `orchestrator`, `researcher`, `designer`, `planner-t1`, `planner-t2`, `planner-t3`, `tdd-implementer`, `admin` |
| `darkish-codex` | `planner-t4`, `verifier`, `reviewer`, `sme`, `darwin` |
| `darkish-pi` | (override-only, no permanent harness) |
| `darkish-gemini` | (override-only, no permanent harness) |

`planner-t1` through `planner-t4` are the four planner tiers (haiku
ad-hoc → sonnet claude-code → opus superpowers → codex spec-kit).
`darwin` is the post-pipeline evolution agent that emits YAML
recommendations gated by `darken apply`.

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

First-time setup needs:

1. `local/scion-claude:latest` already built (see
   `~/projects/scion/image-build/scripts/build-images.sh`).
2. Bones binaries pre-built on the host. The `images/*/bin/` dirs are
   git-ignored — agent-infra is private, so the binaries can't be
   `git clone`'d inside the Docker build context. Operator runs:

   ```bash
   make -C images prebuild-bones
   ```

   This cross-compiles the unified `bones` CLI for `linux/arm64` from
   `~/projects/agent-infra/cmd/bones` into all four `images/<backend>/bin/`
   directories. Re-run after `agent-infra` updates.

Then:

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

Each `.scion/templates/<harness>/scion-agent.yaml` declares its image and
backend; auth comes from hub secrets, not from a `volumes:` block:

```yaml
schema_version: "1"
description: "..."
agent_instructions: agents.md
system_prompt: system-prompt.md
default_harness_config: claude
image: local/darkish-claude:latest
model: claude-sonnet-4-6
max_turns: 30
max_duration: "1h"
detached: false
```

Auth flows through scion's hub secret store: `scripts/stage-creds.sh`
extracts the operator's credentials from macOS Keychain (claude),
`~/.codex/auth.json` (codex), or env vars (pi, gemini) and pushes each
as a hub secret. The broker projects them into the container at the
canonical path each CLI expects.

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

**Codex auth flows through `codex_auth` hub secret.** The operator's
`~/.codex/auth.json` is pushed as a hub file secret targeting
`/home/scion/.codex/auth.json` via `scripts/stage-creds.sh codex`. No
`volumes:` block needed in the harness manifest.

**Trust state.** Codex tracks per-project trust in `~/.codex/config.toml`
as `[projects."<absolute-path>"] trust_level = "trusted"`. The codex
prelude appends this for `/repo-root/.scion/agents/${SCION_AGENT_NAME}/workspace`
at start. In practice the scion-codex image runs codex with `permissions: YOLO mode`
which already bypasses the prompt, so the prelude's trust block is
defensive — it survives if scion-codex's CMD ever changes.

## Pi

`images/pi/` extends `scion-pi` for OpenRouter-backed agents using
`@mariozechner/pi-coding-agent`.

**Status:** scaffold + tool baseline. Pi auth is via `OPENROUTER_API_KEY`
env var (scion injects from operator env or hub secrets). Trust
mechanism for first-encounter dialogs is not yet verified — placeholder
in the prelude. Validate on first pi smoke run and update.

Build:

```bash
make -C images pi
```

Pi-using harness manifest:

```yaml
default_harness_config: pi
image: local/darkish-pi:latest
model: <openrouter-model-id>
```

No `volumes:` block needed — auth is env-var-based.

## Gemini

`images/gemini/` extends `scion-gemini`.

**Status:** scaffold + tool baseline. Gemini supports three auth
paths:

| Path | Mechanism | Cost |
|---|---|---|
| API key | `GEMINI_API_KEY` env var | Free tier on AI Studio; paid above |
| OAuth | `~/.gemini/oauth_creds.json` (scion auto-detects on broker host) | Per gemini-cli plan |
| Vertex AI | `GOOGLE_APPLICATION_CREDENTIALS` service account | GCP billing |

Trust mechanism not yet verified — placeholder in the prelude.

Build:

```bash
make -C images gemini
```

Gemini-using harness manifest:

```yaml
default_harness_config: gemini
image: local/darkish-gemini:latest
model: gemini-3.1-pro-preview
```

For OAuth, mount the auth file directly (same pattern as codex):

```yaml
volumes:
  - source: ~/.gemini/oauth_creds.json
    target: /home/scion/.gemini/oauth_creds.json
    read_only: true
```

## Spawning agents

Use `scripts/spawn.sh` instead of bare `scion start`. It refreshes the
staged OAuth files from the macOS Keychain first, then starts the
harness:

```bash
scripts/spawn.sh smoke-test --type researcher --workspace /tmp/wt "task..."
```

For codex agents, use `scion --no-hub start <agent>` (the hub-mediated
start does not auto-detect `~/.codex/auth.json` from the broker host).
