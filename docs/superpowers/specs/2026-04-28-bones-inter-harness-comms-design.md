# Bones-Mediated Inter-Harness Communication

**Status:** approved (in-conversation, 2026-04-28) — implementation deferred
**Author:** dmestas
**Source PRs:** TBD
**Related specs:**
- `2026-04-28-remote-human-comms-deferral-design.md` (defines the envelope schema this spec consumes)
- `2026-04-28-roles-bases-template-collapse-design.md`

## Context

Today, Darkish Factory worker harnesses communicate via a single host-local file: `.scion/audit.jsonl`. The orchestrator (host-mode Claude Code) reads this file directly, and `scion look <agent>` exposes worker output. This works because every worker container shares one host's filesystem.

This breaks the moment harnesses live in heterogeneous topologies:

- Some workers in local Docker on the operator's laptop.
- Some workers as Kubernetes Pods in a remote cluster.
- Some workers on a teammate's broker.
- Eventually: workers reachable only via Scion Hub WebSocket Control Channel (NAT-traversed).

A worker on a remote broker cannot read or write `.scion/audit.jsonl` on the operator's laptop. Cross-host inter-harness comms requires a transport, not a shared file.

## Goal

Agents, orchestrators, and operators can exchange structured messages **regardless of where the harness physically runs**, using a stable in-container CLI (`bones msg`) whose backend transport is swappable without changing caller code.

## Background — what Scion already gives us

From `contributing/architecture/`, `concepts/`, `hub-user/messaging/`, `hub-user/runtime-broker/`:

| Primitive | Where it lives | What it gives us |
|---|---|---|
| `runtime.Exec(ctx, id, cmd)` | Every Scion runtime (Docker, Podman, Apple, K8s) | Run an arbitrary command inside any agent container — even on a remote broker — through the same Go interface. |
| WebSocket Control Channel | Hub ↔ Broker | Tunnels HTTP requests across NATs/firewalls. Message types: `request/response/event/stream_open/...`. HMAC-SHA256 auth. |
| Message Broker Plugin | `hashicorp/go-plugin` over gRPC | Pluggable backends for "agent notifications and structured messaging" (docs: foundational stage). |
| Hub Inbox Tray + SSE | Hub web UI | Real-time delivery to humans (deferred per spec #1). |

`bones` is the unified CLI baked into every container image (`bones init|tasks|repo|...`), pre-built from `~/projects/agent-infra/cmd/bones`. It is the natural place for a `msg` subcommand — already on PATH, already compiled per-arch, already part of the smoke test (`images/<backend>/test-bones.sh`).

## Goals

After all phases ship:

- Any worker can post a message addressed to another worker by role/name and have it delivered, regardless of broker location.
- Any worker can post a status/notification to the orchestrator without writing to a host file directly.
- The host-mode operator gets the same `darken inbox <agent>` view they have today.
- Migration to Mode A (Hub mode) requires no caller-code change — only an env var swap (`BONES_MSG_ENDPOINT`).

## Non-goals

- Building a general pub/sub message bus.
- Implementing exactly-once delivery semantics. At-least-once with idempotency keys (`corr_id` / message `id`) is sufficient.
- Replacing `.scion/audit.jsonl` immediately — Phase A keeps it as the backing store.
- Encrypting messages in transit beyond what the transport already provides (TLS for HTTP, HMAC for Hub channel).
- Inter-grove messaging. Scope is intra-grove only; per Scion's identity model, agents can only reach peers within `grove--agent` boundary.

## Envelope schema

Defined fully in spec #1 (remote-human-comms-deferral). Reproduced here for reference:

```json
{
  "schema_version": "1",
  "id": "uuid-v4",
  "ts": "<RFC3339>",
  "from": {"kind": "agent|orchestrator|operator", "name": "...", "role": "...", "grove": "..."},
  "to":   {"kind": "agent|orchestrator|operator|broadcast", "name": "...", "role": null},
  "severity": "info|warn|error|urgent",
  "kind": "report|question|status|ask_user|notification",
  "subject": "...",
  "body": "...",
  "actionable": true,
  "corr_id": "...",
  "links": [...]
}
```

The schema is a strict subset of Scion's Inbox / Discord webhook payload shape, so Phase C transports require no transform.

## CLI surface (`bones msg`)

Lives in `~/projects/agent-infra/cmd/bones/msg/` (new package). Subcommands:

```
bones msg send --to <name|role> --kind <kind> --severity <sev> --subject "..." [--body @file] [--corr <id>]
bones msg recv [--for <name>] [--since <ts>] [--limit N]    # one-shot
bones msg tail [--for <name>] [--since <ts>]                # streaming (long-poll or SSE)
bones msg poll [--for <name>] --until <kind|corr_id>        # block until match
bones msg ack  <message-id>                                  # mark read
bones msg deliver --payload <stdin|file>                     # internal: receive a pushed message
```

`bones msg` reads its endpoint from env in priority order:

1. `BONES_MSG_ENDPOINT` — explicit override (Phase B+).
2. `BONES_MSG_FILE` — path to JSONL store (Phase A; default `.scion/audit.jsonl` mounted into container).
3. Default: stderr-error with "no transport configured."

`from` is auto-populated from `$SCION_AGENT_NAME`, `$SCION_AGENT_ROLE`, `$SCION_GROVE_ID` (already injected by Scion per architecture docs).

## Phase A — Extract the API (no behavior change)

**Outcome**: same single-host topology, but every read/write goes through a stable CLI surface and conforms to the envelope schema.

### Changes

1. **Add `bones msg` subcommand** (in `agent-infra/cmd/bones/msg/`) that wraps file I/O against `BONES_MSG_FILE` (default `/workspace/.scion/audit.jsonl` since `/workspace` is the bind-mounted host file).
2. **Migrate orchestrator-side audit reads** in `bin/darken` to read messages via the same JSONL format with the new envelope schema. Existing audit lines need a one-time backfill — write a small migrator that wraps legacy lines into envelopes (mark `schema_version: "0"` if pre-existing).
3. **Add `darken inbox <agent>`** that reads & filters envelopes by `to.name`, `corr_id`, severity, etc. — replaces ad-hoc `scion look` for messaging concerns. `scion look` continues to work for raw container output.
4. **Update images smoke test**: `test-bones.sh` already verifies `bones --help` runs cleanly; extend to `bones msg --help`.

### Acceptance

- A worker can `bones msg send --to orchestrator --kind status --subject "PR ready"` and the orchestrator sees it via `darken inbox orchestrator`.
- All envelope fields populate correctly from env.
- Legacy `.scion/audit.jsonl` lines are still readable (migrator covers them).

### What does NOT change

- Workers still write to a host-mounted file. No network transport yet.
- Cross-broker scenarios still don't work — but the API contract is now stable, so callers won't break in Phase B.

## Phase B — Host-bridged HTTP transport

**Outcome**: workers running on different brokers (local Docker + remote K8s, etc.) can reach each other through an orchestrator-side router, **without requiring Scion Hub mode**.

### Architecture

```
                ┌─────────────────────────────────┐
                │   darken-msg-router (host)      │
                │   listens on TCP :9810          │
                │   persists to .scion/audit.jsonl│
                │   routes by (grove, recipient)  │
                └────┬──────────────────┬─────────┘
                     │                  │
       host.docker.internal      kubectl exec / runtime.Exec
                     │                  │
              ┌──────▼──────┐    ┌──────▼─────────┐
              │ local Docker│    │ remote K8s pod │
              │  (bones msg)│    │   (bones msg)  │
              └─────────────┘    └────────────────┘
```

### Components

**`darken-msg-router`** — small Go service launched alongside `darken init` (or on demand via `darken msg-router start`):

- HTTP server on a configurable port (default 9810). Endpoints:
  - `POST /v1/send` — accepts envelope, persists, queues delivery.
  - `GET /v1/recv?for=<name>&since=<ts>&limit=N` — long-poll fetch.
  - `GET /v1/tail?for=<name>` — SSE stream.
  - `POST /v1/ack` — mark read.
  - `GET /healthz`.
- Persistence: append-only `.scion/audit.jsonl` (same file Phase A used). Optional SQLite index for fast `since`/`recipient` queries.
- Delivery: pull-based (recipients poll via `bones msg tail`). No push needed in Phase B.
- Auth: HMAC token issued at agent provisioning time, injected as `BONES_MSG_TOKEN`. Router signs responses; workers verify.

### Container reachability

Three cases. The router defaults to listening on the host's `0.0.0.0:9810`; the network configuration determines reachability:

| Worker location | How worker reaches router | Implementation |
|---|---|---|
| Local Docker | `host.docker.internal:9810` | Already works on Docker Desktop macOS/Windows. On Linux, set `--add-host=host.docker.internal:host-gateway` in the runtime args. |
| Local Podman | `host.containers.internal:9810` | Native in Podman ≥ 4.0. |
| Remote K8s | Pull-based via `runtime.Exec(podID, ["bones","msg","deliver","--payload",b64])` | The orchestrator-side router pushes deliveries to remote pods using Scion's Runtime interface — no inbound reachability needed. |
| Apple `container` | Set `BONES_MSG_ENDPOINT` to the host's LAN IP. | Documented in operator setup. |

The K8s case uses `runtime.Exec` as a unidirectional push: when a message arrives for a recipient on a remote broker, the router walks the agent registry (Scion's Hub or local cache), finds the broker, and exec's a `bones msg deliver` command inside the recipient container to drop the payload at a known path (`/run/scion/inbox/<id>.json`). The recipient's `bones msg tail` watches that directory.

### Changes

1. **New binary**: `darken-msg-router` in `cmd/darken-msg-router/` (Go). Imports `pkg/runtime` from the Scion source tree to reuse `runtime.Exec` for K8s push.
2. **Lifecycle hook**: `darken init` provisions an HMAC token, writes the router config, optionally starts the router via systemd / launchd.
3. **Bones backend**: add `bones msg` HTTP transport. Selected when `BONES_MSG_ENDPOINT` is set (overrides file backend). Falls back to file backend if router is unreachable (graceful degradation).
4. **Per-agent provisioning**: `bin/darken spawn` injects `BONES_MSG_ENDPOINT=http://<host-gateway>:9810` and `BONES_MSG_TOKEN=...` into the container env.

### Security

- HMAC token per agent, scoped by `grove--agent` (matches Scion's identity boundary).
- Router refuses messages from agents whose `from` doesn't match their token's identity.
- TLS optional in Phase B (loopback / LAN). Mandatory if router ever binds to a non-loopback interface.
- Persistence file remains chmod 600.

### Acceptance

- A worker on local Docker can `bones msg send --to <name>` and a worker on a remote K8s broker receives it within ~5s of the next `tail`/`poll`.
- Router survives Claude Code session restart (state is in `.scion/audit.jsonl`).
- File backend still works when router is down.

## Phase C — Mode A graduation

**Outcome**: when (if) we go Mode A, swap the router for Scion's Message Broker Plugin. Caller code unchanged.

### Changes

1. **Implement a Message Broker Plugin** (`darkish-broker-plugin`) that satisfies Scion's plugin contract (gRPC service over `hashicorp/go-plugin`). It accepts the same envelope shape and routes through the Hub WebSocket Control Channel.
2. **Bones backend**: when `BONES_MSG_ENDPOINT=hub://...`, route to the plugin's gRPC socket. (`hashicorp/go-plugin` plugins expose a Unix socket at provisioning time; Scion docs document the path conventions in `concepts/` plugin section.)
3. **Decommission `darken-msg-router`**: keep available for users who don't run a Hub. The Bones backend selector picks based on env.

### Open questions for Phase C

- The Scion plugin contract for Message Broker is "foundational stage" per `concepts/`. Confirm the gRPC service definition is stable before committing to a plugin implementation. If not stable, stay on Phase B until it is.
- Whether to use Scion's Inbox Tray as the human-facing surface (requires Mode A) or keep `darken inbox <agent>` as the host-side view. Likely both.

## Migration plan

| Step | Owner | Trigger |
|---|---|---|
| 1. Land envelope schema in spec #1 | Done in this commit | — |
| 2. Implement `bones msg` Phase A subcommands | TBD | After spec #3 lands (touches the same images) |
| 3. Backfill migrator for legacy `audit.jsonl` lines | TBD | Same PR as #2 |
| 4. `darken inbox` host-side reader | TBD | Same PR as #2 |
| 5. Phase B router + HTTP backend in bones | TBD | When ≥1 worker needs to run on a non-host runtime |
| 6. Phase B K8s push via `runtime.Exec` | TBD | Same PR as #5, behind a `--enable-k8s-push` flag if it adds risk |
| 7. Phase C plugin | TBD | Mode A pivot trigger fires |

## Risks

- **`runtime.Exec` push requires the orchestrator to import Scion source.** Today `bin/darken` shells out to `scion`. We'd need to either (a) call `scion exec <pod> -- bones msg deliver ...` (cleaner) or (b) link `pkg/runtime` directly (faster). Prefer (a) — it preserves the "configs + hooks + agent definitions on top of a substrate" pivot from PR #1.
- **Two transports diverge.** File backend and HTTP backend must produce byte-identical envelopes. Mitigated by a shared envelope-encoder library inside `bones`.
- **`bones` rebuild discipline.** Adding subcommands means re-running `make -C images prebuild-bones` and re-baking images. Document in operator runbook.
- **Audit-log bifurcation.** If both `bin/darken` and `darken-msg-router` write to the same JSONL file, contention is real. Phase B router takes ownership of the file; `bin/darken` reads only.

## Open questions

- **Authentication model for human operator.** Today the operator IS the host orchestrator process; no auth needed. Phase B router-to-CLI auth: trivial (loopback). If we ever support a remote operator (e.g., laptop ↔ desktop), spec #1 re-opens.
- **Retention policy.** `.scion/audit.jsonl` grows unbounded. Define a rotation policy (size-based, e.g., 100 MB) before Phase B ships.
- **Schema evolution.** When fields get added (`schema_version: "2"`), do old workers ignore unknowns gracefully? Yes by spec — but verify `bones` JSON decoder is permissive.
- **Broadcast semantics.** `to.kind: broadcast` — fan out to all agents in grove? All agents matching a role? Punt to Phase A discussion; simplest is "all agents in grove except sender."

## Dependencies

- Spec #1 (envelope schema) — accepted in same commit.
- Spec #3 (roles/bases collapse) — independent but touches the same image rebuild pipeline; coordinate landing order.
- `agent-infra` repo public-availability — currently private. `bones` rebuilds need source access. Document in operator runbook.
