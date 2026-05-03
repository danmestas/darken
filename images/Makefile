# Build script for darkish-* container images.
#
# Each darkish-* image extends a corresponding scion-* image, adding the
# darken tool baseline, folder-trust prelude, and credential
# mounting hooks. (The "darkish-" image-name prefix is a separate
# naming axis from the "darken" project name; the prefix groups the
# substrate's container images and is unrelated to the project codename.)
#
# Usage:
#   make claude           # build local/darkish-claude:latest
#   make push-claude      # push to GHCR (requires REGISTRY env var)
#   make all              # build all darkish-* images
#
# Override registry / tag:
#   make claude REGISTRY=ghcr.io/danmestas TAG=v0.1.0

REGISTRY         ?= local
TAG              ?= latest
BONES_SOURCE_PATH ?= $(HOME)/projects/bones

# prebuild-bones: cross-compile the unified bones CLI for linux/arm64.
# Required before running `make claude` because agent-infra and its dependency
# EdgeSync/leaf are private repos (no credentials in Docker build context).
#
# bones is the unified CLI (replaces the old agent-init + agent-tasks
# binaries). Subcommands:
#   bones init                 — create workspace (was: agent-init)
#   bones tasks <subcmd>       — task management (was: agent-tasks)
#   bones repo <subcmd>        — fossil-style repo ops (new)
.PHONY: prebuild-bones
prebuild-bones:
	# Clean bin/ first so stale binaries (e.g. the prior agent-init +
	# agent-tasks pair) don't get COPY'd into the Docker image.
	rm -rf claude/bin codex/bin pi/bin gemini/bin
	mkdir -p claude/bin codex/bin pi/bin gemini/bin
	cd $(BONES_SOURCE_PATH) && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/claude/bin/bones ./cmd/bones && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/codex/bin/bones ./cmd/bones && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/pi/bin/bones ./cmd/bones && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/gemini/bin/bones ./cmd/bones

.PHONY: claude
claude:
	docker build \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-claude:$(TAG) \
		-t $(REGISTRY)/darkish-claude:$(TAG) \
		-f claude/Dockerfile \
		claude

.PHONY: push-claude
push-claude:
	docker push $(REGISTRY)/darkish-claude:$(TAG)

.PHONY: codex
codex:
	docker build \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-codex:$(TAG) \
		-t $(REGISTRY)/darkish-codex:$(TAG) \
		-f codex/Dockerfile \
		codex

.PHONY: push-codex
push-codex:
	docker push $(REGISTRY)/darkish-codex:$(TAG)

.PHONY: pi
pi:
	docker build \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-pi:$(TAG) \
		-t $(REGISTRY)/darkish-pi:$(TAG) \
		-f pi/Dockerfile \
		pi

.PHONY: push-pi
push-pi:
	docker push $(REGISTRY)/darkish-pi:$(TAG)

.PHONY: gemini
gemini:
	docker build \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-gemini:$(TAG) \
		-t $(REGISTRY)/darkish-gemini:$(TAG) \
		-f gemini/Dockerfile \
		gemini

.PHONY: push-gemini
push-gemini:
	docker push $(REGISTRY)/darkish-gemini:$(TAG)

.PHONY: all
all: claude codex pi gemini

.PHONY: push-all
push-all: push-claude push-codex push-pi push-gemini

.PHONY: clean
clean:
	docker rmi $(REGISTRY)/darkish-claude:$(TAG) || true
	docker rmi $(REGISTRY)/darkish-codex:$(TAG) || true
	docker rmi $(REGISTRY)/darkish-pi:$(TAG) || true
	docker rmi $(REGISTRY)/darkish-gemini:$(TAG) || true

# ----- Fast-rebuild targets -----
# Rebuild only the final stage (apt, bones, caveman, context-mode, prelude).
# Faster than `make claude` when only the prelude or tool layer changed.

.PHONY: tools-only-claude tools-only-codex tools-only-pi tools-only-gemini tools-only-all

tools-only-claude:
	docker build \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-claude:$(TAG) \
		--target final \
		--no-cache-filter final \
		-t $(REGISTRY)/darkish-claude:$(TAG) \
		-f claude/Dockerfile \
		claude

tools-only-codex:
	docker build \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-codex:$(TAG) \
		--target final \
		--no-cache-filter final \
		-t $(REGISTRY)/darkish-codex:$(TAG) \
		-f codex/Dockerfile \
		codex

tools-only-pi:
	docker build \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-pi:$(TAG) \
		--target final \
		--no-cache-filter final \
		-t $(REGISTRY)/darkish-pi:$(TAG) \
		-f pi/Dockerfile \
		pi

tools-only-gemini:
	docker build \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-gemini:$(TAG) \
		--target final \
		--no-cache-filter final \
		-t $(REGISTRY)/darkish-gemini:$(TAG) \
		-f gemini/Dockerfile \
		gemini

tools-only-all: tools-only-claude tools-only-codex tools-only-pi tools-only-gemini

# prelude-only-*: full no-cache rebuild for any prelude-script change.
# Heavier than tools-only-* but rebuilds the prelude COPY layer reliably.

.PHONY: prelude-only-claude prelude-only-codex prelude-only-pi prelude-only-gemini

prelude-only-claude:
	docker build --no-cache \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-claude:$(TAG) \
		-t $(REGISTRY)/darkish-claude:$(TAG) \
		-f claude/Dockerfile claude

prelude-only-codex:
	docker build --no-cache \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-codex:$(TAG) \
		-t $(REGISTRY)/darkish-codex:$(TAG) \
		-f codex/Dockerfile codex

prelude-only-pi:
	docker build --no-cache \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-pi:$(TAG) \
		-t $(REGISTRY)/darkish-pi:$(TAG) \
		-f pi/Dockerfile pi

prelude-only-gemini:
	docker build --no-cache \
		--build-arg BASE_IMAGE=$(REGISTRY)/scion-gemini:$(TAG) \
		-t $(REGISTRY)/darkish-gemini:$(TAG) \
		-f gemini/Dockerfile gemini
