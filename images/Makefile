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

REGISTRY ?= local
TAG      ?= latest

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

.PHONY: all
all: claude

.PHONY: clean
clean:
	docker rmi $(REGISTRY)/darkish-claude:$(TAG) || true
