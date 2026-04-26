# darkish-gemini — extends scion-gemini with the Darkish Factory's tool baseline.
#
# Gemini auth options:
#   - Free tier: GEMINI_API_KEY env var (~$0/month, rate limited)
#   - OAuth: ~/.gemini/oauth_creds.json (scion auto-detects on broker host)
#   - Vertex AI: GOOGLE_APPLICATION_CREDENTIALS service account
#
# Build:
#   make -C images gemini

ARG BASE_IMAGE=local/scion-gemini:latest
FROM ${BASE_IMAGE}

USER root

# 1. Apt utilities — same baseline as the other darkish-* images.
RUN apt-get update && apt-get install -y --no-install-recommends \
      jq \
      ripgrep \
      fzf \
      less \
      gh \
    && rm -rf /var/lib/apt/lists/*

# 2-4. Tool placeholders.

# 5. Prelude.
COPY darkish-prelude.sh /usr/local/bin/darkish-prelude.sh
RUN chmod +x /usr/local/bin/darkish-prelude.sh

USER scion
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/darkish-prelude.sh"]
