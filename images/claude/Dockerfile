# darkish-claude — extends scion-claude with the Darkish Factory's tool baseline,
# folder-trust prelude, and OAuth credential mounting.
#
# Build:
#   make -C images claude          (or: docker build -t local/darkish-claude:latest -f images/claude/Dockerfile images/claude)
#
# This image must be tagged so scion finds it. Each harness's scion-agent.yaml
# sets `image: local/darkish-claude:latest` (or the ghcr.io tag once published).

ARG BASE_IMAGE=local/scion-claude:latest
FROM ${BASE_IMAGE}

USER root

# 1. Apt utilities.
#    Heaviest layer first so it caches across rebuilds when only the prelude
#    or tool list churns.
RUN apt-get update && apt-get install -y --no-install-recommends \
      jq \
      ripgrep \
      fzf \
      less \
      gh \
    && rm -rf /var/lib/apt/lists/*

# 2. Go-installed tools (custom CLIs, semantic search, etc).
#    Placeholder — operator fills in. Examples:
#      RUN go install github.com/danmestas/mgrep-code-search/cmd/mgrep@latest
#    Kept empty here so this layer rebuilds independently when tools change.

# 3. NPM-installed MCPs.
#    Placeholder. Examples:
#      RUN npm install -g <mcp-package>

# 4. Pip-installed MCPs.
#    Placeholder. Examples:
#      RUN pip install <python-mcp> --no-cache-dir

# 5. Prelude: folder-trust + OAuth file → env var.
COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
RUN chmod +x /usr/local/bin/darkish-prelude.sh

# Reset to scion's standard user.
USER scion
WORKDIR /workspace

# Override the entrypoint so our prelude runs before sciontool init.
ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
# CMD is inherited from the base image.
