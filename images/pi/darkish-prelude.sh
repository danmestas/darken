#!/usr/bin/env bash
# darkish-prelude.sh (pi variant) — runs before sciontool init in
# the darkish-pi container.
#
# Pi uses @mariozechner/pi-coding-agent (separate CLI from Claude Code,
# despite scion-pi's image symlinking `claude` → pi-wrapper.sh). Pi
# auth is via the OPENROUTER_API_KEY env var, which scion injects at
# launch time. No OAuth file mounting required.
#
# TODO: Pi's first-encounter trust mechanism is not yet verified. The
# existing pi-templates in scion-orchestrator run with --non-interactive
# which probably skips any prompt, but if a trust dialog appears at
# startup, add the bypass here. Likely candidates:
#   - ~/.pi/config.json or .toml
#   - A flag in the wrapper script
# Validate against real pi-CLI behavior on first use.

set -euo pipefail

# --- 1. Trust state (placeholder) -------------------------------------------
# (Add Pi-specific trust handling here when verified.)

# --- 2. Hand off to scion ---------------------------------------------------

exec sciontool init -- "$@"
