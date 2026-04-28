# Contributing to darken

Thanks for your interest in `darken`, a containerized orchestration harness
with a 4-axis problem model and embedded-substrate-with-overrides design.
This document covers everything you need to get a working checkout, build the
binary, and submit changes.

## Development setup

Requires Go 1.23 or newer.

```
git clone https://github.com/danmestas/darken
cd darken
make darken
```

`make darken` builds the `darken` CLI to `bin/darken`. The repository ships
with embedded substrate templates that are baked into the binary at build
time via `make sync-skills` and `make sync-embed-data`.

For end-user installation (running darken, not building it), the released
binary is distributed via a Homebrew tap — see the README for `brew install`
instructions.

## Running tests

```
go test ./...                # full suite
go test -race ./...          # race detector
```

Tests live under `tests/` for integration coverage and alongside source
files (`*_test.go`) for unit tests.

## Embedded data

If you change skills, templates, or other embedded substrate content, regenerate
the embedded blobs:

```
make sync-skills        # refresh embedded skills
make sync-embed-data    # refresh other embedded data
```

CI runs the same sync to guarantee the embedded content matches `templates/`
and `skills/` in the repo.

## Code layout

- `cmd/` — CLI entry points (`darken`).
- `internal/` — implementation packages, not exported.
- `templates/` — substrate templates baked into the binary.
- `images/` — container image definitions for the orchestrator.
- `scripts/` — build and sync helper scripts.
- `tests/` — integration tests.
- `docs/` — design docs and the 4-axis problem statement.

## Submitting changes

1. Open a feature branch off `main`. Direct commits to `main` are not accepted.
2. Run `go test ./...` locally before pushing.
3. If you changed embedded content, run `make sync-skills` / `make sync-embed-data`.
4. Open a PR; CI will re-run tests and verify the embedded content is in sync.

## Reporting issues

Open an issue at https://github.com/danmestas/darken/issues with reproduction
steps, the harness backend involved (Claude Code / Codex / Gemini / Copilot),
and the output of `darken --version`.
