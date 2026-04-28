# darkish-gemini — universal baseline + gemini backend.
# Mirrors A-01's pattern.
#
# NOTE: agent-infra is private; the unified bones CLI is pre-compiled on
# the host (linux/arm64) and copied from ./bin/. Refresh:
#   make -C images prebuild-bones
# bones exposes init/tasks/repo as subcommands.
#   - mgrep omitted (paid; context-mode replaces).
#   - Gemini auth (3 options):
#       * GEMINI_API_KEY env var (free tier on AI Studio)
#       * ~/.gemini/oauth_creds.json (gemini-cli OAuth; scion auto-detects)
#       * GOOGLE_APPLICATION_CREDENTIALS service account (Vertex AI)
#     Operator account/key not yet set up; image is structurally ready.
#   - Trust mechanism for gemini-cli first-run dialog: unverified.

ARG BASE_IMAGE=local/scion-gemini:latest

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
