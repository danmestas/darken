# darkish-codex — extends scion-codex with the Darkish Factory's tool
# baseline, folder-trust prelude, and per-grove customizations.
#
# Codex's OAuth handling is simpler than Claude's: scion auto-detects
# ~/.codex/auth.json on the host and supports auth-file mode natively
# (pkg/harness/codex.go:155). No OAuth shim is required in this prelude.
#
# Build:
#   make -C images codex
#
# Status: scaffold. Not yet validated end-to-end. The codex prelude's
# trust mechanism is a placeholder modeled on Claude's pattern — verify
# against real codex CLI behavior on first use and update.

ARG BASE_IMAGE=local/scion-codex:latest
FROM ${BASE_IMAGE}

USER root

# 1. Apt utilities — same baseline as darkish-claude.
RUN apt-get update && apt-get install -y --no-install-recommends \
      jq \
      ripgrep \
      fzf \
      less \
      gh \
    && rm -rf /var/lib/apt/lists/*

# 2-4. Tool placeholders (Go, NPM, Pip). Same pattern as darkish-claude.
#      Operator fills in when known.

# 5. Prelude.
COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
RUN chmod +x /usr/local/bin/darkish-prelude.sh

USER scion
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
