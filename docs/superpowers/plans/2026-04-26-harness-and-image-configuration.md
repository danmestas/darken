# Harness and Image Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the operator-facing surface specified in `docs/superpowers/specs/2026-04-26-harness-and-image-configuration-design.md`: universal image baseline (bones + caveman + context-mode + tools), hub-secrets-everywhere auth, 13-harness pipeline (3 new planner tiers + darwin), skill staging with 5-category taxonomy, and the `darkish` Go CLI wrapping all daily-workflow operations.

**Architecture:** Seven phases. Phase A bakes universal tooling into all four `darkish-*` images. Phase B migrates auth from volume-mounts to hub secrets uniformly. Phase C builds the skill staging machinery and adds `skills:` to manifests. Phase D adds three new planner manifests and the `darwin` evolution-agent harness. Phase E builds the `darkish` Go CLI (stdlib only, shells out to `scion` and `docker`). Phase F handles spec-kit per-harness install for `planner-t4`. Phase G runs end-to-end smoke + final docs sync.

**Tech Stack:** Go 1.23+ (stdlib only — yaml parsing via `scion templates show --format json`). Bash for shell scripts. Docker for images. Make for image build orchestration. scion CLI for runtime control.

---

## Spec coverage map

| Spec section | Implementing tasks |
|---|---|
| §3.1 Backend matrix (13 harnesses) | D-01 through D-08 |
| §3.2 Override mechanism | E-03 (`darkish spawn --backend`) |
| §3.3 Communication tiers | A-02 (caveman clone), D-01..D-08 (system prompts name tier) |
| §4.1 Universal baseline | A-01..A-06 |
| §4.2 Backend-specific deltas | A-01 (Dockerfile per backend) |
| §4.3 Per-harness one-off installs | F-01..F-03 (spec-kit for planner-t4) |
| §5.1 Skill taxonomy | C-04 (manifest skills:), G-02 (docs) |
| §5.2 Skill layering (3 scopes) | A-02 (base), C-01..C-04 (role), C-05 (ad-hoc) |
| §5.3 Per-harness skill defaults | C-04 |
| §5.4 Manifest declaration format | C-04 |
| §5.5 Stage script | C-01..C-03 |
| §6.1 Hub-secrets-everywhere | B-01..B-05 |
| §6.2 Per-backend pattern | B-01 |
| §6.3 stage-creds.sh updates | B-01 |
| §6.4 OAuth preservation | B-01, B-02 |
| §6.5 Manifest auth references | B-03, B-04 |
| §7 Manifest shapes | C-04, D-01..D-05 |
| §8 Routing classifier extension | D-08 (orchestrator agents.md update) |
| §9 Failure modes | E-02 (`darkish doctor` maps errors) |
| §10 Testing & verification | A-06, B-05, C-07, G-01 |
| §11 Open questions | E-04 (darwin recommendations), F-01 (spec-kit choice) |
| §12.1 `darkish doctor` | E-02 |
| §12.2 Spawn workflow | E-03 |
| §12.3 `darkish bootstrap` | E-04 |
| §12.4 `darkish apply` | E-05 |
| §12.5 `darkish create-harness` | E-06 |
| §12.6 `darkish skills` | E-07 |
| §12.7 The `darkish` Go CLI | E-01..E-12 |
| §12.8 What this is NOT | (informational, no task) |
| §13 Cross-references | G-02 (final docs) |

---

## File structure

**Go CLI (new):**
- `cmd/darkish/main.go` — subcommand dispatcher + `flag` parsing.
- `cmd/darkish/doctor.go` — preflight + post-mortem health checks.
- `cmd/darkish/spawn.go` — stage creds + skills + `scion start`.
- `cmd/darkish/bootstrap.go` — first-time machine setup orchestrator.
- `cmd/darkish/apply.go` — darwin recommendation reviewer + applier.
- `cmd/darkish/create_harness.go` — scaffolds new harness directory.
- `cmd/darkish/skills.go` — wraps stage-skills.sh.
- `cmd/darkish/creds.go` — wraps stage-creds.sh.
- `cmd/darkish/images.go` — wraps `make -C images`.
- `cmd/darkish/list.go` — thin wrapper over `scion list`.
- `cmd/darkish/*_test.go` — one test file per subcommand. Stdlib `testing`. Mock `scion` via stub in `testdata/`.
- `cmd/darkish/testdata/scion-stub.sh` — mock scion CLI for tests.
- `cmd/darkish/testdata/docker-stub.sh` — mock docker CLI for tests.
- `internal/staging/manifest.go` — parses manifest JSON (from `scion templates show --format json`) and exposes `Skills() []string`.
- `internal/staging/skills.go` — copies role skills from `~/projects/agent-skills/skills/<name>/` into `<repo>/.scion/skills-staging/<harness>/`.
- `internal/staging/skills_test.go` — round-trip test against a fixture skill repo.
- `internal/staging/manifest_test.go` — JSON parse round-trip.
- `go.mod` / `go.sum` — module file. Empty `require` block (stdlib only).

**Shell scripts (modified or new):**
- `scripts/stage-creds.sh` — MODIFIED: pushes hub secrets for all four backends (claude/codex/pi/gemini); removes the volume-mount staging path.
- `scripts/stage-skills.sh` — NEW: idempotent skill staging with `--add`, `--remove`, `--diff` modes.
- `scripts/spawn.sh` — MODIFIED: invokes both `stage-creds.sh` and `stage-skills.sh <harness>` before `scion start`. Adds `--no-stage` flag.
- `scripts/bootstrap.sh` — NEW: thin wrapper that calls `darkish bootstrap` (so operators can invoke either).

**Image files (modified):**
- `images/claude/Dockerfile` — adds bones builder stage, caveman clone, context-mode MCP install layer.
- `images/codex/Dockerfile` — same baseline as claude.
- `images/pi/Dockerfile` — same baseline as claude.
- `images/gemini/Dockerfile` — same baseline as claude.
- `images/codex/darkish-prelude.sh` — MODIFIED: detects `SCION_TEMPLATE_NAME=planner-t4` and runs spec-kit installer.
- `images/Makefile` — adds `tools-only` and `prelude-only` fast-rebuild targets.
- `images/README.md` — documents universal baseline + per-image deltas.

**Harness manifests (modified or new):**
- `.scion/templates/orchestrator/scion-agent.yaml` — adds `skills:`; removes `volumes:`; mounts skills-staging dir.
- `.scion/templates/researcher/scion-agent.yaml` — same shape change.
- `.scion/templates/designer/scion-agent.yaml` — same.
- `.scion/templates/planner-t1/scion-agent.yaml` — NEW.
- `.scion/templates/planner-t2/scion-agent.yaml` — NEW.
- `.scion/templates/planner-t3/scion-agent.yaml` — RENAMED from `planner/`; system prompt updated for superpowers.
- `.scion/templates/planner-t4/scion-agent.yaml` — NEW (codex backend).
- `.scion/templates/tdd-implementer/scion-agent.yaml` — same shape change.
- `.scion/templates/verifier/scion-agent.yaml` — same; also flips `default_harness_config` to codex per spec §3.1.
- `.scion/templates/reviewer/scion-agent.yaml` — same; also flips to codex.
- `.scion/templates/sme/scion-agent.yaml` — adds skills + skills-staging mount (already on codex + hub secret).
- `.scion/templates/admin/scion-agent.yaml` — same shape change.
- `.scion/templates/darwin/scion-agent.yaml` — NEW (codex backend).
- `.scion/templates/<role>/agents.md` — each gets a "Communication tier" stanza per §3.3.
- `.scion/templates/<role>/system-prompt.md` — each names its tier explicitly.

**Runtime state (gitignored):**
- `.scion/skills-staging/` — gitignored; populated per-harness by `stage-skills.sh`.
- `.scion/darwin-recommendations/` — gitignored; populated by darwin runs.
- `.gitignore` — adds the two paths above.

**Repo top-level (modified):**
- `Makefile` — adds `darkish` target that builds `bin/darkish`.
- `apm.yml` — extends `dependencies.apm` with the union of every harness's role skills.
- `.design/harness-roster.md` — rewritten to list 13 harnesses.
- `.design/pipeline-mechanics.md` — adds 4-planner-tier section and darwin loop.

---

## Tasks

### Phase A — Image baselines (universal tooling layer)

#### Task A-01: Add bones build stage to darkish-claude Dockerfile

Multi-stage build clones `agent-infra`, compiles the 14 binaries listed in §4.1, then COPYs them into the final image.

- [ ] **Write the failing test**

  Create `images/claude/test-bones.sh`:

  ```bash
  #!/usr/bin/env bash
  # Smoke-test that the universal baseline is present in darkish-claude.
  set -euo pipefail

  IMG="${1:-local/darkish-claude:latest}"

  REQUIRED_BIN=(
    agent-init agent-tasks assert autoclaim chat compactanthropic
    dispatch fossil holds jskv presence tasks testutil workspace
    mgrep jq rg fzf gh
  )

  for b in "${REQUIRED_BIN[@]}"; do
    if ! docker run --rm --entrypoint /bin/sh "${IMG}" -c "command -v ${b} >/dev/null"; then
      echo "FAIL: ${b} not on PATH in ${IMG}" >&2
      exit 1
    fi
  done

  echo "PASS: all baseline binaries present in ${IMG}"
  ```

  Run `chmod +x images/claude/test-bones.sh`.

- [ ] **Run the test and verify it fails**

  ```bash
  bash images/claude/test-bones.sh local/darkish-claude:latest
  ```

  Expected: fails on `agent-init` (or `mgrep`) — neither is in the current image.

- [ ] **Write the minimal implementation**

  Replace `images/claude/Dockerfile` (full file rewrite — show full new file):

  ```dockerfile
  # darkish-claude — universal baseline + claude backend.
  # Layers ordered heaviest-at-bottom for cache efficiency.

  ARG BASE_IMAGE=local/scion-claude:latest

  # ----- Stage 1: build bones binaries from agent-infra -----
  FROM golang:1.23 AS bones-builder
  ARG AGENT_INFRA_REF=main
  RUN git clone --depth 1 --branch ${AGENT_INFRA_REF} \
        https://github.com/danmestas/agent-infra /src/agent-infra
  WORKDIR /src/agent-infra
  RUN mkdir -p /out/bin && \
      for cmd in agent-init agent-tasks assert autoclaim chat compactanthropic \
                 dispatch fossil holds jskv presence tasks testutil workspace; do \
        CGO_ENABLED=0 go build -o /out/bin/$cmd ./cmd/$cmd; \
      done

  # ----- Stage 2: build mgrep -----
  FROM golang:1.23 AS mgrep-builder
  RUN CGO_ENABLED=0 go install github.com/danmestas/mgrep-code-search/cmd/mgrep@latest && \
      cp /go/bin/mgrep /out-mgrep

  # ----- Stage 3: final image -----
  FROM ${BASE_IMAGE}

  USER root

  # 1. Apt utilities (heaviest, churns least).
  RUN apt-get update && apt-get install -y --no-install-recommends \
        jq ripgrep fzf less gh ca-certificates curl git \
      && rm -rf /var/lib/apt/lists/*

  # 2. Bones binaries from stage 1.
  COPY --from=bones-builder /out/bin/* /usr/local/bin/

  # 3. mgrep from stage 2.
  COPY --from=mgrep-builder /out-mgrep /usr/local/bin/mgrep
  RUN chmod +x /usr/local/bin/mgrep

  # 4. Universal skill: caveman.
  RUN mkdir -p /home/scion/skills && \
      git clone --depth 1 https://github.com/juliusbrussee/caveman \
        /home/scion/skills/caveman && \
      chown -R scion:scion /home/scion/skills

  # 5. Universal MCP: context-mode.
  RUN npm install -g @mksglu/context-mode || \
      (echo "context-mode npm install failed; falling back to git clone" && \
       git clone --depth 1 https://github.com/mksglu/context-mode \
         /opt/context-mode && \
       cd /opt/context-mode && npm install --production)

  # 6. Prelude.
  COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
  RUN chmod +x /usr/local/bin/darkish-prelude.sh

  USER scion
  WORKDIR /workspace
  ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
  ```

- [ ] **Run the test and verify it passes**

  ```bash
  make -C images claude
  bash images/claude/test-bones.sh local/darkish-claude:latest
  ```

- [ ] **Run lint + types**

  ```bash
  hadolint images/claude/Dockerfile || true   # advisory; non-blocking
  shellcheck images/claude/test-bones.sh
  ```

- [ ] **Commit**

  ```
  feat(images): bake universal baseline into darkish-claude

  Adds bones-builder + mgrep-builder multi-stage layers and copies the 14
  agent-infra binaries plus mgrep into /usr/local/bin. Clones the caveman
  skill into /home/scion/skills/caveman. Installs context-mode MCP via npm
  with a git-clone fallback. Adds images/claude/test-bones.sh as a smoke
  test for the baseline.
  ```

#### Task A-02: Mirror baseline into darkish-codex Dockerfile

Same baseline layer set, applied to the codex Dockerfile.

- [ ] **Write the failing test**

  Create `images/codex/test-bones.sh` — same shape as `images/claude/test-bones.sh` but takes `local/darkish-codex:latest` as default arg.

- [ ] **Run the test and verify it fails**

  ```bash
  bash images/codex/test-bones.sh
  ```

- [ ] **Write the minimal implementation**

  Replace `images/codex/Dockerfile`:

  ```dockerfile
  ARG BASE_IMAGE=local/scion-codex:latest

  FROM golang:1.23 AS bones-builder
  ARG AGENT_INFRA_REF=main
  RUN git clone --depth 1 --branch ${AGENT_INFRA_REF} \
        https://github.com/danmestas/agent-infra /src/agent-infra
  WORKDIR /src/agent-infra
  RUN mkdir -p /out/bin && \
      for cmd in agent-init agent-tasks assert autoclaim chat compactanthropic \
                 dispatch fossil holds jskv presence tasks testutil workspace; do \
        CGO_ENABLED=0 go build -o /out/bin/$cmd ./cmd/$cmd; \
      done

  FROM golang:1.23 AS mgrep-builder
  RUN CGO_ENABLED=0 go install github.com/danmestas/mgrep-code-search/cmd/mgrep@latest && \
      cp /go/bin/mgrep /out-mgrep

  FROM ${BASE_IMAGE}

  USER root

  RUN apt-get update && apt-get install -y --no-install-recommends \
        jq ripgrep fzf less gh ca-certificates curl git \
      && rm -rf /var/lib/apt/lists/*

  COPY --from=bones-builder /out/bin/* /usr/local/bin/
  COPY --from=mgrep-builder /out-mgrep /usr/local/bin/mgrep
  RUN chmod +x /usr/local/bin/mgrep

  RUN mkdir -p /home/scion/skills && \
      git clone --depth 1 https://github.com/juliusbrussee/caveman \
        /home/scion/skills/caveman && \
      chown -R scion:scion /home/scion/skills

  RUN npm install -g @mksglu/context-mode || \
      (git clone --depth 1 https://github.com/mksglu/context-mode \
         /opt/context-mode && \
       cd /opt/context-mode && npm install --production)

  COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
  RUN chmod +x /usr/local/bin/darkish-prelude.sh

  USER scion
  WORKDIR /workspace
  ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
  ```

- [ ] **Run the test and verify it passes**

  ```bash
  make -C images codex
  bash images/codex/test-bones.sh
  ```

- [ ] **Run lint + types**

  ```bash
  hadolint images/codex/Dockerfile || true
  shellcheck images/codex/test-bones.sh
  ```

- [ ] **Commit**

  ```
  feat(images): bake universal baseline into darkish-codex

  Mirrors the bones + caveman + context-mode + apt baseline from
  darkish-claude into darkish-codex. Adds smoke test.
  ```

#### Task A-03: Mirror baseline into darkish-pi Dockerfile

Same change as A-02, applied to `images/pi/Dockerfile`.

- [ ] **Write the failing test**

  Create `images/pi/test-bones.sh` (default arg: `local/darkish-pi:latest`).

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `images/pi/Dockerfile`:

  ```dockerfile
  # darkish-pi — universal baseline + pi (OpenRouter) backend.
  ARG BASE_IMAGE=local/scion-pi:latest

  FROM golang:1.23 AS bones-builder
  ARG AGENT_INFRA_REF=main
  RUN git clone --depth 1 --branch ${AGENT_INFRA_REF} \
        https://github.com/danmestas/agent-infra /src/agent-infra
  WORKDIR /src/agent-infra
  RUN mkdir -p /out/bin && \
      for cmd in agent-init agent-tasks assert autoclaim chat compactanthropic \
                 dispatch fossil holds jskv presence tasks testutil workspace; do \
        CGO_ENABLED=0 go build -o /out/bin/$cmd ./cmd/$cmd; \
      done

  FROM golang:1.23 AS mgrep-builder
  RUN CGO_ENABLED=0 go install github.com/danmestas/mgrep-code-search/cmd/mgrep@latest && \
      cp /go/bin/mgrep /out-mgrep

  FROM ${BASE_IMAGE}

  USER root

  RUN apt-get update && apt-get install -y --no-install-recommends \
        jq ripgrep fzf less gh ca-certificates curl git \
      && rm -rf /var/lib/apt/lists/*

  COPY --from=bones-builder /out/bin/* /usr/local/bin/
  COPY --from=mgrep-builder /out-mgrep /usr/local/bin/mgrep
  RUN chmod +x /usr/local/bin/mgrep

  RUN mkdir -p /home/scion/skills && \
      git clone --depth 1 https://github.com/juliusbrussee/caveman \
        /home/scion/skills/caveman && \
      chown -R scion:scion /home/scion/skills

  RUN npm install -g @mksglu/context-mode || \
      (git clone --depth 1 https://github.com/mksglu/context-mode \
         /opt/context-mode && \
       cd /opt/context-mode && npm install --production)

  # Pi backend trust mechanism: --non-interactive bypasses prompts; no
  # prelude-side trust dialog suppression required (validated separately).
  COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
  RUN chmod +x /usr/local/bin/darkish-prelude.sh

  USER scion
  WORKDIR /workspace
  ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
  ```

- [ ] **Run the test and verify it passes**

  ```bash
  make -C images pi
  bash images/pi/test-bones.sh
  ```

- [ ] **Run lint + types**

  ```bash
  hadolint images/pi/Dockerfile || true
  shellcheck images/pi/test-bones.sh
  ```

- [ ] **Commit**

  ```
  feat(images): bake universal baseline into darkish-pi
  ```

#### Task A-04: Mirror baseline into darkish-gemini Dockerfile

Same change for gemini.

- [ ] **Write the failing test**

  `images/gemini/test-bones.sh` (default `local/darkish-gemini:latest`).

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `images/gemini/Dockerfile`:

  ```dockerfile
  # darkish-gemini — universal baseline + gemini (Google) backend.
  ARG BASE_IMAGE=local/scion-gemini:latest

  FROM golang:1.23 AS bones-builder
  ARG AGENT_INFRA_REF=main
  RUN git clone --depth 1 --branch ${AGENT_INFRA_REF} \
        https://github.com/danmestas/agent-infra /src/agent-infra
  WORKDIR /src/agent-infra
  RUN mkdir -p /out/bin && \
      for cmd in agent-init agent-tasks assert autoclaim chat compactanthropic \
                 dispatch fossil holds jskv presence tasks testutil workspace; do \
        CGO_ENABLED=0 go build -o /out/bin/$cmd ./cmd/$cmd; \
      done

  FROM golang:1.23 AS mgrep-builder
  RUN CGO_ENABLED=0 go install github.com/danmestas/mgrep-code-search/cmd/mgrep@latest && \
      cp /go/bin/mgrep /out-mgrep

  FROM ${BASE_IMAGE}

  USER root

  RUN apt-get update && apt-get install -y --no-install-recommends \
        jq ripgrep fzf less gh ca-certificates curl git \
      && rm -rf /var/lib/apt/lists/*

  COPY --from=bones-builder /out/bin/* /usr/local/bin/
  COPY --from=mgrep-builder /out-mgrep /usr/local/bin/mgrep
  RUN chmod +x /usr/local/bin/mgrep

  RUN mkdir -p /home/scion/skills && \
      git clone --depth 1 https://github.com/juliusbrussee/caveman \
        /home/scion/skills/caveman && \
      chown -R scion:scion /home/scion/skills

  RUN npm install -g @mksglu/context-mode || \
      (git clone --depth 1 https://github.com/mksglu/context-mode \
         /opt/context-mode && \
       cd /opt/context-mode && npm install --production)

  # Gemini trust mechanism is unverified pending operator account setup
  # (spec §11 Open Question). Prelude is the gate when the path is known.
  COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
  RUN chmod +x /usr/local/bin/darkish-prelude.sh

  USER scion
  WORKDIR /workspace
  ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
  ```

- [ ] **Run the test and verify it passes**

  ```bash
  make -C images gemini
  bash images/gemini/test-bones.sh
  ```

- [ ] **Run lint + types**

  ```bash
  hadolint images/gemini/Dockerfile || true
  shellcheck images/gemini/test-bones.sh
  ```

- [ ] **Commit**

  ```
  feat(images): bake universal baseline into darkish-gemini
  ```

#### Task A-05: Add fast-rebuild Makefile targets

Add `tools-only` and `prelude-only` targets that rebuild just the relevant layers using docker BuildKit cache hints. Keeps the operator's daily inner loop fast.

- [ ] **Write the failing test**

  Create `images/test-make-targets.sh`:

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  cd "$(dirname "$0")"
  for target in tools-only-claude prelude-only-claude; do
    if ! make -n ${target} >/dev/null 2>&1; then
      echo "FAIL: make target ${target} missing" >&2; exit 1
    fi
  done
  echo "PASS: fast-rebuild targets present"
  ```

- [ ] **Run the test and verify it fails**

  ```bash
  bash images/test-make-targets.sh
  ```

- [ ] **Write the minimal implementation**

  Append to `images/Makefile`:

  ```makefile
  # Fast-rebuild targets: rebuild only the bottom-most changed layer.
  # Useful when iterating on the prelude script without re-pulling apt
  # or rebuilding bones.

  .PHONY: tools-only-claude
  tools-only-claude:
  	docker build \
  		--build-arg BASE_IMAGE=$(REGISTRY)/scion-claude:$(TAG) \
  		--target final \
  		--no-cache-filter final \
  		-t $(REGISTRY)/darkish-claude:$(TAG) \
  		-f claude/Dockerfile \
  		claude

  # prelude-only-* rebuilds the final Dockerfile with --no-cache so the
  # prelude COPY layer (the last layer in each Dockerfile) is busted. The
  # heavy upstream layers (apt, bones, mgrep, caveman, context-mode) stay
  # cached because docker build matches them by content/instruction.
  .PHONY: prelude-only-claude
  prelude-only-claude:
  	docker build --no-cache \
  		--build-arg BASE_IMAGE=$(REGISTRY)/scion-claude:$(TAG) \
  		-t $(REGISTRY)/darkish-claude:$(TAG) \
  		-f claude/Dockerfile claude

  .PHONY: tools-only-codex tools-only-pi tools-only-gemini
  tools-only-codex:
  	docker build \
  		--build-arg BASE_IMAGE=$(REGISTRY)/scion-codex:$(TAG) \
  		--target final \
  		--no-cache-filter final \
  		-t $(REGISTRY)/darkish-codex:$(TAG) \
  		-f codex/Dockerfile codex
  tools-only-pi:
  	docker build \
  		--build-arg BASE_IMAGE=$(REGISTRY)/scion-pi:$(TAG) \
  		--target final \
  		--no-cache-filter final \
  		-t $(REGISTRY)/darkish-pi:$(TAG) \
  		-f pi/Dockerfile pi
  tools-only-gemini:
  	docker build \
  		--build-arg BASE_IMAGE=$(REGISTRY)/scion-gemini:$(TAG) \
  		--target final \
  		--no-cache-filter final \
  		-t $(REGISTRY)/darkish-gemini:$(TAG) \
  		-f gemini/Dockerfile gemini

  .PHONY: tools-only-all
  tools-only-all: tools-only-claude tools-only-codex tools-only-pi tools-only-gemini
  ```

  Add `AS final` to the final stage of each Dockerfile (a one-line edit per file: `FROM ${BASE_IMAGE} AS final`).

- [ ] **Run the test and verify it passes**

  ```bash
  bash images/test-make-targets.sh
  ```

- [ ] **Run lint + types**

  ```bash
  shellcheck images/test-make-targets.sh
  ```

- [ ] **Commit**

  ```
  feat(images): add fast-rebuild Makefile targets

  Adds tools-only-* targets per backend that rebuild only the final stage
  of each darkish-* image. Names the final stage `AS final` in each
  Dockerfile so --target works.
  ```

#### Task A-06: Update images/README.md to document universal baseline

- [ ] **Write the failing test**

  Create `images/test-readme.sh`:

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  README="$(dirname "$0")/README.md"
  for tok in "Universal baseline" "bones" "caveman" "context-mode" "mgrep"; do
    if ! grep -q "${tok}" "${README}"; then
      echo "FAIL: ${tok} not documented in README" >&2; exit 1
    fi
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Add a `## Universal baseline` section to `images/README.md` describing what every darkish-* image now contains: the 14 bones binaries, mgrep, jq+rg+fzf+gh, the caveman skill at `/home/scion/skills/caveman/`, and the context-mode MCP. Lift the table from spec §4.1 verbatim.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  shellcheck images/test-readme.sh
  ```

- [ ] **Commit**

  ```
  docs(images): document universal baseline in images/README.md
  ```

---

### Phase B — Hub-secrets-everywhere migration

#### Task B-01: Rewrite scripts/stage-creds.sh to push hub secrets for all four backends

Replaces volume-mount staging with hub-secret push uniformly.

- [ ] **Write the failing test**

  Create `scripts/test-stage-creds.sh`:

  ```bash
  #!/usr/bin/env bash
  # Verifies stage-creds.sh has sections for all four backends and pushes
  # each as a hub secret (not as a file under ~/.scion-credentials/).
  set -euo pipefail

  SCRIPT="$(dirname "$0")/stage-creds.sh"

  for backend in claude codex pi gemini; do
    if ! grep -q "stage_${backend}" "${SCRIPT}"; then
      echo "FAIL: stage_${backend} function missing" >&2; exit 1
    fi
  done

  if ! grep -q "scion hub secret set" "${SCRIPT}"; then
    echo "FAIL: stage-creds.sh does not push to hub" >&2; exit 1
  fi

  if grep -q '\${HOME}/.scion-credentials' "${SCRIPT}"; then
    echo "FAIL: stage-creds.sh still writes to ~/.scion-credentials (legacy path)" >&2
    exit 1
  fi

  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

  ```bash
  bash scripts/test-stage-creds.sh
  ```

  Current script: only claude+codex sections, writes to `~/.scion-credentials/` instead of hub.

- [ ] **Write the minimal implementation**

  Replace `scripts/stage-creds.sh` (full new file):

  ```bash
  #!/usr/bin/env bash
  # stage-creds.sh — push every backend's auth into scion's hub secret store.
  #
  # Idempotent: re-running with the same source state is a no-op.
  # Soft-fails per backend (a missing keychain entry skips that backend
  # only; other backends still get staged).
  #
  # Hub secret targets (per spec §6.2):
  #   claude  → /home/scion/.claude/.credentials.json   (file type)
  #   codex   → /home/scion/.codex/auth.json            (file type)
  #   pi      → OPENROUTER_API_KEY                       (env type)
  #   gemini  → /home/scion/.gemini/oauth_creds.json     (file type, OAuth)
  #             OR GEMINI_API_KEY (env type, API-key path)
  #
  # Usage:
  #   scripts/stage-creds.sh              # all backends
  #   scripts/stage-creds.sh claude       # one backend

  set -euo pipefail

  WHAT="${1:-all}"
  TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "${TMP_DIR}"' EXIT

  scion_present() {
    command -v scion >/dev/null 2>&1
  }

  push_file_secret() {
    local name="$1" target="$2" src="$3"
    if ! scion_present; then
      echo "stage-creds: scion CLI not on PATH; cannot push ${name}" >&2
      return 1
    fi
    scion hub secret set --type file --target "${target}" "${name}" "@${src}" >/dev/null
    echo "stage-creds: ${name} pushed (file → ${target})"
  }

  push_env_secret() {
    local name="$1" value="$2"
    if ! scion_present; then
      echo "stage-creds: scion CLI not on PATH; cannot push ${name}" >&2
      return 1
    fi
    printf '%s' "${value}" > "${TMP_DIR}/${name}"
    scion hub secret set --type env --target "${name}" "${name}" "@${TMP_DIR}/${name}" >/dev/null
    rm -f "${TMP_DIR}/${name}"
    echo "stage-creds: ${name} pushed (env)"
  }

  stage_claude() {
    if ! command -v security >/dev/null 2>&1; then
      echo "stage-creds: WARNING — security CLI unavailable (non-macOS host); skipping claude." >&2
      return 1
    fi
    local blob="${TMP_DIR}/claude.json"
    if ! security find-generic-password -s "Claude Code-credentials" -w > "${blob}" 2>/dev/null; then
      echo "stage-creds: WARNING — Keychain entry 'Claude Code-credentials' not found; skipping claude." >&2
      return 1
    fi
    chmod 600 "${blob}"
    push_file_secret claude_auth "/home/scion/.claude/.credentials.json" "${blob}"
  }

  stage_codex() {
    local src="${HOME}/.codex/auth.json"
    if [[ ! -f "${src}" ]]; then
      echo "stage-creds: WARNING — ${src} not found; skipping codex." >&2
      return 1
    fi
    push_file_secret codex_auth "/home/scion/.codex/auth.json" "${src}"
  }

  stage_pi() {
    if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
      echo "stage-creds: WARNING — OPENROUTER_API_KEY not set; skipping pi." >&2
      return 1
    fi
    push_env_secret OPENROUTER_API_KEY "${OPENROUTER_API_KEY}"
  }

  stage_gemini() {
    if [[ -f "${HOME}/.gemini/oauth_creds.json" ]]; then
      push_file_secret gemini_auth "/home/scion/.gemini/oauth_creds.json" "${HOME}/.gemini/oauth_creds.json"
      return 0
    fi
    if [[ -n "${GEMINI_API_KEY:-}" ]]; then
      push_env_secret GEMINI_API_KEY "${GEMINI_API_KEY}"
      return 0
    fi
    echo "stage-creds: WARNING — neither ~/.gemini/oauth_creds.json nor GEMINI_API_KEY found; skipping gemini." >&2
    return 1
  }

  case "${WHAT}" in
    claude) stage_claude ;;
    codex)  stage_codex ;;
    pi)     stage_pi ;;
    gemini) stage_gemini ;;
    all)
      stage_claude || true
      stage_codex  || true
      stage_pi     || true
      stage_gemini || true
      ;;
    *)
      echo "Usage: $0 [claude|codex|pi|gemini|all]" >&2
      exit 2
      ;;
  esac
  ```

  Run `chmod +x scripts/stage-creds.sh`.

- [ ] **Run the test and verify it passes**

  ```bash
  bash scripts/test-stage-creds.sh
  ```

- [ ] **Run lint + types**

  ```bash
  shellcheck scripts/stage-creds.sh scripts/test-stage-creds.sh
  ```

- [ ] **Commit**

  ```
  refactor(scripts): push all four backends' auth as hub secrets

  Replaces ~/.scion-credentials volume-mount staging with hub secret push
  for claude, codex, pi, gemini. Single uniform mechanism per spec §6.1.
  Soft-fails per backend so partial environments still stage.
  ```

#### Task B-02: Run stage-creds.sh end-to-end and verify hub state

- [ ] **Write the failing test**

  Create `scripts/test-stage-creds-integration.sh`:

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  bash "$(dirname "$0")/stage-creds.sh" all
  for name in claude_auth codex_auth; do
    if ! scion hub secret list 2>/dev/null | grep -q "${name}"; then
      echo "FAIL: ${name} not in hub secret list" >&2; exit 1
    fi
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

  Pre-rewrite, claude_auth doesn't exist as a hub secret. Test fails.

- [ ] **Write the minimal implementation**

  No code changes — A-01's rewrite already produces this behavior. This task validates it on real infrastructure.

- [ ] **Run the test and verify it passes**

  ```bash
  bash scripts/test-stage-creds-integration.sh
  ```

- [ ] **Run lint + types**

  ```bash
  shellcheck scripts/test-stage-creds-integration.sh
  ```

- [ ] **Commit**

  ```
  test(scripts): integration smoke for stage-creds.sh hub push
  ```

#### Task B-03: Remove volumes: blocks from 8 manifests using ~/.scion-credentials

The eight manifests currently mount `~/.scion-credentials/claude/.credentials.json` (orchestrator, researcher, designer, planner, tdd-implementer, verifier, reviewer, admin). Hub secret `claude_auth` from B-01 supersedes this mount.

- [ ] **Write the failing test**

  Create `scripts/test-no-legacy-volumes.sh`:

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  cd "$(dirname "$0")/../.scion/templates"
  fail=0
  for manifest in */scion-agent.yaml; do
    if grep -q "scion-credentials" "${manifest}"; then
      echo "FAIL: ${manifest} still references ~/.scion-credentials" >&2
      fail=1
    fi
  done
  exit ${fail}
  ```

- [ ] **Run the test and verify it fails**

  Lists 8 manifests with the legacy volume mount.

- [ ] **Write the minimal implementation**

  For each of the 8 manifests, remove the entire `volumes:` block. Example diff for `researcher/scion-agent.yaml`:

  ```yaml
  # BEFORE
  schema_version: "1"
  description: "Researcher - sandboxed web access, ..."
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: claude
  image: local/darkish-claude:latest
  model: claude-sonnet-4-6
  max_turns: 30
  max_duration: "1h"
  detached: false
  volumes:
    - source: ~/.scion-credentials/claude/.credentials.json
      target: /home/scion/.claude/.credentials.json
      read_only: true

  # AFTER
  schema_version: "1"
  description: "Researcher - sandboxed web access, ..."
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: claude
  image: local/darkish-claude:latest
  model: claude-sonnet-4-6
  max_turns: 30
  max_duration: "1h"
  detached: false
  ```

  Apply identical surgery to: `orchestrator`, `designer`, `planner`, `tdd-implementer`, `verifier`, `reviewer`, `admin`. (sme already lacks the block.)

- [ ] **Run the test and verify it passes**

  ```bash
  bash scripts/test-no-legacy-volumes.sh
  ```

- [ ] **Run lint + types**

  Validate each manifest re-parses:

  ```bash
  for d in .scion/templates/*/; do
    name="$(basename "${d}")"
    scion templates show "${name}" --local --format json >/dev/null
  done
  ```

- [ ] **Commit**

  ```
  refactor(harnesses): drop ~/.scion-credentials volume mounts

  Hub secret claude_auth now projects credentials at the canonical
  container path. The volume-mount approach is superseded across all
  eight claude-backend harnesses.
  ```

#### Task B-04: Smoke-test researcher and sme post-migration

- [ ] **Write the failing test**

  Create `scripts/test-spawn-no-volumes.sh`:

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  cleanup() {
    scion stop researcher-smoke --yes 2>/dev/null || true
    scion delete researcher-smoke --yes 2>/dev/null || true
    scion stop sme-smoke --yes 2>/dev/null || true
    scion delete sme-smoke --yes 2>/dev/null || true
  }
  trap cleanup EXIT

  scion start researcher-smoke --type researcher --notify "echo hello"
  for _ in $(seq 1 30); do
    if scion list 2>/dev/null | grep -q "researcher-smoke.*running\|researcher-smoke.*completed"; then
      break
    fi
    sleep 2
  done
  scion list | grep researcher-smoke

  scion start sme-smoke --type sme --notify "what is the answer to 1+1?"
  for _ in $(seq 1 30); do
    if scion list 2>/dev/null | grep -q "sme-smoke.*running\|sme-smoke.*completed"; then
      break
    fi
    sleep 2
  done
  scion list | grep sme-smoke
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

  Pre-B-01/B-03, claude harnesses can't start without the volume mount; researcher fails.

- [ ] **Write the minimal implementation**

  No new code — this validates B-01 + B-03.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  shellcheck scripts/test-spawn-no-volumes.sh
  ```

- [ ] **Commit**

  ```
  test(scripts): smoke researcher + sme spawn after auth migration
  ```

#### Task B-05: Strip auth notes from images/README.md and re-document hub-secret pattern

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  README="$(dirname "$0")/../images/README.md"
  if grep -q "scion-credentials" "${README}"; then
    echo "FAIL: README references legacy ~/.scion-credentials path" >&2; exit 1
  fi
  if ! grep -q "hub secret" "${README}"; then
    echo "FAIL: README does not document hub-secret auth model" >&2; exit 1
  fi
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace the auth sections in `images/README.md`. Remove all references to `~/.scion-credentials/`. Add a section "Auth model: hub-secrets-everywhere" describing the four secret targets per §6.2.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  docs(images): replace volume-mount auth notes with hub-secret model
  ```

---

### Phase C — Skill staging

#### Task C-01: Author scripts/stage-skills.sh (base mode)

The host-side script that materializes role skills into `<repo>/.scion/skills-staging/<harness>/`.

- [ ] **Write the failing test**

  Create `scripts/test-stage-skills.sh`:

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail

  REPO="$(cd "$(dirname "$0")/.." && pwd)"
  STAGE="${REPO}/.scion/skills-staging"

  rm -rf "${STAGE}/sme"

  bash "${REPO}/scripts/stage-skills.sh" sme

  for skill in ousterhout hipp; do
    if [[ ! -d "${STAGE}/sme/${skill}" ]]; then
      echo "FAIL: ${skill} not staged for sme" >&2; exit 1
    fi
    if [[ -L "${STAGE}/sme/${skill}" ]]; then
      echo "FAIL: ${skill} is a symlink (must be a copy)" >&2; exit 1
    fi
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Create `scripts/stage-skills.sh`:

  ```bash
  #!/usr/bin/env bash
  # stage-skills.sh — materialize a harness's role skills into the
  # staging directory mounted by its scion-agent.yaml.
  #
  # Idempotent. Re-runs are safe; staging is rebuilt from the manifest.
  #
  # Modes:
  #   stage-skills.sh <harness>                  # rebuild
  #   stage-skills.sh <harness> --add <skill>    # mutate manifest + rebuild
  #   stage-skills.sh <harness> --remove <skill> # mutate manifest + rebuild
  #   stage-skills.sh <harness> --diff           # canonical-vs-staged diff
  #
  # Resolution rule (APM-style refs):
  #   "danmestas/agent-skills/skills/<name>"  → ~/projects/agent-skills/skills/<name>
  #   "<name>"                                 → ~/projects/agent-skills/skills/<name>
  # External org refs ("<other-org>/<repo>/skills/<name>") not yet
  # supported; fail loudly with a TODO message.

  set -euo pipefail

  REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  CANONICAL="${HOME}/projects/agent-skills/skills"

  usage() {
    cat <<EOF >&2
  Usage: $0 <harness> [--add <skill> | --remove <skill> | --diff]
  EOF
    exit 2
  }

  if [[ $# -lt 1 ]]; then usage; fi
  HARNESS="$1"; shift

  MODE="rebuild"
  TARGET_SKILL=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --add)    MODE="add";    TARGET_SKILL="$2"; shift 2 ;;
      --remove) MODE="remove"; TARGET_SKILL="$2"; shift 2 ;;
      --diff)   MODE="diff";   shift ;;
      *) usage ;;
    esac
  done

  MANIFEST_DIR="${REPO}/.scion/templates/${HARNESS}"
  if [[ ! -d "${MANIFEST_DIR}" ]]; then
    echo "stage-skills: harness '${HARNESS}' not found at ${MANIFEST_DIR}" >&2
    exit 1
  fi

  STAGE_DIR="${REPO}/.scion/skills-staging/${HARNESS}"

  resolve_ref() {
    local ref="$1"
    case "${ref}" in
      danmestas/agent-skills/skills/*)
        echo "${CANONICAL}/${ref##*/}"
        ;;
      */*/skills/*)
        echo "stage-skills: external skill refs not yet supported: ${ref}" >&2
        return 1
        ;;
      *)
        echo "${CANONICAL}/${ref}"
        ;;
    esac
  }

  read_skills_from_manifest() {
    if ! command -v scion >/dev/null 2>&1; then
      echo "stage-skills: scion CLI required for manifest read" >&2; return 1
    fi
    scion templates show "${HARNESS}" --local --format json \
      | jq -r '.skills[]? // empty'
  }

  do_rebuild() {
    rm -rf "${STAGE_DIR}"
    mkdir -p "${STAGE_DIR}"
    local refs
    refs="$(read_skills_from_manifest || true)"
    if [[ -z "${refs}" ]]; then
      echo "stage-skills: no role skills declared for ${HARNESS}" >&2
      return 0
    fi
    while IFS= read -r ref; do
      [[ -z "${ref}" ]] && continue
      local src dest name
      src="$(resolve_ref "${ref}")"
      name="${ref##*/}"
      dest="${STAGE_DIR}/${name}"
      if [[ ! -d "${src}" ]]; then
        echo "stage-skills: source skill missing at ${src}" >&2
        return 1
      fi
      cp -R "${src}" "${dest}"
      echo "stage-skills: copied ${name} → ${dest}"
    done <<< "${refs}"
  }

  do_diff() {
    local refs
    refs="$(read_skills_from_manifest || true)"
    while IFS= read -r ref; do
      [[ -z "${ref}" ]] && continue
      local name src staged
      name="${ref##*/}"
      src="$(resolve_ref "${ref}")"
      staged="${STAGE_DIR}/${name}"
      if [[ ! -d "${staged}" ]]; then
        echo "drift: ${name} declared but not staged"
        continue
      fi
      if ! diff -qr "${src}" "${staged}" >/dev/null 2>&1; then
        echo "drift: ${name} differs between canonical and staged"
        diff -r "${src}" "${staged}" || true
      else
        echo "in-sync: ${name}"
      fi
    done <<< "${refs}"
  }

  do_mutate_manifest() {
    local op="$1" skill="$2"
    local f="${MANIFEST_DIR}/scion-agent.yaml"
    case "${op}" in
      add)
        if grep -q "  - ${skill}\$" "${f}"; then
          echo "stage-skills: ${skill} already declared"
          return 0
        fi
        if ! grep -q "^skills:" "${f}"; then
          printf '\nskills:\n  - %s\n' "${skill}" >> "${f}"
        else
          awk -v s="  - ${skill}" '
            /^skills:/ { print; print s; in_block=1; next }
            { print }
          ' "${f}" > "${f}.tmp" && mv "${f}.tmp" "${f}"
        fi
        ;;
      remove)
        grep -v "  - ${skill}\$" "${f}" > "${f}.tmp" && mv "${f}.tmp" "${f}"
        ;;
    esac
  }

  case "${MODE}" in
    rebuild) do_rebuild ;;
    diff)    do_diff ;;
    add)     do_mutate_manifest add "${TARGET_SKILL}"; do_rebuild ;;
    remove)  do_mutate_manifest remove "${TARGET_SKILL}"; do_rebuild ;;
  esac
  ```

  Run `chmod +x scripts/stage-skills.sh`.

- [ ] **Run the test and verify it passes**

  Requires `~/projects/agent-skills/skills/{ousterhout,hipp}/` cloned. If absent, test reports the missing source per §9 row 1.

- [ ] **Run lint + types**

  ```bash
  shellcheck scripts/stage-skills.sh scripts/test-stage-skills.sh
  ```

- [ ] **Commit**

  ```
  feat(scripts): add stage-skills.sh with rebuild/add/remove/diff modes
  ```

#### Task C-02: Add .scion/skills-staging and darwin-recommendations to .gitignore

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  GI="$(dirname "$0")/../.gitignore"
  for path in ".scion/skills-staging" ".scion/darwin-recommendations"; do
    if ! grep -qE "^${path}/?$" "${GI}"; then
      echo "FAIL: ${path} not gitignored" >&2; exit 1
    fi
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Append to `.gitignore`:

  ```
  # Per-harness staged skills (rebuilt on each spawn from manifest).
  .scion/skills-staging/

  # Darwin recommendation YAML emitted post-pipeline (operator decides via `darkish apply`).
  .scion/darwin-recommendations/
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  chore: gitignore skills-staging and darwin-recommendations
  ```

#### Task C-03: Add skills: + skills-staging volumes: to all 13 manifests

For each existing manifest plus the 5 new ones being added in Phase D, append the role skills from spec §5.3 plus a volumes: block that mounts the staging dir.

This task only modifies the 8 existing manifests; D-01..D-05 will create the 5 new ones with this shape baked in.

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  cd "$(dirname "$0")/../.scion/templates"
  for h in orchestrator researcher designer tdd-implementer verifier reviewer sme admin; do
    if ! grep -q "^skills:" "${h}/scion-agent.yaml"; then
      echo "FAIL: ${h} missing skills:" >&2; exit 1
    fi
    if ! grep -q "skills-staging/${h}" "${h}/scion-agent.yaml"; then
      echo "FAIL: ${h} missing skills-staging volume" >&2; exit 1
    fi
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  For each of the 8 existing manifests, append:

  ```yaml
  skills:
    - <APM ref 1>
    - <APM ref 2>
  volumes:
    - source: <REPO>/.scion/skills-staging/<harness>/
      target: /home/scion/skills/role/
      read_only: true
  ```

  Where `<REPO>` is `${PWD}` resolved at scion-templates-show time. Per §5.3, use:

  | Harness | Skills |
  |---|---|
  | orchestrator | `danmestas/agent-skills/skills/dx-audit` |
  | researcher | (empty list, omit `skills:` and `volumes:` for skills entirely) |
  | designer | `norman`, `ousterhout` |
  | tdd-implementer | `idiomatic-go`, `tigerstyle` |
  | verifier | `tigerstyle` |
  | reviewer | `ousterhout`, `hipp`, `dx-audit` |
  | sme | `ousterhout`, `hipp` |
  | admin | (empty) |

  Concrete example (sme/scion-agent.yaml after edit):

  ```yaml
  schema_version: "1"
  description: "Subject-matter expert - summoned for one focused software-engineering question; rejects poorly-formed questions"
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: codex
  image: local/darkish-codex:latest
  model: gpt-5.5
  max_turns: 10
  max_duration: "15m"
  detached: false
  skills:
    - danmestas/agent-skills/skills/ousterhout
    - danmestas/agent-skills/skills/hipp
  volumes:
    - source: ./.scion/skills-staging/sme/
      target: /home/scion/skills/role/
      read_only: true
  ```

  Note: scion resolves relative `source:` paths against the workspace root.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  for d in .scion/templates/*/; do
    name="$(basename "${d}")"
    scion templates show "${name}" --local --format json >/dev/null
  done
  ```

- [ ] **Commit**

  ```
  feat(harnesses): add skills: and skills-staging volume to 8 existing manifests
  ```

#### Task C-04: Validate scion templates show parses skills + volumes cleanly

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  for h in orchestrator designer tdd-implementer verifier reviewer sme; do
    out="$(scion templates show "${h}" --local --format json)"
    n_skills="$(echo "${out}" | jq -r '.skills | length')"
    if [[ "${n_skills}" -lt 1 ]]; then
      echo "FAIL: ${h} skills array empty in JSON output" >&2; exit 1
    fi
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  No code change — validates C-03's manifests are well-formed. If scion's JSON serializer drops unknown fields, file an upstream bug; for now, document in `images/README.md` Open Questions.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  test(harnesses): validate skills+volumes round-trip via scion templates show
  ```

#### Task C-05: End-to-end smoke for sme with staged skills

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  bash scripts/stage-skills.sh sme
  scion start sme-skills-smoke --type sme \
    --notify "List the names of the skills you have access to in /home/scion/skills/role/"
  for _ in $(seq 1 60); do
    state="$(scion list | awk -v n=sme-skills-smoke '$1==n {print $2}')"
    [[ "${state}" == "completed" ]] && break
    sleep 2
  done
  out="$(scion look sme-skills-smoke 2>&1)"
  for s in ousterhout hipp; do
    if ! echo "${out}" | grep -qi "${s}"; then
      echo "FAIL: sme did not see ${s}" >&2; exit 1
    fi
  done
  scion stop sme-skills-smoke --yes; scion delete sme-skills-smoke --yes
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

  Pre-C-01..C-04: stage-skills.sh missing or staging dir empty.

- [ ] **Write the minimal implementation**

  No code change — validates the full path.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  test(scripts): smoke sme with staged ousterhout+hipp skills
  ```

#### Task C-06: Update apm.yml with the union of role skills

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  for s in ousterhout hipp tigerstyle idiomatic-go norman dx-audit superpowers spec-kit; do
    if ! grep -q "${s}" "$(dirname "$0")/../apm.yml"; then
      echo "FAIL: ${s} missing from apm.yml" >&2; exit 1
    fi
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

  Current apm.yml has 6 skills; needs `superpowers`, `spec-kit`.

- [ ] **Write the minimal implementation**

  Replace `apm.yml`:

  ```yaml
  name: darkish-factory
  version: 0.1.0
  description: Darkish Factory pipeline - constitution-bound multi-agent software delivery on Scion
  target: claude
  dependencies:
    apm:
      - danmestas/agent-skills/skills/ousterhout
      - danmestas/agent-skills/skills/hipp
      - danmestas/agent-skills/skills/tigerstyle
      - danmestas/agent-skills/skills/idiomatic-go
      - danmestas/agent-skills/skills/norman
      - danmestas/agent-skills/skills/dx-audit
      - danmestas/agent-skills/skills/superpowers
      - danmestas/agent-skills/skills/spec-kit
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  chore(apm): add superpowers + spec-kit to declared dependencies
  ```

#### Task C-07: Add stage-skills.sh integration into spawn.sh

Modify `scripts/spawn.sh` to invoke both stage-creds.sh AND stage-skills.sh `<harness>` before scion start. Add `--no-stage` flag.

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  S="$(dirname "$0")/spawn.sh"
  grep -q "stage-creds.sh" "${S}" || { echo "FAIL: spawn.sh skips stage-creds"; exit 1; }
  grep -q "stage-skills.sh" "${S}" || { echo "FAIL: spawn.sh skips stage-skills"; exit 1; }
  grep -q -- "--no-stage" "${S}" || { echo "FAIL: spawn.sh missing --no-stage flag"; exit 1; }
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `scripts/spawn.sh`:

  ```bash
  #!/usr/bin/env bash
  # spawn.sh — stage credentials + skills, then start a Scion harness.
  set -euo pipefail

  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

  if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <agent-name> [--no-stage] --type <harness> [scion-start-args...]" >&2
    exit 2
  fi

  AGENT_NAME="$1"; shift

  STAGE=true
  HARNESS=""
  ARGS=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --no-stage) STAGE=false; shift ;;
      --type)     HARNESS="$2"; ARGS+=("$1" "$2"); shift 2 ;;
      *)          ARGS+=("$1"); shift ;;
    esac
  done

  if [[ -z "${HARNESS}" ]]; then
    echo "spawn: --type <harness> is required" >&2; exit 2
  fi

  if ${STAGE}; then
    "${ROOT}/scripts/stage-creds.sh" all || true
    "${ROOT}/scripts/stage-skills.sh" "${HARNESS}" || true
  fi

  exec scion start "${AGENT_NAME}" "${ARGS[@]}"
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  shellcheck scripts/spawn.sh
  ```

- [ ] **Commit**

  ```
  feat(scripts): wire stage-skills + --no-stage into spawn.sh
  ```

---

### Phase D — New harness manifests

#### Task D-01: Rename planner/ → planner-t3/ and adopt superpowers framework

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  cd "$(dirname "$0")/../.scion/templates"
  [[ -d planner-t3 ]] || { echo "FAIL: planner-t3/ not present"; exit 1; }
  [[ ! -d planner ]] || { echo "FAIL: legacy planner/ still exists"; exit 1; }
  grep -q "superpowers" planner-t3/system-prompt.md || { echo "FAIL: planner-t3 prompt does not name superpowers"; exit 1; }
  grep -q "danmestas/agent-skills/skills/superpowers" planner-t3/scion-agent.yaml \
    || { echo "FAIL: planner-t3 skills missing superpowers"; exit 1; }
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  ```bash
  git mv .scion/templates/planner .scion/templates/planner-t3
  ```

  Edit `.scion/templates/planner-t3/scion-agent.yaml`:

  ```yaml
  schema_version: "1"
  description: "Planner T3 - superpowers framework: design + detailed plan; escalates taste/ethics/reversibility"
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: claude
  image: local/darkish-claude:latest
  model: claude-opus-4-7
  max_turns: 50
  max_duration: "2h"
  detached: false
  skills:
    - danmestas/agent-skills/skills/hipp
    - danmestas/agent-skills/skills/ousterhout
    - danmestas/agent-skills/skills/superpowers
  volumes:
    - source: ./.scion/skills-staging/planner-t3/
      target: /home/scion/skills/role/
      read_only: true
  ```

  Update `system-prompt.md` to instruct using the `superpowers:writing-plans` skill: produce `docs/spec.md` if missing, then `docs/plan.md` with TDD-strict steps. Add Communication tier section: `caveman standard` toward orchestrator; `caveman ultra` toward sub-agents.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  scion templates show planner-t3 --local --format json >/dev/null
  ```

- [ ] **Commit**

  ```
  refactor(harnesses): rename planner → planner-t3 with superpowers framework
  ```

#### Task D-02: Create .scion/templates/planner-t1/ (think-then-do, sonnet, 15 turns)

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  D="$(dirname "$0")/../.scion/templates/planner-t1"
  for f in scion-agent.yaml agents.md system-prompt.md; do
    [[ -f "${D}/${f}" ]] || { echo "FAIL: ${D}/${f} missing"; exit 1; }
  done
  grep -q "claude-sonnet-4-6" "${D}/scion-agent.yaml" || { echo "FAIL: wrong model"; exit 1; }
  grep -q "max_turns: 15" "${D}/scion-agent.yaml" || { echo "FAIL: wrong max_turns"; exit 1; }
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Create `.scion/templates/planner-t1/scion-agent.yaml`:

  ```yaml
  schema_version: "1"
  description: "Planner T1 - think-then-do; small bug fixes; no plan doc"
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: claude
  image: local/darkish-claude:latest
  model: claude-sonnet-4-6
  max_turns: 15
  max_duration: "30m"
  detached: false
  skills:
    - danmestas/agent-skills/skills/hipp
  volumes:
    - source: ./.scion/skills-staging/planner-t1/
      target: /home/scion/skills/role/
      read_only: true
  ```

  Create `.scion/templates/planner-t1/agents.md` (worker protocol with caveman tier directive).

  Create `.scion/templates/planner-t1/system-prompt.md`:

  ```markdown
  # Planner T1

  You are a Tier-1 planner. Small bugs only. Read the bug report,
  identify the file, propose the change inline. **No plan document.**
  No spec. Skip directly to the change once you understand it.

  ## Communication tier

  - To orchestrator: caveman standard.
  - To any sub-agent (you should rarely spawn one): caveman ultra.
  - Output to operator: never (orchestrator is your only audience).

  ## Skill: hipp

  Available at `/home/scion/skills/role/hipp/`. When tempted to scope
  beyond the bug, hipp is your discipline anchor: minimum-viable-change.
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  scion templates show planner-t1 --local --format json >/dev/null
  ```

- [ ] **Commit**

  ```
  feat(harnesses): add planner-t1 (sonnet, think-then-do, 15 turns)
  ```

#### Task D-03: Create .scion/templates/planner-t2/ (claude-code style, opus, 30 turns)

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  D="$(dirname "$0")/../.scion/templates/planner-t2"
  for f in scion-agent.yaml agents.md system-prompt.md; do
    [[ -f "${D}/${f}" ]] || { echo "FAIL: ${D}/${f} missing"; exit 1; }
  done
  grep -q "claude-opus-4-7" "${D}/scion-agent.yaml" || { echo "FAIL: wrong model"; exit 1; }
  grep -q "max_turns: 30" "${D}/scion-agent.yaml" || { echo "FAIL: wrong max_turns"; exit 1; }
  grep -q 'max_duration: "1h"' "${D}/scion-agent.yaml" || { echo "FAIL: wrong max_duration"; exit 1; }
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  `.scion/templates/planner-t2/scion-agent.yaml`:

  ```yaml
  schema_version: "1"
  description: "Planner T2 - claude-code-style; light plan doc; few clarifying questions"
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: claude
  image: local/darkish-claude:latest
  model: claude-opus-4-7
  max_turns: 30
  max_duration: "1h"
  detached: false
  skills:
    - danmestas/agent-skills/skills/hipp
    - danmestas/agent-skills/skills/ousterhout
  volumes:
    - source: ./.scion/skills-staging/planner-t2/
      target: /home/scion/skills/role/
      read_only: true
  ```

  Create `agents.md` and `system-prompt.md` describing claude-code-style: ask 1–3 clarifying questions, then emit a concise `docs/plan.md` (no separate spec).

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(harnesses): add planner-t2 (opus, claude-code-style, 30 turns)
  ```

#### Task D-04: Create .scion/templates/planner-t4/ (codex spec-kit, 100 turns, 4h)

- [ ] **Write the failing test**

  `tests/manifests/planner-t4.bats`:

  ```bash
  #!/usr/bin/env bats

  setup() {
    cd "${BATS_TEST_DIRNAME}/../../.scion/templates"
  }

  @test "planner-t4 dir exists" {
    [ -d planner-t4 ]
  }

  @test "planner-t4 manifest declares codex backend + gpt-5.5 + spec-kit skill" {
    run scion templates show planner-t4 --local --format json
    [ "$status" -eq 0 ]
    echo "$output" | jq -e '.default_harness_config == "codex"'
    echo "$output" | jq -e '.model == "gpt-5.5"'
    echo "$output" | jq -e '.image == "local/darkish-codex:latest"'
    echo "$output" | jq -e '.max_turns == 100'
    echo "$output" | jq -e '.max_duration == "4h"'
    echo "$output" | jq -e '.skills | any(. == "github/spec-kit")'
  }
  ```

- [ ] **Run the test and verify it fails**

  ```bash
  bats tests/manifests/planner-t4.bats
  ```
  Expected: FAIL — `planner-t4 dir exists` returns "is not a directory".

- [ ] **Write the minimal implementation**

  `.scion/templates/planner-t4/scion-agent.yaml`:

  ```yaml
  schema_version: "1"
  description: "Planner T4 - spec-kit framework; full ratification: constitution + spec.md + plan.md + tasks/"
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: codex
  image: local/darkish-codex:latest
  model: gpt-5.5
  max_turns: 100
  max_duration: "4h"
  detached: false
  skills:
    - danmestas/agent-skills/skills/hipp
    - danmestas/agent-skills/skills/ousterhout
    - danmestas/agent-skills/skills/spec-kit
  volumes:
    - source: ./.scion/skills-staging/planner-t4/
      target: /home/scion/skills/role/
      read_only: true
  ```

  Create `agents.md` (worker protocol). Create `system-prompt.md` documenting spec-kit invocation: `specify init`, `specify constitution`, `specify spec`, `specify plan`, `specify tasks`. Note prelude installs the spec-kit CLI on first spawn (Phase F-01).

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(harnesses): add planner-t4 (codex, spec-kit, 100 turns, 4h)
  ```

#### Task D-05: Create .scion/templates/darwin/ (codex, gpt-5.5, evolution agent)

- [ ] **Write the failing test**

  Checks `.scion/templates/darwin/`, `codex`, `gpt-5.5`, `max_turns: 50`, `max_duration: "4h"`, skills include `dx-audit`.

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  `.scion/templates/darwin/scion-agent.yaml`:

  ```yaml
  schema_version: "1"
  description: "Darwin - reads completed harness sessions; evolves rules and skills; emits operator-gated recommendations"
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: codex
  image: local/darkish-codex:latest
  model: gpt-5.5
  max_turns: 50
  max_duration: "4h"
  detached: false
  skills:
    - danmestas/agent-skills/skills/dx-audit
  volumes:
    - source: ./.scion/skills-staging/darwin/
      target: /home/scion/skills/role/
      read_only: true
  ```

  Create `agents.md` (worker protocol for read-only analysis runs).

  Create `system-prompt.md`:

  ```markdown
  # Darwin

  You are the evolution agent. Your job: read completed harness sessions
  (transcripts + audit log entries) and emit recommendations that improve
  the pipeline over time. You **never** mutate manifests or skills
  directly. The operator ratifies changes via `darkish apply`.

  ## Output

  Write recommendations to
  `<repo>/.scion/darwin-recommendations/<date>-<run-id>.yaml`. Format
  matches spec §12.4. Each recommendation needs: target_harness, type
  (skill_add | skill_remove | skill_upgrade | model_swap | prompt_edit |
  rule_add), rationale, evidence, proposed_change, confidence,
  reversibility.

  ## Communication tier

  - To orchestrator: caveman standard (terse summaries).
  - To sub-agents (rare): caveman ultra.
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(harnesses): add darwin evolution-agent harness (codex, 4h)
  ```

#### Task D-06: Flip verifier and reviewer to codex backend per spec §3.1

The current roster has both on claude; spec §3.1 puts them on codex.

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  for h in verifier reviewer; do
    f=".scion/templates/${h}/scion-agent.yaml"
    grep -q "default_harness_config: codex" "${f}" \
      || { echo "FAIL: ${h} not on codex"; exit 1; }
    grep -q "image: local/darkish-codex:latest" "${f}" \
      || { echo "FAIL: ${h} image wrong"; exit 1; }
    grep -q "model: gpt-5.5" "${f}" \
      || { echo "FAIL: ${h} model wrong"; exit 1; }
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Edit `.scion/templates/verifier/scion-agent.yaml`:

  ```yaml
  schema_version: "1"
  description: "Verifier - adversarial test execution + fuzzing; long-running"
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: codex
  image: local/darkish-codex:latest
  model: gpt-5.5
  max_turns: 50
  max_duration: "2h"
  detached: false
  skills:
    - danmestas/agent-skills/skills/tigerstyle
  volumes:
    - source: ./.scion/skills-staging/verifier/
      target: /home/scion/skills/role/
      read_only: true
  ```

  Edit `.scion/templates/reviewer/scion-agent.yaml`:

  ```yaml
  schema_version: "1"
  description: "Reviewer - senior-engineer block-or-ship; cross-vendor second opinion vs claude implementer"
  agent_instructions: agents.md
  system_prompt: system-prompt.md
  default_harness_config: codex
  image: local/darkish-codex:latest
  model: gpt-5.5
  max_turns: 30
  max_duration: "1h"
  detached: false
  skills:
    - danmestas/agent-skills/skills/ousterhout
    - danmestas/agent-skills/skills/hipp
    - danmestas/agent-skills/skills/dx-audit
  volumes:
    - source: ./.scion/skills-staging/reviewer/
      target: /home/scion/skills/role/
      read_only: true
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  scion templates show verifier --local --format json >/dev/null
  scion templates show reviewer --local --format json >/dev/null
  ```

- [ ] **Commit**

  ```
  refactor(harnesses): flip verifier + reviewer to codex per spec §3.1
  ```

#### Task D-07: Update .design/harness-roster.md to list 13 harnesses

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  R="$(dirname "$0")/../.design/harness-roster.md"
  for h in orchestrator researcher designer planner-t1 planner-t2 planner-t3 planner-t4 \
           tdd-implementer verifier reviewer sme admin darwin; do
    grep -q "\`${h}\`" "${R}" || { echo "FAIL: ${h} not in roster"; exit 1; }
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace the roster table in `.design/harness-roster.md` with a 13-row version. Each row mirrors §3.1 of the spec: Role, Backend, Model, Max turns, Max duration, Detached, Escalation-axis affinity, One-line role.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  docs(roster): list 13 harnesses including planner tiers + darwin
  ```

#### Task D-08: Update .design/pipeline-mechanics.md and orchestrator/agents.md for routing

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  P="$(dirname "$0")/../.design/pipeline-mechanics.md"
  O="$(dirname "$0")/../.scion/templates/orchestrator/agents.md"
  for tok in "planner-t1" "planner-t2" "planner-t3" "planner-t4" "darwin"; do
    grep -q "${tok}" "${P}" || { echo "FAIL: ${tok} missing from pipeline-mechanics"; exit 1; }
    grep -q "${tok}" "${O}" || { echo "FAIL: ${tok} missing from orchestrator/agents.md"; exit 1; }
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Add a new `## 9. Planner tier routing` section to `.design/pipeline-mechanics.md` with the t1..t4 + ambiguous→t3 table from spec §8. Add `## 10. Darwin loop` describing the post-pipeline run + `darkish apply` operator gate.

  Append to `orchestrator/agents.md` a "Routing the planner" subsection naming the four tiers and the override accepts.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  docs: add planner-tier routing + darwin loop to pipeline mechanics
  ```

---

### Phase E — `darkish` Go CLI

#### Task E-01: Bootstrap go module and main.go dispatcher

- [ ] **Write the failing test**

  Create `cmd/darkish/main_test.go`:

  ```go
  package main

  import (
  	"os/exec"
  	"strings"
  	"testing"
  )

  func TestMainHelp(t *testing.T) {
  	out, err := exec.Command("go", "run", ".", "--help").CombinedOutput()
  	if err != nil {
  		t.Fatalf("--help should not error: %v\n%s", err, out)
  	}
  	for _, sub := range []string{"doctor", "spawn", "bootstrap", "apply", "create-harness", "skills", "creds", "images", "list"} {
  		if !strings.Contains(string(out), sub) {
  			t.Fatalf("--help missing subcommand %q\n%s", sub, out)
  		}
  	}
  }

  func TestUnknownSubcommand(t *testing.T) {
  	out, err := exec.Command("go", "run", ".", "no-such-cmd").CombinedOutput()
  	if err == nil {
  		t.Fatalf("expected non-zero exit for unknown subcommand\n%s", out)
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

  ```bash
  cd cmd/darkish && go test ./...
  ```

  Fails — no main.go exists.

- [ ] **Write the minimal implementation**

  Create `go.mod` at repo root:

  ```
  module github.com/danmestas/darkish-factory

  go 1.23
  ```

  Create `cmd/darkish/main.go`:

  ```go
  // Package main is the darkish operator CLI.
  package main

  import (
  	"flag"
  	"fmt"
  	"os"
  )

  type subcommand struct {
  	name string
  	desc string
  	run  func(args []string) error
  }

  var subcommands = []subcommand{
  	{"doctor", "preflight + post-mortem health checks", runDoctor},
  	{"spawn", "stage creds + skills + scion start", runSpawn},
  	{"bootstrap", "first-time machine setup", runBootstrap},
  	{"apply", "review + apply darwin recommendations", runApply},
  	{"create-harness", "scaffold a new harness directory", runCreateHarness},
  	{"skills", "manage staged skills", runSkills},
  	{"creds", "refresh hub secrets", runCreds},
  	{"images", "wrap make -C images", runImages},
  	{"list", "wrap scion list", runList},
  }

  func main() {
  	flag.Usage = printUsage
  	flag.Parse()

  	args := flag.Args()
  	if len(args) == 0 {
  		printUsage()
  		os.Exit(2)
  	}

  	for _, sc := range subcommands {
  		if sc.name == args[0] {
  			if err := sc.run(args[1:]); err != nil {
  				fmt.Fprintln(os.Stderr, "darkish:", err)
  				os.Exit(1)
  			}
  			return
  		}
  	}

  	fmt.Fprintf(os.Stderr, "darkish: unknown subcommand %q\n", args[0])
  	printUsage()
  	os.Exit(2)
  }

  func printUsage() {
  	fmt.Fprintln(os.Stderr, "Usage: darkish <subcommand> [flags] [args]")
  	fmt.Fprintln(os.Stderr)
  	fmt.Fprintln(os.Stderr, "Subcommands:")
  	for _, sc := range subcommands {
  		fmt.Fprintf(os.Stderr, "  %-16s %s\n", sc.name, sc.desc)
  	}
  }
  ```

  Create stubs `cmd/darkish/doctor.go`, `spawn.go`, `bootstrap.go`, `apply.go`, `create_harness.go`, `skills.go`, `creds.go`, `images.go`, `list.go` — each with:

  ```go
  package main

  import "errors"

  func runDoctor(args []string) error { return errors.New("not implemented") }
  ```

  Substitute the function name per file.

  Create the shared test helper at `cmd/darkish/testhelpers_test.go`:

  ```go
  package main

  import "os"

  // captureStdout runs fn with os.Stdout pointed at an in-memory pipe
  // and returns whatever fn wrote. Shared by every *_test.go in the
  // package; defining it once prevents the multiply-defined collision
  // that would otherwise occur between apply_test.go and list_test.go.
  func captureStdout(fn func() error) (string, error) {
  	r, w, _ := os.Pipe()
  	old := os.Stdout
  	os.Stdout = w
  	err := fn()
  	w.Close()
  	os.Stdout = old
  	buf := make([]byte, 4096)
  	n, _ := r.Read(buf)
  	return string(buf[:n]), err
  }
  ```

  Add `Makefile` target:

  ```makefile
  .PHONY: darkish
  darkish:
  	mkdir -p bin
  	go build -trimpath -ldflags="-s -w" -o bin/darkish ./cmd/darkish
  ```

- [ ] **Run the test and verify it passes**

  ```bash
  cd cmd/darkish && go test ./...
  ```

- [ ] **Run lint + types**

  ```bash
  go vet ./...
  gofmt -l cmd/ internal/
  ```

- [ ] **Commit**

  ```
  feat(cli): bootstrap darkish Go module with subcommand dispatcher
  ```

#### Task E-02: Implement `darkish doctor`

Preflight + post-mortem per §12.1. Shells out to docker, scion. Maps known errors to §9 rows.

- [ ] **Write the failing test**

  Create `cmd/darkish/doctor_test.go`:

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestDoctorReportsMissingScion(t *testing.T) {
  	dir := t.TempDir()
  	stub := filepath.Join(dir, "scion")
  	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 127\n"), 0o755); err != nil {
  		t.Fatal(err)
  	}
  	t.Setenv("PATH", dir)

  	report, err := doctorBroad()
  	if err == nil {
  		t.Fatalf("doctor should report failure when scion is broken")
  	}
  	if !strings.Contains(report, "scion") {
  		t.Fatalf("report must mention scion, got %q", report)
  	}
  }

  func TestDoctorHarnessChecksImageSecretAndStaging(t *testing.T) {
  	dir := t.TempDir()
  	t.Setenv("DARKISH_REPO_ROOT", dir)
  	// Build a fake harness manifest tree without a staging dir.
  	hd := filepath.Join(dir, ".scion", "templates", "sme")
  	os.MkdirAll(hd, 0o755)
  	os.WriteFile(filepath.Join(hd, "scion-agent.yaml"),
  		[]byte("default_harness_config: codex\nskills:\n  - danmestas/agent-skills/skills/ousterhout\n"), 0o644)

  	report, err := doctorHarness("sme")
  	if err == nil {
  		t.Fatalf("expected per-harness preflight to fail without staging dir")
  	}
  	if !strings.Contains(report, "skills-staging") {
  		t.Fatalf("report should call out missing staging dir, got %q", report)
  	}
  }

  func TestDoctorHarnessPostMortemMapsAuthError(t *testing.T) {
  	dir := t.TempDir()
  	t.Setenv("DARKISH_REPO_ROOT", dir)
  	// Plant an agent.log with a known broker error pattern.
  	logDir := filepath.Join(dir, ".scion", "agents", "smoke-1")
  	os.MkdirAll(logDir, 0o755)
  	os.WriteFile(filepath.Join(logDir, "agent.log"),
  		[]byte("broker: auth resolution failed: codex\n"), 0o644)

  	report := postMortemFor(filepath.Join(logDir, "agent.log"))
  	if !strings.Contains(report, "stage-creds.sh") {
  		t.Fatalf("post-mortem should map auth error to stage-creds remediation, got %q", report)
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `cmd/darkish/doctor.go`:

  ```go
  package main

  import (
  	"errors"
  	"fmt"
  	"os/exec"
  	"strings"
  )

  type check struct {
  	name string
  	run  func() error
  }

  func runDoctor(args []string) error {
  	report, err := doctorBroad()
  	fmt.Println(report)
  	return err
  }

  func doctorBroad() (string, error) {
  	checks := []check{
  		{"docker daemon reachable", checkDocker},
  		{"scion CLI present", checkScion},
  		{"scion server status", checkScionServer},
  		{"hub secrets present", checkHubSecrets},
  		{"darkish images built", checkImages},
  	}

  	var sb strings.Builder
  	var failed []string
  	for _, c := range checks {
  		if err := c.run(); err != nil {
  			fmt.Fprintf(&sb, "FAIL  %s — %v\n", c.name, err)
  			fmt.Fprintf(&sb, "      remediation: %s\n", remediationFor(c.name, err))
  			failed = append(failed, c.name)
  		} else {
  			fmt.Fprintf(&sb, "OK    %s\n", c.name)
  		}
  	}
  	if len(failed) > 0 {
  		return sb.String(), fmt.Errorf("%d checks failed: %s", len(failed), strings.Join(failed, ", "))
  	}
  	return sb.String(), nil
  }

  func checkDocker() error {
  	out, err := exec.Command("docker", "info").CombinedOutput()
  	if err != nil {
  		return fmt.Errorf("docker info: %s", string(out))
  	}
  	return nil
  }

  func checkScion() error {
  	if _, err := exec.LookPath("scion"); err != nil {
  		return errors.New("scion not on PATH")
  	}
  	return nil
  }

  func checkScionServer() error {
  	out, err := exec.Command("scion", "server", "status").CombinedOutput()
  	if err != nil {
  		return fmt.Errorf("server not running: %s", string(out))
  	}
  	return nil
  }

  func checkHubSecrets() error {
  	out, err := exec.Command("scion", "hub", "secret", "list").CombinedOutput()
  	if err != nil {
  		return fmt.Errorf("hub secret list: %s", string(out))
  	}
  	for _, want := range []string{"claude_auth", "codex_auth"} {
  		if !strings.Contains(string(out), want) {
  			return fmt.Errorf("missing hub secret: %s", want)
  		}
  	}
  	return nil
  }

  func checkImages() error {
  	out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
  	if err != nil {
  		return err
  	}
  	for _, want := range []string{
  		"local/darkish-claude:latest",
  		"local/darkish-codex:latest",
  		"local/darkish-pi:latest",
  		"local/darkish-gemini:latest",
  	} {
  		if !strings.Contains(string(out), want) {
  			return fmt.Errorf("missing image: %s", want)
  		}
  	}
  	return nil
  }

  // remediationFor maps known error patterns to the §9 failure-mode row.
  func remediationFor(check string, err error) string {
  	msg := err.Error()
  	switch {
  	case strings.Contains(msg, "docker info"):
  		return "start Docker Desktop / podman / colima"
  	case strings.Contains(msg, "scion not on PATH"):
  		return "make install in ~/projects/scion"
  	case strings.Contains(msg, "server not running"):
  		return "scion server start"
  	case strings.Contains(msg, "missing hub secret"):
  		return "scripts/stage-creds.sh all"
  	case strings.Contains(msg, "missing image"):
  		return "make -C images all"
  	case strings.Contains(msg, "skills-staging"):
  		return "Run `darkish skills <harness>`"
  	case strings.Contains(msg, "is a directory") || strings.Contains(msg, "directory symlink"):
  		return "Switch to copy-staging via `darkish skills <harness>` (never use directory symlinks)"
  	case strings.Contains(msg, "caveman tier mismatch"):
  		return "Update <harness>/system-prompt.md Communication section; flag to darwin for systematic check"
  	default:
  		return "see spec §9 failure modes"
  	}
  }

  // doctorHarness is the preflight + post-mortem entry point for a
  // specific harness. Verifies image, hub secret, and skills-staging
  // alignment per §12.1. On failure, callers should also pass the
  // agent.log path through postMortemFor for a §9-row mapping.
  func doctorHarness(name string) (string, error) {
  	root, err := repoRoot()
  	if err != nil { return "", err }

  	manifestPath := filepath.Join(root, ".scion", "templates", name, "scion-agent.yaml")
  	body, err := os.ReadFile(manifestPath)
  	if err != nil {
  		return "", fmt.Errorf("manifest read: %w", err)
  	}
  	backend := scanField(string(body), "default_harness_config:")
  	skills := scanList(string(body), "skills:")

  	var sb strings.Builder
  	var failed []string

  	imgTag := fmt.Sprintf("local/darkish-%s:latest", backend)
  	if !imageExists(imgTag) {
  		fmt.Fprintf(&sb, "FAIL  image %s missing — remediation: %s\n",
  			imgTag, remediationFor("image", fmt.Errorf("missing image: %s", imgTag)))
  		failed = append(failed, "image")
  	} else {
  		fmt.Fprintf(&sb, "OK    image %s present\n", imgTag)
  	}

  	wantSecret := map[string]string{
  		"claude": "claude_auth", "codex": "codex_auth",
  		"pi": "OPENROUTER_API_KEY", "gemini": "gemini_auth",
  	}[backend]
  	out, _ := exec.Command("scion", "hub", "secret", "list").CombinedOutput()
  	if !strings.Contains(string(out), wantSecret) {
  		fmt.Fprintf(&sb, "FAIL  hub secret %s missing — remediation: %s\n",
  			wantSecret, remediationFor("secret", fmt.Errorf("missing hub secret: %s", wantSecret)))
  		failed = append(failed, "secret")
  	} else {
  		fmt.Fprintf(&sb, "OK    hub secret %s present\n", wantSecret)
  	}

  	stageDir := filepath.Join(root, ".scion", "skills-staging", name)
  	if _, err := os.Stat(stageDir); err != nil {
  		fmt.Fprintf(&sb, "FAIL  skills-staging dir missing at %s — remediation: %s\n",
  			stageDir, remediationFor("staging", fmt.Errorf("skills-staging missing: %s", stageDir)))
  		failed = append(failed, "staging")
  	} else {
  		for _, ref := range skills {
  			name := ref[strings.LastIndex(ref, "/")+1:]
  			if _, err := os.Stat(filepath.Join(stageDir, name)); err != nil {
  				fmt.Fprintf(&sb, "FAIL  manifest declares %q but skills-staging is missing it\n", name)
  				failed = append(failed, "staging-mismatch")
  			}
  		}
  		if len(failed) == 0 {
  			fmt.Fprintf(&sb, "OK    skills-staging matches manifest\n")
  		}
  	}

  	if len(failed) > 0 {
  		return sb.String(), fmt.Errorf("%d harness checks failed: %s", len(failed), strings.Join(failed, ", "))
  	}
  	return sb.String(), nil
  }

  // postMortemFor parses recent broker output / agent log lines and maps
  // known error patterns to the §9 remediation row.
  func postMortemFor(logPath string) string {
  	body, err := os.ReadFile(logPath)
  	if err != nil { return fmt.Sprintf("post-mortem: cannot read %s: %v", logPath, err) }
  	var sb strings.Builder
  	patterns := []struct{ needle, reason, fix string }{
  		{"auth resolution failed:", "missing hub secret", "Run `scripts/stage-creds.sh <backend>` then re-spawn"},
  		{"pull access denied", "image not built locally", "Run `make -C images <backend>`"},
  		{"is a directory", "skills symlink-to-directory regression", "Use `darkish skills <harness>` (copy-staging)"},
  		{"no such image", "darkish image missing", "Run `make -C images all`"},
  	}
  	for _, p := range patterns {
  		if strings.Contains(string(body), p.needle) {
  			fmt.Fprintf(&sb, "MATCH %q — %s. Remediation: %s\n", p.needle, p.reason, p.fix)
  		}
  	}
  	if sb.Len() == 0 {
  		fmt.Fprintf(&sb, "post-mortem: no known patterns in %s\n", logPath)
  	}
  	return sb.String()
  }

  // scanField reads a single YAML scalar field's value. Hand-rolled to
  // avoid dragging in a YAML dep per constitution §I.
  func scanField(body, prefix string) string {
  	for _, line := range strings.Split(body, "\n") {
  		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
  			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix))
  		}
  	}
  	return ""
  }

  // scanList reads a YAML block sequence under a given header.
  func scanList(body, header string) []string {
  	var out []string
  	in := false
  	for _, line := range strings.Split(body, "\n") {
  		if strings.HasPrefix(strings.TrimSpace(line), header) { in = true; continue }
  		if !in { continue }
  		t := strings.TrimSpace(line)
  		if strings.HasPrefix(t, "- ") {
  			out = append(out, strings.TrimSpace(strings.TrimPrefix(t, "- ")))
  			continue
  		}
  		if t == "" || !strings.HasPrefix(line, "  ") { break }
  	}
  	return out
  }
  ```

  Note: `runDoctor` is updated to dispatch — when called as `darkish doctor <harness>`, it invokes `doctorHarness(name)` and, if a fail is reported, also runs `postMortemFor` on the most recent `.scion/agents/<name>/agent.log`.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  go vet ./cmd/darkish/...
  ```

- [ ] **Commit**

  ```
  feat(cli): implement darkish doctor (preflight + post-mortem)
  ```

#### Task E-03: Implement `darkish spawn`

Wraps stage-creds.sh + stage-skills.sh + `scion start`. Adds `--backend` for override and `--no-stage`.

- [ ] **Write the failing test**

  Create `cmd/darkish/spawn_test.go`:

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestSpawnInvokesStageThenScion(t *testing.T) {
  	dir := t.TempDir()
  	log := filepath.Join(dir, "calls.log")
  	for _, b := range []string{"scion", "bash"} {
  		stub := filepath.Join(dir, b)
  		if err := os.WriteFile(stub, []byte(
  			"#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755); err != nil {
  			t.Fatal(err)
  		}
  	}
  	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

  	if err := runSpawn([]string{"smoke-1", "--type", "researcher", "task..."}); err != nil {
  		t.Fatalf("spawn: %v", err)
  	}
  	body, _ := os.ReadFile(log)
  	if !strings.Contains(string(body), "stage-creds.sh") {
  		t.Fatalf("stage-creds.sh not invoked: %s", body)
  	}
  	if !strings.Contains(string(body), "stage-skills.sh") {
  		t.Fatalf("stage-skills.sh not invoked: %s", body)
  	}
  	if !strings.Contains(string(body), "start smoke-1") {
  		t.Fatalf("scion start not invoked: %s", body)
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `cmd/darkish/spawn.go`:

  ```go
  package main

  import (
  	"errors"
  	"flag"
  	"fmt"
  	"os"
  	"os/exec"
  	"path/filepath"
  )

  func runSpawn(args []string) error {
  	fs := flag.NewFlagSet("spawn", flag.ExitOnError)
  	harnessType := fs.String("type", "", "harness role (e.g. researcher)")
  	backend := fs.String("backend", "", "backend override (claude|codex|pi|gemini)")
  	noStage := fs.Bool("no-stage", false, "skip stage-creds and stage-skills")
  	if err := fs.Parse(args); err != nil {
  		return err
  	}

  	posArgs := fs.Args()
  	if len(posArgs) < 1 {
  		return errors.New("usage: darkish spawn <name> --type <role> [...]")
  	}
  	if *harnessType == "" {
  		return errors.New("--type is required")
  	}
  	name := posArgs[0]

  	repoRoot, err := findRepoRoot()
  	if err != nil {
  		return err
  	}

  	if !*noStage {
  		if err := runShell(filepath.Join(repoRoot, "scripts", "stage-creds.sh"), "all"); err != nil {
  			fmt.Fprintln(os.Stderr, "spawn: stage-creds non-fatal:", err)
  		}
  		if err := runShell(filepath.Join(repoRoot, "scripts", "stage-skills.sh"), *harnessType); err != nil {
  			return fmt.Errorf("stage-skills failed: %w", err)
  		}
  	}

  	cmd := []string{"start", name, "--type", *harnessType}
  	if *backend != "" {
  		image := fmt.Sprintf("local/darkish-%s:latest", *backend)
  		cmd = append(cmd, "--harness", *backend, "--image", image)
  	}
  	if len(posArgs) > 1 {
  		cmd = append(cmd, "--notify", posArgs[1])
  	}

  	c := exec.Command("scion", cmd...)
  	c.Stdout = os.Stdout
  	c.Stderr = os.Stderr
  	return c.Run()
  }

  func runShell(script string, args ...string) error {
  	c := exec.Command("bash", append([]string{script}, args...)...)
  	c.Stdout = os.Stdout
  	c.Stderr = os.Stderr
  	return c.Run()
  }

  func findRepoRoot() (string, error) {
  	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
  	if err != nil {
  		return "", fmt.Errorf("not in a git repo: %w", err)
  	}
  	return string(out[:len(out)-1]), nil
  }
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(cli): implement darkish spawn with stage + scion start
  ```

#### Task E-04: Implement `darkish bootstrap`

Idempotent first-time setup per §12.3.

- [ ] **Write the failing test**

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestBootstrapStepsAreOrdered(t *testing.T) {
  	dir := t.TempDir()
  	log := filepath.Join(dir, "log")
  	for _, b := range []string{"scion", "docker", "make", "bash"} {
  		os.WriteFile(filepath.Join(dir, b),
  			[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
  	}
  	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
  	_ = runBootstrap([]string{})

  	body, _ := os.ReadFile(log)
  	want := []string{"server", "make", "stage-creds.sh", "stage-skills.sh"}
  	pos := -1
  	for _, w := range want {
  		i := strings.Index(string(body), w)
  		if i < pos || i == -1 {
  			t.Fatalf("step %q out of order or missing in: %s", w, body)
  		}
  		pos = i
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `cmd/darkish/bootstrap.go`:

  ```go
  package main

  import (
  	"errors"
  	"fmt"
  	"os"
  	"os/exec"
  	"path/filepath"
  )

  func runBootstrap(args []string) error {
  	steps := []struct {
  		name string
  		fn   func() error
  	}{
  		{"docker daemon reachable", checkDocker},
  		{"scion CLI present", checkScion},
  		{"scion server running", ensureScionServer},
  		{"darkish images built", ensureImages},
  		{"hub secrets pushed", ensureHubSecrets},
  		{"per-harness skills staged", ensureAllSkillsStaged},
  		{"final doctor", finalDoctor},
  	}
  	for i, s := range steps {
  		fmt.Printf("[%d/%d] %s ...\n", i+1, len(steps), s.name)
  		if err := s.fn(); err != nil {
  			return fmt.Errorf("step %q failed: %w", s.name, err)
  		}
  	}
  	fmt.Println("bootstrap: OK")
  	return nil
  }

  func ensureScionServer() error {
  	if err := exec.Command("scion", "server", "status").Run(); err == nil {
  		return nil
  	}
  	return exec.Command("scion", "server", "start").Run()
  }

  func ensureImages() error {
  	for _, b := range []string{"claude", "codex", "pi", "gemini"} {
  		if imageExists("local/darkish-" + b + ":latest") {
  			continue
  		}
  		c := exec.Command("make", "-C", "images", b)
  		c.Stdout = os.Stdout; c.Stderr = os.Stderr
  		if err := c.Run(); err != nil {
  			return fmt.Errorf("make %s: %w", b, err)
  		}
  	}
  	return nil
  }

  func imageExists(tag string) bool {
  	out, err := exec.Command("docker", "images", "-q", tag).Output()
  	return err == nil && len(out) > 0
  }

  func ensureHubSecrets() error {
  	root, err := findRepoRoot()
  	if err != nil { return err }
  	return runShell(filepath.Join(root, "scripts", "stage-creds.sh"), "all")
  }

  func ensureAllSkillsStaged() error {
  	root, err := findRepoRoot()
  	if err != nil { return err }
  	dirs, err := os.ReadDir(filepath.Join(root, ".scion", "templates"))
  	if err != nil { return err }
  	for _, d := range dirs {
  		if !d.IsDir() || d.Name() == "base" { continue }
  		if err := runShell(filepath.Join(root, "scripts", "stage-skills.sh"), d.Name()); err != nil {
  			fmt.Fprintf(os.Stderr, "bootstrap: stage-skills %s failed: %v\n", d.Name(), err)
  		}
  	}
  	return nil
  }

  func finalDoctor() error {
  	report, err := doctorBroad()
  	fmt.Println(report)
  	if err != nil { return errors.New("post-bootstrap doctor reported failures") }
  	return nil
  }
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(cli): implement darkish bootstrap with idempotent steps
  ```

#### Task E-05: Implement `darkish apply`

Reads darwin recommendation YAML (parsed via `scion templates show`-style JSON projection if available; otherwise hand-rolled stdlib parse — recommendations are simple key:value YAML, no anchors). Prompts y/n/skip/edit per recommendation; mutates manifests on approval.

- [ ] **Write the failing test**

  ```go
  package main

  import (
  	"os"
  	"strings"
  	"testing"
  )

  func TestApplyDryRun(t *testing.T) {
  	f, _ := os.CreateTemp("", "rec-*.yaml")
  	defer os.Remove(f.Name())
  	f.WriteString(`session: test
  recommendations:
    - id: rec-001
      target_harness: tdd-implementer
      type: skill_add
      rationale: "evidence shows X"
      proposed_change:
        skill: "danmestas/agent-skills/skills/idiomatic-go"
      confidence: 0.9
      reversibility: trivial
  `)
  	f.Close()

  	out, err := captureStdout(func() error {
  		return runApply([]string{"--dry-run", f.Name()})
  	})
  	if err != nil { t.Fatal(err) }
  	if !strings.Contains(out, "rec-001") {
  		t.Fatalf("dry-run did not show rec-001: %s", out)
  	}
  	if !strings.Contains(out, "skill_add") {
  		t.Fatalf("dry-run did not show type: %s", out)
  	}
  }
  ```

  Note: `captureStdout` lives in `cmd/darkish/testhelpers_test.go` (created in E-01) so each `*_test.go` shares the helper without collision.

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `cmd/darkish/apply.go`. Use a tiny stdlib YAML parser since recommendations are simple key:value (no anchors, no inline arrays beyond basic scalars). For the parse, lean on `scion templates show <harness> --local --format json` for manifest reads, but recommendation YAML is operator-authored — hand-roll the minimal parser:

  ```go
  package main

  import (
  	"bufio"
  	"errors"
  	"flag"
  	"fmt"
  	"os"
  	"os/exec"
  	"path/filepath"
  	"strings"
  )

  type recommendation struct {
  	ID            string
  	TargetHarness string
  	Type          string
  	Rationale     string
  	Skill         string
  	From          string
  	To            string
  	Confidence    string
  	Reversibility string
  }

  func runApply(args []string) error {
  	fs := flag.NewFlagSet("apply", flag.ExitOnError)
  	dryRun := fs.Bool("dry-run", false, "print what would change")
  	if err := fs.Parse(args); err != nil { return err }
  	pos := fs.Args()
  	if len(pos) != 1 {
  		return errors.New("usage: darkish apply [--dry-run] <recommendation-file>")
  	}
  	recs, err := parseRecommendations(pos[0])
  	if err != nil { return err }
  	for _, r := range recs {
  		fmt.Printf("[%s] target=%s type=%s rationale=%s\n", r.ID, r.TargetHarness, r.Type, r.Rationale)
  		if *dryRun { continue }
  		fmt.Print("Apply? [y/n/skip/edit] ")
  		choice := readChoice()
  		switch choice {
  		case "y": if err := applyRec(r); err != nil { return err }
  		case "edit": fmt.Println("(edit mode unimplemented; mark as skip)")
  		default: fmt.Println("skipped")
  		}
  	}
  	return nil
  }

  func readChoice() string {
  	s := bufio.NewScanner(os.Stdin)
  	if s.Scan() { return strings.TrimSpace(s.Text()) }
  	return ""
  }

  // parseRecommendations parses the simple YAML format from spec §12.4.
  // Hand-rolled: recommendations file is small, structured, no anchors.
  func parseRecommendations(path string) ([]recommendation, error) {
  	body, err := os.ReadFile(path)
  	if err != nil { return nil, err }
  	var recs []recommendation
  	var cur *recommendation
  	for _, line := range strings.Split(string(body), "\n") {
  		t := strings.TrimSpace(line)
  		switch {
  		case strings.HasPrefix(t, "- id:"):
  			if cur != nil { recs = append(recs, *cur) }
  			cur = &recommendation{ID: trimVal(t, "- id:")}
  		case cur == nil:
  			continue
  		case strings.HasPrefix(t, "target_harness:"):
  			cur.TargetHarness = trimVal(t, "target_harness:")
  		case strings.HasPrefix(t, "type:"):
  			cur.Type = trimVal(t, "type:")
  		case strings.HasPrefix(t, "rationale:"):
  			cur.Rationale = trimVal(t, "rationale:")
  		case strings.HasPrefix(t, "skill:"):
  			cur.Skill = trimVal(t, "skill:")
  		case strings.HasPrefix(t, "from:"):
  			cur.From = trimVal(t, "from:")
  		case strings.HasPrefix(t, "to:"):
  			cur.To = trimVal(t, "to:")
  		case strings.HasPrefix(t, "confidence:"):
  			cur.Confidence = trimVal(t, "confidence:")
  		case strings.HasPrefix(t, "reversibility:"):
  			cur.Reversibility = trimVal(t, "reversibility:")
  		}
  	}
  	if cur != nil { recs = append(recs, *cur) }
  	return recs, nil
  }

  func trimVal(line, prefix string) string {
  	v := strings.TrimSpace(strings.TrimPrefix(line, prefix))
  	v = strings.Trim(v, `"'`)
  	return v
  }

  // Slice 1 integration: when the escalation-classifier library lands,
  // route each recommendation through it; trivial-reversibility
  // recommendations may auto-ratify per its policy. Until then, all
  // recommendations require explicit operator y/n. classifierRatifies
  // is the placeholder hook — it always returns false (forces manual
  // approval) and is replaced when Slice 1 lands.
  func classifierRatifies(rec recommendation) bool { return false }

  func applyRec(r recommendation) error {
  	root, err := findRepoRoot()
  	if err != nil { return err }

  	// rule_add carries write authority over the constitution and is
  	// never auto-ratified — even by classifierRatifies.
  	if r.Type == "rule_add" && !classifierRatifies(r) {
  		fmt.Println("rule_add requires explicit operator approval. Continue? [y/N]")
  		if c := readChoice(); c != "y" {
  			return errors.New("rule_add declined")
  		}
  	}

  	switch r.Type {
  	case "skill_add":
  		c := exec.Command("bash",
  			filepath.Join(root, "scripts", "stage-skills.sh"),
  			r.TargetHarness, "--add", r.Skill)
  		c.Stdout = os.Stdout; c.Stderr = os.Stderr
  		if err := c.Run(); err != nil { return err }
  	case "skill_remove":
  		c := exec.Command("bash",
  			filepath.Join(root, "scripts", "stage-skills.sh"),
  			r.TargetHarness, "--remove", r.Skill)
  		c.Stdout = os.Stdout; c.Stderr = os.Stderr
  		if err := c.Run(); err != nil { return err }
  	case "skill_upgrade":
  		// Re-runs stage-skills.sh <harness>; picks up canonical-source
  		// changes. The skill version applied is whatever's in the
  		// canonical agent-skills repo at apply time — the recommendation
  		// does not pin a version.
  		c := exec.Command("bash",
  			filepath.Join(root, "scripts", "stage-skills.sh"),
  			r.TargetHarness)
  		c.Stdout = os.Stdout; c.Stderr = os.Stderr
  		if err := c.Run(); err != nil { return err }
  	case "model_swap":
  		if err := swapModel(root, r.TargetHarness, r.From, r.To); err != nil { return err }
  	case "prompt_edit":
  		if err := editPrompt(root, r.TargetHarness, r.From, r.To); err != nil { return err }
  	case "rule_add":
  		// Append-only — never overwrites; constitution edits are deliberate.
  		path := filepath.Join(root, ".specify", "memory", "constitution.md")
  		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
  		if err != nil { return err }
  		defer f.Close()
  		if _, err := f.WriteString("\n" + r.Rationale + "\n"); err != nil { return err }
  	default:
  		return fmt.Errorf("unsupported rec type %q", r.Type)
  	}

  	// Commit + audit-log every applied change.
  	msg := fmt.Sprintf("auto-apply(darwin): %s %s %s", r.ID, r.Type, r.TargetHarness)
  	if err := exec.Command("git", "-C", root, "add", "-A").Run(); err != nil { return err }
  	if err := exec.Command("git", "-C", root, "commit", "-m", msg).Run(); err != nil { return err }
  	return appendAudit(root, r)
  }

  func swapModel(root, harness, from, to string) error {
  	manifest := filepath.Join(root, ".scion", "templates", harness, "scion-agent.yaml")
  	body, err := os.ReadFile(manifest)
  	if err != nil { return err }
  	out := strings.Replace(string(body),
  		"model: "+from, "model: "+to, 1)
  	return os.WriteFile(manifest, []byte(out), 0o644)
  }

  // editPrompt does an exact-string before→after replace on the harness's
  // system-prompt.md file. Single-occurrence — fails if the before string
  // is absent or non-unique.
  func editPrompt(root, harness, before, after string) error {
  	path := filepath.Join(root, ".scion", "templates", harness, "system-prompt.md")
  	body, err := os.ReadFile(path)
  	if err != nil { return err }
  	if !strings.Contains(string(body), before) {
  		return fmt.Errorf("prompt_edit: before-string not found in %s", path)
  	}
  	if strings.Count(string(body), before) > 1 {
  		return fmt.Errorf("prompt_edit: before-string non-unique in %s", path)
  	}
  	out := strings.Replace(string(body), before, after, 1)
  	return os.WriteFile(path, []byte(out), 0o644)
  }

  // appendAudit writes a JSONL row to .scion/audit.jsonl for each
  // applied recommendation. Stdlib encoding/json only.
  func appendAudit(root string, r recommendation) error {
  	type entry struct {
  		Timestamp        string `json:"timestamp"`
  		RecommendationID string `json:"recommendation_id"`
  		TargetHarness    string `json:"target_harness"`
  		Type             string `json:"type"`
  		Decision         string `json:"decision"`
  		Operator         string `json:"operator"`
  	}
  	e := entry{
  		Timestamp:        time.Now().UTC().Format(time.RFC3339),
  		RecommendationID: r.ID,
  		TargetHarness:    r.TargetHarness,
  		Type:             r.Type,
  		Decision:         "applied",
  		Operator:         os.Getenv("USER"),
  	}
  	b, err := json.Marshal(e); if err != nil { return err }
  	path := filepath.Join(root, ".scion", "audit.jsonl")
  	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
  	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
  	if err != nil { return err }
  	defer f.Close()
  	_, err = f.Write(append(b, '\n'))
  	return err
  }
  ```

  Update the `apply.go` import block to add `"encoding/json"` and `"time"`.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(cli): implement darkish apply for darwin recommendations

  Supports skill_add, skill_remove, model_swap. --dry-run prints
  recommendations without applying. Hand-rolled YAML parser stays
  inside stdlib per constitution §I.
  ```

#### Task E-06: Implement `darkish create-harness`

Scaffolds 3 files + roster entry per §12.5.

- [ ] **Write the failing test**

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestCreateHarnessProducesFiles(t *testing.T) {
  	tmp := t.TempDir()
  	t.Setenv("DARKISH_REPO_ROOT", tmp)
  	os.MkdirAll(filepath.Join(tmp, ".scion", "templates"), 0o755)
  	os.MkdirAll(filepath.Join(tmp, ".design"), 0o755)
  	os.WriteFile(filepath.Join(tmp, ".design", "harness-roster.md"),
  		[]byte("# Harness Roster\n\n## Roster\n\n| Role | Model |\n|---|---|\n"), 0o644)

  	err := runCreateHarness([]string{
  		"newrole",
  		"--backend", "claude",
  		"--model", "claude-sonnet-4-6",
  		"--skills", "danmestas/agent-skills/skills/hipp",
  		"--description", "Test role",
  	})
  	if err != nil { t.Fatal(err) }

  	for _, f := range []string{
  		"scion-agent.yaml", "agents.md", "system-prompt.md",
  	} {
  		path := filepath.Join(tmp, ".scion", "templates", "newrole", f)
  		if _, err := os.Stat(path); err != nil {
  			t.Fatalf("%s not created: %v", path, err)
  		}
  	}
  	roster, _ := os.ReadFile(filepath.Join(tmp, ".design", "harness-roster.md"))
  	if !strings.Contains(string(roster), "newrole") {
  		t.Fatalf("roster missing newrole entry")
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `cmd/darkish/create_harness.go`:

  ```go
  package main

  import (
  	"errors"
  	"flag"
  	"fmt"
  	"os"
  	"path/filepath"
  	"strings"
  )

  func runCreateHarness(args []string) error {
  	fs := flag.NewFlagSet("create-harness", flag.ExitOnError)
  	backend := fs.String("backend", "claude", "claude|codex|pi|gemini")
  	model := fs.String("model", "", "model id (e.g. claude-sonnet-4-6)")
  	skills := fs.String("skills", "", "comma-separated APM-style skill refs")
  	desc := fs.String("description", "", "one-sentence description")
  	maxTurns := fs.Int("max-turns", 50, "scion max_turns for the manifest")
  	axes := fs.String("axes", "(none)", "escalation-axis affinity (e.g. taste,reversibility)")
  	if err := fs.Parse(args); err != nil { return err }
  	pos := fs.Args()
  	if len(pos) != 1 {
  		return errors.New("usage: darkish create-harness <role> --backend X --model Y --skills A,B --description '...' [--max-turns N --axes 'taste,reversibility']")
  	}
  	role := pos[0]
  	if *model == "" || *desc == "" {
  		return errors.New("--model and --description are required")
  	}
  	_ = maxTurns; _ = axes // surface to buildManifest + roster row below.

  	root, err := repoRoot()
  	if err != nil { return err }
  	dir := filepath.Join(root, ".scion", "templates", role)
  	if err := os.MkdirAll(dir, 0o755); err != nil { return err }

  	skillList := splitNonEmpty(*skills, ",")
  	manifest := buildManifest(role, *backend, *model, *desc, skillList, *maxTurns)
  	if err := os.WriteFile(filepath.Join(dir, "scion-agent.yaml"), []byte(manifest), 0o644); err != nil {
  		return err
  	}
  	if err := os.WriteFile(filepath.Join(dir, "agents.md"), []byte(agentsTemplate(role)), 0o644); err != nil {
  		return err
  	}
  	if err := os.WriteFile(filepath.Join(dir, "system-prompt.md"), []byte(promptTemplate(role, *desc)), 0o644); err != nil {
  		return err
  	}

  	rosterPath := filepath.Join(root, ".design", "harness-roster.md")
  	body, err := os.ReadFile(rosterPath)
  	if err != nil { return err }
  	row := fmt.Sprintf("| `%s` | %s | %d | %s | false | %s | %s |\n",
  		role, *model, *maxTurns, "1h", *axes, *desc)
  	// The real harness-roster.md has a blank line after the heading, so
  	// the anchor is "## Roster\n\n" (matches both fixtures and live doc).
  	out := strings.Replace(string(body), "## Roster\n\n",
  		"## Roster\n\n"+row, 1)
  	return os.WriteFile(rosterPath, []byte(out), 0o644)
  }

  func repoRoot() (string, error) {
  	if v := os.Getenv("DARKISH_REPO_ROOT"); v != "" { return v, nil }
  	return findRepoRoot()
  }

  func splitNonEmpty(s, sep string) []string {
  	var out []string
  	for _, p := range strings.Split(s, sep) {
  		p = strings.TrimSpace(p)
  		if p != "" { out = append(out, p) }
  	}
  	return out
  }

  func buildManifest(role, backend, model, desc string, skills []string, maxTurns int) string {
  	var sb strings.Builder
  	fmt.Fprintf(&sb, "schema_version: \"1\"\n")
  	fmt.Fprintf(&sb, "description: %q\n", desc)
  	fmt.Fprintf(&sb, "agent_instructions: agents.md\n")
  	fmt.Fprintf(&sb, "system_prompt: system-prompt.md\n")
  	fmt.Fprintf(&sb, "default_harness_config: %s\n", backend)
  	fmt.Fprintf(&sb, "image: local/darkish-%s:latest\n", backend)
  	fmt.Fprintf(&sb, "model: %s\n", model)
  	fmt.Fprintf(&sb, "max_turns: %d\n", maxTurns)
  	fmt.Fprintf(&sb, "max_duration: \"1h\"\n")
  	fmt.Fprintf(&sb, "detached: false\n")
  	if len(skills) > 0 {
  		fmt.Fprintln(&sb, "skills:")
  		for _, s := range skills {
  			fmt.Fprintf(&sb, "  - %s\n", s)
  		}
  		fmt.Fprintln(&sb, "volumes:")
  		fmt.Fprintf(&sb, "  - source: ./.scion/skills-staging/%s/\n", role)
  		fmt.Fprintf(&sb, "    target: /home/scion/skills/role/\n")
  		fmt.Fprintf(&sb, "    read_only: true\n")
  	}
  	return sb.String()
  }

  func agentsTemplate(role string) string {
  	return fmt.Sprintf(`# %s — agent instructions

  Worker protocol. See README §5.1 for the role definition.

  ## Communication tier

  - To orchestrator: caveman standard.
  - To sub-agents (if any): caveman ultra.
  `, role)
  }

  func promptTemplate(role, desc string) string {
  	return fmt.Sprintf(`# %s

  %s

  Fill in this prompt with role-specific identity, constraints, and
  output format expectations. Operator must complete this stub.
  `, role, desc)
  }
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(cli): implement darkish create-harness scaffolder
  ```

#### Task E-07: Implement `darkish skills`

Wraps stage-skills.sh per §12.6.

- [ ] **Write the failing test**

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestSkillsForwardsToScript(t *testing.T) {
  	dir := t.TempDir()
  	log := filepath.Join(dir, "log")
  	os.WriteFile(filepath.Join(dir, "bash"),
  		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
  	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
  	_ = runSkills([]string{"sme", "--diff"})
  	body, _ := os.ReadFile(log)
  	if !strings.Contains(string(body), "stage-skills.sh") {
  		t.Fatalf("skills did not invoke stage-skills.sh: %s", body)
  	}
  	if !strings.Contains(string(body), "--diff") {
  		t.Fatalf("--diff not forwarded: %s", body)
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  ```go
  package main

  import (
  	"errors"
  	"path/filepath"
  )

  func runSkills(args []string) error {
  	if len(args) < 1 {
  		return errors.New("usage: darkish skills <harness> [--diff|--add SKILL|--remove SKILL]")
  	}
  	root, err := repoRoot()
  	if err != nil { return err }
  	script := filepath.Join(root, "scripts", "stage-skills.sh")
  	return runShell(script, args...)
  }
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(cli): implement darkish skills (wraps stage-skills.sh)
  ```

#### Task E-08: Implement `darkish creds`

Wraps stage-creds.sh.

- [ ] **Write the failing test**

  Create `cmd/darkish/creds_test.go`:

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestCredsForwardsToScript(t *testing.T) {
  	dir := t.TempDir()
  	log := filepath.Join(dir, "log")
  	os.WriteFile(filepath.Join(dir, "bash"),
  		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
  	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
  	_ = runCreds([]string{"claude"})
  	body, _ := os.ReadFile(log)
  	if !strings.Contains(string(body), "stage-creds.sh") {
  		t.Fatalf("creds did not invoke stage-creds.sh: %s", body)
  	}
  	if !strings.Contains(string(body), "claude") {
  		t.Fatalf("backend arg `claude` not forwarded: %s", body)
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  ```go
  package main

  import "path/filepath"

  func runCreds(args []string) error {
  	root, err := repoRoot()
  	if err != nil { return err }
  	if len(args) == 0 { args = []string{"all"} }
  	return runShell(filepath.Join(root, "scripts", "stage-creds.sh"), args...)
  }
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(cli): implement darkish creds (wraps stage-creds.sh)
  ```

#### Task E-09: Implement `darkish images`

Wraps `make -C images`.

- [ ] **Write the failing test**

  Create `cmd/darkish/images_test.go`:

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestImagesForwardsToMake(t *testing.T) {
  	dir := t.TempDir()
  	log := filepath.Join(dir, "log")
  	os.WriteFile(filepath.Join(dir, "make"),
  		[]byte("#!/bin/sh\necho \"$0 $@\" >> "+log+"\n"), 0o755)
  	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
  	_ = runImages([]string{"claude"})
  	body, _ := os.ReadFile(log)
  	if !strings.Contains(string(body), "claude") {
  		t.Fatalf("make claude not invoked: %s", body)
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  ```go
  package main

  import (
  	"os"
  	"os/exec"
  )

  func runImages(args []string) error {
  	if len(args) == 0 { args = []string{"all"} }
  	c := exec.Command("make", append([]string{"-C", "images"}, args...)...)
  	c.Stdout = os.Stdout; c.Stderr = os.Stderr
  	return c.Run()
  }
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(cli): implement darkish images (wraps make -C images)
  ```

#### Task E-10: Implement `darkish list`

Thin wrapper over `scion list` with darkish-specific column reformat.

- [ ] **Write the failing test**

  Create `cmd/darkish/list_test.go`:

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestListInvokesScion(t *testing.T) {
  	dir := t.TempDir()
  	os.WriteFile(filepath.Join(dir, "scion"),
  		[]byte(`#!/bin/sh
  echo "NAME STATE TURNS"
  echo "researcher running 5"
  `), 0o755)
  	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
  	out, err := captureStdout(func() error { return runList(nil) })
  	if err != nil { t.Fatal(err) }
  	if !strings.Contains(out, "researcher") {
  		t.Fatalf("list did not show researcher: %s", out)
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  ```go
  package main

  import (
  	"os"
  	"os/exec"
  )

  // runList is a thin passthrough to `scion list` for now.
  //
  // FUTURE: Spec §12.7 envisions darkish-specific column reformat (e.g.
  // template / grove / broker / phase) on top of `scion list --format
  // json`. Tracked in Open Questions; this initial implementation
  // streams scion output unchanged so operators get parity day-1.
  func runList(args []string) error {
  	c := exec.Command("scion", append([]string{"list"}, args...)...)
  	c.Stdout = os.Stdout; c.Stderr = os.Stderr
  	return c.Run()
  }
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  feat(cli): implement darkish list
  ```

#### Task E-11: Add internal/staging package shared by CLI and bash

Per spec §12.7, the bash scripts and Go CLI share implementation. Implement the manifest reader + skill copier in Go, expose a small wrapper binary for shell invocation.

- [ ] **Write the failing test**

  Create `internal/staging/manifest_test.go`:

  ```go
  package staging

  import (
  	"os"
  	"path/filepath"
  	"testing"
  )

  func TestParseSkillsFromJSON(t *testing.T) {
  	tmp := t.TempDir()
  	path := filepath.Join(tmp, "manifest.json")
  	os.WriteFile(path, []byte(`{
  		"name":"sme","skills":["danmestas/agent-skills/skills/ousterhout","danmestas/agent-skills/skills/hipp"]
  	}`), 0o644)
  	skills, err := ParseSkillsFromFile(path)
  	if err != nil { t.Fatal(err) }
  	if len(skills) != 2 {
  		t.Fatalf("want 2 skills, got %d", len(skills))
  	}
  }
  ```

  Create `internal/staging/skills_test.go`:

  ```go
  package staging

  import (
  	"os"
  	"path/filepath"
  	"testing"
  )

  func TestStageCopiesSkills(t *testing.T) {
  	tmp := t.TempDir()
  	canon := filepath.Join(tmp, "canon", "skills")
  	stage := filepath.Join(tmp, "stage")
  	os.MkdirAll(filepath.Join(canon, "ousterhout"), 0o755)
  	os.WriteFile(filepath.Join(canon, "ousterhout", "SKILL.md"), []byte("hi"), 0o644)

  	if err := Stage([]string{"danmestas/agent-skills/skills/ousterhout"}, canon, stage); err != nil {
  		t.Fatal(err)
  	}
  	if _, err := os.Stat(filepath.Join(stage, "ousterhout", "SKILL.md")); err != nil {
  		t.Fatalf("ousterhout not staged: %v", err)
  	}
  }
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Create `internal/staging/manifest.go`:

  ```go
  // Package staging materializes role skills into a per-harness staging
  // directory the manifest mounts as a read-only volume.
  package staging

  import (
  	"encoding/json"
  	"os"
  )

  type manifest struct {
  	Name   string   `json:"name"`
  	Skills []string `json:"skills"`
  }

  // ParseSkillsFromFile reads a manifest JSON file (the output of
  // `scion templates show <name> --local --format json`) and returns
  // the declared skill refs.
  func ParseSkillsFromFile(path string) ([]string, error) {
  	body, err := os.ReadFile(path)
  	if err != nil { return nil, err }
  	var m manifest
  	if err := json.Unmarshal(body, &m); err != nil { return nil, err }
  	return m.Skills, nil
  }
  ```

  Create `internal/staging/skills.go`:

  ```go
  package staging

  import (
  	"errors"
  	"fmt"
  	"io"
  	"os"
  	"path/filepath"
  	"strings"
  )

  // Stage copies each skill from canonicalRoot/<name>/ to stageRoot/<name>/.
  // canonicalRoot is typically ~/projects/agent-skills/skills.
  // stageRoot is typically <repo>/.scion/skills-staging/<harness>.
  func Stage(refs []string, canonicalRoot, stageRoot string) error {
  	if err := os.RemoveAll(stageRoot); err != nil { return err }
  	if err := os.MkdirAll(stageRoot, 0o755); err != nil { return err }
  	for _, ref := range refs {
  		name := refName(ref)
  		src := filepath.Join(canonicalRoot, name)
  		dst := filepath.Join(stageRoot, name)
  		if _, err := os.Stat(src); err != nil {
  			return fmt.Errorf("skill source missing: %s (%w)", src, err)
  		}
  		if err := copyTree(src, dst); err != nil { return err }
  	}
  	return nil
  }

  func refName(ref string) string {
  	if i := strings.LastIndex(ref, "/"); i >= 0 { return ref[i+1:] }
  	return ref
  }

  func copyTree(src, dst string) error {
  	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
  		if err != nil { return err }
  		rel, _ := filepath.Rel(src, p)
  		target := filepath.Join(dst, rel)
  		switch {
  		case info.IsDir():
  			return os.MkdirAll(target, info.Mode())
  		case info.Mode()&os.ModeSymlink != 0:
  			return errors.New("refusing to follow symlink: " + p)
  		default:
  			return copyFile(p, target, info.Mode())
  		}
  	})
  }

  func copyFile(src, dst string, mode os.FileMode) error {
  	in, err := os.Open(src); if err != nil { return err }; defer in.Close()
  	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
  	if err != nil { return err }; defer out.Close()
  	_, err = io.Copy(out, in)
  	return err
  }
  ```

- [ ] **Run the test and verify it passes**

  ```bash
  go test ./internal/staging/...
  ```

- [ ] **Run lint + types**

  ```bash
  go vet ./internal/staging/...
  ```

- [ ] **Commit**

  ```
  feat(internal/staging): add manifest reader + skill copier

  Stdlib-only. Copies (never symlinks). Parses JSON output from
  `scion templates show --format json`. Tests use real fixture skill
  trees on tmp filesystems.
  ```

#### Task E-12: Add scripts/bootstrap.sh wrapper that calls darkish bootstrap

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  S="$(dirname "$0")/bootstrap.sh"
  [[ -x "${S}" ]] || { echo "FAIL: bootstrap.sh missing/non-exec"; exit 1; }
  grep -q "darkish bootstrap" "${S}" || { echo "FAIL: bootstrap.sh does not call darkish"; exit 1; }
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Create `scripts/bootstrap.sh`:

  ```bash
  #!/usr/bin/env bash
  # bootstrap.sh — thin wrapper around `darkish bootstrap`.
  # Operators with the bin/darkish binary built can call either entry
  # point; bash users without Go in their PATH use this.
  set -euo pipefail
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  if [[ -x "${ROOT}/bin/darkish" ]]; then
    exec "${ROOT}/bin/darkish" bootstrap "$@"
  fi
  if command -v darkish >/dev/null 2>&1; then
    exec darkish bootstrap "$@"
  fi
  echo "bootstrap: bin/darkish not built; run 'make darkish' first" >&2
  exit 1
  ```

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  shellcheck scripts/bootstrap.sh
  ```

- [ ] **Commit**

  ```
  feat(scripts): add bootstrap.sh wrapper around darkish bootstrap
  ```

---

### Phase F — spec-kit per-harness install (planner-t4 only)

#### Task F-01: Modify codex prelude to install spec-kit on planner-t4 spawn

Per spec §11 Open Question, the install mechanism is investigated at task time. Two viable paths: `npm install -g @github/spec-kit` (if published) or `curl -L <release-tarball>`. Choose the one that works on first run. Document the choice in `images/codex/darkish-prelude.sh` itself.

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  P="$(dirname "$0")/../images/codex/darkish-prelude.sh"
  grep -q "SCION_TEMPLATE_NAME" "${P}" || { echo "FAIL: prelude does not branch on SCION_TEMPLATE_NAME"; exit 1; }
  grep -q "planner-t4" "${P}" || { echo "FAIL: prelude does not detect planner-t4"; exit 1; }
  grep -q "spec-kit" "${P}" || { echo "FAIL: prelude does not install spec-kit"; exit 1; }
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Replace `images/codex/darkish-prelude.sh`:

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail

  WORKSPACE_PATH="/repo-root/.scion/agents/${SCION_AGENT_NAME:-unknown}/workspace"
  CONFIG="${HOME}/.codex/config.toml"

  mkdir -p "${HOME}/.codex"

  if [[ -f "${CONFIG}" ]] && grep -qE "^\[projects\.\"${WORKSPACE_PATH//\//\\/}\"\]" "${CONFIG}"; then
    :
  else
    {
      echo ""
      echo "[projects.\"${WORKSPACE_PATH}\"]"
      echo "trust_level = \"trusted\""
    } >> "${CONFIG}"
  fi

  # planner-t4 needs spec-kit installed once. Idempotent: skip if present.
  if [[ "${SCION_TEMPLATE_NAME:-}" == "planner-t4" ]]; then
    if ! command -v specify >/dev/null 2>&1; then
      echo "darkish-prelude: installing spec-kit for planner-t4..." >&2
      # Investigated install paths (try in order, stop on first success):
      #   1. npm package (if published as @github/spec-kit)
      #   2. release tarball download
      if npm install -g @github/spec-kit 2>/dev/null; then
        echo "darkish-prelude: spec-kit via npm OK" >&2
      else
        TARBALL_URL="https://github.com/github/spec-kit/releases/latest/download/spec-kit-linux-x64.tar.gz"
        if curl -fsSL "${TARBALL_URL}" -o /tmp/spec-kit.tgz; then
          mkdir -p /opt/spec-kit && tar -xzf /tmp/spec-kit.tgz -C /opt/spec-kit
          ln -sf /opt/spec-kit/specify /usr/local/bin/specify
          echo "darkish-prelude: spec-kit via tarball OK" >&2
        else
          echo "darkish-prelude: WARNING — spec-kit install failed; planner-t4 will exit early" >&2
        fi
      fi
    fi
  fi

  exec sciontool init -- "$@"
  ```

  Rebuild darkish-codex with the new prelude (`make -C images codex`).

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

  ```bash
  shellcheck images/codex/darkish-prelude.sh
  ```

- [ ] **Commit**

  ```
  feat(images): install spec-kit in codex prelude for planner-t4 only

  Branches on SCION_TEMPLATE_NAME. Tries npm first, falls back to
  release tarball. Idempotent — skips when specify is already on PATH.
  ```

#### Task F-02: Smoke-test planner-t4 with a trivial spec task

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  cleanup() {
    scion stop pt4-smoke --yes 2>/dev/null || true
    scion delete pt4-smoke --yes 2>/dev/null || true
  }
  trap cleanup EXIT

  bash scripts/spawn.sh pt4-smoke --type planner-t4 \
    "Create a one-line spec for a 'reverse a string' utility. Use spec-kit."

  for _ in $(seq 1 120); do
    state="$(scion list | awk -v n=pt4-smoke '$1==n {print $2}')"
    [[ "${state}" == "completed" ]] && break
    sleep 5
  done

  out="$(scion look pt4-smoke 2>&1)"
  if ! echo "${out}" | grep -qi "specify"; then
    echo "FAIL: planner-t4 did not invoke specify CLI" >&2; exit 1
  fi
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

  Pre-F-01: spec-kit not installed in container.

- [ ] **Write the minimal implementation**

  No code change.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  test: smoke planner-t4 spec-kit invocation
  ```

#### Task F-03: Update planner-t4 system-prompt.md with spec-kit invocation expectations

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  P=".scion/templates/planner-t4/system-prompt.md"
  for tok in "specify constitution" "specify spec" "specify plan" "specify tasks"; do
    grep -q "${tok}" "${P}" || { echo "FAIL: ${tok} missing from prompt"; exit 1; }
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Edit `.scion/templates/planner-t4/system-prompt.md` to include a "Spec-kit invocation" section listing the 4 commands and expected outputs (`memory/constitution.md` → `specs/<feature>/spec.md` → `specs/<feature>/plan.md` → `specs/<feature>/tasks.md`).

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  docs(harnesses): document spec-kit invocations in planner-t4 prompt
  ```

---

### Phase G — Smoke + final docs

#### Task G-01: End-to-end bootstrap on tmp dir

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  TMP="$(mktemp -d)"
  trap 'rm -rf "${TMP}"' EXIT
  cp -R . "${TMP}/factory"
  cd "${TMP}/factory"
  make darkish
  bin/darkish bootstrap
  bin/darkish doctor
  bin/darkish spawn smoke-r --type researcher "echo hi"
  bin/darkish spawn smoke-s --type sme --backend codex "what is 1+1?"
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

  Pre-Phase E: `bin/darkish` doesn't exist.

- [ ] **Write the minimal implementation**

  No code change — validates the full chain.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  test: end-to-end bootstrap + spawn smoke
  ```

#### Task G-02: Final docs sync — README, harness-roster, pipeline-mechanics

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  for f in images/README.md .design/harness-roster.md .design/pipeline-mechanics.md; do
    for tok in darwin planner-t1 planner-t2 planner-t3 planner-t4; do
      grep -q "${tok}" "${f}" || { echo "FAIL: ${tok} missing from ${f}"; exit 1; }
    done
  done
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

- [ ] **Write the minimal implementation**

  Sweep the three docs files. Each must mention the 5 new harnesses and the universal baseline. Earlier tasks added most of this; G-02 closes any remaining gaps and ensures cross-references are coherent.

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  docs: final sync — roster, pipeline mechanics, images README
  ```

#### Task G-03: Tag branch as ready-for-merge

- [ ] **Write the failing test**

  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  git status --porcelain | grep -q . && { echo "FAIL: dirty tree"; exit 1; }
  git rev-parse HEAD >/dev/null
  bash scripts/test-stage-creds.sh
  bash scripts/test-no-legacy-volumes.sh
  bash scripts/test-stage-skills.sh
  go test ./...
  echo "PASS"
  ```

- [ ] **Run the test and verify it fails**

  Verify all earlier tests still green.

- [ ] **Write the minimal implementation**

  No code change. Run `git tag -a v0.2.0-rc1 -m 'harness + image config phase 1'` after manual operator sign-off (do NOT auto-tag in CI per constitution §V — operator-merge gate).

- [ ] **Run the test and verify it passes**

- [ ] **Run lint + types**

- [ ] **Commit**

  ```
  chore: ready-for-merge — all tests green
  ```

---

## Open questions (deferred from spec §11)

These remain unresolved at plan-authoring time and should be revisited during implementation, not blocking.

- **Operator's own CLIs.** Whether to bake-in or per-harness install. Default: bake the smallest stable set; per-harness install for niche CLIs.
- **Spec-kit install path.** F-01 tries npm first, falls back to release tarball. If both fail, document the working path on the second run.
- **Pi trust mechanism.** Validate during the first pi smoke spawn; update `images/pi/darkish-prelude.sh` if needed.
- **Gemini trust mechanism.** Same as pi.
- **Caveman tier enforcement.** Currently soft directive in prose. Promote to a `caveman_tier:` manifest field in a follow-up only if drift becomes a recurring darwin observation.
- **Skill cycle detection.** stage-skills.sh assumes a flat skill graph (no skill includes another). Revisit when APM-resolution starts pulling chains.
- **Darwin's mutation authority.** Plan ships with recommendations-only. `darkish apply` is the operator gate. Confirm during first darwin run.
- **Stage-skills.sh in `--add` mode persistence.** Plan persists by mutating manifest; `darwin` flagged ad-hoc additions are recorded in audit log. Session-scoped vs persistent is a manifest-edit decision per call.
- **Bones authorization.** Sub-agents inherit parent harness credentials. Plan does not change this; revisit if a sub-agent needs scoped auth (rare).
- **`darkish list` column reformat.** Spec §12.7 calls for darkish-specific columns (template / grove / broker / phase) on top of `scion list --format json`. E-10 ships as a thin passthrough to keep the initial slice small; column reformat is tracked here as a follow-up task to be scheduled when the operator's column preferences settle.
