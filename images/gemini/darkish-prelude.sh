#!/usr/bin/env bash
# darkish-prelude.sh (gemini variant) — runs before sciontool init in
# the darkish-gemini container.
#
# Gemini auth: scion auto-detects ~/.gemini/oauth_creds.json on the
# broker host (pkg/harness/auth.go) for OAuth, or accepts GEMINI_API_KEY
# env var for key-based auth. The user can fund either path:
#   - Free tier: GEMINI_API_KEY from https://aistudio.google.com/apikey
#   - Vertex AI: GOOGLE_APPLICATION_CREDENTIALS for service account
#   - OAuth (gemini-cli): ~/.gemini/oauth_creds.json
#
# TODO: Gemini CLI's first-encounter trust mechanism is not yet
# verified. Likely candidates:
#   - ~/.gemini/settings.json with a trust block
#   - A per-project file
# Validate against real gemini-cli behavior on first use.

set -euo pipefail

# --- 1. Trust state (placeholder) -------------------------------------------
# (Add Gemini-specific trust handling here when verified.)

# --- 2. Hand off to scion ---------------------------------------------------

exec sciontool init -- "$@"
