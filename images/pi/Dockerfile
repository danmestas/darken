# darkish-pi — universal baseline + pi backend (OpenRouter-routed).
# Mirrors A-01's pattern.
#
# NOTE: agent-infra is private; the unified bones CLI is pre-compiled on
# the host (linux/arm64) and copied from ./bin/. Refresh:
#   make -C images prebuild-bones
# bones exposes init/tasks/repo as subcommands.
#   - mgrep omitted (paid; context-mode replaces).
#   - Pi auth: OPENROUTER_API_KEY env var (no OAuth file mount).
#   - Pi uses pi-coding-agent CLI; --non-interactive flag bypasses any
#     first-run trust dialog observed in existing pi-templates.

ARG BASE_IMAGE=local/scion-pi:latest

FROM ${BASE_IMAGE} AS final

USER root

# 1. Apt utilities.
RUN apt-get update && apt-get install -y --no-install-recommends \
      jq ripgrep fzf less gh ca-certificates curl git \
    && rm -rf /var/lib/apt/lists/*

# 2. Pre-built bones unified CLI.
COPY bin/ /usr/local/bin/
RUN chmod +x /usr/local/bin/bones

# 3. Universal skill: caveman.
RUN mkdir -p /home/scion/skills && \
    git clone --depth 1 https://github.com/juliusbrussee/caveman \
      /home/scion/skills/caveman && \
    chown -R scion:scion /home/scion/skills

# 4. Universal MCP: context-mode.
RUN npm install -g @mksglu/context-mode || \
    (echo "context-mode npm install failed; falling back to git clone" && \
     git clone --depth 1 https://github.com/mksglu/context-mode \
       /opt/context-mode && \
     cd /opt/context-mode && npm install --production)

# 5. Prelude.
COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
RUN chmod +x /usr/local/bin/darkish-prelude.sh

USER scion
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
