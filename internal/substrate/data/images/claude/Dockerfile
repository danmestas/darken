# darkish-claude — universal baseline + claude backend.
# Layers ordered heaviest-at-bottom for cache efficiency.
#
# NOTE (A-01 blockers resolved):
#   - agent-infra and its dependency EdgeSync/leaf are private repos.
#     Binaries are pre-compiled on the host (CGO_ENABLED=0 GOOS=linux) and
#     copied from ./bin/ into the image. The bones-builder stage is omitted
#     until SSH/netrc credentials can be mounted at build time.
#     Restore the multi-stage build once the repos are public or CI has creds.
#   - mgrep-code-search is also private; omitted until accessible.
#   - Only 4 cmds exist in the current agent-infra (agent-init, agent-tasks,
#     orchestrator-validate-plan, space-invaders-orchestrate); the 12 remaining
#     binaries from the spec do not yet exist. Only agent-init and agent-tasks
#     are included as they are the core operational primitives.
#   - agent-infra requires go 1.26; pre-built on go 1.26.2 darwin/arm64 for
#     linux/arm64 target.

ARG BASE_IMAGE=local/scion-claude:latest

FROM ${BASE_IMAGE} AS final

USER root

# 1. Apt utilities (heaviest, churns least).
RUN apt-get update && apt-get install -y --no-install-recommends \
      jq ripgrep fzf less gh ca-certificates curl git \
    && rm -rf /var/lib/apt/lists/*

# 2. Pre-built bones binaries (agent-init, agent-tasks).
#    Compiled on host: CGO_ENABLED=0 GOOS=linux GOARCH=arm64
#    Refresh with: make -C images prebuild-bones
COPY bin/ /usr/local/bin/
RUN chmod +x /usr/local/bin/agent-init /usr/local/bin/agent-tasks

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
