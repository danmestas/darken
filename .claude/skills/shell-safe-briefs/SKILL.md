---
name: shell-safe-briefs
description: Use when composing the brief argument passed to darken spawn or scion message; the brief flows through scion sh -c wrapper that misparses three ASCII characters
---

# Shell-Safe Briefs

## Overview

Scion's harness layer wraps the agent invocation in `sh -c "tmux ... claude --system-prompt \"...\" \"<TASK>\""`. The TASK arg lives inside double quotes. Three ASCII characters break the outer quoting and cause the agent to crash at boot with `Syntax error: end of file unexpected` or `Syntax error: word unexpected`:

| Character | Effect inside `"..."` |
|---|---|
| `'` (U+0027 apostrophe) | Closes the outer single-quoted shell argument when embedded |
| `"` (U+0022 double quote) | Closes the outer double-quoted shell argument |
| `` ` `` (U+0060 backtick) | Triggers command substitution |

Tracked upstream as bug #15 (scion harness shell-escaping). Until fixed in scion, **every brief composed by this orchestrator must use Unicode equivalents at compose time.**

## When to Use

- Composing any `bin/darken spawn ... "<brief>"` call
- Composing any `scion message <agent> "<message>"` call
- Composing any `scion message --broadcast "<message>"` call
- Editing template files at `.scion/templates/<role>/system-prompt.md` or `agents.md`

## The Three Substitutions

Use these Unicode equivalents — read identically to humans and to LLMs but are not shell-special:

| ASCII | Unicode | Looks like |
|---|---|---|
| `'` U+0027 | `'` U+2019 | Right single quotation mark |
| `"` U+0022 | `"` U+201C / `"` U+201D | Curly quotes (paired) |
| `` ` `` U+0060 | `'` U+2019 | Lose backtick formatting; use prose |

Or restructure the prose to avoid them:
- "do not" instead of "don't"
- "the planner" instead of "planner's"
- inline-code stays inline, just no backticks: write `cmd/darken/spawn.go` as cmd/darken/spawn.go

## Recorded Failures

- **impl-2 (v0.1.16)**: brief used apostrophes (`don't`, `planner's`); spawn crashed with `Syntax error: end of file unexpected`. Cleared via `scion delete`, three retries with progressive sanitization required (apostrophes → double quotes → backticks).
- **All 14 role templates (v0.1.16)**: had ASCII apostrophes/quotes/backticks in their system-prompt.md and agents.md. Sanitized in PR #28 — 1,237 chars replaced across 28 files.

## Quick Reference

Before any spawn or message call, scan the brief text for `'`, `"`, `` ` ``. If present, apply Unicode replacement. One bash heuristic to verify:

```
echo "$BRIEF" | grep -cE "['\"\`]"
# If output > 0, sanitize before sending
```

## Anti-Patterns

- Adding apostrophes back when "feels natural to write" — feels-natural is exactly when the bug bites
- Using markdown backticks for code references in briefs — strip them, write the path as prose
- Trusting that scion will escape correctly — it does not
