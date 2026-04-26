# darkish-pi — extends scion-pi with the Darkish Factory's tool baseline.
#
# Pi auth: via OPENROUTER_API_KEY env var (scion injects it from the
# operator's env or hub secrets). No OAuth file mounting needed.
#
# Pi binary: /usr/local/bin/pi (and /usr/local/bin/claude, which
# scion-pi symlinks to a wrapper script that invokes pi). Scion's
# claude harness CMD ends up running pi when this image is used.
#
# Build:
#   make -C images pi

ARG BASE_IMAGE=local/scion-pi:latest
FROM ${BASE_IMAGE}

USER root

# 1. Apt utilities — same baseline as darkish-claude / darkish-codex.
RUN apt-get update && apt-get install -y --no-install-recommends \
      jq \
      ripgrep \
      fzf \
      less \
      gh \
    && rm -rf /var/lib/apt/lists/*

# 2-4. Tool placeholders. Same pattern.

# 5. Prelude.
COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
RUN chmod +x /usr/local/bin/darkish-prelude.sh

USER scion
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
