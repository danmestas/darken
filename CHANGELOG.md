# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Apache-2.0 LICENSE.

### Changed

- Switch images from `agent-init` + `agent-tasks` to the unified `bones` CLI.

## [0.1.2] - 2026-04-27

### Fixed

- GoReleaser formula subdir layout for the Homebrew tap.

## [0.1.1] - 2026-04-27

### Added

- README and `darken --version` flag (phase 4 polish).

## [0.1.0] - 2026-04-27

Initial release of `darken`, a containerized orchestration harness with a
4-axis problem model and an embedded substrate plus per-machine,
per-project, and per-invocation override layers.

### Added

- `darken` CLI for creating and managing containerized harnesses.
- Embedded substrate with override-layer system (machine / project / invocation).
- Module renamed to `github.com/danmestas/darken`.
