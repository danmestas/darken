# Build script for darkish-* container images.
#
# Each darkish-* image extends a corresponding scion-* image, adding the
# Darkish Factory's tool baseline, folder-trust prelude, and credential
# mounting hooks.
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
AGENT_INFRA_PATH ?= $(HOME)/projects/agent-infra

# prebuild-bones: cross-compile agent-infra binaries on the host for linux/arm64.
# Required before running `make claude` because agent-infra and its dependency
# EdgeSync/leaf are private repos (no credentials in Docker build context).
.PHONY: prebuild-bones
prebuild-bones:
	mkdir -p claude/bin codex/bin pi/bin gemini/bin
	cd $(AGENT_INFRA_PATH) && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/claude/bin/agent-init ./cmd/agent-init && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/claude/bin/agent-tasks ./cmd/agent-tasks && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/codex/bin/agent-init ./cmd/agent-init && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/codex/bin/agent-tasks ./cmd/agent-tasks && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/pi/bin/agent-init ./cmd/agent-init && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/pi/bin/agent-tasks ./cmd/agent-tasks && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/gemini/bin/agent-init ./cmd/agent-init && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(CURDIR)/gemini/bin/agent-tasks ./cmd/agent-tasks

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
